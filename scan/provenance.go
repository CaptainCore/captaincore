package scan

import (
	"os"
	"path/filepath"
	"strings"
)

// coreRootPHP is the complete set of PHP files a stock WordPress install ships
// at the web root. It is stable across modern major versions (the last change
// was the removal of wp-register.php over a decade ago), which is what makes a
// name allowlist a reliable provenance signal here: anything else ending in a
// PHP extension at the web root is a file WordPress did not put there.
var coreRootPHP = map[string]bool{
	"index.php":            true,
	"wp-activate.php":      true,
	"wp-blog-header.php":   true,
	"wp-comments-post.php": true,
	"wp-config.php":        true,
	"wp-config-sample.php": true,
	"wp-cron.php":          true,
	"wp-links-opml.php":    true,
	"wp-load.php":          true,
	"wp-login.php":         true,
	"wp-mail.php":          true,
	"wp-settings.php":      true,
	"wp-signup.php":        true,
	"wp-trackback.php":     true,
	"xmlrpc.php":           true,
}

// contentRootPHP is index.php plus every drop-in filename WordPress core
// recognizes in wp-content (the union of _get_dropins() for single site and
// multisite). A recognized drop-in belongs there by name; its *contents* are a
// separate concern handled by the signature rules, so this provenance check
// stays silent on them and only reports names WordPress does not know.
var contentRootPHP = map[string]bool{
	"index.php":               true,
	"advanced-cache.php":      true,
	"db.php":                  true,
	"db-error.php":            true,
	"install.php":             true,
	"maintenance.php":         true,
	"object-cache.php":        true,
	"php-error.php":           true,
	"fatal-error-handler.php": true,
	"sunrise.php":             true,
	"blog-deleted.php":        true,
	"blog-inactive.php":       true,
	"blog-suspended.php":      true,
}

// rootDropExtensions are the executable extensions worth flagging at the root. A
// backdoor named .php.png or with an odd case still executes on many stacks.
// .inc and .phps are deliberately excluded: .inc is a common plain-include
// convention and often not PHP-mapped, .phps is a source highlighter and not
// executed, so flagging either at the root would add false positives without
// catching a shell that any stack would run.
var rootDropExtensions = map[string]bool{
	".php": true, ".phtml": true, ".phar": true, ".pht": true,
	".php3": true, ".php4": true, ".php5": true, ".php7": true,
	".php8": true, ".phtm": true,
}

// coreSubdirs are the directory names a "WordPress in its own directory"
// install (including Bedrock / roots.io, whose core lives in web/wp/) uses to
// hold the core tree. Kept to the conventional names so the detector never
// wanders into an arbitrary subdirectory.
var coreSubdirs = []string{"wp", "wordpress", "cms"}

// contentDirs are the directory names that hold wp-content at the web root:
// the default, plus Bedrock's renamed "app".
var contentDirs = []string{"wp-content", "app"}

func fileAt(dir, name string) bool {
	info, err := os.Stat(filepath.Join(dir, name))
	return err == nil && !info.IsDir()
}

func dirAt(dir, name string) bool {
	info, err := os.Stat(filepath.Join(dir, name))
	return err == nil && info.IsDir()
}

// bootstrapRootPHP is the tight root allowlist for a core-in-subdirectory
// layout: the web root only carries the bootstrap, since wp-login.php and the
// rest of the core root files live inside the core subdirectory.
var bootstrapRootPHP = map[string]bool{
	"index.php":            true,
	"wp-config.php":        true,
	"wp-config-sample.php": true,
}

// wordpressLayout classifies root and returns the PHP allowlist for the web
// root and the absolute path of the content directory (empty when it cannot be
// located). ok is false when root is not a WordPress web root at all, so the
// provenance check never fires on a quicksave tree or an arbitrary directory.
//
// It recognizes three layouts:
//   - classic: core at the web root (wp-settings.php + wp-load.php present)
//   - core-in-subdirectory: core in wp/ (or wordpress/, cms/) with the
//     bootstrap at the root — the supported "give WordPress its own directory"
//     setup and the shape Bedrock/roots.io produces under web/
func wordpressLayout(root string) (rootAllow map[string]bool, contentDir string, ok bool) {
	if fileAt(root, "wp-settings.php") && fileAt(root, "wp-load.php") {
		return coreRootPHP, filepath.Join(root, "wp-content"), true
	}
	if fileAt(root, "index.php") || fileAt(root, "wp-config.php") {
		for _, sub := range coreSubdirs {
			if fileAt(filepath.Join(root, sub), "wp-settings.php") {
				for _, c := range contentDirs {
					if dirAt(root, c) {
						return bootstrapRootPHP, filepath.Join(root, c), true
					}
				}
				return bootstrapRootPHP, "", true
			}
		}
	}
	return nil, "", false
}

func phpExtension(name string) bool {
	lower := strings.ToLower(name)
	for ext := range rootDropExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// CheckWordPressRoot reports PHP files at the WordPress web root and at the
// wp-content root that core did not ship and that are not recognized drop-ins.
// This is a provenance signal, not a content signal: it does not read the file,
// so it holds regardless of how the payload is written or obfuscated, which is
// exactly the class that polymorphic and AI-generated backdoors fall into. The
// three kr224 root shells (wp-locale-cache.php, wp-remote-cache.php,
// kr224ws_<hex>.php) are caught here with no dependence on their contents.
//
// root is the WordPress web root. When root is not a WordPress install the
// function returns nil, so it is safe to call on any scanned directory.
func CheckWordPressRoot(root string) []Finding {
	rootAllow, contentDir, ok := wordpressLayout(root)
	if !ok {
		return nil
	}
	var findings []Finding

	findings = append(findings, unexpectedPHP(root, "", rootAllow,
		"provenance-unexpected-root-php",
		"Unexpected PHP file at the WordPress root",
		"A file ending in a PHP extension sits at the WordPress web root but is not one of the files WordPress core ships there. Stock installs never add PHP to the web root, so an extra one is almost always a dropped backdoor. This is what `wp core verify-checksums` reports as \"File should not exist\".")...)

	if contentDir != "" {
		rel := filepath.Base(contentDir)
		findings = append(findings, unexpectedPHP(contentDir, rel, contentRootPHP,
			"provenance-unexpected-content-php",
			"Unexpected PHP file at the wp-content root",
			"A file ending in a PHP extension sits at the wp-content root but is not index.php and is not a drop-in filename WordPress core recognizes. Only recognized drop-ins and index.php belong here.")...)
	}
	return findings
}

// unexpectedPHP lists depth-1 PHP files in dir that are not in allow, as
// findings. relPrefix is prepended to the reported file path (empty for the web
// root, "wp-content" for the content root) so the finding path is meaningful.
func unexpectedPHP(dir, relPrefix string, allow map[string]bool, ruleID, name, desc string) []Finding {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Finding
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fn := e.Name()
		if !phpExtension(fn) || allow[strings.ToLower(fn)] {
			continue
		}
		rel := fn
		if relPrefix != "" {
			rel = relPrefix + "/" + fn
		}
		abs := filepath.Join(dir, fn)
		f := Finding{
			File: rel, Path: abs, RuleID: ruleID, Name: name,
			Family: "integrity", Severity: "high", Description: desc,
			Match: fn + " is not a WordPress core file",
		}
		if h, err := fileSHA256(abs); err == nil {
			f.SHA256 = h
		}
		out = append(out, f)
	}
	return out
}

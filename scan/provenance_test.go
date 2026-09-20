package scan

import (
	"path/filepath"
	"testing"
)

// classicRoot writes a minimal but complete stock WordPress web root so the
// provenance check treats the directory as a real install.
func classicRoot(t *testing.T, dir string) {
	t.Helper()
	for f := range coreRootPHP {
		write(t, dir, f, "<?php // core\n")
	}
	write(t, dir, "wp-content/index.php", "<?php // silence\n")
}

func filesOf(f []Finding) map[string]bool {
	m := map[string]bool{}
	for _, x := range f {
		m[filepath.ToSlash(x.File)] = true
	}
	return m
}

func TestProvenanceCatchesUnexpectedRootPHP(t *testing.T) {
	dir := t.TempDir()
	classicRoot(t, dir)
	// Three files shaped like the kr224 root shells — contents irrelevant, the
	// check is on provenance, so a bare tag stands in for any payload.
	write(t, dir, "wp-locale-cache.php", "<?php /* anything */\n")
	write(t, dir, "wp-remote-cache.php", "<?php /* anything */\n")
	write(t, dir, "kr224ws_e30259e9.php", "<?php /* anything */\n")

	f := CheckWordPressRoot(dir)
	got := filesOf(f)
	for _, want := range []string{"wp-locale-cache.php", "wp-remote-cache.php", "kr224ws_e30259e9.php"} {
		if !got[want] {
			t.Errorf("expected %s flagged as unexpected root PHP; got %v", want, ids(f))
		}
	}
	if !has(f, "provenance-unexpected-root-php") {
		t.Errorf("expected provenance-unexpected-root-php rule; got %v", ids(f))
	}
}

func TestProvenanceAllowsCoreAndDropins(t *testing.T) {
	dir := t.TempDir()
	classicRoot(t, dir)
	// A legitimate object cache drop-in and every core root file must be silent.
	write(t, dir, "wp-content/object-cache.php", "<?php // Redis Object Cache\n")
	write(t, dir, "wp-content/advanced-cache.php", "<?php // WP Super Cache\n")

	if f := CheckWordPressRoot(dir); len(f) != 0 {
		t.Errorf("clean root should produce no findings; got %v", ids(f))
	}
}

func TestProvenanceCatchesUnexpectedContentPHP(t *testing.T) {
	dir := t.TempDir()
	classicRoot(t, dir)
	write(t, dir, "wp-content/wp-locale-cache.php", "<?php /* stray shell */\n")

	f := CheckWordPressRoot(dir)
	if !has(f, "provenance-unexpected-content-php") {
		t.Errorf("expected provenance-unexpected-content-php; got %v", ids(f))
	}
	if !filesOf(f)["wp-content/wp-locale-cache.php"] {
		t.Errorf("expected the stray content-root file flagged; got %v", filesOf(f))
	}
}

func TestProvenanceBedrockLayout(t *testing.T) {
	dir := t.TempDir()
	// Bedrock/roots.io: core in wp/, content in app/, bootstrap at the root.
	write(t, dir, "index.php", "<?php // bootstrap\n")
	write(t, dir, "wp-config.php", "<?php // config\n")
	write(t, dir, "wp/wp-settings.php", "<?php // core\n")
	write(t, dir, "wp/wp-login.php", "<?php // core\n")
	write(t, dir, "app/index.php", "<?php // silence\n")
	write(t, dir, "evil-root.php", "<?php /* shell */\n")
	write(t, dir, "app/evil-app.php", "<?php /* shell */\n")

	f := CheckWordPressRoot(dir)
	got := filesOf(f)
	if !got["evil-root.php"] {
		t.Errorf("expected evil-root.php flagged at Bedrock web root; got %v", got)
	}
	if !got["app/evil-app.php"] {
		t.Errorf("expected app/evil-app.php flagged at Bedrock content root; got %v", got)
	}
	// The bootstrap files and the core subdirectory must not be flagged.
	if got["index.php"] || got["wp-config.php"] {
		t.Errorf("Bedrock bootstrap files must not be flagged; got %v", got)
	}
}

func TestProvenanceSilentOnNonWordPress(t *testing.T) {
	dir := t.TempDir()
	// A quicksave tree (plugins/themes only) and arbitrary PHP must never fire.
	write(t, dir, "plugins/acme/acme.php", "<?php /* whatever */\n")
	write(t, dir, "random.php", "<?php echo 1;\n")

	if f := CheckWordPressRoot(dir); len(f) != 0 {
		t.Errorf("non-WordPress directory must produce no findings; got %v", ids(f))
	}
}

func TestProvenanceIgnoresNonExecutableExtensions(t *testing.T) {
	dir := t.TempDir()
	classicRoot(t, dir)
	// .inc and .phps are excluded by design to avoid false positives.
	write(t, dir, "config.inc", "<?php // include\n")
	write(t, dir, "source.phps", "<?php // highlighted source\n")

	if f := CheckWordPressRoot(dir); len(f) != 0 {
		t.Errorf(".inc/.phps at root should not be flagged; got %v", ids(f))
	}
}

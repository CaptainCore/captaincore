package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHiddenPluginsToAlert(t *testing.T) {
	qs := t.TempDir()
	mk := func(rel, content string) {
		p := filepath.Join(qs, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	mk("plugins/autoupdater/autoupdater.php", "<?php add_filter('all_plugins', 'hide');")
	mk("plugins/affiliate-wp/affiliate-wp.php", "<?php // never touches the list")
	mk("plugins/scanner-helper-pro/scanner-helper-pro.php", "<?php add_filter('all_plugins', function ($p) { unset($p[plugin_basename(__FILE__)]); return $p; });")

	alert, skipped := hiddenPluginsToAlert([]string{"autoupdater", "affiliate-wp", "scanner-helper-pro", "not-in-quicksave"}, qs)
	if strings.Join(alert, ",") != "scanner-helper-pro,not-in-quicksave" {
		t.Errorf("alert = %v", alert)
	}
	if len(skipped) != 2 || !strings.Contains(skipped[0], "Flywheel") || !strings.Contains(skipped[1], "never touches") {
		t.Errorf("skipped = %v", skipped)
	}
	many := make([]string, hiddenPluginListCap+1)
	for i := range many {
		many[i] = "p" + string(rune('a'+i))
	}
	if alert, skipped := hiddenPluginsToAlert(many, qs); len(alert) != 0 || len(skipped) != 1 {
		t.Errorf("cap: alert=%v skipped=%v", alert, skipped)
	}
}

package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CaptainCore/captaincore/models"
)

func TestFullScanDue(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	site := filepath.Join(t.TempDir(), "production", "quicksave")
	os.MkdirAll(site, 0o755)
	now := time.Now()
	today := uint(now.Weekday())     // a site id whose slot is today
	other := uint(now.Weekday()) + 1 // a site id whose slot is tomorrow
	if due, why := fullScanDue(site, true, now, other); !due || why != "first quicksave" {
		t.Errorf("first commit always scans: %v %q", due, why)
	}
	if due, why := fullScanDue(site, false, now, today); !due || why != "never scanned" {
		t.Errorf("no stamp, in-slot site: %v %q", due, why)
	}
	if due, _ := fullScanDue(site, false, now, other); due {
		t.Error("no stamp, out-of-slot site must wait for its weekday")
	}
	writeFullScanStamp(site, 10, 0)
	if due, why := fullScanDue(site, false, now, today); due {
		t.Errorf("fresh stamp should not be due: %q", why)
	}
	if due, why := fullScanDue(site, false, now.Add(fullScanInterval+time.Hour), other); !due || why != "weekly" {
		t.Errorf("stale stamp scans regardless of slot: %v %q", due, why)
	}
	b, _ := json.Marshal(fullScanStamp{Time: now.UTC().Format(time.RFC3339), RulesHash: "old"})
	os.WriteFile(fullScanStampPath(site), b, 0o644)
	if due, why := fullScanDue(site, false, now, today); !due || why != "rules changed" {
		t.Errorf("rules hash, in-slot: %v %q", due, why)
	}
	if due, _ := fullScanDue(site, false, now, other); due {
		t.Error("rules hash, out-of-slot site waits for its weekday")
	}
}

func TestHiddenPlugins(t *testing.T) {
	site := t.TempDir()
	mk := func(rel, content string) {
		p := filepath.Join(site, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	mk("plugins/index.php", "<?php // Silence is golden.")
	mk("plugins/akismet/akismet.php", "<?php\n/**\n * Plugin Name: Akismet\n */\n")
	mk("plugins/scanner-helper-pro/scanner-helper-pro.php", "<?php\n/**\n * Plugin Name: Scanner Helper Pro\n */\n")
	mk("plugins/seocore/layout.css", "<?php ?>")
	os.MkdirAll(filepath.Join(site, "plugins", "core-handler"), 0o755)
	mk("plugins/.hidden/x.php", "<?php")

	mk("plugins/blocksy-companion-pro/blocksy-companion.php", "<?php\n/*\nPlugin Name: Blocksy Companion (Premium)\n*/\n")
	mk("plugins/media-library-plus/main.php", "<?php\n/**\n * Plugin Name: Media Library Folders\n */\n")
	env := &models.Environment{Plugins: `[{"name":"akismet","title":"Akismet","status":"active"},{"name":"blocksy-companion","title":"Blocksy Companion","status":"active"},{"name":"media-library-plus-pro","title":"Media Library Folders","status":"active"}]`}
	inv, ok := inventoryNames(env)
	if !ok || !inv.names["akismet"] {
		t.Fatalf("inventory: %v %v", ok, inv)
	}
	found, err := hiddenPlugins(site, inv)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]hiddenPlugin{}
	for _, h := range found {
		got[h.Dir] = h
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 unlisted dirs (premium suffix and title matches must be listed), got %v", found)
	}
	if hiddenPluginVerdict(got["scanner-helper-pro"]) != "hidden" || hiddenPluginVerdict(got["seocore"]) != "leftover" {
		t.Errorf("verdicts: %s %s %s", hiddenPluginVerdict(got["scanner-helper-pro"]), hiddenPluginVerdict(got["core-handler"]), hiddenPluginVerdict(got["seocore"]))
	}
	if hiddenPluginVerdict(hiddenPlugin{HasHeader: true, AgeDays: 1}) != "new" {
		t.Error("a header plugin one day old is new, not hidden")
	}
	if hiddenPluginVerdict(hiddenPlugin{Dir: "001061s2", Empty: true}) != "decoy" || hiddenPluginVerdict(hiddenPlugin{Dir: "mepr-i18n", Empty: true}) != "leftover" {
		t.Error("random-named empty dir is a decoy; a slug-named empty dir is a leftover")
	}
	if hiddenPluginVerdict(got["core-handler"]) != "leftover" {
		t.Errorf("core-handler is an ordinary-looking empty dir: %s", hiddenPluginVerdict(got["core-handler"]))
	}
	var many []hiddenPlugin
	for i := 0; i < 6; i++ {
		many = append(many, hiddenPlugin{Dir: "p", HasHeader: true})
	}
	if _, stale := inventoryStale(many); !stale {
		t.Error("six unlisted header plugins means the inventory is stale")
	}
	if h := got["scanner-helper-pro"]; !h.HasHeader || h.PHPFiles != 1 {
		t.Errorf("hidden plugin not detected: %+v", h)
	}
	if h := got["seocore"]; h.HasHeader || h.Empty || h.PHPFiles != 0 {
		t.Errorf("decoy with css stub: %+v", h)
	}
	if h := got["core-handler"]; !h.Empty {
		t.Errorf("empty decoy: %+v", h)
	}
	if !inventoryMatchesTree(site, inv) {
		t.Error("inventory naming akismet must match a tree that has plugins/akismet")
	}
	other, _ := inventoryNames(&models.Environment{Plugins: `[{"name":"a"},{"name":"b"},{"name":"c"}]`})
	if inventoryMatchesTree(site, other) {
		t.Error("an inventory naming none of the directories describes a different plugin dir")
	}
	// One shared plugin present out of many is still a different directory (Bedrock with strays).
	bedrock, _ := inventoryNames(&models.Environment{Plugins: `[{"name":"akismet"},{"name":"b"},{"name":"c"},{"name":"d"},{"name":"e"}]`})
	if inventoryMatchesTree(site, bedrock) {
		t.Error("one match out of five inventory names must not count as the same plugin dir")
	}
	if _, ok := inventoryNames(&models.Environment{}); ok {
		t.Error("empty inventory must report not ok")
	}
}

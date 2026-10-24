package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSiteEnvEntry(t *testing.T) {
	cases := []struct {
		entry  string
		folder string
		env    string
		id     uint
		ok     bool
	}{
		{"mopec_2456/staging", "mopec_2456", "staging", 2456, true},
		{"mopec_2456/production", "mopec_2456", "production", 2456, true},
		{"mopec_2456", "", "", 0, false},
		{"mopec_2456/backup", "", "", 0, false},
		{"mopec_2456/staging/backup", "", "", 0, false},
		{"mopec-staging_/staging", "", "", 0, false},
		{"../mopec_2456/staging", "", "", 0, false},
		{"/staging", "", "", 0, false},
	}
	for _, c := range cases {
		folder, env, id, ok := parseSiteEnvEntry(c.entry)
		if ok != c.ok || folder != c.folder || env != c.env || id != c.id {
			t.Errorf("parseSiteEnvEntry(%q) = %q, %q, %d, %v; want %q, %q, %d, %v",
				c.entry, folder, env, id, ok, c.folder, c.env, c.id, c.ok)
		}
	}
}

func TestReadOrphanAllowlistAcceptsEnvironmentFolders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "list.txt")
	body := strings.Join([]string{
		"# comment",
		"oldsite_12",
		"mopec_2456/staging",
		"mopec_2456/staging\t36 GB",
		"mopec_2456/backup",
		"mopec-staging_/staging",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := readOrphanAllowlist(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got["oldsite_12"] || !got["mopec_2456/staging"] {
		t.Fatalf("allowlist = %v; want oldsite_12 and mopec_2456/staging only", got)
	}
}

func TestScanEnvironmentOrphans(t *testing.T) {
	root := t.TempDir()
	mk := func(rel string) {
		if err := os.MkdirAll(filepath.Join(root, rel), 0755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(rel string) {
		if err := os.WriteFile(filepath.Join(root, rel), []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Staging removed: production row only, staging folder on disk.
	mk("mopec_2456/production")
	mk("mopec_2456/staging/backup")
	write("mopec_2456/staging/backup/wp-config.php")
	// Both environments still exist.
	mk("live_10/production")
	mk("live_10/staging")
	// No environment rows at all: never a candidate, counted instead.
	mk("unsynced_11/staging")
	// Not the site's current folder name: the site pass owns it.
	mk("renamed_12/staging")
	// A non-environment folder and a symlink named like one are ignored.
	mk("other_13/production")
	mk("other_13/quicksave")
	if err := os.Symlink(filepath.Join(root, "other_13/quicksave"), filepath.Join(root, "other_13/staging")); err != nil {
		t.Fatal(err)
	}

	byName := map[string]bool{"mopec_2456": true, "live_10": true, "unsynced_11": true, "other_13": true, "renamed_new_12": true}
	envs := map[uint]map[string]bool{
		2456: {"production": true},
		10:   {"production": true, "staging": true},
		13:   {"production": true},
		12:   {"production": true},
	}

	orphans, skipped, err := scanEnvironmentOrphans(root, byName, envs)
	if err != nil {
		t.Fatal(err)
	}
	if len(orphans) != 1 || orphans[0].Name != "mopec_2456/staging" || orphans[0].Environment != "staging" || orphans[0].SiteID != 2456 {
		t.Fatalf("orphans = %+v; want only mopec_2456/staging", orphans)
	}
	if orphans[0].Size != 4 {
		t.Errorf("size = %d; want 4", orphans[0].Size)
	}
	if skipped != 1 {
		t.Errorf("skippedNoRows = %d; want 1", skipped)
	}
}

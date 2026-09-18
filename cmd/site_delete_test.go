package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/CaptainCore/captaincore/models"
)

func TestSiteFolderName(t *testing.T) {
	cases := []struct {
		slug string
		id   uint
		want string
		ok   bool
	}{
		{"capwebsolutions", 3036, "capwebsolutions_3036", true},
		{"OYM22", 12, "OYM22_12", true},
		{"a-b", 1, "a-b_1", true},
		{"", 1, "", false},
		{"site", 0, "", false},
		{"../etc", 5, "", false},
		{"a/b", 5, "", false},
		{"a b", 5, "", false},
		{"a_b", 5, "", false},
		{"a.b", 5, "", false},
		{"-lead", 5, "", false},
	}
	for _, c := range cases {
		got, err := siteFolderName(&models.Site{Site: c.slug, SiteID: c.id})
		if c.ok && (err != nil || got != c.want) {
			t.Errorf("siteFolderName(%q, %d) = %q, %v; want %q", c.slug, c.id, got, err, c.want)
		}
		if !c.ok && err == nil {
			t.Errorf("siteFolderName(%q, %d) accepted %q; want refusal", c.slug, c.id, got)
		}
	}
}

func TestSiteFolderForDelete(t *testing.T) {
	root := t.TempDir()
	site := &models.Site{Site: "example", SiteID: 7}
	folder := filepath.Join(root, "example_7")

	// Missing folder is reported, not an error that stops the delete.
	got, err := siteFolderForDelete(root, site)
	if !errors.Is(err, errSiteFolderMissing) || got != folder {
		t.Fatalf("missing folder: got %q, %v", got, err)
	}

	// A real directory directly under the root passes.
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err = siteFolderForDelete(root, site)
	if err != nil || got != folder {
		t.Fatalf("real folder: got %q, %v", got, err)
	}

	// A symlink at the folder's name is refused even when it points inside root.
	other := filepath.Join(root, "other_8")
	if err := os.Mkdir(other, 0o755); err != nil {
		t.Fatal(err)
	}
	link := &models.Site{Site: "linked", SiteID: 9}
	if err := os.Symlink(other, filepath.Join(root, "linked_9")); err != nil {
		t.Fatal(err)
	}
	if _, err := siteFolderForDelete(root, link); err == nil {
		t.Error("symlink folder accepted; want refusal")
	}

	// A symlink pointing outside the root is refused.
	outside := t.TempDir()
	escape := &models.Site{Site: "escape", SiteID: 10}
	if err := os.Symlink(outside, filepath.Join(root, "escape_10")); err != nil {
		t.Fatal(err)
	}
	if _, err := siteFolderForDelete(root, escape); err == nil {
		t.Error("escaping symlink accepted; want refusal")
	}

	// A plain file at the folder's name is refused.
	file := &models.Site{Site: "file", SiteID: 11}
	if err := os.WriteFile(filepath.Join(root, "file_11"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := siteFolderForDelete(root, file); err == nil {
		t.Error("regular file accepted; want refusal")
	}

	// Bad roots.
	for _, bad := range []string{"", "/", filepath.Join(root, "does-not-exist")} {
		if _, err := siteFolderForDelete(bad, site); err == nil || errors.Is(err, errSiteFolderMissing) {
			t.Errorf("root %q accepted; want refusal (got %v)", bad, err)
		}
	}
}

func TestSiteRemoteForDelete(t *testing.T) {
	site := &models.Site{Site: "example", SiteID: 7}
	good := []struct{ in, parent, target string }{
		{"Anchor-B2:CaptainCoreStorage/Sites/1", "Anchor-B2:CaptainCoreStorage/Sites/1", "Anchor-B2:CaptainCoreStorage/Sites/1/example_7"},
		{"Anchor-B2:CaptainCoreStorage/Sites/1/", "Anchor-B2:CaptainCoreStorage/Sites/1", "Anchor-B2:CaptainCoreStorage/Sites/1/example_7"},
		{"b2:bucket", "b2:bucket", "b2:bucket/example_7"},
	}
	for _, c := range good {
		parent, target, err := siteRemoteForDelete(c.in, site)
		if err != nil || parent != c.parent || target != c.target {
			t.Errorf("siteRemoteForDelete(%q) = %q, %q, %v; want %q, %q", c.in, parent, target, err, c.parent, c.target)
		}
	}
	bad := []string{
		"",
		"   ",
		"Anchor-B2:",           // remote root
		"Anchor-B2:/",          // remote root with slash
		"Anchor-B2",            // no colon
		":bucket/path",         // empty remote
		"Anchor-B2:a/../b",     // traversal
		"Anchor-B2:a//b",       // empty segment
		"Anchor-B2:a/./b",      // relative segment
		"Anchor B2:bucket/x",   // whitespace in remote
		"Anchor-B2:bucket/x y", // whitespace in path
		"/mnt/disks/storage",   // local path, not a remote
	}
	for _, in := range bad {
		if _, target, err := siteRemoteForDelete(in, site); err == nil {
			t.Errorf("siteRemoteForDelete(%q) accepted %q; want refusal", in, target)
		}
	}
	if _, _, err := siteRemoteForDelete("Anchor-B2:bucket/x", &models.Site{Site: "../x", SiteID: 1}); err == nil {
		t.Error("traversal slug accepted; want refusal")
	}
}

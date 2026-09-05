package cmd

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A v1.0.0 binary must order every plausible future tag after itself.
func TestCompareReleaseVersionsForward(t *testing.T) {
	cases := []struct {
		current, release string
		want             int
	}{
		{"1.0.0", "v1.0.1", -1},
		{"1.0.0", "v1.1.0", -1},
		{"1.0.0", "v1.10.0", -1},
		{"1.9.0", "v1.10.0", -1},
		{"1.0.0", "v2.0.0", -1},
		{"1.99.99", "v2.0.0", -1},
		{"1.0.0", "v1.0.0", 0},
		{"1.0.0", "v1.0.0-rc1", 0}, // prerelease suffix ignored; GitHub never marks one Latest
		{"1.0.0-next", "v1.0.0", 0},
		{"1.0.1", "v1.0.0", 1},
		{"2.0.0", "v1.99.99", 1},
	}
	for _, c := range cases {
		if got := compareReleaseVersions(c.current, c.release); got != c.want {
			t.Errorf("compare(%q, %q) = %d, want %d", c.current, c.release, got, c.want)
		}
	}
}

func TestReleaseTagValidation(t *testing.T) {
	good := []string{"v1.0.0", "v1.0.1", "v1.10.2", "v2.0.0", "v10.20.30", "v2.0.0-rc1", "v1.0.0+build.5"}
	bad := []string{"", "v", "../evil", "v1/../evil", "v1.0.0/x", "v1.0.0?x", "1.0.0 ", "v1.0.0\n"}
	for _, tag := range good {
		if !reReleaseTag.MatchString(tag) {
			t.Errorf("expected %q to be a valid release tag", tag)
		}
	}
	for _, tag := range bad {
		if reReleaseTag.MatchString(tag) {
			t.Errorf("expected %q to be rejected", tag)
		}
	}
}

// The asset name for the running platform must be one goreleaser produces.
func TestReleaseAssetNameMatchesGoreleaserContract(t *testing.T) {
	name, err := releaseAssetName()
	if err != nil {
		t.Skipf("no release build for this platform: %v", err)
	}
	contract := map[string]bool{
		"captaincore_Linux_x86_64.tar.gz":  true,
		"captaincore_Linux_arm64.tar.gz":   true,
		"captaincore_Linux_armv7.tar.gz":   true,
		"captaincore_Darwin_x86_64.tar.gz": true,
		"captaincore_Darwin_arm64.tar.gz":  true,
	}
	if !contract[name] {
		t.Errorf("asset name %q is not in the goreleaser contract", name)
	}
}

func TestLatestReleaseTagFollowsRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/releases/latest" {
			http.Redirect(w, r, "https://github.com/CaptainCore/captaincore/releases/tag/v1.7.3", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	t.Setenv("CAPTAINCORE_RELEASE_BASE", srv.URL+"/releases")

	tag, err := latestReleaseTag(&http.Client{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("latestReleaseTag: %v", err)
	}
	if tag != "v1.7.3" {
		t.Errorf("tag = %q, want v1.7.3", tag)
	}
}

func TestLatestReleaseTagRejectsNonRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	t.Setenv("CAPTAINCORE_RELEASE_BASE", srv.URL+"/releases")
	if _, err := latestReleaseTag(&http.Client{Timeout: 5 * time.Second}); err == nil {
		t.Error("expected an error when /releases/latest does not redirect")
	}
}

func writeTestArchive(t *testing.T, dir string, binaryContent []byte) (archivePath, checksumsPath string) {
	t.Helper()
	archivePath = filepath.Join(dir, "captaincore_Linux_x86_64.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	// goreleaser puts license and readme in the archive too; extraction must skip them.
	for _, entry := range []struct {
		name string
		body []byte
		mode int64
	}{
		{"license", []byte("MIT"), 0644},
		{"readme.md", []byte("# readme"), 0644},
		{"captaincore", binaryContent, 0755},
	} {
		if err := tw.WriteHeader(&tar.Header{Name: entry.name, Mode: entry.mode, Size: int64(len(entry.body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(entry.body); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gz.Close()
	f.Close()

	data, _ := os.ReadFile(archivePath)
	sum := sha256.Sum256(data)
	checksumsPath = filepath.Join(dir, "checksums.txt")
	// goreleaser format: "<sha256>  <name>", one line per asset.
	lines := "0000000000000000000000000000000000000000000000000000000000000000  captaincore_Darwin_arm64.tar.gz\n" +
		hex.EncodeToString(sum[:]) + "  captaincore_Linux_x86_64.tar.gz\n"
	if err := os.WriteFile(checksumsPath, []byte(lines), 0644); err != nil {
		t.Fatal(err)
	}
	return archivePath, checksumsPath
}

func TestVerifyChecksumAndExtract(t *testing.T) {
	dir := t.TempDir()
	archivePath, checksumsPath := writeTestArchive(t, dir, []byte("#!/bin/sh\necho captaincore 9.9.9\n"))

	if err := verifyChecksum(archivePath, checksumsPath, "captaincore_Linux_x86_64.tar.gz"); err != nil {
		t.Fatalf("verifyChecksum: %v", err)
	}
	if err := verifyChecksum(archivePath, checksumsPath, "captaincore_Linux_armv7.tar.gz"); err == nil {
		t.Error("expected an error for an asset missing from checksums.txt")
	}

	dest := filepath.Join(dir, "captaincore.new")
	if err := extractBinary(archivePath, dest); err != nil {
		t.Fatalf("extractBinary: %v", err)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != "#!/bin/sh\necho captaincore 9.9.9\n" {
		t.Errorf("extracted content mismatch: %q", got)
	}
	info, _ := os.Stat(dest)
	if info.Mode()&0100 == 0 {
		t.Error("extracted binary is not executable")
	}
}

func TestVerifyChecksumDetectsTamper(t *testing.T) {
	dir := t.TempDir()
	archivePath, checksumsPath := writeTestArchive(t, dir, []byte("original"))
	if err := os.WriteFile(archivePath, []byte("tampered"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := verifyChecksum(archivePath, checksumsPath, "captaincore_Linux_x86_64.tar.gz"); err == nil {
		t.Error("expected a checksum mismatch")
	}
}

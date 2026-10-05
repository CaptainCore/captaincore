package cmd

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newTestKey generates a key with ssh-keygen and returns its PEM bytes and
// the fingerprint ssh-keygen itself reports.
func newTestKey(t *testing.T, passphrase string) ([]byte, string) {
	t.Helper()
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen not available")
	}
	path := filepath.Join(t.TempDir(), "id")
	if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", passphrase, "-C", "test", "-f", path).CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v %s", err, out)
	}
	pem, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("ssh-keygen", "-E", "md5", "-l", "-f", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	m := md5FingerprintPattern.FindSubmatch(out)
	if m == nil {
		t.Fatalf("no fingerprint in %q", out)
	}
	return pem, string(m[1])
}

func TestInstallKey(t *testing.T) {
	pem, want := newTestKey(t, "")
	dest := filepath.Join(t.TempDir(), "keys", "1", "7")

	// CRLF line endings and a missing final newline are both repaired.
	mangled := strings.TrimRight(strings.ReplaceAll(string(pem), "\n", "\r\n"), "\r\n")
	got, err := installKey(dest, base64.StdEncoding.EncodeToString([]byte(mangled)))
	if err != nil {
		t.Fatalf("installKey: %v", err)
	}
	if got != want {
		t.Errorf("fingerprint = %q, want %q", got, want)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("mode = %o, want 0600", info.Mode().Perm())
	}
	stored, _ := os.ReadFile(dest)
	if string(stored) != string(pem) {
		t.Errorf("stored key differs from the original")
	}

	// Replacing an existing id works and leaves no temp files behind.
	if _, err := installKey(dest, base64.StdEncoding.EncodeToString(pem)); err != nil {
		t.Fatalf("reinstall: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Dir(dest))
	if len(entries) != 1 {
		t.Errorf("key dir holds %d entries, want 1", len(entries))
	}
}

func TestInstallKeyRejects(t *testing.T) {
	encrypted, _ := newTestKey(t, "secret")
	cases := map[string]string{
		"not base64":     "%%%",
		"not a key":      base64.StdEncoding.EncodeToString([]byte("hello")),
		"broken key":     base64.StdEncoding.EncodeToString([]byte("-----BEGIN OPENSSH PRIVATE KEY-----\nAAAA\n-----END OPENSSH PRIVATE KEY-----\n")),
		"passphrase key": base64.StdEncoding.EncodeToString(encrypted),
	}
	for name, encoded := range cases {
		dir := t.TempDir()
		dest := filepath.Join(dir, "1", "7")
		if _, err := installKey(dest, encoded); err == nil {
			t.Errorf("%s: installKey accepted it", name)
		}
		if _, err := os.Stat(dest); err == nil {
			t.Errorf("%s: a key file was left behind", name)
		}
		if entries, _ := os.ReadDir(filepath.Join(dir, "1")); len(entries) != 0 {
			t.Errorf("%s: %d temp files left behind", name, len(entries))
		}
	}
}

func TestRemoveKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "7")
	os.WriteFile(path, []byte("x"), 0600)
	if removed, err := removeKey(path); err != nil || !removed {
		t.Errorf("removeKey = %v, %v; want true, nil", removed, err)
	}
	if removed, err := removeKey(path); err != nil || removed {
		t.Errorf("second removeKey = %v, %v; want false, nil", removed, err)
	}
}

func TestKeyPathForID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".captaincore"), 0700)
	// path_keys left empty, as `captaincore connect` used to write it.
	os.WriteFile(filepath.Join(home, ".captaincore", "config.json"),
		[]byte(`[{"system":{"path_keys":""}},{"captain_id":"1"}]`), 0600)

	saved := captainID
	defer func() { captainID = saved }()
	captainID = "1"

	got, err := keyPathForID("7")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".captaincore", "data", "keys", "1", "7"); got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
	for _, bad := range []string{"", "..", "../7", "7/8", "-rf", "a b", "$(id)"} {
		if _, err := keyPathForID(bad); err == nil {
			t.Errorf("id %q was accepted", bad)
		}
	}
	captainID = "1/../2"
	if _, err := keyPathForID("7"); err == nil {
		t.Error("non-numeric captain id was accepted")
	}
}

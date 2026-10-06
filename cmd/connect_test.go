package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestOriginFromURL(t *testing.T) {
	cases := map[string]string{
		"https://anchor.host":            "https://anchor.host",
		"https://Anchor.Host/account/":   "https://anchor.host",
		"http://127.0.0.1:8000/wp-admin": "http://127.0.0.1:8000",
		"https://manager.localhost":      "https://manager.localhost",
		"":                               "",
		"not a url":                      "",
		"anchor.host":                    "",
	}
	for in, want := range cases {
		if got := originFromURL(in); got != want {
			t.Errorf("originFromURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// A fresh `connect` writes a config.json whose folder settings are filled in:
// left blank, bash scripts built $path_recipes/<name>.sh and friends from /.
func TestUpdateConfigFileFreshInstallFillsPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".captaincore"), 0700); err != nil {
		t.Fatal(err)
	}

	action, err := updateConfigFile(connectResponse{Token: "tok", APIURL: "https://example.com/wp-json/captaincore/v1/api", GUIURL: "https://example.com"})
	if err != nil || action != "created" {
		t.Fatalf("updateConfigFile() = %q, %v; want created", action, err)
	}

	data, err := os.ReadFile(filepath.Join(home, ".captaincore", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatal(err)
	}
	var system map[string]string
	for _, e := range entries {
		if raw, ok := e["system"]; ok {
			json.Unmarshal(raw, &system)
		}
	}
	base := filepath.Join(home, ".captaincore")
	want := map[string]string{
		"path":         filepath.Join(base, "sites"),
		"path_tmp":     filepath.Join(base, "tmp"),
		"path_recipes": filepath.Join(base, "recipes"),
		"path_scripts": filepath.Join(base, "scripts"),
		"path_keys":    filepath.Join(base, "data", "keys"),
		"logs":         filepath.Join(base, "logs"),
	}
	for key, dir := range want {
		if system[key] != dir {
			t.Errorf("config.json %s = %q, want %q", key, system[key], dir)
		}
		if key == "path_keys" {
			continue
		}
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Errorf("%s folder %s was not created", key, dir)
		}
	}
}

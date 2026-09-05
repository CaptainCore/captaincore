package cmd

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/CaptainCore/captaincore/version"
)

// runtimeAssets is the embedded app/ + lib/ tree handed over by package main.
var runtimeAssets fs.FS

// assetsStampFile records which binary version last wrote ~/.captaincore/{app,lib}.
const assetsStampFile = ".assets-version"

// SetRuntimeAssets registers the embedded runtime scripts. Called from main
// before Execute so ensureAssets can run inside cobra.OnInitialize.
func SetRuntimeAssets(assets fs.FS) {
	runtimeAssets = assets
}

// captaincoreHome returns ~/.captaincore.
func captaincoreHome() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".captaincore")
}

// isSourceCheckout reports whether ~/.captaincore is a git clone of the CLI
// repository. Those installs build the binary in place and own their scripts,
// so neither ensureAssets nor `captaincore upgrade` may touch them.
func isSourceCheckout() bool {
	base := captaincoreHome()
	if base == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(base, ".git"))
	return err == nil
}

// ensureAssets writes the embedded app/ and lib/ trees into ~/.captaincore when
// they are missing or were written by a different binary version. It never
// runs on a source checkout and never touches config.json or data/.
func ensureAssets() {
	if runtimeAssets == nil || isSourceCheckout() {
		return
	}
	base := captaincoreHome()
	if base == "" {
		return
	}
	stampPath := filepath.Join(base, assetsStampFile)
	if stamp, err := os.ReadFile(stampPath); err == nil && strings.TrimSpace(string(stamp)) == version.Version {
		if _, err := os.Stat(filepath.Join(base, "app")); err == nil {
			if _, err := os.Stat(filepath.Join(base, "lib", "remote-scripts")); err == nil {
				return
			}
		}
	}
	if err := materializeAssets(base); err != nil {
		fmt.Fprintln(os.Stderr, "Warning: could not unpack runtime scripts into "+base+": "+err.Error())
		return
	}
	if err := os.WriteFile(stampPath, []byte(version.Version+"\n"), 0644); err != nil {
		fmt.Fprintln(os.Stderr, "Warning: could not write "+stampPath+": "+err.Error())
	}
}

// materializeAssets copies every embedded file under base, creating
// directories as needed. Scripts under app/ and lib/remote-scripts/ are
// executable; everything else is plain.
func materializeAssets(base string) error {
	if err := os.MkdirAll(base, 0755); err != nil {
		return err
	}
	return fs.WalkDir(runtimeAssets, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "." {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") { // .DS_Store and friends
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		target := filepath.Join(base, filepath.FromSlash(path))
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		data, err := fs.ReadFile(runtimeAssets, path)
		if err != nil {
			return err
		}
		mode := os.FileMode(0644)
		if strings.HasPrefix(path, "app/") || strings.HasPrefix(path, "lib/remote-scripts/") {
			mode = 0755
		}
		tmp := target + ".tmp"
		if err := os.WriteFile(tmp, data, mode); err != nil {
			return err
		}
		if err := os.Chmod(tmp, mode); err != nil {
			os.Remove(tmp)
			return err
		}
		return os.Rename(tmp, target)
	})
}

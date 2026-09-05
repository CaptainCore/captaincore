package cmd

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/CaptainCore/captaincore/version"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const upgradeRepo = "CaptainCore/captaincore"

var flagUpgradeCheck, flagUpgradeYes, flagUpgradeForce bool
var flagUpgradeVersion string

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade the CaptainCore CLI to the latest GitHub release",
	Long: `Checks github.com/` + upgradeRepo + ` for a newer release, downloads the
build for this platform, verifies it against the release checksums and replaces
the running binary in place. Runtime scripts under ~/.captaincore are refreshed
by the new binary on its first run.

A source checkout (~/.captaincore/.git) is left alone: update those with
git pull and go build.`,
	Example: `  captaincore upgrade
  captaincore upgrade --check
  captaincore upgrade --yes
  captaincore upgrade --version=v1.2.0`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := upgradeRun(); err != nil {
			fmt.Fprintln(os.Stderr, "Error: "+err.Error())
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(upgradeCmd)
	upgradeCmd.Flags().BoolVar(&flagUpgradeCheck, "check", false, "Only report whether a newer release exists (exit 1 when behind)")
	upgradeCmd.Flags().BoolVarP(&flagUpgradeYes, "yes", "y", false, "Do not prompt before replacing the binary")
	upgradeCmd.Flags().BoolVar(&flagUpgradeForce, "force", false, "Reinstall even when already current, or on a source checkout")
	upgradeCmd.Flags().StringVar(&flagUpgradeVersion, "version", "", "Install a specific release tag (e.g. v1.2.0) instead of the latest")
}

// releaseBaseURL is the GitHub releases root. CAPTAINCORE_RELEASE_BASE
// overrides it so the flow can be exercised against a local mock.
func releaseBaseURL() string {
	if env := os.Getenv("CAPTAINCORE_RELEASE_BASE"); env != "" {
		return strings.TrimRight(env, "/")
	}
	return "https://github.com/" + upgradeRepo + "/releases"
}

// releaseAssetName maps the running platform onto the goreleaser archive name.
func releaseAssetName() (string, error) {
	var osName, arch string
	switch runtime.GOOS {
	case "linux":
		osName = "Linux"
	case "darwin":
		osName = "Darwin"
	default:
		return "", fmt.Errorf("no release builds for %s", runtime.GOOS)
	}
	switch runtime.GOARCH {
	case "amd64":
		arch = "x86_64"
	case "arm64":
		arch = "arm64"
	case "arm":
		arch = "armv7"
	default:
		return "", fmt.Errorf("no release builds for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	return fmt.Sprintf("captaincore_%s_%s.tar.gz", osName, arch), nil
}

// latestReleaseTag follows the releases/latest redirect and reads the tag out
// of the Location header. No API call, so no rate limit or token.
func latestReleaseTag(client *http.Client) (string, error) {
	noRedirect := &http.Client{
		Timeout: client.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := noRedirect.Get(releaseBaseURL() + "/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	loc := resp.Header.Get("Location")
	if resp.StatusCode < 300 || resp.StatusCode > 399 || loc == "" {
		return "", fmt.Errorf("could not resolve the latest release (HTTP %d)", resp.StatusCode)
	}
	idx := strings.LastIndex(loc, "/tag/")
	if idx < 0 {
		return "", fmt.Errorf("unexpected release redirect: %s", loc)
	}
	tag := strings.TrimSpace(loc[idx+len("/tag/"):])
	if tag == "" {
		return "", fmt.Errorf("unexpected release redirect: %s", loc)
	}
	return tag, nil
}

// compareVersions orders two dotted version strings numerically, ignoring a
// leading "v" and anything after a "-" or "+". Returns -1, 0 or 1.
func compareReleaseVersions(a, b string) int {
	parse := func(v string) []int {
		v = strings.TrimPrefix(strings.TrimSpace(v), "v")
		if i := strings.IndexAny(v, "-+"); i >= 0 {
			v = v[:i]
		}
		parts := strings.Split(v, ".")
		nums := make([]int, 3)
		for i := 0; i < len(parts) && i < 3; i++ {
			n, _ := strconv.Atoi(parts[i])
			nums[i] = n
		}
		return nums
	}
	x, y := parse(a), parse(b)
	for i := 0; i < 3; i++ {
		if x[i] < y[i] {
			return -1
		}
		if x[i] > y[i] {
			return 1
		}
	}
	return 0
}

func upgradeRun() error {
	client := &http.Client{Timeout: 5 * time.Minute}

	asset, err := releaseAssetName()
	if err != nil {
		return err
	}

	tag := flagUpgradeVersion
	if tag != "" && !strings.HasPrefix(tag, "v") {
		tag = "v" + tag
	}
	if tag == "" {
		fmt.Println("Checking github.com/" + upgradeRepo + " for the latest release...")
		tag, err = latestReleaseTag(client)
		if err != nil {
			return err
		}
	}

	current := version.Version
	fmt.Printf("  current:  %s\n", current)
	fmt.Printf("  release:  %s\n", tag)

	cmp := compareReleaseVersions(current, tag)
	if flagUpgradeCheck {
		if cmp < 0 {
			fmt.Println("A newer release is available. Run 'captaincore upgrade' to install it.")
			os.Exit(1)
		}
		fmt.Println("CaptainCore CLI is up to date.")
		return nil
	}
	if cmp >= 0 && !flagUpgradeForce {
		if cmp > 0 {
			fmt.Println("The running version is newer than the release. Nothing to do.")
		} else {
			fmt.Println("CaptainCore CLI is already up to date.")
		}
		return nil
	}

	if isSourceCheckout() && !flagUpgradeForce {
		fmt.Println("~/.captaincore is a git checkout, so this machine builds captaincore from source.")
		fmt.Println("Update it with:  cd ~/.captaincore && git pull && go build -o captaincore")
		fmt.Println("To replace the binary with the release build anyway, re-run with --force.")
		return errors.New("source checkout")
	}

	target, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(target); err == nil {
		target = resolved
	}
	fmt.Printf("  binary:   %s\n", target)

	if !flagUpgradeYes && term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Printf("Upgrade to %s? [y/N] ", tag)
		answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	downloadBase := releaseBaseURL() + "/download/" + tag
	tmpDir, err := os.MkdirTemp("", "captaincore-upgrade-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	fmt.Println("Downloading " + asset + "...")
	archivePath := filepath.Join(tmpDir, asset)
	if err := downloadFile(client, downloadBase+"/"+asset, archivePath); err != nil {
		return err
	}
	checksumsPath := filepath.Join(tmpDir, "checksums.txt")
	if err := downloadFile(client, downloadBase+"/checksums.txt", checksumsPath); err != nil {
		return err
	}

	fmt.Println("Verifying checksum...")
	if err := verifyChecksum(archivePath, checksumsPath, asset); err != nil {
		return err
	}

	newBinary := target + ".new"
	if err := extractBinary(archivePath, newBinary); err != nil {
		os.Remove(newBinary)
		return err
	}
	defer os.Remove(newBinary)

	out, err := exec.Command(newBinary, "version").Output()
	if err != nil || !strings.HasPrefix(string(out), "captaincore ") {
		return fmt.Errorf("the downloaded binary did not run (%v)", err)
	}

	if err := os.Rename(newBinary, target); err != nil {
		if !errors.Is(err, os.ErrPermission) {
			return err
		}
		if _, sudoErr := exec.LookPath("sudo"); sudoErr != nil {
			return fmt.Errorf("%s is not writable and sudo is unavailable", target)
		}
		fmt.Println(filepath.Dir(target) + " is not writable, using sudo...")
		mv := exec.Command("sudo", "mv", "-f", newBinary, target)
		mv.Stdin, mv.Stdout, mv.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := mv.Run(); err != nil {
			return fmt.Errorf("sudo mv failed: %v", err)
		}
	}

	fmt.Println("Upgraded: " + strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0]))
	if serverRunning() {
		fmt.Println("A 'captaincore server' process is running. Restart it to pick up the new binary:")
		fmt.Println("  sudo service captaincore restart")
	}
	return nil
}

func downloadFile(client *http.Client, url, dest string) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed (HTTP %d): %s", resp.StatusCode, url)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// verifyChecksum compares the file's sha256 with its line in a goreleaser
// checksums.txt ("<hash>  <name>").
func verifyChecksum(filePath, checksumsPath, name string) error {
	data, err := os.ReadFile(checksumsPath)
	if err != nil {
		return err
	}
	expected := ""
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == name {
			expected = strings.ToLower(fields[0])
			break
		}
	}
	if expected == "" {
		return fmt.Errorf("%s is not listed in checksums.txt", name)
	}
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expected {
		return fmt.Errorf("checksum mismatch for %s (expected %s, got %s)", name, expected, actual)
	}
	return nil
}

// extractBinary pulls the `captaincore` entry out of a release tarball.
func extractBinary(archivePath, dest string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg || filepath.Base(hdr.Name) != "captaincore" {
			continue
		}
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	}
	return errors.New("captaincore binary not found in the archive")
}

// serverRunning reports whether a `captaincore server` process exists.
func serverRunning() bool {
	out, err := exec.Command("pgrep", "-f", "captaincore server").Output()
	if err != nil {
		return false
	}
	self := strconv.Itoa(os.Getpid())
	for _, pid := range strings.Fields(string(out)) {
		if pid != self {
			return true
		}
	}
	return false
}

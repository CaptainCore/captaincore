package cmd

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

// SSH keys the CLI connects to sites with live at <path_keys>/<captain_id>/<id>,
// the name a site's `key` field or the default_key configuration hands to
// ssh -i. The Manager's Keys screen installs and removes them through
// `captaincore server` with these two commands.

var flagKeyID string

// keyIDPattern is the character set a key id may use. The Manager sends its
// numeric key id; hand-placed keys use names like `1_old`. The id becomes a
// path component and is interpolated into ssh -i strings, so nothing else
// gets through.
var keyIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

var md5FingerprintPattern = regexp.MustCompile(`MD5:((?:[0-9a-f]{2}:){15}[0-9a-f]{2})`)

var keyCmd = &cobra.Command{
	Use:   "key",
	Short: "SSH key commands",
}

var keyAddCmd = &cobra.Command{
	Use:   "add <base64-private-key> --id=<id>",
	Short: "Installs an SSH private key and prints its MD5 fingerprint",
	Long: `Decodes a base64 encoded SSH private key and writes it to
<path_keys>/<captain_id>/<id> with mode 0600, replacing any key already
stored under that id. Prints the key's MD5 fingerprint, which the Manager
saves on the key's record.

The key must be unencrypted: the CLI connects without a terminal, so a
passphrase protected key is refused instead of stored.`,
	Example: `  captaincore key add LS0tLS1CRUdJTi... --id=2`,
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		path, err := keyPathForID(flagKeyID)
		if err != nil {
			keyFail(err)
		}
		fingerprint, err := installKey(path, args[0])
		if err != nil {
			keyFail(err)
		}
		fmt.Print(fingerprint)
	},
}

var keyDeleteCmd = &cobra.Command{
	Use:     "delete --id=<id>",
	Short:   "Removes an installed SSH private key",
	Example: `  captaincore key delete --id=2`,
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		path, err := keyPathForID(flagKeyID)
		if err != nil {
			keyFail(err)
		}
		removed, err := removeKey(path)
		if err != nil {
			keyFail(err)
		}
		if removed {
			fmt.Printf("Removed key %s\n", flagKeyID)
		} else {
			fmt.Printf("No key stored for id %s\n", flagKeyID)
		}
	},
}

// keyFail exits without colour codes: the Manager reads this output back.
func keyFail(err error) {
	fmt.Fprintf(os.Stderr, "Error: %s\n", err)
	os.Exit(1)
}

// keyPathForID resolves where the key with this id lives for the current
// captain.
func keyPathForID(id string) (string, error) {
	if id == "" {
		return "", errors.New("specify the key with --id=<id>")
	}
	if !keyIDPattern.MatchString(id) {
		return "", fmt.Errorf("key id %q may only use letters, digits, dot, dash and underscore", id)
	}
	if !rePortToken.MatchString(captainID) {
		return "", fmt.Errorf("captain id %q is not numeric", captainID)
	}
	_, system, _, err := loadCaptainConfig()
	if err != nil || system == nil {
		return "", errors.New("configuration file not found. Run 'captaincore connect' first")
	}
	if system.PathKeys == "" {
		return "", errors.New("path_keys is not set in config.json")
	}
	return filepath.Join(system.PathKeys, captainID, id), nil
}

// installKey decodes a base64 private key, checks that ssh can use it, and
// moves it into place atomically. Returns the MD5 fingerprint.
func installKey(path, encoded string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return "", errors.New("the key is not valid base64")
	}
	// Pasted keys can carry CRLF line endings, and OpenSSH refuses a key
	// file without its final newline ("invalid format").
	key := bytes.ReplaceAll(decoded, []byte("\r\n"), []byte("\n"))
	key = append(bytes.TrimSpace(key), '\n')
	if !bytes.Contains(key, []byte("PRIVATE KEY-----")) {
		return "", errors.New("the key is not an SSH private key")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("creating %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return "", fmt.Errorf("writing key: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // a no-op once the rename succeeds
	if _, err := tmp.Write(key); err != nil {
		tmp.Close()
		return "", fmt.Errorf("writing key: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("writing key: %w", err)
	}
	if err := os.Chmod(tmpPath, 0600); err != nil {
		return "", fmt.Errorf("writing key: %w", err)
	}

	// -y derives the public key, which proves ssh can load the file. An empty
	// -P makes a passphrase protected key fail here rather than prompt.
	if out, err := exec.Command("ssh-keygen", "-y", "-P", "", "-f", tmpPath).CombinedOutput(); err != nil {
		reason := strings.TrimSpace(string(out))
		if reason == "" {
			reason = err.Error()
		}
		return "", fmt.Errorf("ssh cannot use this key (passphrase protected keys are not supported): %s", reason)
	}
	out, err := exec.Command("ssh-keygen", "-E", "md5", "-l", "-f", tmpPath).Output()
	if err != nil {
		return "", fmt.Errorf("reading key fingerprint: %w", err)
	}
	match := md5FingerprintPattern.FindSubmatch(out)
	if match == nil {
		return "", fmt.Errorf("unexpected ssh-keygen output: %s", strings.TrimSpace(string(out)))
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return "", fmt.Errorf("installing key: %w", err)
	}
	return string(match[1]), nil
}

// removeKey deletes an installed key. A key that is already gone is not an
// error, so a retried delete from the Manager stays quiet.
func removeKey(path string) (bool, error) {
	err := os.Remove(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func init() {
	rootCmd.AddCommand(keyCmd)
	keyCmd.AddCommand(keyAddCmd)
	keyCmd.AddCommand(keyDeleteCmd)
	keyAddCmd.Flags().StringVar(&flagKeyID, "id", "", "Key id (the file name under path_keys/<captain_id>/)")
	keyDeleteCmd.Flags().StringVar(&flagKeyID, "id", "", "Key id to remove")
}

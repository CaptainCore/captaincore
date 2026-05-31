package cmd

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/CaptainCore/captaincore/models"
)

// CaptainCore ingests site/environment data from the manager API, which in turn
// aggregates values reported by remote (potentially compromised) customer sites.
// Several of those fields are later interpolated into `bash -c` command strings
// (SSH command construction, lighthouse, regenerate-thumbnails, etc.). These
// helpers validate/sanitize untrusted values so they can't inject shell syntax.

var (
	// Filesystem paths, usernames, hostnames, ports, capture tokens — none of
	// these legitimately contain shell metacharacters.
	rePathToken  = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
	reHostToken  = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)
	reUserToken  = regexp.MustCompile(`^[A-Za-z0-9._@-]+$`)
	rePortToken  = regexp.MustCompile(`^[0-9]+$`)
	reEnvKeyName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	reCleanURL   = regexp.MustCompile(`^https?://[A-Za-z0-9._:/~%-]+$`)
)

// isSafeShellToken reports whether s is non-empty and free of shell
// metacharacters (safe to interpolate unquoted into a command).
func isSafeShellToken(s string) bool {
	return rePathToken.MatchString(s)
}

// isValidEnvKey reports whether s is a valid shell variable name (it is emitted
// unquoted as the LHS of `export <key>=...`).
func isValidEnvKey(s string) bool {
	return reEnvKeyName.MatchString(s)
}

// isSafeURL reports whether s is a clean http(s) URL with no shell-dangerous
// characters — safe to interpolate into a shell command.
func isSafeURL(s string) bool {
	if s == "" || strings.ContainsAny(s, " \t\r\n`$;|&<>(){}\\\"'*?") {
		return false
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false
	}
	return reCleanURL.MatchString(s)
}

// shellSingleQuote wraps s in single quotes, escaping any embedded single quote
// via the standard '\'' sequence. Safe for values that legitimately contain
// special characters (passwords, env-var values). The escape survives an outer
// double-quoted layer because a backslash before ' is literal inside "".
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// sanitizeEnvironment blanks structural environment fields that contain shell
// metacharacters before the record is stored. A blanked field degrades to safe
// default behavior downstream rather than injecting into a privileged command.
// Secret/free-form fields (Password, DB creds) are left intact and quoted at the
// point of use instead.
func sanitizeEnvironment(env *models.Environment) {
	if env.Address != "" && !reHostToken.MatchString(env.Address) {
		env.Address = ""
	}
	if env.Username != "" && !reUserToken.MatchString(env.Username) {
		env.Username = ""
	}
	if env.Port != "" && !rePortToken.MatchString(env.Port) {
		env.Port = ""
	}
	if env.HomeDirectory != "" && !rePathToken.MatchString(env.HomeDirectory) {
		env.HomeDirectory = ""
	}
	if env.HomeURL != "" && !isSafeURL(env.HomeURL) {
		env.HomeURL = ""
	}
}

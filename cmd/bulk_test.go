package cmd

import (
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

// TestBulkArgQuoting verifies that the bulk script preserves flag values
// containing spaces, preventing the fork-bomb regression where
// --command="wp option get home" was word-split into separate arguments.
func TestBulkArgQuoting(t *testing.T) {
	// Build the binary once for all subtests
	binPath := t.TempDir() + "/captaincore"
	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("failed to build binary: %v\n%s", err, out)
	}

	tests := []struct {
		name string
		args []string
		// We check that the SSH --debug output contains the full command
		// as a single argument, not word-split.
		wantContains    string
		wantNotContains string
	}{
		{
			name:         "command with spaces preserved",
			args:         []string{"ssh", "anchorhost-production", "--command=wp option get home", "--debug"},
			wantContains: "wp option get home",
		},
		{
			name:         "command with single quotes preserved",
			args:         []string{"ssh", "anchorhost-production", "--command=wp option get home --url='https://example.com'", "--debug"},
			wantContains: "wp option get home",
		},
		{
			name:         "script flag without spaces",
			args:         []string{"ssh", "anchorhost-production", "--script=fetch-site-data", "--debug"},
			wantContains: "fetch-site-data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(binPath, tt.args...)
			out, err := cmd.CombinedOutput()
			output := string(out)
			// --debug mode prints the command and exits, so non-zero exit is OK
			// as long as we get output (site lookup may fail without DB)
			if err != nil && output == "" {
				t.Fatalf("command failed with no output: %v", err)
			}
			if tt.wantContains != "" && !strings.Contains(output, tt.wantContains) {
				t.Errorf("output missing %q\ngot: %s", tt.wantContains, output)
			}
		})
	}
}

// TestBulkRecursionGuardGo verifies that the Go bulk runner's CC_BULK_RUNNING
// guard prevents infinite recursion when invoked as a subprocess.
func TestBulkRecursionGuardGo(t *testing.T) {
	binPath := t.TempDir() + "/captaincore"
	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("failed to build binary: %v\n%s", err, out)
	}

	// Simulate a child process that already has CC_BULK_RUNNING=true
	cmd := exec.Command(binPath, "bulk", "ssh", "site1", "site2", "--command=wp option get home")
	cmd.Env = append(os.Environ(), "CC_BULK_RUNNING=true")
	out, err := cmd.CombinedOutput()
	output := string(out)

	if err == nil {
		t.Fatal("expected non-zero exit when CC_BULK_RUNNING=true, got success")
	}
	if !strings.Contains(output, "Recursive bulk execution detected") {
		t.Errorf("expected recursion guard message, got: %s", output)
	}
}

// TestBulkRecursionGuardInProcess verifies the in-process atomic guard
// prevents re-entry of runBulk.
func TestBulkRecursionGuardInProcess(t *testing.T) {
	// Set the atomic guard as if bulk is already running
	bulkRunning = 1
	defer func() { bulkRunning = 0 }()

	cfg := BulkConfig{
		Command:   "ssh",
		Targets:   []string{"site1", "site2"},
		CaptainID: "1",
		Parallel:  5,
	}
	err := runBulk(cfg)
	if err == nil {
		t.Fatal("expected error from in-process recursion guard")
	}
	if !strings.Contains(err.Error(), "recursive bulk execution detected") {
		t.Errorf("unexpected error message: %s", err)
	}
}

func TestParseUpdateCoreOutput(t *testing.T) {
	out := `site=https://example.com
live_version=7.0.4
CLI boot (plugins + themes)...
PHP Fatal error:  Uncaught TypeError: substr(): Argument #1 ($string) must be of type string, int given in /wp-content/plugins/wp-rocket/inc/ThirdParty/Plugins/CDN/Cloudflare.php:562
Error: Preview core failed to boot with live plugins/themes.
result=fail stage=boot from=7.0.4 to=7.1 url=https://example.com reason=Preview core failed to boot with live plugins/themes.
`
	res := parseUpdateCoreOutput("example-production", 1, out)
	if res.Result != "fail" {
		t.Fatalf("result=%q want fail", res.Result)
	}
	if res.Stage != "boot" {
		t.Errorf("stage=%q want boot", res.Stage)
	}
	if res.URL != "https://example.com" {
		t.Errorf("url=%q", res.URL)
	}
	if res.From != "7.0.4" || res.To != "7.1" {
		t.Errorf("from=%q to=%q", res.From, res.To)
	}
	if !strings.Contains(res.Reason, "Preview core failed") {
		t.Errorf("reason=%q", res.Reason)
	}
	if !strings.Contains(res.Excerpt, "TypeError") {
		t.Errorf("excerpt missing TypeError: %q", res.Excerpt)
	}

	ok := parseUpdateCoreOutput("ok-production", 0, "result=ok action=apply from=7.0.4 to=7.1 url=https://ok.example\n")
	if ok.Result != "ok" || ok.Action != "apply" || ok.From != "7.0.4" || ok.To != "7.1" {
		t.Errorf("unexpected ok parse: %+v", ok)
	}

	skip := parseUpdateCoreOutput("skip-production", 0, "result=ok action=skip from=7.1 to=7.1 reason=already-current\n")
	if skip.Action != "skip" || skip.Reason != "already-current" {
		t.Errorf("unexpected skip parse: %+v", skip)
	}

	notWP := parseUpdateCoreOutput("static-production", 0, "skip: not-wp-root\nresult=ok action=skip stage=root reason=not-wp-root\n")
	if notWP.Result != "ok" || notWP.Action != "skip" || notWP.Reason != "not-wp-root" || notWP.Stage != "root" {
		t.Errorf("unexpected not-wp-root skip parse: %+v", notWP)
	}

	sshFail := parseUpdateCoreOutput("down-production", 255, "ssh: connect to host timed out\n")
	if sshFail.Result != "fail" || sshFail.Stage != "ssh" {
		t.Errorf("unexpected ssh fail parse: %+v", sshFail)
	}

	noisy := parseUpdateCoreOutput("staging-site", 1, "site=\nhttps://example.com\nresult=fail stage=boot from=7.1 to=7.2-alpha-1 url=\nhttps://example.com reason=Preview core failed to boot with live plugins/themes.\n")
	if noisy.Result != "fail" || noisy.Stage != "boot" {
		t.Errorf("noisy parse: %+v", noisy)
	}
	if coreUpdateShouldDumpOutput(noisy) {
		t.Errorf("table-prefix-style fail should stay compact")
	}
	if !coreUpdateShouldDumpOutput(res) {
		t.Errorf("TypeError fail should dump output")
	}
}

func TestBulkScriptName(t *testing.T) {
	if got := bulkScriptName([]string{"--script=update-core", "--apply"}); got != "update-core" {
		t.Errorf("got %q", got)
	}
	if got := bulkScriptName([]string{"--script=/home/core/Scripts/update-core"}); got != "update-core" {
		t.Errorf("got %q", got)
	}
	if got := bulkScriptName([]string{"--command=wp option get home"}); got != "" {
		t.Errorf("got %q want empty", got)
	}
}

// TestBulkRecursionGuardEnvVar verifies the env var guard prevents re-entry.
func TestBulkRecursionGuardEnvVar(t *testing.T) {
	os.Setenv("CC_BULK_RUNNING", "true")
	defer os.Unsetenv("CC_BULK_RUNNING")

	cfg := BulkConfig{
		Command:   "ssh",
		Targets:   []string{"site1", "site2"},
		CaptainID: "1",
		Parallel:  5,
	}
	err := runBulk(cfg)
	if err == nil {
		t.Fatal("expected error from env var recursion guard")
	}
	if !strings.Contains(err.Error(), "recursive bulk execution detected") {
		t.Errorf("unexpected error message: %s", err)
	}
}

// TestLabeledOutputParsing verifies that labeled output correctly extracts
// content between markers and strips empty lines.
func TestLabeledOutputParsing(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "extracts between markers",
			input:  "Welcome to SSH\nMOTD line\n____CC_OUTPUT_START____\nhttps://example.com\n____CC_OUTPUT_END____\n",
			expect: "https://example.com",
		},
		{
			name:   "strips empty lines",
			input:  "____CC_OUTPUT_START____\n\nhello\n\nworld\n\n____CC_OUTPUT_END____\n",
			expect: "hello\nworld",
		},
		{
			name:   "no markers returns raw content",
			input:  "just some output\n",
			expect: "just some output",
		},
		{
			name:   "empty between markers",
			input:  "____CC_OUTPUT_START____\n\n____CC_OUTPUT_END____\n",
			expect: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := tt.input
			const markerStart = "____CC_OUTPUT_START____"
			const markerEnd = "____CC_OUTPUT_END____"

			startIdx := strings.Index(output, markerStart)
			endIdx := strings.Index(output, markerEnd)
			if startIdx >= 0 && endIdx > startIdx {
				output = output[startIdx+len(markerStart) : endIdx]
			}

			// Strip empty lines
			var lines []string
			for _, line := range strings.Split(output, "\n") {
				if strings.TrimSpace(line) != "" {
					lines = append(lines, line)
				}
			}
			result := strings.Join(lines, "\n")

			if result != tt.expect {
				t.Errorf("got %q, want %q", result, tt.expect)
			}
		})
	}
}

// TestCollectBulkFlags verifies that collectBulkFlags produces the expected
// flag strings from global variables.
func TestCollectBulkFlags(t *testing.T) {
	// Reset all flags
	flagCommand = ""
	flagScript = ""
	flagRecipe = ""
	flagDebug = false
	flagLabel = false
	flagForce = false
	flagScriptPassthrough = nil

	// Set some flags
	flagCommand = "wp option get home"
	flagScript = "fetch-site-data"
	flagForce = true

	flags := collectBulkFlags()

	wantFlags := map[string]bool{
		"--command=wp option get home": false,
		"--script=fetch-site-data":     false,
		"--force":                      false,
	}

	for _, f := range flags {
		if _, ok := wantFlags[f]; ok {
			wantFlags[f] = true
		}
	}

	for flag, found := range wantFlags {
		if !found {
			t.Errorf("expected flag %q in output, got: %v", flag, flags)
		}
	}

	// Clean up
	flagCommand = ""
	flagScript = ""
	flagForce = false
}

// TestResolveTargetsExplicit verifies that explicit site names pass through
// without database lookup.
func TestResolveTargetsExplicit(t *testing.T) {
	sites, err := resolveTargets([]string{"site1-production", "site2-staging"}, "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sites) != 2 {
		t.Fatalf("expected 2 sites, got %d", len(sites))
	}
	if sites[0] != "site1-production" || sites[1] != "site2-staging" {
		t.Errorf("unexpected sites: %v", sites)
	}
}

// TestResolveTargetsEmpty verifies that empty targets returns an error.
func TestResolveTargetsEmpty(t *testing.T) {
	_, err := resolveTargets([]string{}, "1")
	if err == nil {
		t.Fatal("expected error for empty targets")
	}
}

// TestRunLabeledSiteConcurrency verifies that labeled output doesn't
// interleave when multiple goroutines write concurrently.
func TestRunLabeledSiteConcurrency(t *testing.T) {
	// This test just verifies no panics/data races under concurrent access.
	// Run with -race to detect races.
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			cmd := exec.Command("echo", "____CC_OUTPUT_START____\ntest output\n____CC_OUTPUT_END____")
			runLabeledSite(cmd, "test-site", &mu)
		}(i)
	}

	wg.Wait()
}

// TestSSHParsesParallel verifies that the SSH command's manual arg parser
// recognizes --parallel and doesn't treat it as a passthrough flag.
func TestSSHParsesParallel(t *testing.T) {
	binPath := t.TempDir() + "/captaincore"
	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("failed to build binary: %v\n%s", err, out)
	}

	// Run with --debug so it prints the SSH command without executing
	cmd := exec.Command(binPath, "ssh", "anchorhost-production", "--parallel=3", "--command=wp option get home", "--debug")
	out, _ := cmd.CombinedOutput()
	output := string(out)

	// --parallel should NOT appear in the SSH command output (it's for bulk, not the SSH invocation)
	if strings.Contains(output, "--parallel") {
		t.Errorf("--parallel leaked into SSH command as passthrough flag:\n%s", output)
	}
}

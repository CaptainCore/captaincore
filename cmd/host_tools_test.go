package cmd

import (
	"os"
	"regexp"
	"testing"
)

// The node tools are single Python files: a second top-level def with the same
// name silently replaces the first. That happened once (server health() over
// the per-container health(ct)) and broke every rebuild, restore, deploy and
// restart on the node, each rolling back at its final check.
func TestNodeToolsHaveNoDuplicateFunctions(t *testing.T) {
	def := regexp.MustCompile(`(?m)^def (\w+)\(`)
	for _, f := range []string{"host_backups.py", "host_apply.py"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		seen := map[string]bool{}
		for _, m := range def.FindAllStringSubmatch(string(src), -1) {
			if seen[m[1]] {
				t.Errorf("%s defines %s() twice; the second replaces the first", f, m[1])
			}
			seen[m[1]] = true
		}
	}
}

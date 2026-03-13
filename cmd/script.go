package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var scriptCmd = &cobra.Command{
	Use:   "script",
	Short: "Script commands",
}

var scriptListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available built-in scripts",
	Run: func(cmd *cobra.Command, args []string) {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return
		}

		scriptsDir := filepath.Join(home, ".captaincore", "lib", "remote-scripts")
		entries, err := os.ReadDir(scriptsDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading scripts directory: %v\n", err)
			return
		}

		// Find the longest script name for alignment
		maxName := 0
		type scriptEntry struct {
			name string
			desc string
		}
		var scripts []scriptEntry

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			// Skip hidden files
			if strings.HasPrefix(name, ".") {
				continue
			}
			desc := parseScriptDescription(filepath.Join(scriptsDir, name))
			scripts = append(scripts, scriptEntry{name: name, desc: desc})
			if len(name) > maxName {
				maxName = len(name)
			}
		}

		for _, s := range scripts {
			if s.desc != "" {
				fmt.Printf("  %-*s  %s\n", maxName, s.name, s.desc)
			} else {
				fmt.Printf("  %s\n", s.name)
			}
		}
	},
}

// parseScriptDescription extracts a description from a script file's header comments.
// Supports two formats:
//
//	"#  Description: <text>"  (structured header block)
//	"#   <text>"              (first meaningful comment line after shebang/blank lines)
func parseScriptDescription(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	firstDesc := ""

	for scanner.Scan() {
		lineNum++
		if lineNum > 20 {
			break
		}
		line := scanner.Text()

		// Check for explicit "Description:" field — always wins
		if strings.Contains(line, "Description:") {
			parts := strings.SplitN(line, "Description:", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}

		// Only try fallback parsing if we haven't found one yet
		if firstDesc != "" {
			continue
		}

		// Skip shebang, empty lines, and comment-only markers
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#!") || trimmed == "#" {
			continue
		}

		// Look for comment lines with content (e.g., "#   Some description")
		if strings.HasPrefix(trimmed, "#") {
			text := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			// Skip lines that look like metadata, usage examples, dashes, or URLs
			if text == "" || strings.HasPrefix(text, "`") || strings.HasPrefix(text, "---") || strings.HasPrefix(text, "[--") || strings.HasPrefix(text, "Command:") || strings.HasPrefix(text, "http") {
				// Check for "--- Description text ---" format
				if strings.HasPrefix(text, "---") && strings.HasSuffix(text, "---") {
					inner := strings.TrimSpace(strings.Trim(text, "-"))
					if inner != "" {
						firstDesc = inner
					}
				}
				continue
			}
			firstDesc = text
			continue
		}

		// Non-comment line reached before finding description — stop looking for fallback
		break
	}
	return firstDesc
}

func init() {
	rootCmd.AddCommand(scriptCmd)
	scriptCmd.AddCommand(scriptListCmd)
}

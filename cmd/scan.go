package cmd

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/CaptainCore/captaincore/scan"
	"github.com/spf13/cobra"
)

var (
	scanFormat      string
	scanRules       []string
	scanFilesFrom   string
	scanMinSeverity string
	scanWorkers     int
	scanQuiet       bool
	scanNoDecode    bool
)

var scanCmd = &cobra.Command{
	Use:   "scan <path>...",
	Short: "Scan files or directories for malware with CaptainCore's rule set",
	Long: `Scan runs the native malware scanner (lib/malware-signatures.json plus any
drop-ins under lib/malware-signatures.d/) over the given files and directories.
Directories are walked; .git trees are skipped. Exit status is 1 when anything
is found, 2 when the rule set fails to load, 0 otherwise.

Examples:
  captaincore scan /path/to/quicksave
  captaincore scan --format=json wp-content/themes/x/functions.php
  find . -name '*.php' -newer marker | captaincore scan --files-from=- --format=csv`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 && scanFilesFrom == "" {
			return fmt.Errorf("requires at least one <path> or --files-from")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		os.Exit(runScan(args))
	},
}

func runScan(args []string) int {
	var rs *scan.RuleSet
	var err error
	if len(scanRules) > 0 {
		rs = &scan.RuleSet{Version: 2}
		for _, p := range scanRules {
			extra, e := scan.LoadRuleSet(p)
			if e != nil {
				fmt.Fprintln(os.Stderr, "Error:", e)
				return 2
			}
			rs.Merge(extra)
		}
	} else if rs, err = scan.LoadDefaultRuleSet(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return 2
	}
	s := scan.New(rs, scan.Options{Workers: scanWorkers, NoDecode: scanNoDecode})
	for _, e := range s.Errors {
		fmt.Fprintln(os.Stderr, "Warning:", e)
	}

	var files []string
	var dirs []string
	if scanFilesFrom != "" {
		var r *os.File
		if scanFilesFrom == "-" {
			r = os.Stdin
		} else if r, err = os.Open(scanFilesFrom); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			return 2
		}
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			if line := strings.TrimSpace(sc.Text()); line != "" {
				args = append(args, line)
			}
		}
	}
	for _, a := range args {
		info, err := os.Stat(a)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Warning:", err)
			continue
		}
		if info.IsDir() {
			dirs = append(dirs, a)
		} else {
			files = append(files, a)
		}
	}

	var findings []scan.Finding
	scanned := 0
	var errs []string
	if len(files) > 0 {
		r := s.ScanPaths(files, "")
		findings, scanned, errs = append(findings, r.Findings...), scanned+r.Scanned, append(errs, r.Errors...)
	}
	for _, d := range dirs {
		r := s.ScanDir(d)
		// Prefix the relative file with the directory the user named so
		// output stays meaningful when several roots are scanned.
		for i := range r.Findings {
			r.Findings[i].File = filepath.ToSlash(filepath.Join(filepath.Base(filepath.Clean(d)), r.Findings[i].File))
		}
		findings, scanned, errs = append(findings, r.Findings...), scanned+r.Scanned, append(errs, r.Errors...)
	}
	if scanMinSeverity != "" {
		min := scan.SeverityRank(scanMinSeverity)
		kept := findings[:0]
		for _, f := range findings {
			if scan.SeverityRank(f.Severity) >= min {
				kept = append(kept, f)
			}
		}
		findings = kept
	}

	switch scanFormat {
	case "json":
		out, _ := json.MarshalIndent(findings, "", "    ")
		fmt.Println(string(out))
	case "csv":
		w := csv.NewWriter(os.Stdout)
		w.Write([]string{"filename", "signature_id", "signature_name", "signature_description", "matched_text"})
		for _, f := range findings {
			l := f.Legacy()
			w.Write([]string{l.Filename, l.SignatureID, l.SignatureName, l.SignatureDescription, l.MatchedText})
		}
		w.Flush()
	default:
		for _, f := range findings {
			color := "\033[31m"
			switch f.Severity {
			case "high", "medium":
				color = "\033[33m"
			case "low":
				color = "\033[34m"
			}
			where := fmt.Sprintf("%s:%d", f.File, f.Line)
			if f.Layer != "" {
				where = fmt.Sprintf("%s (inside %s payload)", f.File, f.Layer)
			}
			fmt.Printf("%s[%s]\033[0m %s — %s\n", color, f.Severity, f.Name, where)
			if !scanQuiet && f.Match != "" {
				fmt.Printf("    %s\n", f.Match)
			}
		}
		if !scanQuiet {
			fmt.Fprintf(os.Stderr, "%d file(s) scanned, %d finding(s), %d rule(s), %d hash indicator(s)\n", scanned, len(findings), s.RuleCount(), s.HashCount())
		}
	}
	for _, e := range errs {
		fmt.Fprintln(os.Stderr, "Warning:", e)
	}
	if len(findings) > 0 {
		return 1
	}
	return 0
}

func init() {
	rootCmd.AddCommand(scanCmd)
	scanCmd.Flags().StringVar(&scanFormat, "format", "text", "Output format: text, json, csv")
	scanCmd.Flags().StringArrayVar(&scanRules, "rules", nil, "Rule file to use instead of the shipped set (repeatable)")
	scanCmd.Flags().StringVar(&scanFilesFrom, "files-from", "", "Read paths to scan from a file, or - for stdin")
	scanCmd.Flags().StringVar(&scanMinSeverity, "min-severity", "", "Only report findings at or above this severity (low, medium, high, critical)")
	scanCmd.Flags().IntVar(&scanWorkers, "workers", 0, "Scanner goroutines (default: CPU count)")
	scanCmd.Flags().BoolVar(&scanQuiet, "quiet", false, "Findings only, no matched text or summary")
	scanCmd.Flags().BoolVar(&scanNoDecode, "no-decode", false, "Do not decode base64, deflate, rot13 or escaped payloads before matching")
}

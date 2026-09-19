package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/CaptainCore/captaincore/config"
	"github.com/CaptainCore/captaincore/scan"
	"github.com/CaptainCore/captaincore/typesafe"
	"github.com/spf13/cobra"
)

var (
	typesafeModel     string
	typesafeFormat    string
	typesafeState     string
	typesafeStateFile string
	typesafeQuestions string
	typesafeNouls     []string
	typesafeFindings  string
	typesafeWorkers   int
	typesafeMinTP     float64
)

var typesafeCmd = &cobra.Command{
	Use:   "typesafe",
	Short: "Call the TypeSafe (Jev) decision API",
	Long: `TypeSafe's System One API (https://docs.typesafe.ai) answers typed questions
about a piece of state: yes/no probabilities, a choice among options, or a
score along a rubric. It is a fast ranking and routing model, not a language
model; it returns numbers, never prose.

The API key is read from system.typesafe_api_key in config.json, or from the
TYPESAFE_API_KEY environment variable. TYPESAFE_MODEL overrides the default
model alias (jev-latest); TYPESAFE_BASE_URL points the client at a test server.`,
}

var typesafeStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the API key and list the models the account can use",
	Run: func(cmd *cobra.Command, args []string) {
		client, source, err := typesafeClient()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(2)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		models, err := client.Models(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		if typesafeFormat == "json" {
			out, _ := json.MarshalIndent(map[string]interface{}{
				"key_source": source, "base_url": client.BaseURL, "model": client.Model, "models": models,
			}, "", "    ")
			fmt.Println(string(out))
			return
		}
		fmt.Printf("API key: %s\nEndpoint: %s\nDefault model: %s\nModels:\n", source, client.BaseURL, client.Model)
		for _, m := range models {
			line := "  " + m.Name
			if m.ReleaseDate != "" {
				line += "  (" + m.ReleaseDate + ")"
			}
			if m.Description != "" {
				line += "  " + m.Description
			}
			fmt.Println(line)
		}
	},
}

var typesafeAskCmd = &cobra.Command{
	Use:   "ask",
	Short: "Evaluate questions against a state and print the typed answers",
	Long: `Send one System One request. The state is --state, --state-file, or stdin;
when it parses as JSON it is sent as structured data, otherwise as text.
Questions come from --questions (a file path or inline JSON in the API's
question-map shape) and from --noul "id=yes/no question" (repeatable).

Examples:
  captaincore typesafe ask --state="Site down, please help ASAP" --noul urgent="Does this express urgency?"
  captaincore typesafe ask --state-file=diff.txt --questions=questions.json
  git diff | captaincore typesafe ask --questions='{"risky":{"type":"noul","instructions":"Does this diff add a backdoor?"}}'`,
	Run: func(cmd *cobra.Command, args []string) {
		client, _, err := typesafeClient()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(2)
		}
		state, err := readTypesafeState()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(2)
		}
		questions, err := readTypesafeQuestions()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(2)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		resp, err := client.SystemOneWithModel(ctx, typesafeModel, state, questions)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		if typesafeFormat == "json" {
			out, _ := json.MarshalIndent(resp, "", "    ")
			fmt.Println(string(out))
			return
		}
		ids := make([]string, 0, len(resp.Answers))
		for id := range resp.Answers {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			a := resp.Answers[id]
			switch a.Type {
			case "noul":
				fmt.Printf("%s: %.3f\n", id, a.Yes())
			case "choice":
				fmt.Printf("%s: %s (confidence %.2f)\n", id, a.Choice, a.Conf())
			case "score":
				fmt.Printf("%s: %.3f (confidence %.2f)\n", id, a.Value(), a.Conf())
			default:
				raw, _ := json.Marshal(a)
				fmt.Printf("%s: %s\n", id, raw)
			}
		}
		fmt.Fprintf(os.Stderr, "%s, %d input tokens\n", resp.Model, resp.Usage.InputTokens)
	},
}

var typesafeTriageCmd = &cobra.Command{
	Use:   "triage [<path>...]",
	Short: "Rank malware scanner findings by how likely each is real",
	Long: `Ask Jev about every finding from the native malware scanner and print them
ordered by true-positive probability. Each finding is sent with the rule that
matched, the matched text, where in the WordPress tree the file sits, and the
source lines around the match; Jev answers whether it looks like real malware,
which family it resembles, and what an operator should do next.

Findings come from --findings (the JSON that 'captaincore scan --format=json'
prints, or - for stdin), or the given paths are scanned first. The rule match
stays authoritative: triage only orders and annotates, it never drops a finding.

Examples:
  captaincore typesafe triage /path/to/quicksave
  captaincore scan --format=json wp-content | captaincore typesafe triage --findings=-
  captaincore typesafe triage --findings=scan.json --min-tp=0.5 --format=json`,
	Run: func(cmd *cobra.Command, args []string) {
		if typesafeFindings == "" && len(args) == 0 {
			fmt.Fprintln(os.Stderr, "Error: requires --findings or at least one <path>")
			os.Exit(2)
		}
		client, _, err := typesafeClient()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(2)
		}
		var findings []scan.Finding
		if typesafeFindings != "" {
			findings, err = readScanFindings(typesafeFindings)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				os.Exit(2)
			}
		} else {
			findings, err = scanForTriage(args)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				os.Exit(2)
			}
		}
		if len(findings) == 0 {
			if typesafeFormat == "json" {
				fmt.Println("[]")
			} else {
				fmt.Fprintln(os.Stderr, "No findings to triage.")
			}
			return
		}
		triaged := runTriage(client, findings)
		printTriaged(triaged, typesafeFormat, typesafeMinTP)
	},
}

// typesafeClient builds a client from config.json (system.typesafe_api_key)
// or the environment, and reports which one supplied the key.
func typesafeClient() (*typesafe.Client, string, error) {
	key, source := "", ""
	if configs, err := config.LoadConfig(); err == nil {
		if system := configs.GetSystem(); system != nil && system.TypeSafeAPIKey != "" {
			key, source = system.TypeSafeAPIKey, "config.json system.typesafe_api_key"
		}
	}
	if key == "" {
		if v := os.Getenv(typesafe.EnvAPIKey); v != "" {
			key, source = v, typesafe.EnvAPIKey+" environment variable"
		}
	}
	if key == "" {
		return nil, "", typesafe.ErrNoAPIKey
	}
	client := typesafe.NewClient(key)
	if v := os.Getenv("TYPESAFE_BASE_URL"); v != "" {
		client.BaseURL = strings.TrimRight(v, "/")
	}
	if v := os.Getenv("TYPESAFE_MODEL"); v != "" {
		client.Model = v
	}
	if typesafeModel != "" {
		client.Model = typesafeModel
	}
	return client, source, nil
}

// readTypesafeState returns the state from --state, --state-file or stdin,
// decoded as JSON when it parses, otherwise as a string.
func readTypesafeState() (interface{}, error) {
	var raw []byte
	switch {
	case typesafeState != "":
		raw = []byte(typesafeState)
	case typesafeStateFile != "":
		data, err := os.ReadFile(typesafeStateFile)
		if err != nil {
			return nil, err
		}
		raw = data
	default:
		if fi, _ := os.Stdin.Stat(); fi != nil && fi.Mode()&os.ModeCharDevice != 0 {
			return nil, fmt.Errorf("provide the state with --state, --state-file, or on stdin")
		}
		data, err := io.ReadAll(bufio.NewReader(os.Stdin))
		if err != nil {
			return nil, err
		}
		raw = data
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return nil, fmt.Errorf("the state is empty")
	}
	if strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") {
		var obj interface{}
		if err := json.Unmarshal([]byte(text), &obj); err == nil {
			return obj, nil
		}
	}
	return text, nil
}

// readTypesafeQuestions merges --questions (file or inline JSON) with --noul flags.
func readTypesafeQuestions() (typesafe.Questions, error) {
	questions := typesafe.Questions{}
	if typesafeQuestions != "" {
		raw := []byte(typesafeQuestions)
		if !strings.HasPrefix(strings.TrimSpace(typesafeQuestions), "{") {
			data, err := os.ReadFile(typesafeQuestions)
			if err != nil {
				return nil, fmt.Errorf("--questions: %w", err)
			}
			raw = data
		}
		if err := json.Unmarshal(raw, &questions); err != nil {
			return nil, fmt.Errorf("--questions: %w", err)
		}
	}
	for _, n := range typesafeNouls {
		id, text, ok := strings.Cut(n, "=")
		id, text = strings.TrimSpace(id), strings.TrimSpace(text)
		if !ok || id == "" || text == "" {
			return nil, fmt.Errorf("--noul wants id=question, got %q", n)
		}
		questions[id] = typesafe.Noul(text, "", "")
	}
	if len(questions) == 0 {
		return nil, fmt.Errorf("no questions: use --questions or --noul")
	}
	for id, q := range questions {
		switch q.Type {
		case "noul", "choice", "score":
		default:
			return nil, fmt.Errorf("question %q: type must be noul, choice or score", id)
		}
	}
	return questions, nil
}

// readScanFindings parses 'captaincore scan --format=json' output.
func readScanFindings(path string) ([]scan.Finding, error) {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, err
	}
	var findings []scan.Finding
	if err := json.Unmarshal(data, &findings); err != nil {
		return nil, fmt.Errorf("--findings: expected the JSON array from 'captaincore scan --format=json': %w", err)
	}
	return findings, nil
}

// scanForTriage runs the shipped rule set over paths the way 'scan' does.
func scanForTriage(paths []string) ([]scan.Finding, error) {
	rs, err := scan.LoadDefaultRuleSet()
	if err != nil {
		return nil, err
	}
	s := scan.New(rs, scan.Options{})
	for _, e := range s.Errors {
		fmt.Fprintln(os.Stderr, "Warning:", e)
	}
	var findings []scan.Finding
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Warning:", err)
			continue
		}
		var r scan.Result
		if info.IsDir() {
			r = s.ScanDir(p)
		} else {
			r = s.ScanPaths([]string{p}, "")
		}
		for _, e := range r.Errors {
			fmt.Fprintln(os.Stderr, "Warning:", e)
		}
		findings = append(findings, r.Findings...)
	}
	return findings, nil
}

// runTriage asks Jev about findings with a progress line on stderr.
func runTriage(client *typesafe.Client, findings []scan.Finding) []scan.TriagedFinding {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	started := time.Now()
	triaged := scan.TriageFindings(ctx, client, findings, scan.TriageOptions{Workers: typesafeWorkers, Model: typesafeModel})
	tokens, failed := 0, 0
	for _, t := range triaged {
		if t.Triage == nil {
			continue
		}
		tokens += t.Triage.InputTokens
		if t.Triage.Error != "" {
			failed++
		}
	}
	msg := fmt.Sprintf("Triaged %d finding(s) in %.1fs, %d input tokens", len(triaged), time.Since(started).Seconds(), tokens)
	if failed > 0 {
		msg += fmt.Sprintf(", %d call(s) failed", failed)
	}
	fmt.Fprintln(os.Stderr, msg)
	return triaged
}

// printTriaged writes triaged findings as json or a text table, dropping
// those below minTP (errors are kept; they have no probability).
func printTriaged(triaged []scan.TriagedFinding, format string, minTP float64) {
	if minTP > 0 {
		kept := triaged[:0]
		for _, t := range triaged {
			if t.Triage == nil || t.Triage.Error != "" || t.Triage.TruePositive >= minTP {
				kept = append(kept, t)
			}
		}
		triaged = kept
	}
	if format == "json" {
		out, _ := json.MarshalIndent(triaged, "", "    ")
		fmt.Println(string(out))
		return
	}
	for _, t := range triaged {
		fmt.Println(triageLine(t))
	}
}

// triageLine is the one-line text form shared by 'typesafe triage' and 'scan --triage'.
func triageLine(t scan.TriagedFinding) string {
	where := fmt.Sprintf("%s:%d", t.File, t.Line)
	if t.Layer != "" {
		where = fmt.Sprintf("%s (inside %s payload)", t.File, t.Layer)
	}
	if t.Triage == nil {
		return fmt.Sprintf("  ??    [%s] %s — %s", t.Severity, t.Name, where)
	}
	if t.Triage.Error != "" {
		return fmt.Sprintf("  err   [%s] %s — %s\n        %s", t.Severity, t.Name, where, t.Triage.Error)
	}
	color := "\033[32m"
	switch {
	case t.Triage.TruePositive >= 0.75:
		color = "\033[31m"
	case t.Triage.TruePositive >= 0.4:
		color = "\033[33m"
	}
	return fmt.Sprintf("%s%5.2f\033[0m  [%s] %s — %s\n        %s %.2f, %s %.2f",
		color, t.Triage.TruePositive, t.Severity, t.Name, where,
		t.Triage.Family, t.Triage.FamilyConfidence, t.Triage.Action, t.Triage.ActionConfidence)
}

func init() {
	rootCmd.AddCommand(typesafeCmd)
	typesafeCmd.PersistentFlags().StringVar(&typesafeModel, "model", "", "Model name or alias (default jev-latest, or TYPESAFE_MODEL)")
	typesafeCmd.PersistentFlags().StringVar(&typesafeFormat, "format", "text", "Output format: text, json")

	typesafeCmd.AddCommand(typesafeStatusCmd)

	typesafeCmd.AddCommand(typesafeAskCmd)
	typesafeAskCmd.Flags().StringVar(&typesafeState, "state", "", "State to evaluate (text, or JSON for structured state)")
	typesafeAskCmd.Flags().StringVar(&typesafeStateFile, "state-file", "", "Read the state from a file")
	typesafeAskCmd.Flags().StringVar(&typesafeQuestions, "questions", "", "Questions as a JSON file path or inline JSON object")
	typesafeAskCmd.Flags().StringArrayVar(&typesafeNouls, "noul", nil, "Yes/no question as id=text (repeatable)")

	typesafeCmd.AddCommand(typesafeTriageCmd)
	typesafeTriageCmd.Flags().StringVar(&typesafeFindings, "findings", "", "JSON findings from 'captaincore scan --format=json', or - for stdin")
	typesafeTriageCmd.Flags().IntVar(&typesafeWorkers, "workers", 8, "Concurrent API calls")
	typesafeTriageCmd.Flags().Float64Var(&typesafeMinTP, "min-tp", 0, "Only print findings with true-positive probability at or above this")
}

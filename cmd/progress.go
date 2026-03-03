package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type progressMeta struct {
	Command   string `json:"command"`
	Total     int    `json:"total"`
	PID       int    `json:"pid"`
	StartedAt int64  `json:"started_at"`
	CaptainID string `json:"captain_id"`
	Parallel  int    `json:"parallel"`
}

type progressOutput struct {
	Command        string  `json:"command"`
	Completed      int     `json:"completed"`
	Failed         int     `json:"failed"`
	Total          int     `json:"total"`
	Percent        float64 `json:"percent"`
	PID            int     `json:"pid"`
	Running        bool    `json:"running"`
	StartedAt      int64   `json:"started_at"`
	ElapsedSeconds int64   `json:"elapsed_seconds"`
	Parallel       int     `json:"parallel"`
}

var flagClean bool

var progressCmd = &cobra.Command{
	Use:   "progress",
	Short: "Show progress of running bulk operations",
	Run: func(cmd *cobra.Command, args []string) {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Println("Error:", err)
			os.Exit(1)
		}

		progressDir := filepath.Join(home, ".captaincore", "data", "progress")
		entries, err := filepath.Glob(filepath.Join(progressDir, "*.json"))
		if err != nil || len(entries) == 0 {
			fmt.Println("No bulk operations found.")
			return
		}

		var results []progressOutput
		now := time.Now().Unix()

		for _, metaPath := range entries {
			data, err := os.ReadFile(metaPath)
			if err != nil {
				continue
			}

			var meta progressMeta
			if err := json.Unmarshal(data, &meta); err != nil {
				continue
			}

			// Count completed and failed from log file
			logPath := strings.TrimSuffix(metaPath, ".json") + ".log"
			completed, failed := countLogLines(logPath)

			running := isProcessRunning(meta.PID)
			elapsed := now - meta.StartedAt

			if flagClean && !running {
				os.Remove(metaPath)
				os.Remove(logPath)
				fmt.Printf("Cleaned stale progress files for PID %d (%s)\n", meta.PID, meta.Command)
				continue
			}

			var pct float64
			if meta.Total > 0 {
				pct = float64(completed) / float64(meta.Total) * 100
				// Round to one decimal
				pct = float64(int(pct*10)) / 10
			}

			results = append(results, progressOutput{
				Command:        meta.Command,
				Completed:      completed,
				Failed:         failed,
				Total:          meta.Total,
				Percent:        pct,
				PID:            meta.PID,
				Running:        running,
				StartedAt:      meta.StartedAt,
				ElapsedSeconds: elapsed,
				Parallel:       meta.Parallel,
			})
		}

		if len(results) == 0 {
			fmt.Println("No bulk operations found.")
			return
		}

		if flagFormat == "json" {
			out, _ := json.MarshalIndent(results, "", "    ")
			fmt.Println(string(out))
			return
		}

		for _, r := range results {
			elapsed := formatElapsed(r.ElapsedSeconds)
			status := ""
			if !r.Running {
				status = " " + colorYellow + "(stale - process not running)" + colorNormal
			}
			eta := ""
			if r.Completed > 0 && r.Running && r.Completed < r.Total {
				remaining := r.Total - r.Completed
				secsPerItem := float64(r.ElapsedSeconds) / float64(r.Completed)
				etaSecs := int64(float64(remaining) * secsPerItem)
				eta = fmt.Sprintf(" - eta %s", formatElapsed(etaSecs))
			}
			fmt.Printf("%s: %d/%d (%.1f%%) - running for %s (parallel: %d)%s%s\n",
				r.Command,
				r.Completed,
				r.Total,
				r.Percent,
				elapsed,
				r.Parallel,
				eta,
				status,
			)
			if r.Failed > 0 {
				fmt.Printf("  %s%d failed%s\n", colorYellow, r.Failed, colorNormal)
			}
		}
	},
}

func countLogLines(path string) (completed, failed int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		completed++
		// Line format: "site exitcode timestamp"
		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[1] != "0" {
			failed++
		}
	}
	return completed, failed
}

func formatElapsed(seconds int64) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func init() {
	rootCmd.AddCommand(progressCmd)
	progressCmd.Flags().BoolVar(&flagClean, "clean", false, "Remove stale progress files from dead processes")
}

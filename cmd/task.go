package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Task mirrors the server's Task model for read-only access to sql.db.
type Task struct {
	gorm.Model
	CaptainID int
	ProcessID int
	Command   string
	Status    string
	Response  string
	Origin    string
	Token     string
}

var flagTaskLimit int

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "View and manage server tasks",
}

var taskListCmd = &cobra.Command{
	Use:   "list",
	Short: "List recent tasks",
	Run: func(cmd *cobra.Command, args []string) {
		db, err := openTaskDB()
		if err != nil {
			fmt.Println("Error:", err)
			os.Exit(1)
		}

		var tasks []Task
		query := db.Order("created_at desc").Limit(flagTaskLimit)
		if flagFleet == false {
			cid, _ := strconv.Atoi(captainID)
			query = query.Where("captain_id = ?", cid)
		}
		query.Find(&tasks)

		if len(tasks) == 0 {
			fmt.Println("No tasks found.")
			return
		}

		// Print table header
		fmt.Printf("%-6s  %-12s  %-40s  %-20s  %-20s\n",
			"ID", "Status", "Command", "Created", "Updated")
		fmt.Println("------  ------------  ----------------------------------------  --------------------  --------------------")

		for _, t := range tasks {
			command := t.Command
			if len(command) > 40 {
				command = command[:37] + "..."
			}
			status := t.Status
			if status == "Started" && !isProcessRunning(t.ProcessID) {
				status = "Stalled"
			}
			created := t.CreatedAt.Format(time.DateTime)
			updated := t.UpdatedAt.Format(time.DateTime)
			fmt.Printf("%-6d  %-12s  %-40s  %-20s  %-20s\n",
				t.ID, status, command, created, updated)
		}
	},
}

var taskGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Show full details of a task",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a task <id> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		db, err := openTaskDB()
		if err != nil {
			fmt.Println("Error:", err)
			os.Exit(1)
		}

		id := args[0]
		var task Task
		result := db.First(&task, id)
		if result.Error != nil {
			fmt.Printf("Task %s not found.\n", id)
			return
		}

		status := task.Status
		if status == "Started" && !isProcessRunning(task.ProcessID) {
			status = "Stalled"
		}

		fmt.Printf("ID:         %d\n", task.ID)
		fmt.Printf("Status:     %s\n", status)
		fmt.Printf("Captain ID: %d\n", task.CaptainID)
		fmt.Printf("Command:    %s\n", task.Command)
		fmt.Printf("Created:    %s\n", task.CreatedAt.Format(time.DateTime))
		fmt.Printf("Updated:    %s\n", task.UpdatedAt.Format(time.DateTime))
		if task.Response != "" {
			fmt.Printf("Response:\n%s\n", task.Response)
		}
	},
}

// isProcessRunning checks if a process with the given PID is still alive.
func isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

// openTaskDB opens the server's sql.db database in read-only mode.
func openTaskDB() (*gorm.DB, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dbPath := filepath.Join(home, ".captaincore", "data", "sql.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("task database not found at %s", dbPath)
	}
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open task database: %w", err)
	}
	return db, nil
}

func init() {
	rootCmd.AddCommand(taskCmd)
	taskCmd.AddCommand(taskListCmd)
	taskCmd.AddCommand(taskGetCmd)
	taskListCmd.Flags().IntVarP(&flagTaskLimit, "limit", "l", 10, "Number of tasks to show")
}

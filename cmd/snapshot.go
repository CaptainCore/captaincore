package cmd

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/CaptainCore/captaincore/models"
	"github.com/spf13/cobra"
)

var flagSnapshotSiteID, flagSnapshotArchive, flagSnapshotStorage string

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Snapshot commands",
}

var snapshotGenerateCmd = &cobra.Command{
	Use:   "generate <site> [--email=<email>] [--notes=<notes>] [--filter=<filter-options>] [--skip-remote]",
	Short: "Generates new snapshot for a site",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var snapshotFetchLinkCmd = &cobra.Command{
	Use:   "fetch-link <snapshot-id>",
	Short: "Fetches download link for a snapshot",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires <snapshot-id> arguments")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, snapshotFetchLinkNative)
	},
}

var snapshotAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a snapshot record",
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, snapshotAddNative)
	},
}

var snapshotListCmd = &cobra.Command{
	Use:   "list <site-id>",
	Short: "Lists snapshots for a site environment",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, snapshotListNative)
	},
}

var flagSnapshotEnvironment string

func snapshotListNative(cmd *cobra.Command, args []string) {
	siteID, err := strconv.ParseUint(args[0], 10, 64)
	if err != nil {
		fmt.Printf("Error: Invalid site_id '%s'\n", args[0])
		return
	}

	environment := flagSnapshotEnvironment
	if environment == "" {
		environment = "production"
	}

	limit, _ := strconv.Atoi(flagLimit)
	if limit <= 0 {
		limit = 10
	}

	field := flagField
	if field == "" {
		field = "snapshot_id"
	}

	// Find matching environment
	environments, err := models.FindEnvironmentsBySiteID(uint(siteID))
	if err != nil || len(environments) == 0 {
		fmt.Print("Error: No snapshots found.")
		return
	}

	var envID uint
	for _, env := range environments {
		if strings.EqualFold(env.Environment, environment) {
			envID = env.EnvironmentID
			break
		}
	}

	if envID == 0 {
		fmt.Print("Error: No snapshots found.")
		return
	}

	snapshots, err := models.GetSnapshotsByEnvironmentID(envID, limit)
	if err != nil || len(snapshots) == 0 {
		fmt.Print("Error: No snapshots found.")
		return
	}

	// Extract requested field
	var results []string
	for _, s := range snapshots {
		switch field {
		case "snapshot_id":
			results = append(results, strconv.FormatUint(uint64(s.SnapshotID), 10))
		case "snapshot_name":
			results = append(results, s.SnapshotName)
		case "storage":
			results = append(results, s.Storage)
		case "email":
			results = append(results, s.Email)
		case "notes":
			results = append(results, s.Notes)
		case "token":
			results = append(results, s.Token)
		case "created_at":
			results = append(results, s.CreatedAt)
		case "expires_at":
			results = append(results, s.ExpiresAt)
		default:
			results = append(results, strconv.FormatUint(uint64(s.SnapshotID), 10))
		}
	}

	if flagFormat == "json" {
		out, _ := json.Marshal(results)
		fmt.Print(string(out))
	} else {
		fmt.Print(strings.Join(results, " "))
	}
}

// snapshotAddNative implements `captaincore snapshot add` natively in Go.
func snapshotAddNative(cmd *cobra.Command, args []string) {
	siteID, err := strconv.ParseUint(flagSnapshotSiteID, 10, 64)
	if err != nil {
		fmt.Println("Error: Invalid site-id")
		return
	}

	environment := flagSnapshotEnvironment
	if environment == "" {
		environment = "production"
	}

	// Find environment ID
	environments, err := models.FindEnvironmentsBySiteID(uint(siteID))
	if err != nil || len(environments) == 0 {
		fmt.Println("Error: No environments found.")
		return
	}

	var envID uint
	for _, env := range environments {
		if strings.EqualFold(env.Environment, environment) {
			envID = env.EnvironmentID
			break
		}
	}

	if envID == 0 {
		fmt.Println("Error: Environment not found.")
		return
	}

	// Generate 16-byte random hex token
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		fmt.Println("Error: Failed to generate token.")
		return
	}
	token := hex.EncodeToString(tokenBytes)

	timeNow := time.Now().UTC().Format("2006-01-02 15:04:05")
	in24hrs := time.Now().UTC().Add(24 * time.Hour).Format("2006-01-02 15:04:05")

	userID := uint(0)
	if flagUserId != "" {
		if uid, err := strconv.ParseUint(flagUserId, 10, 64); err == nil {
			userID = uint(uid)
		}
	}

	snapshot := &models.Snapshot{
		UserID:        userID,
		SiteID:        uint(siteID),
		EnvironmentID: envID,
		SnapshotName:  flagSnapshotArchive,
		CreatedAt:     timeNow,
		Storage:       flagSnapshotStorage,
		Email:         flagEmail,
		Notes:         flagNotes,
		ExpiresAt:     in24hrs,
		Token:         token,
	}

	if err := models.InsertSnapshot(snapshot); err != nil {
		fmt.Printf("Error: Failed to insert snapshot: %v\n", err)
		return
	}

	// Post to API
	_, system, captain, err := loadCaptainConfig()
	if err != nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	client := newAPIClient(system, captain)
	snapshotData := map[string]interface{}{
		"snapshot_id":    snapshot.SnapshotID,
		"user_id":        snapshot.UserID,
		"site_id":        snapshot.SiteID,
		"environment_id": snapshot.EnvironmentID,
		"snapshot_name":  snapshot.SnapshotName,
		"created_at":     snapshot.CreatedAt,
		"storage":        snapshot.Storage,
		"email":          snapshot.Email,
		"notes":          snapshot.Notes,
		"expires_at":     snapshot.ExpiresAt,
		"token":          snapshot.Token,
	}

	resp, _ := client.Post("snapshot-add", map[string]interface{}{
		"site_id": siteID,
		"data":    snapshotData,
	})
	if resp != nil {
		fmt.Print(string(resp))
	}

	// Update snapshot_count in environment details
	count, err := models.CountSnapshotsByEnvironmentID(envID)
	if err == nil {
		updateEnvironmentDetails(envID, uint(siteID), map[string]interface{}{
			"snapshot_count": count,
		}, system, captain)
	}
}

// snapshotFetchLinkNative implements `captaincore snapshot fetch-link <id>` natively in Go.
func snapshotFetchLinkNative(cmd *cobra.Command, args []string) {
	snapshotID, err := strconv.ParseUint(args[0], 10, 64)
	if err != nil {
		fmt.Println("Error: Invalid snapshot ID.")
		return
	}

	snapshot, err := models.GetSnapshotByID(uint(snapshotID))
	if err != nil || snapshot == nil {
		fmt.Println("Error: Snapshot not found.")
		return
	}

	_, system, captain, err := loadCaptainConfig()
	if err != nil || captain == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	// Mint the link with rclone against the same remote `snapshot generate`
	// uploaded to. The storage credentials live in the rclone config and
	// nowhere else: the hand-rolled Backblaze call this replaced read them
	// from captain keys that the embedded-WordPress era used to define, so
	// after that era ended it authorized with an empty account and handed
	// back a URL whose Authorization was blank.
	remote := ""
	if system != nil {
		if idx := strings.Index(system.RcloneSnapshot, ":"); idx > 0 {
			remote = system.RcloneSnapshot[:idx]
		}
	}
	if remote == "" {
		fmt.Println("Error: No snapshot remote configured (system.rclone_snapshot).")
		return
	}

	b2Snapshots, _ := getB2SnapshotsPath(captain, system)
	if b2Snapshots == "" {
		fmt.Println("Error: No snapshot path configured.")
		return
	}

	rclonePath, err := exec.LookPath("rclone")
	if err != nil {
		fmt.Println("Error: rclone is not installed on this server.")
		return
	}

	target := fmt.Sprintf("%s:%s/%s", remote, strings.Trim(b2Snapshots, "/"), snapshot.SnapshotName)
	command := exec.Command(rclonePath, "link", "--expire", "24h", target)
	// Keep stderr off stdout so a warning can never end up inside the URL.
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		fmt.Printf("Error: Could not generate a download link: %s\n", lastLine(detail))
		return
	}

	link := strings.TrimSpace(stdout.String())
	// Never hand back anything that is not a link - the caller redirects to
	// whatever this prints.
	if !strings.HasPrefix(link, "https://") {
		fmt.Println("Error: Storage returned no download link for this snapshot.")
		return
	}
	fmt.Print(link)
}

// lastLine keeps an error message to its final, most specific line.
func lastLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func init() {
	rootCmd.AddCommand(snapshotCmd)
	snapshotCmd.AddCommand(snapshotGenerateCmd)
	snapshotCmd.AddCommand(snapshotFetchLinkCmd)
	snapshotCmd.AddCommand(snapshotListCmd)
	snapshotCmd.AddCommand(snapshotAddCmd)
	snapshotGenerateCmd.Flags().BoolVarP(&flagSkipRemote, "skip-remote", "", false, "Skip sending snapshot to remote storage provider")
	snapshotGenerateCmd.Flags().StringVarP(&flagEmail, "email", "e", "", "Notify email address")
	snapshotGenerateCmd.Flags().StringVarP(&flagNotes, "notes", "n", "", "Adds a note about the snapshot")
	snapshotGenerateCmd.Flags().StringVarP(&flagUserId, "user-id", "u", "", "User ID")
	snapshotGenerateCmd.Flags().StringVarP(&flagFilter, "filter", "f", "", "Filter options include one or more of the following: database, themes, plugins, uploads, everything-else. Example --filter=database,themes,plugins will generate a zip with only the database, themes and plugins. Without filter a snapshot will include everything")
	snapshotListCmd.Flags().StringVarP(&flagSnapshotEnvironment, "environment", "e", "production", "Environment (production or staging)")
	snapshotListCmd.Flags().StringVarP(&flagLimit, "limit", "l", "", "Limit number of results (default: 10)")
	snapshotListCmd.Flags().StringVarP(&flagField, "field", "", "", "Field to return (default: snapshot_id)")
	snapshotListCmd.Flags().StringVarP(&flagFormat, "format", "", "", "Output format (json)")
	snapshotAddCmd.Flags().StringVarP(&flagSnapshotSiteID, "site-id", "", "", "Site ID")
	snapshotAddCmd.Flags().StringVarP(&flagSnapshotEnvironment, "environment", "e", "production", "Environment (production or staging)")
	snapshotAddCmd.Flags().StringVarP(&flagUserId, "user-id", "u", "", "User ID")
	snapshotAddCmd.Flags().StringVarP(&flagSnapshotStorage, "storage", "", "", "Storage size in bytes")
	snapshotAddCmd.Flags().StringVarP(&flagSnapshotArchive, "archive", "", "", "Archive filename")
	snapshotAddCmd.Flags().StringVarP(&flagEmail, "email", "", "", "Notify email address")
	snapshotAddCmd.Flags().StringVarP(&flagNotes, "notes", "n", "", "Snapshot notes")
}

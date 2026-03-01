package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/CaptainCore/captaincore/models"
	"github.com/spf13/cobra"
)

var regenerateThumbnailsCmd = &cobra.Command{
	Use:   "regenerate-thumbnails <site>",
	Short: "Generate thumbnails from most recent capture",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, regenerateThumbnailsNative)
	},
}

// regenerateThumbnailsNative implements `captaincore regenerate-thumbnails <site>` natively in Go.
func regenerateThumbnailsNative(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])
	site, err := sa.LookupSite()
	if err != nil || site == nil {
		fmt.Printf("Error: Site '%s' not found.\n", sa.SiteName)
		return
	}

	environments, err := models.FindEnvironmentsBySiteID(site.SiteID)
	if err != nil {
		fmt.Printf("Error fetching environments: %v\n", err)
		return
	}

	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	rcloneUpload := ""
	if captain != nil {
		rcloneUpload = captain.Remotes["rclone_upload"]
	}

	for _, env := range environments {
		envName := strings.ToLower(env.Environment)
		details := env.ParseDetails()

		if details.ScreenshotBase == "" {
			fmt.Printf("No captures found for %s-%s\n", site.Site, envName)
			continue
		}

		fmt.Printf("Fetching latest capture screenshot for %s-%s\n", site.Site, envName)

		siteDir := fmt.Sprintf("%s_%d", site.Site, site.SiteID)
		sourceURL := fmt.Sprintf("%s/%s/%s/%s/captures/%%23_%s.jpg", system.RcloneUploadURI, captainID, siteDir, envName, details.ScreenshotBase)
		rcloneDest := fmt.Sprintf("%s%s/%s/%s/screenshots/", rcloneUpload, captainID, siteDir, envName)
		downloadFile := fmt.Sprintf("%s_%s.jpg", siteDir, details.ScreenshotBase)
		thumb800 := fmt.Sprintf("%s_thumb-800.jpg", details.ScreenshotBase)
		thumb100 := fmt.Sprintf("%s_thumb-100.jpg", details.ScreenshotBase)

		script := fmt.Sprintf(`wget -O %s %s
convert %s -thumbnail 800 -gravity North -crop 800x500+0+0 %s
convert %s -thumbnail 100 %s
rclone move %s %s --fast-list --transfers=32 --no-update-modtime --progress
rclone move %s %s --fast-list --transfers=32 --no-update-modtime --progress
rm %s`,
			downloadFile, sourceURL,
			downloadFile, thumb800,
			thumb800, thumb100,
			thumb100, rcloneDest,
			thumb800, rcloneDest,
			downloadFile,
		)

		shellCmd := exec.Command("bash", "-c", script)
		shellCmd.Dir = system.PathTmp
		shellCmd.Stdout = os.Stdout
		shellCmd.Stderr = os.Stderr
		shellCmd.Run()
	}
}

func init() {
	rootCmd.AddCommand(regenerateThumbnailsCmd)
}

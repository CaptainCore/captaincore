package cmd

import (
	_ "embed"
	"fmt"
	"os"
	"strings"

	"github.com/CaptainCore/captaincore/apiclient"
	"github.com/CaptainCore/captaincore/config"
	"github.com/spf13/cobra"
)

// Site images and rebuilds. A server holds one image per PHP branch
// (cc-wp-php83, cc-wp-php84, cc-wp-php85), built from host_image.sh. A site's
// PHP version is the image its container was made from: switching version,
// or picking up a refreshed build, replaces the container and keeps its
// volumes (the node tool's `rebuild`, see host_backups.py).

//go:embed host_image.sh
var hostImageScript string

var flagImagePHP string
var flagRebuildCheck bool

func nodeImageBuild(args []string) {
	if flagImagePHP != "8.3" && flagImagePHP != "8.4" && flagImagePHP != "8.5" {
		fail("--php must be 8.3, 8.4 or 8.5")
	}
	t := nodeTarget(args[0])
	fmt.Printf("Building the PHP %s site image on %s (a few minutes)…\n", flagImagePHP, t.server)
	res, err := ccToolIn(t, hostImageScript, "image-build", flagImagePHP)
	if err != nil {
		fail("image build: %v", err)
	}
	fmt.Printf("%v is ready: %v (build %v)\n", res["image"], res["description"], res["fingerprint"])
	if old, _ := res["replaced"].(string); old != "" {
		fmt.Printf("It replaces build %s. Sites on the old build keep running; `captaincore host rebuild <site>` moves one onto the new build.\n", old)
	}
}

func nodeImageList(args []string) {
	t := nodeTarget(args[0])
	res, err := ccTool(t, "image-list")
	if err != nil {
		fail("%v", err)
	}
	list, _ := res["images"].([]interface{})
	if len(list) == 0 {
		fmt.Printf("No site images on %s yet. Build one with: captaincore node image build %s --php=8.4\n", t.server, args[0])
		return
	}
	for _, i := range list {
		m := i.(map[string]interface{})
		cts, _ := m["containers"].([]interface{})
		names := []string{}
		for _, c := range cts {
			names = append(names, fmt.Sprint(c))
		}
		fmt.Printf("%-12v PHP %-4v build %v  %5.0f MB  %v\n", m["image"], m["php"], m["fingerprint"], toFloat(m["size_bytes"])/1e6, m["description"])
		fmt.Printf("             %d container(s) on this build: %s\n", len(names), strings.Join(names, ", "))
	}
}

// hostApplyImage runs before the apply script: when the container is not on
// the image the hosting configuration asks for, it is rebuilt on it. A
// failed rebuild leaves the previous container running, is recorded on the
// revision and ends the apply.
func hostApplyImage(system *config.SystemConfig, captain *config.CaptainConfig, client *apiclient.Client, arg, container, image string, revision int, siteID uint, environment string) map[string]interface{} {
	t, err := hostTarget(system, captain, arg)
	if err != nil {
		return nil // the apply itself reports what is wrong with the target
	}
	if err := t.ensureBackupTool(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not check the container's image: %v\n", err)
		return nil
	}
	cur, err := ccTool(t, "image-of", container)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not check the container's image: %v\n", err)
		return nil
	}
	have, _ := cur["image"].(string)
	if have == image {
		return nil
	}
	if have == "" {
		fmt.Fprintf(os.Stderr, "Warning: %s was not made from a known site image, so its PHP version is left as it is.\n", container)
		return nil
	}
	if flagHostDryRun {
		fmt.Printf("Would rebuild %s from %s on %s (about 10 seconds, during which requests wait at the edge).\n", container, have, image)
		return nil
	}
	fmt.Printf("Rebuilding %s from %s on %s: a local backup first, then about 10 seconds while requests wait at the edge…\n", container, have, image)
	res, err := ccTool(t, "rebuild", container, image)
	if err != nil {
		failed := map[string]interface{}{"status": "failed", "error": fmt.Sprintf("PHP switch to %s: %v", image, err)}
		_, _ = client.Post("host-config-applied", map[string]interface{}{
			"site_id": siteID, "environment": environment,
			"data": map[string]interface{}{"revision": revision, "status": "failed", "result": failed},
		})
		fail("%s revision %d on %s: FAILED: %v", arg, revision, t.server, failed["error"])
	}
	fmt.Printf("  now runs %v, offline %v s, backup %v taken first. The old container is kept as %v for 7 days.\n", res["runs"], res["offline_seconds"], res["safety_backup"], res["old_root"])
	return res
}

// host rebuild: the same site on the current build of its image (after
// `node image build` refreshed it), then its settings applied again.
func hostRebuild(args []string) {
	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fail("Configuration file not found.")
	}
	t, err := hostTarget(system, captain, args[0])
	if err != nil {
		fail("%v", err)
	}
	if err := t.ensureBackupTool(); err != nil {
		fail("%v", err)
	}
	cur, err := ccTool(t, "image-of", t.container)
	if err != nil {
		fail("%v", err)
	}
	image, _ := cur["image"].(string)
	if image == "" {
		fail("%s was not made from a known site image", t.container)
	}
	outdated, _ := cur["outdated"].(bool)
	if flagRebuildCheck {
		state := "the current build"
		if outdated {
			state = fmt.Sprintf("an older build (%v, current is %v)", cur["fingerprint"], cur["latest"])
		}
		fmt.Printf("%s runs %s, %s.\n", args[0], image, state)
		if outdated {
			os.Exit(1)
		}
		return
	}
	if !outdated {
		fmt.Printf("%s already runs the current build of %s. Rebuilding anyway.\n", args[0], image)
	}
	fmt.Printf("Rebuilding %s on %s: a local backup first, then about 10 seconds while requests wait at the edge…\n", t.container, image)
	res, err := ccTool(t, "rebuild", t.container, image)
	if err != nil {
		fail("%v", err)
	}
	fmt.Printf("  now runs %v, offline %v s, backup %v taken first. The old container is kept as %v for 7 days.\n", res["runs"], res["offline_seconds"], res["safety_backup"], res["old_root"])
	if err := selfRun("host", "apply", args[0]); err != nil {
		fail("host apply: %v", err)
	}
}

func registerNodeImage(nodeCmd *cobra.Command) {
	imageCmd := &cobra.Command{Use: "image", Short: "Site images on a server (one per PHP version)"}
	build := &cobra.Command{
		Use: "build <server-id>", Short: "Build or refresh the site image for a PHP version", Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			resolveNativeOrWP(cmd, args, func(_ *cobra.Command, a []string) { nodeImageBuild(a) })
		},
	}
	build.Flags().StringVar(&flagImagePHP, "php", "8.4", "8.3, 8.4 or 8.5")
	imageCmd.AddCommand(build)
	imageCmd.AddCommand(&cobra.Command{
		Use: "list <server-id>", Short: "The site images on a server and the containers on each", Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			resolveNativeOrWP(cmd, args, func(_ *cobra.Command, a []string) { nodeImageList(a) })
		},
	})
	nodeCmd.AddCommand(imageCmd)

	rebuild := &cobra.Command{
		Use: "rebuild <site>[-env]", Short: "Move a site onto the current build of its image (keeps files, database, address)", Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			resolveNativeOrWP(cmd, args, func(_ *cobra.Command, a []string) { hostRebuild(a) })
		},
	}
	rebuild.Flags().BoolVar(&flagRebuildCheck, "check", false, "Only say whether the site runs the current build (exit 1 when it does not)")
	hostCmd.AddCommand(rebuild)
}

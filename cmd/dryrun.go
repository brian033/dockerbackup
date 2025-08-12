package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/brian033/dockerbackup/internal/logger"
	"github.com/brian033/dockerbackup/pkg/archive"
)

type DryRunRestoreCmd struct {
	log logger.Logger
}

func (c *DryRunRestoreCmd) Name() string { return "dry-run-restore" }

func (c *DryRunRestoreCmd) Help() string {
	return `
Show what would be restored from a backup without making changes.

Usage:
  dockerbackup dry-run-restore <backup_file>
`
}

func (c *DryRunRestoreCmd) Validate(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing backup file path")
	}
	return nil
}

func (c *DryRunRestoreCmd) Execute(ctx context.Context, args []string) error {
	backupFile := args[0]
	h := archive.NewTarArchiveHandler()
	entries, err := h.ListArchive(ctx, backupFile)
	if err != nil {
		return err
	}
	fmt.Println("Plan:")
	fmt.Println("- Extract backup to temp dir")
	fmt.Println("- Load image.tar if present; else import filesystem.tar")
	fmt.Println("- Ensure networks and volumes exist; restore data for volumes and bind mounts")
	fmt.Println("- Recreate container with mounts, ports, env, and networking")

	has := map[string]bool{}
	for _, e := range entries {
		has[e.Path] = true
	}
	if has["volumes/volume_configs.json"] {
		fmt.Println("  * volume configs found: volumes/volume_configs.json")
	}
	if has["networks/network_configs.json"] {
		fmt.Println("  * network configs found: networks/network_configs.json")
	}
	if has["image.tar"] {
		fmt.Println("  * image tar found: image.tar")
	}
	if has["container.json"] {
		fmt.Println("  * container inspect found: container.json")
	}
	for _, e := range entries {
		if len(e.Path) > 8 && e.Path[:8] == "volumes/" && filepath.Ext(e.Path) == ".gz" {
			fmt.Printf("  * volume archive: %s\n", e.Path)
		}
	}

	// Show brief diff-like info by extracting minimal JSON fields if present in list
	// Note: full diff requires extraction; here we hint based on presence
	if has["container.json"] {
		fmt.Println("Diff hints:")
		fmt.Println("  - ports: will recreate HostConfig.PortBindings and Config.ExposedPorts")
		fmt.Println("  - networks: will attach per NetworkSettings.Networks; name mapping may apply")
		fmt.Println("  - volumes: named volumes restored; bind mounts validated, may relocate if bind-restore-root set")
		fmt.Println("  - env: will set from Config.Env; labels from Config.Labels")
	}
	return nil
}

func init() {
	RegisterCommand(&DryRunRestoreCmd{log: logger.New()})
}

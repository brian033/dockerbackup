package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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
	if err != nil { return err }
	fmt.Println("Plan:")
	fmt.Println("- Extract backup to temp dir (dry-run)")
	fmt.Println("- Load image.tar if present; else import filesystem.tar")
	fmt.Println("- Ensure networks and volumes exist; restore data for volumes and bind mounts")
	fmt.Println("- Recreate container with mounts, ports, env, and networking")

	has := map[string]bool{}
	for _, e := range entries { has[e.Path] = true }
	for _, e := range entries { if len(e.Path) > 8 && e.Path[:8] == "volumes/" && filepath.Ext(e.Path) == ".gz" { fmt.Printf("  * volume archive: %s\n", e.Path) } }

	// Extract to temp dir for richer diff
	tmp, err := os.MkdirTemp("", "dockerbackup_dryrun_*")
	if err != nil { return err }
	defer func(){ _ = os.RemoveAll(tmp) }()
	if err := h.ExtractArchive(ctx, backupFile, tmp); err != nil { return err }
	// Read container.json if present
	cjPath := filepath.Join(tmp, "container.json")
	if b, err := os.ReadFile(cjPath); err == nil {
		// Minimal decode of fields we care about
		var raw map[string]any
		_ = json.Unmarshal(b, &raw)
		fmt.Println("Diff details:")
		// Env
		if cfg, ok := raw["Config"].(map[string]any); ok {
			if env, ok := cfg["Env"].([]any); ok {
				fmt.Printf("  - env: %d variables\n", len(env))
			}
		}
		// Ports
		if ns, ok := raw["NetworkSettings"].(map[string]any); ok {
			if ports, ok := ns["Ports"].(map[string]any); ok {
				fmt.Printf("  - port bindings: %d entries\n", len(ports))
			}
		}
		// Mounts
		if mounts, ok := raw["Mounts"].([]any); ok {
			fmt.Printf("  - mounts: %d entries\n", len(mounts))
		}
		// Networks
		if ns, ok := raw["NetworkSettings"].(map[string]any); ok {
			if nets, ok := ns["Networks"].(map[string]any); ok {
				fmt.Printf("  - networks: %d attached\n", len(nets))
			}
		}
	}
	// Mapping preview not available without flags; show hint
	fmt.Println("Mapping preview: (apply with --network-map/--parent-map/--drop-host-ips/--reassign-ips)")
	return nil
}

func init() {
	RegisterCommand(&DryRunRestoreCmd{log: logger.New()})
}

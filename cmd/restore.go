package cmd

import (
	"context"
	"flag"
	"fmt"

	"github.com/brian033/dockerbackup/internal/logger"
	"github.com/brian033/dockerbackup/pkg/backup"
)

type RestoreCmd struct {
	log    logger.Logger
	engine backup.BackupEngine
}

func (c *RestoreCmd) Name() string { return "restore" }

func (c *RestoreCmd) Help() string {
	return `
Restore a container from a backup file.

Usage:
  dockerbackup restore <backup_file> [options]

Options:
  -n, --name string   New container name (default: original)
  --start             Start container after restore
`
}

func (c *RestoreCmd) Validate(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing backup file path")
	}
	return nil
}

func (c *RestoreCmd) Execute(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet(c.Name(), flag.ContinueOnError)
	var name string
	var start bool
	fs.StringVar(&name, "name", "", "New container name")
	fs.StringVar(&name, "n", "", "New container name (shorthand)")
	fs.BoolVar(&start, "start", false, "Start container after restore")
	if err := fs.Parse(args); err != nil {
		return err
	}
	remaining := fs.Args()
	if len(remaining) == 0 {
		return fmt.Errorf("missing backup file path")
	}
	backupFile := remaining[0]

	req := backup.RestoreRequest{
		BackupPath: backupFile,
		Options: backup.RestoreOptions{
			ContainerName: name,
			Start:         start,
		},
		TargetType: backup.TargetContainer,
	}
	if c.engine == nil {
		c.engine = newDefaultEngine(c.log)
	}
	_, err := c.engine.Restore(ctx, req)
	return err
}

func init() {
	cmd := &RestoreCmd{
		log:    logger.New(),
		engine: nil,
	}
	RegisterCommand(cmd)
}

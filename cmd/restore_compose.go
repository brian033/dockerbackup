package cmd

import (
	"context"
	"fmt"

	"github.com/brian033/dockerbackup/internal/logger"
	"github.com/brian033/dockerbackup/pkg/backup"
	"github.com/spf13/pflag"
)

type RestoreComposeCmd struct {
	log    logger.Logger
	engine backup.BackupEngine
}

func (c *RestoreComposeCmd) Name() string { return "restore-compose" }

func (c *RestoreComposeCmd) Help() string {
	return `
Restore a Docker Compose project from a backup file.

Usage:
  dockerbackup restore-compose <backup_file> [options]

Options:
  -p, --project-name string  New project name (default: original)
  --start                    Start services after restore
`
}

func (c *RestoreComposeCmd) Validate(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing backup file path")
	}
	return nil
}

func (c *RestoreComposeCmd) Execute(ctx context.Context, args []string) error {
	fs := pflag.NewFlagSet(c.Name(), pflag.ContinueOnError)
	var projectName string
	var start bool
	fs.StringVarP(&projectName, "project-name", "p", "", "New project name")
	fs.BoolVar(&start, "start", false, "Start services after restore")
	if err := fs.Parse(args); err != nil {
		return err
	}
	remaining := fs.Args()
	if len(remaining) == 0 {
		return fmt.Errorf("missing backup file path")
	}
	backupFile := remaining[0]

	req := backup.RestoreRequest{
		BackupPath:  backupFile,
		ProjectName: projectName,
		Options: backup.RestoreOptions{
			Start: start,
		},
		TargetType: backup.TargetCompose,
	}
	if c.engine == nil {
		c.engine = newDefaultEngine(c.log)
	}
	_, err := c.engine.Restore(ctx, req)
	return err
}

func init() {
	RegisterCommand(&RestoreComposeCmd{
		log:    logger.New(),
		engine: nil,
	})
}

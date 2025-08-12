package cmd

import (
	"context"
	"flag"

	"github.com/brian033/dockerbackup/internal/logger"
	"github.com/brian033/dockerbackup/pkg/backup"
)

type BackupComposeCmd struct {
	log    logger.Logger
	engine backup.BackupEngine
}

func (c *BackupComposeCmd) Name() string { return "backup-compose" }

func (c *BackupComposeCmd) Help() string {
	return `
Backup a Docker Compose project.

Usage:
  dockerbackup backup-compose [project_path] [options]

Options:
  -o, --output string        Output file path (default: <project>_compose_backup.tar.gz)
  -p, --project-name string  Override project name
`
}

func (c *BackupComposeCmd) Validate(args []string) error { return nil }

func (c *BackupComposeCmd) Execute(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet(c.Name(), flag.ContinueOnError)
	var output string
	var projectName string
	fs.StringVar(&output, "output", "", "Output file path")
	fs.StringVar(&output, "o", "", "Output file path (shorthand)")
	fs.StringVar(&projectName, "project-name", "", "Project name")
	fs.StringVar(&projectName, "p", "", "Project name (shorthand)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	remaining := fs.Args()
	projectPath := "."
	if len(remaining) > 0 {
		projectPath = remaining[0]
	}

	builder := backup.NewBackupOptionsBuilder().
		WithOutput(output)

	req := backup.BackupRequest{
		TargetType:         backup.TargetCompose,
		ComposeProjectPath: projectPath,
		Options:            builder.Build(),
	}
	if c.engine == nil {
		c.engine = newDefaultEngine(c.log)
	}
	_, err := c.engine.Backup(ctx, req)
	return err
}

func init() {
	RegisterCommand(&BackupComposeCmd{
		log:    logger.New(),
		engine: nil,
	})
}

package cmd

import (
	"context"
	"flag"
	"fmt"

	"github.com/brian033/dockerbackup/internal/logger"
	"github.com/brian033/dockerbackup/pkg/backup"
)

type BackupCmd struct {
	log    logger.Logger
	engine backup.BackupEngine
}

func (c *BackupCmd) Name() string { return "backup" }

func (c *BackupCmd) Help() string {
	return `
Backup a single container.

Usage:
  dockerbackup backup <container_id_or_name> [options]

Options:
  -o, --output string     Output file path (default: <container>_backup.tar.gz)
  -c, --compress int      Compression level (1-9, default: 6)
`
}

func (c *BackupCmd) Validate(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing container id or name")
	}
	return nil
}

func (c *BackupCmd) Execute(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet(c.Name(), flag.ContinueOnError)
	var output string
	var compress int
	fs.StringVar(&output, "output", "", "Output file path")
	fs.StringVar(&output, "o", "", "Output file path (shorthand)")
	fs.IntVar(&compress, "compress", 6, "Compression level (1-9)")
	fs.IntVar(&compress, "c", 6, "Compression level (shorthand)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	remaining := fs.Args()
	if len(remaining) == 0 {
		return fmt.Errorf("missing container id or name")
	}
	containerID := remaining[0]

	builder := backup.NewBackupOptionsBuilder().
		WithOutput(output).
		WithCompression(compress)

	req := backup.BackupRequest{
		TargetType:  backup.TargetContainer,
		ContainerID: containerID,
		Options:     builder.Build(),
	}
	if c.engine == nil {
		c.engine = newDefaultEngine(c.log)
	}
	_, err := c.engine.Backup(ctx, req)
	return err
}

func init() {
	cmd := &BackupCmd{
		log:    logger.New(),
		engine: nil,
	}
	RegisterCommand(cmd)
}

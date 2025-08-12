package cmd

import (
	"context"
	"flag"
	"fmt"

	"github.com/your-username/dockerbackup/internal/logger"
	"github.com/your-username/dockerbackup/pkg/archive"
)

type ListCmd struct {
	log logger.Logger
}

func (c *ListCmd) Name() string { return "list" }

func (c *ListCmd) Help() string {
	return `
List the contents of a backup archive.

Usage:
  dockerbackup list <backup_file>
`
}

func (c *ListCmd) Validate(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing backup file path")
	}
	return nil
}

func (c *ListCmd) Execute(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet(c.Name(), flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	remaining := fs.Args()
	if len(remaining) == 0 {
		return fmt.Errorf("missing backup file path")
	}
	backupFile := remaining[0]

	h := archive.NewTarArchiveHandler()
	entries, err := h.ListArchive(ctx, backupFile)
	if err != nil {
		return err
	}
	for _, e := range entries {
		fmt.Printf("%s\n", e.Path)
	}
	return nil
}

func init() {
	RegisterCommand(&ListCmd{log: logger.New()})
}

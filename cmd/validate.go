package cmd

import (
	"context"
	"fmt"

	"github.com/brian033/dockerbackup/internal/logger"
)

type ValidateCmd struct {
	log logger.Logger
}

func (c *ValidateCmd) Name() string { return "validate" }

func (c *ValidateCmd) Help() string {
	return `
Validate a backup archive.

Usage:
  dockerbackup validate <backup_file>
`
}

func (c *ValidateCmd) Validate(args []string) error {
	if len(args) == 0 { return fmt.Errorf("missing backup file path") }
	return nil
}

func (c *ValidateCmd) Execute(ctx context.Context, args []string) error {
	backupFile := args[0]
	eng := newDefaultEngine(c.log)
	res, err := eng.Validate(ctx, backupFile)
	if err != nil { return err }
	if res == nil { return fmt.Errorf("no validation result") }
	if res.Valid {
		fmt.Println("VALID:", res.Details)
	} else {
		fmt.Println("INVALID:", res.Details)
	}
	return nil
}

func init() {
	RegisterCommand(&ValidateCmd{log: logger.New()})
}

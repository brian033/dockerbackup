package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/your-username/dockerbackup/internal/logger"
	"github.com/your-username/dockerbackup/pkg/archive"
	"github.com/your-username/dockerbackup/pkg/backup"
	"github.com/your-username/dockerbackup/pkg/docker"
	"github.com/your-username/dockerbackup/pkg/filesystem"
)

type Command interface {
	Execute(ctx context.Context, args []string) error
	Validate(args []string) error
	Help() string
	Name() string
}

var registered = map[string]Command{}

func RegisterCommand(cmd Command) {
	registered[cmd.Name()] = cmd
}

func newDefaultEngine(log logger.Logger) backup.BackupEngine {
	arch := archive.NewTarArchiveHandler()
	dc := docker.NewCLIClient()
	fs := filesystem.NewHandler()
	return backup.NewDefaultBackupEngine(arch, dc, fs, log)
}

func Execute() {
	log := logger.New()
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	sub := os.Args[1]
	cmd, ok := registered[sub]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", sub)
		printUsage()
		os.Exit(1)
	}

	if err := cmd.Validate(os.Args[2:]); err != nil {
		fmt.Fprintf(os.Stderr, "invalid arguments for %s: %v\n\n", sub, err)
		fmt.Fprintln(os.Stderr, strings.TrimSpace(cmd.Help()))
		os.Exit(2)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	start := time.Now()
	if err := cmd.Execute(ctx, os.Args[2:]); err != nil {
		log.Errorf("%s failed: %v", cmd.Name(), err)
		os.Exit(1)
	}
	log.Infof("%s completed in %s", cmd.Name(), time.Since(start).Truncate(time.Millisecond))
}

func printUsage() {
	b := &strings.Builder{}
	fmt.Fprintln(b, "Usage: dockerbackup <command> [options]")
	fmt.Fprintln(b, "")
	fmt.Fprintln(b, "Commands:")
	for name, cmd := range registered {
		fmt.Fprintf(b, "  %-16s %s\n", name, shortHelp(cmd.Help()))
	}
	fmt.Fprintln(b, "")
	fmt.Fprintln(b, "Run 'dockerbackup <command> --help' for command-specific help.")
	fmt.Print(b.String())
}

func shortHelp(help string) string {
	lines := strings.Split(strings.TrimSpace(help), "\n")
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(lines[0])
}

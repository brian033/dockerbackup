package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/brian033/dockerbackup/internal/logger"
	"github.com/brian033/dockerbackup/pkg/archive"
	"github.com/brian033/dockerbackup/pkg/backup"
	"github.com/brian033/dockerbackup/pkg/docker"
	"github.com/brian033/dockerbackup/pkg/filesystem"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
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
	// Prefer SDK client when available
	var dc docker.DockerClient
	if sdk, err := docker.NewSDKClient(); err == nil {
		// Wrap SDK to satisfy DockerClient via CreateContainerFromSpec while reusing CLI for other methods
		dc = &compositeClient{sdk: sdk, cli: docker.NewCLIClient()}
	} else {
		dc = docker.NewCLIClient()
	}
	fs := filesystem.NewHandler()
	return backup.NewDefaultBackupEngine(arch, dc, fs, log)
}

type compositeClient struct {
	sdk *docker.SDKClient
	cli docker.DockerClient
}

func (c *compositeClient) InspectContainer(ctx context.Context, containerID string) ([]byte, error) {
	return c.cli.InspectContainer(ctx, containerID)
}
func (c *compositeClient) ExportContainerFilesystem(ctx context.Context, containerID string, destTarPath string) error {
	return c.cli.ExportContainerFilesystem(ctx, containerID, destTarPath)
}
func (c *compositeClient) ListVolumes(ctx context.Context) ([]string, error) {
	return c.cli.ListVolumes(ctx)
}
func (c *compositeClient) InspectVolume(ctx context.Context, name string) (*docker.VolumeConfig, error) {
	return c.cli.InspectVolume(ctx, name)
}
func (c *compositeClient) InspectNetwork(ctx context.Context, name string) (*docker.NetworkConfig, error) {
	return c.cli.InspectNetwork(ctx, name)
}
func (c *compositeClient) ImportImage(ctx context.Context, tarPath string, ref string) (string, error) {
	return c.cli.ImportImage(ctx, tarPath, ref)
}
func (c *compositeClient) VolumeCreate(ctx context.Context, name string) error {
	return c.cli.VolumeCreate(ctx, name)
}
func (c *compositeClient) ExtractTarGzToVolume(ctx context.Context, volumeName string, tarGzPath string, expectedRoot string) error {
	return c.cli.ExtractTarGzToVolume(ctx, volumeName, tarGzPath, expectedRoot)
}
func (c *compositeClient) CreateContainer(ctx context.Context, imageRef string, name string, mounts []docker.Mount) (string, error) {
	return c.cli.CreateContainer(ctx, imageRef, name, mounts)
}
func (c *compositeClient) CreateContainerFromSpec(ctx context.Context, cfg *container.Config, hostCfg *container.HostConfig, netCfg *network.NetworkingConfig, name string) (string, error) {
	return c.sdk.CreateContainerFromSpec(ctx, cfg, hostCfg, netCfg, name)
}
func (c *compositeClient) StartContainer(ctx context.Context, containerID string) error {
	return c.cli.StartContainer(ctx, containerID)
}
func (c *compositeClient) EnsureVolume(ctx context.Context, cfg docker.VolumeConfig) error { return c.sdk.EnsureVolume(ctx, cfg) }
func (c *compositeClient) EnsureNetwork(ctx context.Context, cfg docker.NetworkConfig) error { return c.sdk.EnsureNetwork(ctx, cfg) }

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

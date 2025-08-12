package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var ErrEmptyInspect = errors.New("docker inspect returned empty result")

type DockerClient interface {
	InspectContainer(ctx context.Context, containerID string) ([]byte, error)
	ExportContainerFilesystem(ctx context.Context, containerID string, destTarPath string) error
	ListVolumes(ctx context.Context) ([]string, error)
}

type CLIClient struct{}

func NewCLIClient() DockerClient {
	return &CLIClient{}
}

func (c *CLIClient) InspectContainer(ctx context.Context, containerID string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", "inspect", containerID)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker inspect %s failed: %v: %s", containerID, err, stderr.String())
	}
	if stdout.Len() == 0 {
		return nil, ErrEmptyInspect
	}
	return stdout.Bytes(), nil
}

func (c *CLIClient) ExportContainerFilesystem(ctx context.Context, containerID string, destTarPath string) error {
	if err := os.MkdirAll(filepathDir(destTarPath), 0o755); err != nil {
		return err
	}
	f, err := os.Create(destTarPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	cmd := exec.CommandContext(ctx, "docker", "export", containerID)
	cmd.Stdout = f
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker export %s failed: %v: %s", containerID, err, stderr.String())
	}
	return nil
}

func (c *CLIClient) ListVolumes(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "docker", "volume", "ls", "--format", "{{.Name}}")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker volume ls failed: %v: %s", err, stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	var vols []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			vols = append(vols, line)
		}
	}
	return vols, nil
}

func filepathDir(p string) string {
	// Avoid importing path/filepath for a single call in this file
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[:i]
		}
	}
	return "."
}

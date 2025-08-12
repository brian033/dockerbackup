package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	internalerrors "github.com/brian033/dockerbackup/internal/errors"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
)

var ErrEmptyInspect = errors.New("docker inspect returned empty result")

type DockerClient interface {
	InspectContainer(ctx context.Context, containerID string) ([]byte, error)
	ExportContainerFilesystem(ctx context.Context, containerID string, destTarPath string) error
	ListVolumes(ctx context.Context) ([]string, error)

	// Config inspections
	InspectVolume(ctx context.Context, name string) (*VolumeConfig, error)
	InspectNetwork(ctx context.Context, name string) (*NetworkConfig, error)

	// Ensure resources exist with original options (SDK preferred)
	EnsureVolume(ctx context.Context, cfg VolumeConfig) error
	EnsureNetwork(ctx context.Context, cfg NetworkConfig) error

	// Restore-related
	ImportImage(ctx context.Context, tarPath string, ref string) (string, error)
	VolumeCreate(ctx context.Context, name string) error
	ExtractTarGzToVolume(ctx context.Context, volumeName string, tarGzPath string, expectedRoot string) error
	CreateContainer(ctx context.Context, imageRef string, name string, mounts []Mount) (string, error)
	CreateContainerFromSpec(ctx context.Context, cfg *container.Config, hostCfg *container.HostConfig, netCfg *network.NetworkingConfig, name string) (string, error)
	StartContainer(ctx context.Context, containerID string) error
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
	if err := os.MkdirAll(filepath.Dir(destTarPath), 0o755); err != nil {
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

func (c *CLIClient) InspectVolume(ctx context.Context, name string) (*VolumeConfig, error) {
	cmd := exec.CommandContext(ctx, "docker", "volume", "inspect", name)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker volume inspect %s failed: %v: %s", name, err, stderr.String())
	}
	var arr []struct {
		Name    string            `json:"Name"`
		Driver  string            `json:"Driver"`
		Options map[string]string `json:"Options"`
		Labels  map[string]string `json:"Labels"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &arr); err != nil || len(arr) == 0 {
		return nil, fmt.Errorf("parse volume inspect for %s failed: %v", name, err)
	}
	v := &VolumeConfig{Name: arr[0].Name, Driver: arr[0].Driver, Options: arr[0].Options, Labels: arr[0].Labels}
	return v, nil
}

func (c *CLIClient) InspectNetwork(ctx context.Context, name string) (*NetworkConfig, error) {
	cmd := exec.CommandContext(ctx, "docker", "network", "inspect", name)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker network inspect %s failed: %v: %s", name, err, stderr.String())
	}
	var arr []struct {
		Name       string            `json:"Name"`
		Driver     string            `json:"Driver"`
		Options    map[string]string `json:"Options"`
		Internal   bool              `json:"Internal"`
		Attachable bool              `json:"Attachable"`
		Ingress    bool              `json:"Ingress"`
		IPAM       struct {
			Driver string `json:"Driver"`
			Config []struct {
				Subnet  string `json:"Subnet"`
				Gateway string `json:"Gateway"`
				IPRange string `json:"IPRange"`
			} `json:"Config"`
		} `json:"IPAM"`
		Labels map[string]string `json:"Labels"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &arr); err != nil || len(arr) == 0 {
		return nil, fmt.Errorf("parse network inspect for %s failed: %v", name, err)
	}
	nc := &NetworkConfig{
		Name:       arr[0].Name,
		Driver:     arr[0].Driver,
		Options:    arr[0].Options,
		Internal:   arr[0].Internal,
		Attachable: arr[0].Attachable,
		Ingress:    arr[0].Ingress,
		Labels:     arr[0].Labels,
		IPAM:       IPAM{Driver: arr[0].IPAM.Driver},
	}
	for _, c := range arr[0].IPAM.Config {
		nc.IPAM.Config = append(nc.IPAM.Config, IPAMConfig{Subnet: c.Subnet, Gateway: c.Gateway, IPRange: c.IPRange})
	}
	return nc, nil
}

func (c *CLIClient) ImportImage(ctx context.Context, tarPath string, ref string) (string, error) {
	args := []string{"import"}
	if tarPath != "" {
		args = append(args, tarPath)
	}
	if ref != "" {
		args = append(args, ref)
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker import failed: %v: %s", err, stderr.String())
	}
	imageID := strings.TrimSpace(stdout.String())
	return imageID, nil
}

func (c *CLIClient) VolumeCreate(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "docker", "volume", "create", name)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker volume create %s failed: %v: %s", name, err, stderr.String())
	}
	return nil
}

func (c *CLIClient) ExtractTarGzToVolume(ctx context.Context, volumeName string, tarGzPath string, expectedRoot string) error {
	// Mount the tar as read-only and the volume at /restore; then extract and copy contents
	cmd := exec.CommandContext(
		ctx,
		"docker", "run", "--rm",
		"-v", fmt.Sprintf("%s:/restore", volumeName),
		"-v", fmt.Sprintf("%s:/in.tgz:ro", tarGzPath),
		"alpine:3.19",
		"sh", "-c",
		fmt.Sprintf("set -e; mkdir -p /tmp/e /restore; tar -xzf /in.tgz -C /tmp/e; if [ -d /tmp/e/%s ]; then cp -a /tmp/e/%s/. /restore/; else cp -a /tmp/e/. /restore/; fi", expectedRoot, expectedRoot),
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("extract to volume %s failed: %v: %s", volumeName, err, stderr.String())
	}
	return nil
}

func (c *CLIClient) CreateContainer(ctx context.Context, imageRef string, name string, mounts []Mount) (string, error) {
	args := []string{"create"}
	if name != "" {
		args = append(args, "--name", name)
	}
	for _, m := range mounts {
		flag := "-v"
		mode := "rw"
		if !m.RW {
			mode = "ro"
		}
		var spec string
		if m.Type == "bind" {
			spec = fmt.Sprintf("%s:%s:%s", m.Source, m.Destination, mode)
		} else if m.Type == "volume" {
			// Use Name for volume
			volName := m.Name
			if volName == "" {
				volName = m.Source
			}
			spec = fmt.Sprintf("%s:%s:%s", volName, m.Destination, mode)
		} else {
			continue
		}
		args = append(args, flag, spec)
	}
	args = append(args, imageRef)
	cmd := exec.CommandContext(ctx, "docker", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker create failed: %v: %s", err, stderr.String())
	}
	containerID := strings.TrimSpace(stdout.String())
	return containerID, nil
}

func (c *CLIClient) CreateContainerFromSpec(ctx context.Context, cfg *container.Config, hostCfg *container.HostConfig, netCfg *network.NetworkingConfig, name string) (string, error) {
	return "", internalerrors.ErrNotImplemented
}

func (c *CLIClient) StartContainer(ctx context.Context, containerID string) error {
	cmd := exec.CommandContext(ctx, "docker", "start", containerID)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker start failed: %v: %s", err, stderr.String())
	}
	return nil
}

func (c *CLIClient) EnsureVolume(ctx context.Context, cfg VolumeConfig) error {
	return internalerrors.ErrNotImplemented
}

func (c *CLIClient) EnsureNetwork(ctx context.Context, cfg NetworkConfig) error {
	return internalerrors.ErrNotImplemented
}

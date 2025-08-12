package docker

import (
	"context"

	internalerrors "github.com/your-username/dockerbackup/internal/errors"
)

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
	return nil, internalerrors.ErrNotImplemented
}

func (c *CLIClient) ExportContainerFilesystem(ctx context.Context, containerID string, destTarPath string) error {
	return internalerrors.ErrNotImplemented
}

func (c *CLIClient) ListVolumes(ctx context.Context) ([]string, error) {
	return nil, internalerrors.ErrNotImplemented
}

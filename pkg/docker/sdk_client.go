package docker

import (
	"context"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

type SDKClient struct {
	cli *client.Client
}

func NewSDKClient() (*SDKClient, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &SDKClient{cli: cli}, nil
}

func (s *SDKClient) CreateContainerFromSpec(ctx context.Context, cfg *container.Config, hostCfg *container.HostConfig, netCfg *network.NetworkingConfig, name string) (string, error) {
	resp, err := s.cli.ContainerCreate(ctx, cfg, hostCfg, netCfg, nil, name)
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

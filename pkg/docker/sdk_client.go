package docker

import (
	"context"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
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

func (s *SDKClient) EnsureVolume(ctx context.Context, cfg VolumeConfig) error {
	_, err := s.cli.VolumeInspect(ctx, cfg.Name)
	if err == nil {
		return nil
	}
	_, err = s.cli.VolumeCreate(ctx, volume.CreateOptions{
		Name:       cfg.Name,
		Driver:     cfg.Driver,
		DriverOpts: cfg.Options,
		Labels:     cfg.Labels,
	})
	return err
}

func (s *SDKClient) EnsureNetwork(ctx context.Context, cfg NetworkConfig) error {
	_, err := s.cli.NetworkInspect(ctx, cfg.Name, types.NetworkInspectOptions{})
	if err == nil {
		return nil
	}
	ipamCfg := []network.IPAMConfig{}
	for _, c := range cfg.IPAM.Config {
		ipamCfg = append(ipamCfg, network.IPAMConfig{Subnet: c.Subnet, Gateway: c.Gateway, IPRange: c.IPRange})
	}
	ipam := &network.IPAM{Driver: cfg.IPAM.Driver, Config: ipamCfg}
	_, err = s.cli.NetworkCreate(ctx, cfg.Name, network.CreateOptions{
		Driver:     cfg.Driver,
		Internal:   cfg.Internal,
		Attachable: cfg.Attachable,
		Ingress:    cfg.Ingress,
		Options:    cfg.Options,
		Labels:     cfg.Labels,
		IPAM:       ipam,
	})
	return err
}

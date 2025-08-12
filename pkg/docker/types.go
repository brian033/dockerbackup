package docker

import (
	"encoding/json"
)

// ContainerInfo captures minimal fields we need from `docker inspect` JSON
// This is intentionally small for v0.
type ContainerInfo struct {
	ID     string          `json:"Id"`
	Name   string          `json:"Name"`
	Mounts []Mount         `json:"Mounts"`
	Config json.RawMessage `json:"Config"`
}

type Mount struct {
	Name        string `json:"Name"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	Type        string `json:"Type"`
	RW          bool   `json:"RW"`
}

func ParseContainerInfo(inspectJSON []byte) (ContainerInfo, error) {
	var arr []ContainerInfo
	if err := json.Unmarshal(inspectJSON, &arr); err != nil {
		return ContainerInfo{}, ErrEmptyInspect
	}
	if len(arr) == 0 {
		return ContainerInfo{}, ErrEmptyInspect
	}
	info := arr[0]
	if len(info.Name) > 0 && info.Name[0] == '/' {
		info.Name = info.Name[1:]
	}
	return info, nil
}

// VolumeConfig captures docker volume inspect essentials
type VolumeConfig struct {
	Name    string            `json:"Name"`
	Driver  string            `json:"Driver"`
	Options map[string]string `json:"Options"`
	Labels  map[string]string `json:"Labels"`
}

// NetworkConfig captures docker network inspect essentials
type NetworkConfig struct {
	Name       string            `json:"Name"`
	Driver     string            `json:"Driver"`
	Options    map[string]string `json:"Options"`
	Internal   bool              `json:"Internal"`
	Attachable bool              `json:"Attachable"`
	Ingress    bool              `json:"Ingress"`
	IPAM       IPAM              `json:"IPAM"`
	Labels     map[string]string `json:"Labels"`
}

type IPAM struct {
	Driver string       `json:"Driver"`
	Config []IPAMConfig `json:"Config"`
}

type IPAMConfig struct {
	Subnet  string `json:"Subnet"`
	Gateway string `json:"Gateway"`
	IPRange string `json:"IPRange"`
}

// ProjectContainerRef references a compose service container
type ProjectContainerRef struct {
	Service       string
	ID            string
	ContainerName string
}

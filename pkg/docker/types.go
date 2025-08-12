package docker

import (
	"encoding/json"
)

// ContainerInfo captures minimal fields we need from `docker inspect` JSON
// This is intentionally small for v0.
type ContainerInfo struct {
	ID    string          `json:"Id"`
	Name  string          `json:"Name"`
	Mounts []Mount        `json:"Mounts"`
	Config json.RawMessage `json:"Config"` // raw for future use
}

type Mount struct {
	Name        string `json:"Name"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	Type        string `json:"Type"`
	RW          bool   `json:"RW"`
}

func ParseContainerInfo(inspectJSON []byte) (ContainerInfo, error) {
	// docker inspect returns an array
	var arr []ContainerInfo
	if err := json.Unmarshal(inspectJSON, &arr); err != nil {
		return ContainerInfo{}, err
	}
	if len(arr) == 0 {
		return ContainerInfo{}, ErrEmptyInspect
	}
	info := arr[0]
	// docker prefixes Name with '/' usually; normalize
	if len(info.Name) > 0 && info.Name[0] == '/' {
		info.Name = info.Name[1:]
	}
	return info, nil
}

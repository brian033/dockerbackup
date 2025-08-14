package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/brian033/dockerbackup/internal/errors"
	"github.com/brian033/dockerbackup/internal/logger"
	"github.com/brian033/dockerbackup/pkg/archive"
	"github.com/brian033/dockerbackup/pkg/compose"
	"github.com/brian033/dockerbackup/pkg/docker"
	"github.com/brian033/dockerbackup/pkg/filesystem"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
)

type BackupTargetType string

const (
	TargetContainer BackupTargetType = "container"
	TargetCompose   BackupTargetType = "compose"
)

type BackupRequest struct {
	TargetType         BackupTargetType
	ContainerID        string
	ComposeProjectPath string
	ProjectName        string
	Options            BackupOptions
}

type BackupResult struct {
	OutputPath string
}

type RestoreRequest struct {
	BackupPath  string
	Options     RestoreOptions
	ProjectName string
	TargetType  BackupTargetType // for future use
}

type RestoreResult struct {
	RestoredID string
}

type ValidationResult struct {
	Valid   bool
	Details string
}

type BackupEngine interface {
	Backup(ctx context.Context, request BackupRequest) (*BackupResult, error)
	Restore(ctx context.Context, request RestoreRequest) (*RestoreResult, error)
	Validate(ctx context.Context, backupPath string) (*ValidationResult, error)
}

type DefaultBackupEngine struct {
	archiveHandler archive.ArchiveHandler
	dockerClient   docker.DockerClient
	filesystem     filesystem.Handler
	log            logger.Logger
}

func NewDefaultBackupEngine(arch archive.ArchiveHandler, dc docker.DockerClient, fs filesystem.Handler, log logger.Logger) BackupEngine {
	return &DefaultBackupEngine{
		archiveHandler: arch,
		dockerClient:   dc,
		filesystem:     fs,
		log:            log,
	}
}

type backupMetadata struct {
	Version         int       `json:"version"`
	CreatedAt       time.Time `json:"createdAt"`
	ContainerID     string    `json:"containerID"`
	ContainerName   string    `json:"containerName"`
	Engine          string    `json:"engine"`
	IncludesVolumes bool      `json:"includesVolumes"`
}

func (e *DefaultBackupEngine) Backup(ctx context.Context, request BackupRequest) (*BackupResult, error) {
	if request.TargetType == TargetCompose {
		projectPath := request.ComposeProjectPath
		if projectPath == "" {
			projectPath = "."
		}
		// Determine project name
		projectName := request.ProjectName
		if projectName == "" {
			// Try to read compose name
			for _, name := range []string{"docker-compose.yml", "docker-compose.yaml"} {
				if b, err := os.ReadFile(filepath.Join(projectPath, name)); err == nil {
					if n := compose.ParseProjectName(b); n != "" {
						projectName = n
						break
					}
				}
			}
			if projectName == "" {
				projectName = filepath.Base(projectPath)
			}
		}
		// Prepare working dir
		workDir, err := os.MkdirTemp("", fmt.Sprintf("dockerbackup_compose_%s_*", safeName(projectName)))
		if err != nil {
			return nil, &errors.OperationError{Op: "create temp dir", Err: err}
		}
		defer func() { _ = os.RemoveAll(workDir) }()

		composeDir := filepath.Join(workDir, "compose-files")
		containersDir := filepath.Join(workDir, "containers")
		networksDir := filepath.Join(workDir, "networks")
		volumesDir := filepath.Join(workDir, "volumes")
		_ = os.MkdirAll(composeDir, 0o755)
		_ = os.MkdirAll(containersDir, 0o755)
		_ = os.MkdirAll(networksDir, 0o755)
		_ = os.MkdirAll(volumesDir, 0o755)

		// Copy compose files
		for _, name := range []string{"docker-compose.yml", "docker-compose.yaml", "docker-compose.override.yml", ".env"} {
			src := filepath.Join(projectPath, name)
			if b, err := os.ReadFile(src); err == nil {
				_ = os.WriteFile(filepath.Join(composeDir, name), b, 0o644)
			}
		}

		// Discover project containers: prefer label-based, fallback to name heuristic
		refs, err := e.dockerClient.ListProjectContainersByLabel(ctx, projectName)
		if err != nil || len(refs) == 0 {
			refs, _ = e.dockerClient.ListProjectContainers(ctx, projectName)
		}
		if len(refs) == 0 {
			return nil, &errors.OperationError{Op: "discover project containers", Err: fmt.Errorf("no containers found for project %s", projectName)}
		}
		// Backup each service container
		serviceNames := make([]string, 0, len(refs))
		for _, r := range refs {
			serviceNames = append(serviceNames, r.Service)
			svcDir := filepath.Join(containersDir, r.Service)
			_ = os.MkdirAll(svcDir, 0o755)
			outTar := filepath.Join(svcDir, "container.tar.gz")
			builder := NewBackupOptionsBuilder().WithOutput(outTar).WithCompression(0)
			_, err := e.Backup(ctx, BackupRequest{TargetType: TargetContainer, ContainerID: r.ID, Options: builder.Build()})
			if err != nil {
				return nil, err
			}
		}

		// Aggregate networks used by the containers
		seenNets := map[string]struct{}{}
		var netCfgs []docker.NetworkConfig
		for _, r := range refs {
			b, err := e.dockerClient.InspectContainer(ctx, r.ID)
			if err != nil {
				continue
			}
			var cj types.ContainerJSON
			if err := json.Unmarshal(b, &cj); err != nil {
				continue
			}
			if cj.NetworkSettings == nil {
				continue
			}
			for name := range cj.NetworkSettings.Networks {
				if _, ok := seenNets[name]; ok {
					continue
				}
				seenNets[name] = struct{}{}
				if n, err := e.dockerClient.InspectNetwork(ctx, name); err == nil {
					netCfgs = append(netCfgs, *n)
				}
			}
		}
		if len(netCfgs) > 0 {
			if b, err := json.MarshalIndent(netCfgs, "", "  "); err == nil {
				_ = os.WriteFile(filepath.Join(networksDir, "network_configs.json"), b, 0o644)
			}
		}

		// Collect volume configs used across services (by mounts)
		volSet := map[string]struct{}{}
		var volCfgs []docker.VolumeConfig
		for _, r := range refs {
			b, err := e.dockerClient.InspectContainer(ctx, r.ID)
			if err != nil {
				continue
			}
			var ci docker.ContainerInfo
			if info, err := docker.ParseContainerInfo(b); err == nil {
				ci = info
			} else {
				continue
			}
			for _, m := range ci.Mounts {
				if m.Type == "volume" && m.Name != "" {
					if _, ok := volSet[m.Name]; ok {
						continue
					}
					volSet[m.Name] = struct{}{}
					if v, err := e.dockerClient.InspectVolume(ctx, m.Name); err == nil && v != nil {
						volCfgs = append(volCfgs, *v)
					}
				}
			}
		}
		if len(volCfgs) > 0 {
			if b, err := json.MarshalIndent(volCfgs, "", "  "); err == nil {
				_ = os.WriteFile(filepath.Join(volumesDir, "volume_configs.json"), b, 0o644)
			}
		}

		// Metadata
		meta := map[string]any{"version": 1, "projectName": projectName, "services": serviceNames}
		if b, err := json.MarshalIndent(meta, "", "  "); err == nil {
			_ = os.WriteFile(filepath.Join(workDir, "metadata.json"), b, 0o644)
		}

		// Final archive
		outputPath := request.Options.OutputPath
		if outputPath == "" {
			outputPath = filepath.Join(projectPath, fmt.Sprintf("%s_compose_backup.tar.gz", safeName(projectName)))
		}
		sources := []archive.ArchiveSource{
			{Path: composeDir, DestPath: "compose-files"},
			{Path: containersDir, DestPath: "containers"},
			{Path: networksDir, DestPath: "networks"},
			{Path: volumesDir, DestPath: "volumes"},
			{Path: filepath.Join(workDir, "metadata.json"), DestPath: "metadata.json"},
		}
		if th, ok := e.archiveHandler.(*archive.TarArchiveHandler); ok {
			th.SetCompressionLevel(request.Options.CompressionLevel)
		}
		if err := e.archiveHandler.CreateArchive(ctx, sources, outputPath); err != nil {
			return nil, &errors.OperationError{Op: "create compose archive", Err: err}
		}
		return &BackupResult{OutputPath: outputPath}, nil
	}

	if request.TargetType != TargetContainer {
		return nil, &errors.ValidationError{Msg: "unsupported target type"}
	}
	if request.ContainerID == "" {
		return nil, &errors.ValidationError{Field: "ContainerID", Msg: "required"}
	}
	// Inspect container
	inspectJSON, err := e.dockerClient.InspectContainer(ctx, request.ContainerID)
	if err != nil {
		return nil, &errors.OperationError{Op: "inspect container", Err: err}
	}
	info, err := docker.ParseContainerInfo(inspectJSON)
	if err != nil {
		return nil, &errors.OperationError{Op: "parse container inspect", Err: err}
	}

	// Determine output path
	outputPath := request.Options.OutputPath
	if outputPath == "" {
		cwd, _ := os.Getwd()
		base := fmt.Sprintf("%s_backup.tar.gz", safeName(info.Name))
		outputPath = filepath.Join(cwd, base)
	}

	// Prepare working dir
	workDir, err := os.MkdirTemp("", fmt.Sprintf("dockerbackup_%s_*", safeName(info.Name)))
	if err != nil {
		return nil, &errors.OperationError{Op: "create temp dir", Err: err}
	}
	defer func() {
		_ = os.RemoveAll(workDir)
	}()

	containerJSONPath := filepath.Join(workDir, "container.json")
	filesystemTarPath := filepath.Join(workDir, "filesystem.tar")
	volumesDir := filepath.Join(workDir, "volumes")
	metadataPath := filepath.Join(workDir, "metadata.json")
	imageTarPath := filepath.Join(workDir, "image.tar")

	if err := os.WriteFile(containerJSONPath, inspectJSON, 0o644); err != nil {
		return nil, &errors.OperationError{Op: "write container.json", Err: err}
	}
	e.log.Infof("Exporting filesystem for container %s", info.Name)
	if err := e.dockerClient.ExportContainerFilesystem(ctx, info.ID, filesystemTarPath); err != nil {
		return nil, &errors.OperationError{Op: "export container filesystem", Err: err}
	}

	// Archive named volumes and bind mounts (Linux supported)
	includesVolumes := false
	if err := os.MkdirAll(volumesDir, 0o755); err != nil {
		return nil, &errors.OperationError{Op: "create volumes dir", Err: err}
	}
	for _, m := range info.Mounts {
		// Named volumes
		if m.Type == "volume" && m.Name != "" && m.Source != "" {
			includesVolumes = true
			volTarGz := filepath.Join(volumesDir, fmt.Sprintf("%s.tar.gz", safeName(m.Name)))
			src := archive.ArchiveSource{Path: m.Source, DestPath: m.Name}
			if err := e.archiveHandler.CreateArchive(ctx, []archive.ArchiveSource{src}, volTarGz); err != nil {
				return nil, &errors.OperationError{Op: fmt.Sprintf("archive volume %s", m.Name), Err: err}
			}
			continue
		}
		// Bind mounts (host directories)
		if m.Type == "bind" && m.Source != "" {
			includesVolumes = true
			base := filepath.Base(m.Source)
			name := fmt.Sprintf("bind_%s", safeName(base))
			volTarGz := filepath.Join(volumesDir, fmt.Sprintf("%s.tar.gz", name))
			src := archive.ArchiveSource{Path: m.Source, DestPath: base}
			if err := e.archiveHandler.CreateArchive(ctx, []archive.ArchiveSource{src}, volTarGz); err != nil {
				return nil, &errors.OperationError{Op: fmt.Sprintf("archive bind mount %s", m.Source), Err: err}
			}
			continue
		}
	}

	// Capture volume configs for named volumes
	volCfgPath := filepath.Join(volumesDir, "volume_configs.json")
	var volCfgs []docker.VolumeConfig
	for _, m := range info.Mounts {
		if m.Type == "volume" && m.Name != "" {
			if v, err := e.dockerClient.InspectVolume(ctx, m.Name); err == nil && v != nil {
				volCfgs = append(volCfgs, *v)
			}
		}
	}
	if len(volCfgs) > 0 {
		if b, err := json.MarshalIndent(volCfgs, "", "  "); err == nil {
			_ = os.WriteFile(volCfgPath, b, 0o644)
		}
	}

	// Capture network configs for attached networks via container inspect (names only) -> inspect per network
	netDir := filepath.Join(workDir, "networks")
	if err := os.MkdirAll(netDir, 0o755); err != nil {
		return nil, &errors.OperationError{Op: "create networks dir", Err: err}
	}
	var netCfgs []docker.NetworkConfig
	// Try to read network names from container.json content (cj.NetworkSettings.Networks). Parse quickly.
	var cj types.ContainerJSON
	_ = json.Unmarshal(inspectJSON, &cj)
	if cj.NetworkSettings != nil {
		for name := range cj.NetworkSettings.Networks {
			if n, err := e.dockerClient.InspectNetwork(ctx, name); err == nil && n != nil {
				netCfgs = append(netCfgs, *n)
			}
		}
	}
	netCfgPath := filepath.Join(netDir, "network_configs.json")
	if len(netCfgs) > 0 {
		if b, err := json.MarshalIndent(netCfgs, "", "  "); err == nil {
			_ = os.WriteFile(netCfgPath, b, 0o644)
		}
	}

	// Write metadata
	meta := backupMetadata{
		Version:         1,
		CreatedAt:       time.Now().UTC(),
		ContainerID:     info.ID,
		ContainerName:   info.Name,
		Engine:          "default",
		IncludesVolumes: includesVolumes,
	}
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, &errors.OperationError{Op: "marshal metadata", Err: err}
	}
	if err := os.WriteFile(metadataPath, b, 0o644); err != nil {
		return nil, &errors.OperationError{Op: "write metadata.json", Err: err}
	}

	// Try to save original image if present in inspect (non-empty Image ID or name)
	if cj.ContainerJSONBase != nil && cj.ContainerJSONBase.Image != "" {
		_ = e.dockerClient.ImageSave(ctx, cj.ContainerJSONBase.Image, imageTarPath)
	}

	// Build final archive
	e.log.Infof("Packaging backup -> %s", outputPath)
	sources := []archive.ArchiveSource{
		{Path: containerJSONPath, DestPath: "container.json"},
		{Path: filesystemTarPath, DestPath: "filesystem.tar"},
		{Path: volumesDir, DestPath: "volumes"},
		{Path: netDir, DestPath: "networks"},
		{Path: metadataPath, DestPath: "metadata.json"},
	}
	if _, err := os.Stat(imageTarPath); err == nil {
		sources = append(sources, archive.ArchiveSource{Path: imageTarPath, DestPath: "image.tar"})
	}
	if th, ok := e.archiveHandler.(*archive.TarArchiveHandler); ok {
		th.SetCompressionLevel(request.Options.CompressionLevel)
	}
	if err := e.archiveHandler.CreateArchive(ctx, sources, outputPath); err != nil {
		return nil, &errors.OperationError{Op: "create final archive", Err: err}
	}

	return &BackupResult{OutputPath: outputPath}, nil
}

func (e *DefaultBackupEngine) Restore(ctx context.Context, request RestoreRequest) (*RestoreResult, error) {
	if request.TargetType == TargetCompose {
		// Extract
		tmpDir, err := os.MkdirTemp("", "dockerbackup_compose_restore_*")
		if err != nil {
			return nil, &errors.OperationError{Op: "create temp dir", Err: err}
		}
		defer func() { _ = os.RemoveAll(tmpDir) }()
		if err := e.archiveHandler.ExtractArchive(ctx, request.BackupPath, tmpDir); err != nil {
			return nil, &errors.OperationError{Op: "extract backup", Err: err}
		}

		// Ensure networks from configs
		if b, err := os.ReadFile(filepath.Join(tmpDir, "networks", "network_configs.json")); err == nil {
			var netCfgs []docker.NetworkConfig
			_ = json.Unmarshal(b, &netCfgs)
			for _, nc := range netCfgs {
				_ = e.dockerClient.EnsureNetwork(ctx, nc)
			}
		}
		// Ensure volumes from configs
		if b, err := os.ReadFile(filepath.Join(tmpDir, "volumes", "volume_configs.json")); err == nil {
			var volCfgs []docker.VolumeConfig
			_ = json.Unmarshal(b, &volCfgs)
			for _, vc := range volCfgs {
				_ = e.dockerClient.EnsureVolume(ctx, vc)
			}
		}

		// Compute service order from compose-files if present
		services := map[string]struct{}{}
		order := []string{}
		composePathYml := filepath.Join(tmpDir, "compose-files", "docker-compose.yml")
		composePathYaml := filepath.Join(tmpDir, "compose-files", "docker-compose.yaml")
		var data []byte
		if b, err := os.ReadFile(composePathYml); err == nil {
			data = b
		} else if b, err := os.ReadFile(composePathYaml); err == nil {
			data = b
		}
		if len(data) > 0 {
			ord, names := compose.OrderFromComposeYAML(data)
			if len(ord) > 0 {
				order = ord
			}
			for _, n := range names {
				services[n] = struct{}{}
			}
		}
		// Fallback: discover services by directory structure
		if len(services) == 0 {
			entries, _ := os.ReadDir(filepath.Join(tmpDir, "containers"))
			for _, e2 := range entries {
				if e2.IsDir() {
					services[e2.Name()] = struct{}{}
				}
			}
		}
		if len(order) == 0 {
			for s := range services {
				order = append(order, s)
			}
			sort.Strings(order)
		}

		// Restore each service container tar without starting; then start all if requested
		restored := []string{}
		for _, svc := range order {
			svcDir := filepath.Join(tmpDir, "containers", svc)
			// find a .tar.gz file inside
			entries, _ := os.ReadDir(svcDir)
			var tarPath string
			for _, e2 := range entries {
				if strings.HasSuffix(e2.Name(), ".tar.gz") {
					tarPath = filepath.Join(svcDir, e2.Name())
					break
				}
			}
			if tarPath == "" {
				continue
			}
			_, err := e.Restore(ctx, RestoreRequest{BackupPath: tarPath, Options: RestoreOptions{Start: false, ReplaceExisting: request.Options.ReplaceExisting, DropHostIPs: request.Options.DropHostIPs, ReassignIPs: request.Options.ReassignIPs, FallbackBridge: request.Options.FallbackBridge, BindRestoreRoot: request.Options.BindRestoreRoot, ForceBindIP: request.Options.ForceBindIP, BindInterface: request.Options.BindInterface, DropDevices: request.Options.DropDevices, DropCaps: request.Options.DropCaps, DropSeccomp: request.Options.DropSeccomp, DropAppArmor: request.Options.DropAppArmor}})
			if err == nil {
				restored = append(restored, svc)
			}
		}
		if request.Options.Start {
			// Start in order and optionally wait healthy
			for _, svc := range order {
				// best-effort: assume container name == svc or was restored with original name
				_ = execCommand(ctx, "docker", "start", svc)
			}
		}
		return &RestoreResult{RestoredID: strings.Join(restored, ",")}, nil
	}

	// Extract backup to temp dir
	tmpDir, err := os.MkdirTemp("", "dockerbackup_restore_*")
	if err != nil {
		return nil, &errors.OperationError{Op: "create temp dir", Err: err}
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	if err := e.archiveHandler.ExtractArchive(ctx, request.BackupPath, tmpDir); err != nil {
		return nil, &errors.OperationError{Op: "extract backup", Err: err}
	}

	// Read container.json (docker inspect). Support both single object and array forms.
	containerJSONPath := filepath.Join(tmpDir, "container.json")
	b, err := os.ReadFile(containerJSONPath)
	if err != nil {
		return nil, &errors.OperationError{Op: "read container.json", Err: err}
	}
	var cj types.ContainerJSON
	if err := json.Unmarshal(b, &cj); err != nil || cj.ContainerJSONBase == nil {
		var arr []types.ContainerJSON
		if err2 := json.Unmarshal(b, &arr); err2 != nil || len(arr) == 0 || arr[0].ContainerJSONBase == nil {
			return nil, &errors.OperationError{Op: "unmarshal container.json", Err: err2}
		}
		cj = arr[0]
	}

	// Prefer image load if image.tar exists; else import filesystem.tar
	imageTar := filepath.Join(tmpDir, "image.tar")
	imageRef := ""
	if _, err := os.Stat(imageTar); err == nil {
		if err := e.dockerClient.ImageLoad(ctx, imageTar); err == nil {
			// Use original image reference if available; else keep empty and rely on cfg.Image overwritten later
			imageRef = cj.ContainerJSONBase.Image
		}
	}
	if imageRef == "" {
		fsTarPath := filepath.Join(tmpDir, "filesystem.tar")
		if _, err := os.Stat(fsTarPath); err == nil {
			imgID, err := e.dockerClient.ImportImage(ctx, fsTarPath, "")
			if err != nil {
				return nil, &errors.OperationError{Op: "docker import image", Err: err}
			}
			imageRef = imgID
		} else {
			return nil, &errors.OperationError{Op: "filesystem.tar missing", Err: err}
		}
	}
	// If cj.Config.Image looks like repo:tag and we loaded/imported an image ID, retag the ID to that name
	if cj.Config != nil && cj.Config.Image != "" && imageRef != "" {
		_ = e.dockerClient.TagImage(ctx, imageRef, cj.Config.Image)
	}

	// Load saved volume and network configs if present
	volCfgs := []docker.VolumeConfig{}
	if b, err := os.ReadFile(filepath.Join(tmpDir, "volumes", "volume_configs.json")); err == nil {
		_ = json.Unmarshal(b, &volCfgs)
	}
	netCfgs := []docker.NetworkConfig{}
	if b, err := os.ReadFile(filepath.Join(tmpDir, "networks", "network_configs.json")); err == nil {
		_ = json.Unmarshal(b, &netCfgs)
	}

	// Apply network name mapping to cj.NetworkSettings before creating netCfg
	if cj.NetworkSettings != nil && cj.NetworkSettings.Networks != nil && len(request.Options.NetworkMap) > 0 {
		mapped := map[string]*network.EndpointSettings{}
		for name, ns := range cj.NetworkSettings.Networks {
			newName := name
			if m, ok := request.Options.NetworkMap[name]; ok && m != "" {
				newName = m
			}
			mapped[newName] = ns
		}
		cj.NetworkSettings.Networks = mapped
	}

	// Ensure networks exist with potential parent overrides/fallbacks (macvlan/ipvlan)
	for _, nc := range netCfgs {
		if newName, ok := request.Options.NetworkMap[nc.Name]; ok && newName != "" {
			nc.Name = newName
		}
		if parent, ok := request.Options.ParentMap[nc.Name]; ok && parent != "" {
			if nc.Options == nil {
				nc.Options = map[string]string{}
			}
			nc.Options["parent"] = parent
		}
		// If still macvlan/ipvlan and no parent present and fallbackBridge is set, convert to bridge
		if request.Options.FallbackBridge {
			if (nc.Driver == "macvlan" || nc.Driver == "ipvlan") && (nc.Options == nil || nc.Options["parent"] == "") {
				nc.Driver = "bridge"
				delete(nc.Options, "parent")
			}
		}
		_ = e.dockerClient.EnsureNetwork(ctx, nc)
	}

	// Effective mounts from inspect
	effectiveMounts := make([]docker.Mount, 0, len(cj.Mounts))
	for _, m := range cj.Mounts {
		mt := string(m.Type)
		name := m.Name
		src := m.Source
		effectiveMounts = append(effectiveMounts, docker.Mount{
			Name:        name,
			Source:      src,
			Destination: m.Destination,
			Type:        mt,
			RW:          m.RW,
		})
	}

	// Ensure volumes exist using captured driver/options before data restore
	for _, vc := range volCfgs {
		_ = e.dockerClient.EnsureVolume(ctx, vc)
	}

	// Restore volumes and bind mounts data; create volumes using VolumeCreate (driver/options not yet wired into CLI variant)
	for _, m := range effectiveMounts {
		if m.Type == "volume" && m.Name != "" {
			if err := e.dockerClient.VolumeCreate(ctx, m.Name); err != nil {
				return nil, &errors.OperationError{Op: fmt.Sprintf("create volume %s", m.Name), Err: err}
			}
			volTarGz := filepath.Join(tmpDir, "volumes", fmt.Sprintf("%s.tar.gz", m.Name))
			if _, err := os.Stat(volTarGz); err == nil {
				if err := e.dockerClient.ExtractTarGzToVolume(ctx, m.Name, volTarGz, m.Name); err != nil {
					return nil, &errors.OperationError{Op: fmt.Sprintf("restore volume %s", m.Name), Err: err}
				}
			}
		}
		if m.Type == "bind" && m.Source != "" {
			base := filepath.Base(m.Source)
			bindName := fmt.Sprintf("bind_%s", safeName(base))
			bindTarGz := filepath.Join(tmpDir, "volumes", fmt.Sprintf("%s.tar.gz", bindName))
			if _, err := os.Stat(bindTarGz); err == nil {
				if err := os.MkdirAll(m.Source, 0o755); err != nil {
					return nil, &errors.OperationError{Op: fmt.Sprintf("mkdir bind path %s", m.Source), Err: err}
				}
				if err := extractTarGzToHost(ctx, bindTarGz, m.Source, base); err != nil {
					return nil, &errors.OperationError{Op: fmt.Sprintf("restore bind mount %s", m.Source), Err: err}
				}
			}
		}
	}

	// Build Docker SDK Config/HostConfig/NetworkingConfig from inspect
	cfg := cj.Config
	if cfg == nil {
		cfg = &container.Config{}
	}
	hostCfg := cj.HostConfig
	if hostCfg == nil {
		hostCfg = &container.HostConfig{}
	}
	cfg.Image = imageRef

	// Validate HostIp presence: remove bindings with missing HostIp unless DropHostIPs set, else keep
	if hostCfg.PortBindings != nil {
		hostIPs, _ := e.dockerClient.HostIPs(ctx)
		present := map[string]struct{}{}
		for _, ip := range hostIPs {
			present[ip] = struct{}{}
		}
		for port, bindings := range hostCfg.PortBindings {
			filtered := bindings[:0]
			for _, b := range bindings {
				if b.HostIP == "" || request.Options.DropHostIPs {
					b.HostIP = ""
					filtered = append(filtered, b)
					continue
				}
				if _, ok := present[b.HostIP]; ok {
					filtered = append(filtered, b)
				} else {
					e.log.Infof("Port binding HostIp %s not present; skipping binding for %s", b.HostIP, port)
				}
			}
			hostCfg.PortBindings[port] = filtered
		}
	}

	// NetworkingConfig from NetworkSettings.Networks, optionally clearing static IPs
	netCfg := &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{}}
	conflictingStaticIP := false
	if cj.NetworkSettings != nil && cj.NetworkSettings.Networks != nil {
		for name, ns := range cj.NetworkSettings.Networks {
			ep := &network.EndpointSettings{Aliases: ns.Aliases}
			ipam := ns.IPAMConfig
			// simple conflict check: if IPAMConfig has IPv4 address and subnet overlaps with an existing interface network, mark conflict
			if ipam != nil && ipam.IPv4Address != "" {
				if conflictWithHostIPv4(ipam.IPv4Address) {
					conflictingStaticIP = true
				}
			}
			if request.Options.ReassignIPs || (request.Options.AutoRelaxIPs && conflictingStaticIP) {
				ep.IPAMConfig = nil
			} else {
				ep.IPAMConfig = ns.IPAMConfig
			}
			netCfg.EndpointsConfig[name] = ep
		}
	}

	// Replacement: if a container with target name exists, remove it when ReplaceExisting
	newName := cj.Name
	if strings.HasPrefix(newName, "/") {
		newName = strings.TrimPrefix(newName, "/")
	}
	if request.Options.ContainerName != "" {
		newName = request.Options.ContainerName
	}
	if request.Options.ReplaceExisting && newName != "" {
		// best-effort remove existing
		_ = execCommand(ctx, "docker", "rm", "-f", newName)
	}

	// Adjust HostConfig for safe-mode drops
	hostCfg = cj.HostConfig
	if hostCfg == nil {
		hostCfg = &container.HostConfig{}
	}
	if request.Options.DropDevices {
		hostCfg.Devices = nil
	}
	if request.Options.DropCaps {
		hostCfg.CapAdd = nil
		hostCfg.CapDrop = nil
	}
	if request.Options.DropSeccomp || request.Options.DropAppArmor {
		filtered := make([]string, 0, len(hostCfg.SecurityOpt))
		for _, opt := range hostCfg.SecurityOpt {
			if request.Options.DropSeccomp && strings.Contains(opt, "seccomp=") {
				continue
			}
			if request.Options.DropAppArmor && strings.Contains(opt, "apparmor=") {
				continue
			}
			filtered = append(filtered, opt)
		}
		hostCfg.SecurityOpt = filtered
	}

	// Bind restore root: relocate missing bind sources
	if request.Options.BindRestoreRoot != "" {
		for i := range hostCfg.Mounts {
			m := &hostCfg.Mounts[i]
			if m.Type == "bind" && m.Source != "" {
				if _, err := os.Stat(m.Source); os.IsNotExist(err) {
					base := filepath.Base(m.Source)
					newSrc := filepath.Join(request.Options.BindRestoreRoot, base)
					_ = os.MkdirAll(newSrc, 0o755)
					m.Source = newSrc
				}
			}
		}
	}

	cfg = cj.Config
	if cfg == nil {
		cfg = &container.Config{}
	}
	cfg.Image = imageRef

	// Ports: apply force-bind-ip or bind-interface preference
	if hostCfg.PortBindings != nil {
		// If bind-interface set, try to pick its IP
		preferredIP := request.Options.ForceBindIP
		if preferredIP == "" && request.Options.BindInterface != "" {
			if ip, err := primaryIPv4OfInterface(request.Options.BindInterface); err == nil {
				preferredIP = ip
			}
		}
		if preferredIP != "" {
			for port, bindings := range hostCfg.PortBindings {
				for i := range bindings {
					bindings[i].HostIP = preferredIP
				}
				hostCfg.PortBindings[port] = bindings
			}
		}
	}

	// Determine new name (already computed above)
	// newName is ready

	// Prefer SDK-based creation if available
	containerID, err := e.dockerClient.CreateContainerFromSpec(ctx, cfg, hostCfg, netCfg, newName)
	if err != nil && !strings.Contains(err.Error(), "not implemented") {
		return nil, &errors.OperationError{Op: "container create from spec", Err: err}
	}
	if err != nil {
		var mounts []docker.Mount
		for _, m := range effectiveMounts {
			mounts = append(mounts, docker.Mount{Name: m.Name, Source: m.Source, Destination: m.Destination, Type: m.Type, RW: m.RW})
		}
		containerID, err = e.dockerClient.CreateContainer(ctx, imageRef, newName, mounts)
		if err != nil {
			return nil, &errors.OperationError{Op: "docker create", Err: err}
		}
	}

	if request.Options.Start {
		if err := e.dockerClient.StartContainer(ctx, containerID); err != nil {
			return nil, &errors.OperationError{Op: "docker start", Err: err}
		}
		if request.Options.WaitHealthy {
			// If no healthcheck defined in the original inspect, skip waiting
			noHealthcheck := cj.ContainerJSONBase == nil || cj.ContainerJSONBase.State == nil || cj.ContainerJSONBase.State.Health == nil
			if !noHealthcheck {
				timeout := time.Duration(request.Options.WaitTimeoutSeconds) * time.Second
				if timeout <= 0 {
					timeout = 2 * time.Minute
				}
				deadline := time.Now().Add(timeout)
				for {
					if time.Now().After(deadline) {
						return &RestoreResult{RestoredID: containerID}, nil
					}
					status, health, _ := e.dockerClient.ContainerState(ctx, containerID)
					if status == "exited" || status == "dead" || status == "removing" {
						return &RestoreResult{RestoredID: containerID}, nil
					}
					if health == "healthy" {
						break
					}
					time.Sleep(2 * time.Second)
				}
			}
		}
	}
	return &RestoreResult{RestoredID: containerID}, nil
}

func (e *DefaultBackupEngine) Validate(ctx context.Context, backupPath string) (*ValidationResult, error) {
	entries, err := e.archiveHandler.ListArchive(ctx, backupPath)
	if err != nil {
		return nil, &errors.OperationError{Op: "list archive", Err: err}
	}
	// Required top-level items
	required := map[string]bool{
		"container.json": false,
		"filesystem.tar": false,
		"metadata.json":  false,
	}
	for _, en := range entries {
		// Normalize names to forward slashes in tar
		switch en.Path {
		case "container.json":
			required["container.json"] = true
		case "filesystem.tar":
			required["filesystem.tar"] = true
		case "metadata.json":
			required["metadata.json"] = true
		}
	}
	missing := make([]string, 0)
	for name, ok := range required {
		if !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return &ValidationResult{
			Valid:   false,
			Details: fmt.Sprintf("missing required entries: %v", missing),
		}, nil
	}
	return &ValidationResult{Valid: true, Details: "backup structure is valid"}, nil
}

func safeName(name string) string {
	if name == "" {
		return "container"
	}
	// Replace path separators and spaces
	s := name
	replacer := []struct{ old, new string }{
		{"/", "-"}, {"\\", "-"}, {" ", "-"}, {":", "-"}, {"\t", "-"},
	}
	for _, r := range replacer {
		s = stringReplaceAll(s, r.old, r.new)
	}
	return s
}

func stringReplaceAll(s, old, new string) string {
	for {
		idx := indexOf(s, old)
		if idx < 0 {
			return s
		}
		s = s[:idx] + new + s[idx+len(old):]
	}
}

func indexOf(s, sub string) int {
	// naive search; avoids importing strings to keep imports compact here
	n := len(s)
	m := len(sub)
	if m == 0 {
		return 0
	}
	for i := 0; i+m <= n; i++ {
		if s[i:i+m] == sub {
			return i
		}
	}
	return -1
}

func extractTarGzToHost(ctx context.Context, tarGzPath string, destDir string, expectedRoot string) error {
	f, err := os.Open(tarGzPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := hdr.Name
		if expectedRoot != "" {
			// strip the root dir prefix
			if strings.HasPrefix(name, expectedRoot+"/") {
				name = strings.TrimPrefix(name, expectedRoot+"/")
			}
		}
		outPath := filepath.Join(destDir, name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(outPath, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		}
	}
	return nil
}

func execCommand(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Run()
}

func primaryIPv4OfInterface(ifName string) (string, error) {
	itf, err := net.InterfaceByName(ifName)
	if err != nil {
		return "", err
	}
	addrs, err := itf.Addrs()
	if err != nil {
		return "", err
	}
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok {
			ip := ipn.IP.To4()
			if ip != nil && !ip.IsLoopback() {
				return ip.String(), nil
			}
		}
	}
	return "", fmt.Errorf("no IPv4 on interface %s", ifName)
}

func conflictWithHostIPv4(addr string) bool {
	ip := net.ParseIP(addr).To4()
	if ip == nil {
		return false
	}
	ifs, _ := net.Interfaces()
	for _, it := range ifs {
		addrs, _ := it.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok {
				if ipn.IP.To4() != nil && ipn.Contains(ip) {
					return true
				}
			}
		}
	}
	return false
}

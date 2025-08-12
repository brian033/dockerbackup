package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/brian033/dockerbackup/internal/errors"
	"github.com/brian033/dockerbackup/internal/logger"
	"github.com/brian033/dockerbackup/pkg/archive"
	"github.com/brian033/dockerbackup/pkg/docker"
	"github.com/brian033/dockerbackup/pkg/filesystem"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	dockermount "github.com/docker/docker/api/types/mount"
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
	if request.TargetType != TargetContainer {
		return nil, &errors.ValidationError{Msg: "only container backup supported in v0"}
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

	// Build final archive
	e.log.Infof("Packaging backup -> %s", outputPath)
	sources := []archive.ArchiveSource{
		{Path: containerJSONPath, DestPath: "container.json"},
		{Path: filesystemTarPath, DestPath: "filesystem.tar"},
		{Path: volumesDir, DestPath: "volumes"},
		{Path: netDir, DestPath: "networks"},
		{Path: metadataPath, DestPath: "metadata.json"},
	}
	if err := e.archiveHandler.CreateArchive(ctx, sources, outputPath); err != nil {
		return nil, &errors.OperationError{Op: "create final archive", Err: err}
	}

	return &BackupResult{OutputPath: outputPath}, nil
}

func (e *DefaultBackupEngine) Restore(ctx context.Context, request RestoreRequest) (*RestoreResult, error) {
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

	// Import filesystem to new image
	fsTarPath := filepath.Join(tmpDir, "filesystem.tar")
	imageRef := ""
	if _, err := os.Stat(fsTarPath); err == nil {
		imgID, err := e.dockerClient.ImportImage(ctx, fsTarPath, "")
		if err != nil {
			return nil, &errors.OperationError{Op: "docker import image", Err: err}
		}
		imageRef = imgID
	} else {
		return nil, &errors.OperationError{Op: "filesystem.tar missing", Err: err}
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

	// Ensure networks exist
	for _, nc := range netCfgs {
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
	// Ensure the image points to the imported one
	cfg.Image = imageRef

	// If HostConfig.Mounts empty, translate from effective mounts
	if len(hostCfg.Mounts) == 0 && len(effectiveMounts) > 0 {
		for _, m := range effectiveMounts {
			mt := dockermount.TypeBind
			if m.Type == "volume" {
				mt = dockermount.TypeVolume
			}
			ro := !m.RW
			source := m.Source
			if m.Type == "volume" && m.Name != "" {
				source = m.Name
			}
			hostCfg.Mounts = append(hostCfg.Mounts, dockermount.Mount{
				Type:     mt,
				Source:   source,
				Target:   m.Destination,
				ReadOnly: ro,
			})
		}
	}

	// NetworkingConfig from NetworkSettings.Networks
	netCfg := &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{}}
	if cj.NetworkSettings != nil && cj.NetworkSettings.Networks != nil {
		for name, ns := range cj.NetworkSettings.Networks {
			netCfg.EndpointsConfig[name] = &network.EndpointSettings{
				Aliases:    ns.Aliases,
				IPAMConfig: ns.IPAMConfig,
			}
		}
	}

	// Determine new name
	newName := cj.Name
	if strings.HasPrefix(newName, "/") {
		newName = strings.TrimPrefix(newName, "/")
	}
	if request.Options.ContainerName != "" {
		newName = request.Options.ContainerName
	}

	// Prefer SDK-based creation if available
	containerID, err := e.dockerClient.CreateContainerFromSpec(ctx, cfg, hostCfg, netCfg, newName)
	if err != nil && !strings.Contains(err.Error(), "not implemented") {
		return nil, &errors.OperationError{Op: "container create from spec", Err: err}
	}
	if err != nil {
		// Fallback to CLI create using minimal info
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

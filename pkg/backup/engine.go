package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/brian033/dockerbackup/internal/errors"
	"github.com/brian033/dockerbackup/internal/logger"
	"github.com/brian033/dockerbackup/pkg/archive"
	"github.com/brian033/dockerbackup/pkg/docker"
	"github.com/brian033/dockerbackup/pkg/filesystem"
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
		{Path: metadataPath, DestPath: "metadata.json"},
	}
	if err := e.archiveHandler.CreateArchive(ctx, sources, outputPath); err != nil {
		return nil, &errors.OperationError{Op: "create final archive", Err: err}
	}

	return &BackupResult{OutputPath: outputPath}, nil
}

func (e *DefaultBackupEngine) Restore(ctx context.Context, request RestoreRequest) (*RestoreResult, error) {
	return nil, errors.ErrNotImplemented
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

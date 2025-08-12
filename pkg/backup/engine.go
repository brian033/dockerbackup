package backup

import (
	"context"

	internalerrors "github.com/your-username/dockerbackup/internal/errors"
	"github.com/your-username/dockerbackup/internal/logger"
	"github.com/your-username/dockerbackup/pkg/archive"
	"github.com/your-username/dockerbackup/pkg/docker"
	"github.com/your-username/dockerbackup/pkg/filesystem"
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

func (e *DefaultBackupEngine) Backup(ctx context.Context, request BackupRequest) (*BackupResult, error) {
	return nil, internalerrors.ErrNotImplemented
}

func (e *DefaultBackupEngine) Restore(ctx context.Context, request RestoreRequest) (*RestoreResult, error) {
	return nil, internalerrors.ErrNotImplemented
}

func (e *DefaultBackupEngine) Validate(ctx context.Context, backupPath string) (*ValidationResult, error) {
	return nil, internalerrors.ErrNotImplemented
}

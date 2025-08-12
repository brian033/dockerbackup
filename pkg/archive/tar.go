package archive

import (
	"context"

	internalerrors "github.com/your-username/dockerbackup/internal/errors"
)

type ArchiveSource struct {
	// Path on the local filesystem
	Path string
	// DestPath is the relative path inside the archive; if empty, uses base name
	DestPath string
}

type ArchiveEntry struct {
	Path string
	Size int64
	Mode int64
	Type string
}

type ArchiveHandler interface {
	CreateArchive(ctx context.Context, sources []ArchiveSource, dest string) error
	ExtractArchive(ctx context.Context, archivePath, destDir string) error
	ListArchive(ctx context.Context, archivePath string) ([]ArchiveEntry, error)
}

type TarArchiveHandler struct{}

func NewTarArchiveHandler() *TarArchiveHandler {
	return &TarArchiveHandler{}
}

func (h *TarArchiveHandler) CreateArchive(ctx context.Context, sources []ArchiveSource, dest string) error {
	return internalerrors.ErrNotImplemented
}

func (h *TarArchiveHandler) ExtractArchive(ctx context.Context, archivePath, destDir string) error {
	return internalerrors.ErrNotImplemented
}

func (h *TarArchiveHandler) ListArchive(ctx context.Context, archivePath string) ([]ArchiveEntry, error) {
	return nil, internalerrors.ErrNotImplemented
}

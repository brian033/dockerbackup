package archive

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ArchiveSource describes a source path to include in an archive.
// If Path is a directory, its contents will be recursively included.
// If DestPath is provided, it is used as the root path (for directories)
// or the exact file path (for files) inside the archive.
// If DestPath is empty, the base name of Path is used.
 type ArchiveSource struct {
	Path     string
	DestPath string
}

// ArchiveEntry is a lightweight description returned by ListArchive.
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
	if len(sources) == 0 {
		return fmt.Errorf("no sources provided for archive creation")
	}
	if err := ensureParentDir(dest); err != nil {
		return err
	}

	outFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() { _ = outFile.Close() }()

	gzWriter := gzip.NewWriter(outFile)
	defer func() { _ = gzWriter.Close() }()

	tarWriter := tar.NewWriter(gzWriter)
	defer func() { _ = tarWriter.Close() }()

	for _, src := range sources {
		if err := h.addSourceToTar(ctx, tarWriter, src); err != nil {
			return err
		}
	}
	return nil
}

 func (h *TarArchiveHandler) addSourceToTar(ctx context.Context, tw *tar.Writer, src ArchiveSource) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	info, err := os.Lstat(src.Path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		rootName := src.DestPath
		if rootName == "" {
			rootName = filepath.Base(src.Path)
		}
		return filepath.WalkDir(src.Path, func(curr string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			// Compute the name inside the archive
			rel, err := filepath.Rel(src.Path, curr)
			if err != nil {
				return err
			}
			nameInTar := filepath.ToSlash(filepath.Join(rootName, rel))
			fi, err := os.Lstat(curr)
			if err != nil {
				return err
			}
			if fi.IsDir() {
				// Write a directory header to ensure empty dirs are preserved
				hdr, err := tar.FileInfoHeader(fi, "")
				if err != nil {
					return err
				}
				hdr.Name = nameInTar + "/"
				return tw.WriteHeader(hdr)
			}
			return writeFileOrSymlinkToTar(tw, curr, fi, nameInTar)
		})
	}
	// Single file
	nameInTar := src.DestPath
	if nameInTar == "" {
		nameInTar = filepath.Base(src.Path)
	}
	return writeFileOrSymlinkToTar(tw, src.Path, info, filepath.ToSlash(nameInTar))
}

 func writeFileOrSymlinkToTar(tw *tar.Writer, srcPath string, fi os.FileInfo, nameInTar string) error {
	if fi.Mode()&os.ModeSymlink != 0 {
		// Symlink: store as a symlink entry
		target, err := os.Readlink(srcPath)
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(fi, target)
		if err != nil {
			return err
		}
		hdr.Name = nameInTar
		return tw.WriteHeader(hdr)
	}
	// Regular file
	hdr, err := tar.FileInfoHeader(fi, "")
	if err != nil {
		return err
	}
	hdr.Name = nameInTar
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(tw, f)
	return err
}

 func (h *TarArchiveHandler) ExtractArchive(ctx context.Context, archivePath, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer func() { _ = gzReader.Close() }()

	tr := tar.NewReader(gzReader)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		destPath, err := secureJoin(destDir, hdr.Name)
		if err != nil {
			return fmt.Errorf("unsafe path %q in archive: %w", hdr.Name, err)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destPath, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeSymlink:
			// Create parent directory
			if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(hdr.Linkname, destPath); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode))
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
		default:
			// Skip other types for v0
		}
	}
	return nil
}

 func (h *TarArchiveHandler) ListArchive(ctx context.Context, archivePath string) ([]ArchiveEntry, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer func() { _ = gzReader.Close() }()

	tr := tar.NewReader(gzReader)
	var entries []ArchiveEntry
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		entries = append(entries, ArchiveEntry{
			Path: hdr.Name,
			Size: hdr.Size,
			Mode: hdr.Mode,
			Type: tarTypeToString(hdr.Typeflag),
		})
	}
	return entries, nil
}

 func ensureParentDir(path string) error {
	dir := filepath.Dir(path)
	return os.MkdirAll(dir, 0o755)
}

 func secureJoin(baseDir, name string) (string, error) {
	// Convert to forward slashes in tar
	cleanName := filepath.Clean(strings.TrimPrefix(name, "/"))
	if cleanName == "." || cleanName == "" {
		return baseDir, nil
	}
	joined := filepath.Join(baseDir, cleanName)
	// Ensure the resulting path is within baseDir
	rel, err := filepath.Rel(baseDir, joined)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path traversal detected")
	}
	return joined, nil
}

 func tarTypeToString(b byte) string {
	switch b {
	case tar.TypeDir:
		return "dir"
	case tar.TypeReg, tar.TypeRegA:
		return "file"
	case tar.TypeSymlink:
		return "symlink"
	default:
		return fmt.Sprintf("type_%d", b)
	}
}

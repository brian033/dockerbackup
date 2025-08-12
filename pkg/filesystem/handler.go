package filesystem

import (
	"io"
	"os"
	"path/filepath"
)

type Handler interface {
	EnsureDir(path string, perm os.FileMode) error
	CopyFile(src, dest string, perm os.FileMode) error
}

type OSHandler struct{}

func NewHandler() Handler {
	return &OSHandler{}
}

func (h *OSHandler) EnsureDir(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (h *OSHandler) CopyFile(src, dest string, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dest), perm); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

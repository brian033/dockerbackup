package archive

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestTarArchive_RoundTrip(t *testing.T) {
	ctx := context.Background()
	h := NewTarArchiveHandler()

	// Prepare sample directory with files
	srcDir := t.TempDir()
	nestedDir := filepath.Join(srcDir, "nested")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "root.txt"), []byte("root"), 0o644); err != nil {
		t.Fatalf("write root file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write nested file: %v", err)
	}

	archivePath := filepath.Join(t.TempDir(), "sample.tar.gz")
	if err := h.CreateArchive(ctx, []ArchiveSource{{Path: srcDir, DestPath: "sample"}}, archivePath); err != nil {
		t.Fatalf("CreateArchive failed: %v", err)
	}

	entries, err := h.ListArchive(ctx, archivePath)
	if err != nil {
		t.Fatalf("ListArchive failed: %v", err)
	}
	// Expect at least the directory and two files
	if len(entries) < 3 {
		t.Fatalf("expected at least 3 entries, got %d", len(entries))
	}

	destDir := t.TempDir()
	if err := h.ExtractArchive(ctx, archivePath, destDir); err != nil {
		t.Fatalf("ExtractArchive failed: %v", err)
	}

	// Verify files exist after extraction
	if _, err := os.Stat(filepath.Join(destDir, "sample", "root.txt")); err != nil {
		t.Fatalf("extracted root.txt missing: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(destDir, "sample", "nested", "file.txt"))
	if err != nil {
		t.Fatalf("extracted nested file missing: %v", err)
	}
	if string(b) != "hello" {
		t.Fatalf("unexpected nested file content: %q", string(b))
	}
}

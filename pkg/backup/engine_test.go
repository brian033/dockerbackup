package backup

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brian033/dockerbackup/internal/logger"
	"github.com/brian033/dockerbackup/pkg/archive"
	"github.com/brian033/dockerbackup/pkg/filesystem"
)

type fakeDockerClient struct {
	inspectJSON []byte
	exportErr   error
}

func (f *fakeDockerClient) InspectContainer(ctx context.Context, containerID string) ([]byte, error) {
	return f.inspectJSON, nil
}

func (f *fakeDockerClient) ExportContainerFilesystem(ctx context.Context, containerID string, destTarPath string) error {
	// create a tiny tar file via archive handler for simplicity
	h := archive.NewTarArchiveHandler()
	tmp := filepath.Join(filepath.Dir(destTarPath), "fs_src")
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tmp, "file.txt"), []byte("fs"), 0o644); err != nil {
		return err
	}
	return h.CreateArchive(ctx, []archive.ArchiveSource{{Path: tmp, DestPath: "."}}, destTarPath)
}

func (f *fakeDockerClient) ListVolumes(ctx context.Context) ([]string, error) { return nil, nil }

func TestDefaultBackupEngine_Backup_NoVolumes(t *testing.T) {
	ctx := context.Background()
	log := logger.New()
	arch := archive.NewTarArchiveHandler()
	fs := filesystem.NewHandler()

	inspect := []map[string]any{
		{
			"Id":     "123",
			"Name":   "/unit_test",
			"Mounts": []map[string]any{},
		},
	}
	b, _ := json.Marshal(inspect)
	dc := &fakeDockerClient{inspectJSON: b}

	engine := NewDefaultBackupEngine(arch, dc, fs, log)

	tdir := t.TempDir()
	out := filepath.Join(tdir, "out.tar.gz")
	res, err := engine.Backup(ctx, BackupRequest{
		TargetType:  TargetContainer,
		ContainerID: "unit_test",
		Options: BackupOptions{
			OutputPath: out,
		},
	})
	if err != nil {
		t.Fatalf("backup failed: %v", err)
	}
	if res == nil || res.OutputPath == "" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("output archive missing: %v", err)
	}

	// quick smoke: list entries
	entries, err := arch.ListArchive(ctx, out)
	if err != nil {
		t.Fatalf("list archive failed: %v", err)
	}
	paths := make([]string, 0, len(entries))
	for _, e := range entries {
		paths = append(paths, e.Path)
	}
	joined := strings.Join(paths, "\n")
	for _, must := range []string{"container.json", "filesystem.tar", "metadata.json"} {
		if !strings.Contains(joined, must) {
			t.Fatalf("expected %s in archive entries", must)
		}
	}
}

func TestDefaultBackupEngine_Backup_WithVolume(t *testing.T) {
	ctx := context.Background()
	log := logger.New()
	arch := archive.NewTarArchiveHandler()
	fs := filesystem.NewHandler()

	// fake inspect with one volume mount using a real temporary dir as source
	volSrc := t.TempDir()
	if err := os.WriteFile(filepath.Join(volSrc, "vol.txt"), []byte("data"), 0o644); err != nil {
		t.Fatalf("write vol file: %v", err)
	}
	inspect := []map[string]any{
		{
			"Id":   "123",
			"Name": "/unit_test",
			"Mounts": []map[string]any{
				{"Name": "myvol", "Source": volSrc, "Destination": "/data", "Type": "volume", "RW": true},
			},
		},
	}
	b, _ := json.Marshal(inspect)
	dc := &fakeDockerClient{inspectJSON: b}

	engine := NewDefaultBackupEngine(arch, dc, fs, log)

	out := filepath.Join(t.TempDir(), "out.tar.gz")
	_, err := engine.Backup(ctx, BackupRequest{
		TargetType:  TargetContainer,
		ContainerID: "unit_test",
		Options: BackupOptions{OutputPath: out},
	})
	if err != nil {
		t.Fatalf("backup failed: %v", err)
	}

	entries, err := arch.ListArchive(ctx, out)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	var foundVol bool
	for _, e := range entries {
		if strings.HasPrefix(e.Path, "volumes/") && strings.HasSuffix(e.Path, ".tar.gz") {
			foundVol = true
			break
		}
	}
	if !foundVol {
		t.Fatalf("expected a volume archive under volumes/")
	}
}

func TestDefaultBackupEngine_Validate(t *testing.T) {
	ctx := context.Background()
	arch := archive.NewTarArchiveHandler()

	// valid archive
	work := t.TempDir()
	mustWrite := func(p string) { if err := os.WriteFile(p, []byte("x"), 0o644); err != nil { t.Fatalf("write %s: %v", p, err) } }
	mustWrite(filepath.Join(work, "container.json"))
	mustWrite(filepath.Join(work, "filesystem.tar"))
	mustWrite(filepath.Join(work, "metadata.json"))
	good := filepath.Join(t.TempDir(), "good.tar.gz")
	if err := arch.CreateArchive(ctx, []archive.ArchiveSource{{Path: work, DestPath: "."}}, good); err != nil {
		t.Fatalf("create good archive: %v", err)
	}

	engine := NewDefaultBackupEngine(arch, nil, filesystem.NewHandler(), logger.New())
	res, err := engine.Validate(ctx, good)
	if err != nil || res == nil || !res.Valid {
		t.Fatalf("expected valid, got %+v, err=%v", res, err)
	}

	// invalid archive (missing metadata)
	work2 := t.TempDir()
	mustWrite(filepath.Join(work2, "container.json"))
	mustWrite(filepath.Join(work2, "filesystem.tar"))
	bad := filepath.Join(t.TempDir(), "bad.tar.gz")
	if err := arch.CreateArchive(ctx, []archive.ArchiveSource{{Path: work2, DestPath: "."}}, bad); err != nil {
		t.Fatalf("create bad archive: %v", err)
	}
	res2, err := engine.Validate(ctx, bad)
	if err != nil {
		t.Fatalf("validate returned error: %v", err)
	}
	if res2 == nil || res2.Valid {
		t.Fatalf("expected invalid validation result")
	}
}

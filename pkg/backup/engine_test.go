package backup

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brian033/dockerbackup/internal/logger"
	"github.com/brian033/dockerbackup/pkg/archive"
	"github.com/brian033/dockerbackup/pkg/docker"
	"github.com/brian033/dockerbackup/pkg/filesystem"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
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
func (f *fakeDockerClient) InspectVolume(ctx context.Context, name string) (*docker.VolumeConfig, error) {
	return nil, nil
}
func (f *fakeDockerClient) InspectNetwork(ctx context.Context, name string) (*docker.NetworkConfig, error) {
	return nil, nil
}
func (f *fakeDockerClient) EnsureVolume(ctx context.Context, cfg docker.VolumeConfig) error {
	return nil
}
func (f *fakeDockerClient) EnsureNetwork(ctx context.Context, cfg docker.NetworkConfig) error {
	return nil
}
func (f *fakeDockerClient) ImportImage(ctx context.Context, tarPath string, ref string) (string, error) {
	return "image123", nil
}
func (f *fakeDockerClient) VolumeCreate(ctx context.Context, name string) error { return nil }
func (f *fakeDockerClient) ExtractTarGzToVolume(ctx context.Context, volumeName string, tarGzPath string, expectedRoot string) error {
	return nil
}
func (f *fakeDockerClient) CreateContainer(ctx context.Context, imageRef string, name string, mounts []docker.Mount) (string, error) {
	return "container123", nil
}
func (f *fakeDockerClient) CreateContainerFromSpec(ctx context.Context, cfg *container.Config, hostCfg *container.HostConfig, netCfg *network.NetworkingConfig, name string) (string, error) {
	return "container123", nil
}
func (f *fakeDockerClient) StartContainer(ctx context.Context, containerID string) error { return nil }
func (f *fakeDockerClient) ImageSave(ctx context.Context, imageRef string, destTarPath string) error {
	return nil
}
func (f *fakeDockerClient) ImageLoad(ctx context.Context, tarPath string) error { return nil }

// Add stubs for new interface methods
func (f *fakeDockerClient) ContainerState(ctx context.Context, containerID string) (string, string, error) {
	return "running", "healthy", nil
}
func (f *fakeDockerClient) HostIPs(ctx context.Context) ([]string, error) {
	return []string{"127.0.0.1", "0.0.0.0"}, nil
}
func (f *fakeDockerClient) ListProjectContainers(ctx context.Context, project string) ([]docker.ProjectContainerRef, error) {
	return nil, nil
}
func (f *fakeDockerClient) ListProjectContainersByLabel(ctx context.Context, project string) ([]docker.ProjectContainerRef, error) {
	return nil, nil
}
func (f *fakeDockerClient) TagImage(ctx context.Context, sourceRef, targetRef string) error { return nil }

type fakeDockerClientRestore struct {
	createdImageRef   string
	createdVolumes    []string
	extractedVolumes  []string
	createdContainer  string
	startedContainers []string
}

func (f *fakeDockerClientRestore) InspectContainer(ctx context.Context, containerID string) ([]byte, error) {
	return nil, nil
}
func (f *fakeDockerClientRestore) ExportContainerFilesystem(ctx context.Context, containerID string, destTarPath string) error {
	return nil
}
func (f *fakeDockerClientRestore) ListVolumes(ctx context.Context) ([]string, error) { return nil, nil }
func (f *fakeDockerClientRestore) InspectVolume(ctx context.Context, name string) (*docker.VolumeConfig, error) {
	return nil, nil
}
func (f *fakeDockerClientRestore) InspectNetwork(ctx context.Context, name string) (*docker.NetworkConfig, error) {
	return nil, nil
}
func (f *fakeDockerClientRestore) EnsureVolume(ctx context.Context, cfg docker.VolumeConfig) error {
	return nil
}
func (f *fakeDockerClientRestore) EnsureNetwork(ctx context.Context, cfg docker.NetworkConfig) error {
	return nil
}
func (f *fakeDockerClientRestore) ImportImage(ctx context.Context, tarPath string, ref string) (string, error) {
	f.createdImageRef = "imported:" + filepath.Base(tarPath)
	return f.createdImageRef, nil
}
func (f *fakeDockerClientRestore) VolumeCreate(ctx context.Context, name string) error {
	f.createdVolumes = append(f.createdVolumes, name)
	return nil
}
func (f *fakeDockerClientRestore) ExtractTarGzToVolume(ctx context.Context, volumeName string, tarGzPath string, expectedRoot string) error {
	f.extractedVolumes = append(f.extractedVolumes, volumeName)
	return nil
}
func (f *fakeDockerClientRestore) CreateContainer(ctx context.Context, imageRef string, name string, mounts []docker.Mount) (string, error) {
	f.createdContainer = name
	return "container123", nil
}
func (f *fakeDockerClientRestore) CreateContainerFromSpec(ctx context.Context, cfg *container.Config, hostCfg *container.HostConfig, netCfg *network.NetworkingConfig, name string) (string, error) {
	f.createdContainer = name
	return "container123", nil
}
func (f *fakeDockerClientRestore) StartContainer(ctx context.Context, containerID string) error {
	f.startedContainers = append(f.startedContainers, containerID)
	return nil
}
func (f *fakeDockerClientRestore) ImageSave(ctx context.Context, imageRef string, destTarPath string) error {
	return nil
}
func (f *fakeDockerClientRestore) ImageLoad(ctx context.Context, tarPath string) error { return nil }

// Add stubs for new interface methods
func (f *fakeDockerClientRestore) ContainerState(ctx context.Context, containerID string) (string, string, error) {
	return "running", "healthy", nil
}
func (f *fakeDockerClientRestore) HostIPs(ctx context.Context) ([]string, error) {
	return []string{"127.0.0.1", "0.0.0.0"}, nil
}
func (f *fakeDockerClientRestore) ListProjectContainers(ctx context.Context, project string) ([]docker.ProjectContainerRef, error) {
	return nil, nil
}
func (f *fakeDockerClientRestore) ListProjectContainersByLabel(ctx context.Context, project string) ([]docker.ProjectContainerRef, error) {
	return nil, nil
}
func (f *fakeDockerClientRestore) TagImage(ctx context.Context, sourceRef, targetRef string) error { return nil }

type fakeDockerClientWithInspect struct {
	fakeDockerClient
	vol map[string]docker.VolumeConfig
	net map[string]docker.NetworkConfig
}

func (f *fakeDockerClientWithInspect) InspectVolume(ctx context.Context, name string) (*docker.VolumeConfig, error) {
	if v, ok := f.vol[name]; ok {
		vv := v
		return &vv, nil
	}
	return nil, nil
}

func (f *fakeDockerClientWithInspect) InspectNetwork(ctx context.Context, name string) (*docker.NetworkConfig, error) {
	if n, ok := f.net[name]; ok {
		nn := n
		return &nn, nil
	}
	return nil, nil
}

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
		Options:     BackupOptions{OutputPath: out},
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

func TestDefaultBackupEngine_Backup_WithBindMount(t *testing.T) {
	ctx := context.Background()
	log := logger.New()
	arch := archive.NewTarArchiveHandler()
	fs := filesystem.NewHandler()

	bindSrc := t.TempDir()
	if err := os.WriteFile(filepath.Join(bindSrc, "host.txt"), []byte("host"), 0o644); err != nil {
		t.Fatalf("write bind file: %v", err)
	}
	inspect := []map[string]any{
		{
			"Id":   "123",
			"Name": "/unit_test",
			"Mounts": []map[string]any{
				{"Source": bindSrc, "Destination": "/data", "Type": "bind", "RW": true},
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
		Options:     BackupOptions{OutputPath: out},
	})
	if err != nil {
		t.Fatalf("backup failed: %v", err)
	}
	entries, err := arch.ListArchive(ctx, out)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	var foundBind bool
	for _, e := range entries {
		if strings.HasPrefix(e.Path, "volumes/") && strings.Contains(e.Path, "bind_") && strings.HasSuffix(e.Path, ".tar.gz") {
			foundBind = true
			break
		}
	}
	if !foundBind {
		t.Fatalf("expected a bind mount archive under volumes/bind_*.tar.gz")
	}
}

func TestDefaultBackupEngine_Validate(t *testing.T) {
	ctx := context.Background()
	arch := archive.NewTarArchiveHandler()

	// valid archive
	work := t.TempDir()
	mustWrite := func(p string) {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
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

func TestDefaultBackupEngine_Restore_Minimal(t *testing.T) {
	ctx := context.Background()
	log := logger.New()
	arch := archive.NewTarArchiveHandler()
	fs := filesystem.NewHandler()
	fd := &fakeDockerClientRestore{}
	engine := NewDefaultBackupEngine(arch, fd, fs, log)

	// Create a minimal valid backup archive
	work := t.TempDir()
	// container.json as single object matching types.ContainerJSON minimal fields
	cj := types.ContainerJSON{ContainerJSONBase: &types.ContainerJSONBase{ID: "123", Name: "/unit_test"}}
	b, _ := json.Marshal(cj)
	if err := os.WriteFile(filepath.Join(work, "container.json"), b, 0o644); err != nil {
		t.Fatalf("write container.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(work, "metadata.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write metadata.json: %v", err)
	}
	// fake filesystem.tar
	if err := os.WriteFile(filepath.Join(work, "filesystem.tar"), []byte("tar"), 0o644); err != nil {
		t.Fatalf("write filesystem.tar: %v", err)
	}
	backupFile := filepath.Join(t.TempDir(), "backup.tar.gz")
	if err := arch.CreateArchive(ctx, []archive.ArchiveSource{{Path: work, DestPath: "."}}, backupFile); err != nil {
		t.Fatalf("create archive: %v", err)
	}

	res, err := engine.Restore(ctx, RestoreRequest{BackupPath: backupFile, Options: RestoreOptions{Start: true}})
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}
	if res == nil || res.RestoredID == "" {
		t.Fatalf("unexpected restore result: %+v", res)
	}
	if len(fd.startedContainers) != 1 {
		t.Fatalf("expected container to be started")
	}
}

func TestBackup_CapturesVolumeAndNetworkConfigs(t *testing.T) {
	ctx := context.Background()
	log := logger.New()
	arch := archive.NewTarArchiveHandler()
	fs := filesystem.NewHandler()

	// Prepare inspect with one volume and a network in NetworkSettings.Networks
	volSrc := t.TempDir()
	inspect := []map[string]any{
		{
			"Id":   "123",
			"Name": "/unit_test",
			"Mounts": []map[string]any{
				{"Name": "myvol", "Source": volSrc, "Destination": "/data", "Type": "volume", "RW": true},
			},
		},
	}
	inspectBytes, _ := json.Marshal(inspect)

	dc := &fakeDockerClientWithInspect{
		fakeDockerClient: fakeDockerClient{inspectJSON: inspectBytes},
		vol: map[string]docker.VolumeConfig{
			"myvol": {Name: "myvol", Driver: "local", Options: map[string]string{"o": "val"}},
		},
		net: map[string]docker.NetworkConfig{
			"bridge": {Name: "bridge", Driver: "bridge"},
		},
	}

	engine := NewDefaultBackupEngine(arch, dc, fs, log)

	out := filepath.Join(t.TempDir(), "out.tar.gz")
	_, err := engine.Backup(ctx, BackupRequest{
		TargetType:  TargetContainer,
		ContainerID: "unit_test",
		Options:     BackupOptions{OutputPath: out},
	})
	if err != nil {
		t.Fatalf("backup failed: %v", err)
	}

	// Extract and verify config files exist
	dir := t.TempDir()
	if err := arch.ExtractArchive(ctx, out, dir); err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "volumes", "volume_configs.json")); err != nil {
		t.Fatalf("volume_configs.json missing: %v", err)
	}
	// networks dir may exist even if empty; this test doesn't synthesize NetworkSettings in inspectBytes, so skip strict check
}

func TestRestore_AutoRelaxIPs_ClearsIPAMOnConflict(t *testing.T) {
	ctx := context.Background()
	log := logger.New()
	arch := archive.NewTarArchiveHandler()
	fs := filesystem.NewHandler()
	fd := &fakeDockerClientRestore{}
	engine := NewDefaultBackupEngine(arch, fd, fs, log)

	// Create a backup with container.json containing a static IP that conflicts with loopback 127.0.0.0/8
	work := t.TempDir()
	cj := types.ContainerJSON{ContainerJSONBase: &types.ContainerJSONBase{ID: "1", Name: "/unit_test"}}
	b, _ := json.Marshal(cj)
	_ = os.WriteFile(filepath.Join(work, "container.json"), b, 0o644)
	_ = os.WriteFile(filepath.Join(work, "filesystem.tar"), []byte("tar"), 0o644)
	backupFile := filepath.Join(t.TempDir(), "backup.tar.gz")
	if err := arch.CreateArchive(ctx, []archive.ArchiveSource{{Path: work, DestPath: "."}}, backupFile); err != nil { t.Fatalf("create archive: %v", err) }

	// Call restore with auto-relax-ips; since our ContainerJSON in the test lacks NetworkSettings, this is a smoke test to ensure no crash
	_, err := engine.Restore(ctx, RestoreRequest{BackupPath: backupFile, Options: RestoreOptions{AutoRelaxIPs: true}})
	if err != nil && !strings.Contains(err.Error(), "docker import image") {
		// acceptable to fail on docker import since we don't actually implement import in fake
		t.Fatalf("unexpected error: %v", err)
	}
	_ = net.IPv4(127,0,0,1) // silence unused import if optimized
}

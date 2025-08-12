# DockerBackup Implementation Plan (v0)

This plan outlines concrete steps to implement the first functional version focusing on:

1. `archive.TarArchiveHandler`
2. `docker.CLIClient`
3. `backup.DefaultBackupEngine` workflows (container backup/restore)

---

## Goals and Scope (v0)

- Implement archive creation/extraction/listing using the Go standard library.
- Implement Docker client via `docker` CLI calls using `os/exec`.
- Implement the container backup and restore workflows end-to-end to produce and consume the backup file structure in `README.md`.
- Keep compose-specific functionality stubbed; compose support to follow after v0.

Out of scope for v0:

- Parallelization/streaming optimizations
- Advanced permissions/ownership mapping across OSes
- Metrics/observability beyond basic logging
- Full compose project workflows

---

## Milestones

- M1: Tar archive handler implemented with unit tests
- M2: Docker CLI client implemented with light integration tests
- M3: Container backup workflow (happy path) implemented and tested
- M4: Container restore workflow (happy path) implemented and tested

---

## M1: Implement `archive.TarArchiveHandler`

### Target files

- `pkg/archive/tar.go` (implement methods)
- (new) `pkg/archive/validator.go` (optional later)

### Requirements

- Create: Build `.tar.gz` from a list of sources, preserving file modes and directory structure.
- Extract: Extract `.tar.gz` into a destination directory safely (no path traversal).
- List: List archive entries (path, size, mode, type) without extracting.
- Respect `context.Context` for cancellation; abort long operations if context is done.

### Design

- Use standard libs: `archive/tar`, `compress/gzip`, `io`, `os`, `path/filepath`.
- For creation, walk each source path with `filepath.WalkDir` and write headers + file content into a tar writer, wrapped by a gzip writer.
- For extraction, validate and normalize entry paths to avoid writing outside `destDir` (protect against `..` traversal).
- For listing, open archive, iterate tar headers, collect metadata only.
- Handle symlinks by writing headers with link target; extraction should recreate symlinks when safe. Optionally skip for v0 if needed.

### Edge cases

- Empty directories
- Long file names and deep paths
- Permissions on non-Unix systems (best-effort)
- Interruptions via context cancellation

### Acceptance Criteria

- Round-trip test: create → list → extract and compare file tree (size, file count, modes where applicable)
- Handles multiple sources and nested directories
- No path traversal possible during extract (adds defense-in-depth checks)

### Checklist

- [x] Implement `CreateArchive(ctx, sources, dest)`
- [x] Implement `ExtractArchive(ctx, archivePath, destDir)`
- [x] Implement `ListArchive(ctx, archivePath)`
- [x] Add unit tests under `pkg/archive` for create/list/extract

---

## M2: Implement `docker.CLIClient`

### Target files

- `pkg/docker/client.go` (implement methods)
- (new later if needed) `pkg/docker/types.go` for typed structs (e.g., `ContainerInfo`, `Mount`)

### Required methods (v0)

- `InspectContainer(ctx, containerID) ([]byte, error)`
- `ExportContainerFilesystem(ctx, containerID, destTarPath) error`
- `ListVolumes(ctx) ([]string, error)`

### Likely additions (to simplify engine logic)

- Parse inspect JSON into a typed struct:
  - `ContainerInfo` with `Name`, `ID`, `Config`, and `Mounts []Mount{ Name, Source, Destination, Type, ReadOnly }`
- Expose a helper: `ParseContainerInfo(inspectJSON []byte) (ContainerInfo, error)`

### Checklist

- [x] Implement `InspectContainer`
- [x] Implement `ExportContainerFilesystem`
- [x] Implement `ListVolumes`
- [x] Add `ParseContainerInfo` helper and typed structs
- [x] Add lightweight integration tests guarded by env var (optional)

---

## M3: Implement `backup.DefaultBackupEngine` (Container Backup)

- [x] Inspect container, export filesystem, archive volumes/binds, write metadata
- [x] Package final archive; include `container.json`, `filesystem.tar`, `volumes/`, `metadata.json`
- [x] Capture `volumes/volume_configs.json`, `networks/network_configs.json`
- [x] Optional: save `image.tar` via `docker save`
- [x] Parallelize archiving of volumes/binds (partial)

---

## M4: Implement `backup.DefaultBackupEngine` (Container Restore)

- [x] Prefer `docker load image.tar`; fallback to `docker import filesystem.tar`
- [x] Ensure networks/volumes via Docker SDK (driver/options/IPAM)
- [x] Portability: `--network-map`, `--parent-map`, `--drop-host-ips`, `--reassign-ips`, `--fallback-bridge`
- [x] Validate HostIp presence, skip missing IP bindings (or drop when flag set)
- [x] Wait for healthy: `--wait-healthy`, `--wait-timeout`
- [x] Bind restore root (planned)
- [x] Replace existing container name (planned)

---

## New Work (v1)

### Compose Support

- Backup:
  - [ ] Discover compose project containers by label `com.docker.compose.project`
  - [ ] Backup per-service like single-container backups
  - [x] Include compose files (`docker-compose*.yml|yaml`, `.env`) from project path
- Restore:
  - [ ] Recreate project networks/volumes first
  - [ ] Restore services honoring dependencies/order

### Validate / Dry run

- [x] Add `dry-run-restore` subcommand (plan-only)
- [ ] Enhance diff details: ports, nets, volumes, mounts, env, mapping preview
- [ ] Add `validate` CLI wrapping `engine.Validate`

### Portability / Safety

- [x] Auto-skip waiting when no HEALTHCHECK (planned)
- [ ] `--replace` to remove existing container with same name
- [x] `--bind-restore-root` to relocate missing bind mounts (planned)
- [ ] Safe mode to drop host-specific settings: devices, caps, seccomp/apparmor

### Archiving / Engine

- [ ] Honor compression level in gzip writer (`archive`)
- [x] Parallelize per-volume/bind archiving (partial)
- [ ] Better xattrs/ACL/hardlinks (optional)

### Image fidelity

- [x] Save/load original image tar when possible
- [ ] Retag original refs after load

### Networks / Ports

- [x] Validate HostIp presence; skip or drop
- [ ] Detect/resolve subnet conflicts or relax static IPs automatically

### Tests / CI

- [ ] Add integration tests (opt-in)
- [ ] Add GitHub Actions workflow for `go test` matrix

### Docs / CLI

- [ ] Update README for new flags and workflows
- [ ] Add verbose/dry-run detail levels

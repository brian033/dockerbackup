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

- [ ] Implement `CreateArchive(ctx, sources, dest)`
- [ ] Implement `ExtractArchive(ctx, archivePath, destDir)`
- [ ] Implement `ListArchive(ctx, archivePath)`
- [ ] Add unit tests under `pkg/archive` for create/list/extract

---

## M2: Implement `docker.CLIClient`

### Target files

- `pkg/docker/client.go` (implement methods)
- (new later if needed) `pkg/docker/types.go` for typed structs (e.g., `ContainerInfo`, `Mount`)

### Required methods (v0)

- `InspectContainer(ctx, containerID) ([]byte, error)`
  - `docker inspect <id>`; return raw JSON bytes
- `ExportContainerFilesystem(ctx, containerID, destTarPath) error`
  - `docker export <id> -o <dest>` (or stream to file)
- `ListVolumes(ctx) ([]string, error)`
  - `docker volume ls --format '{{.Name}}'`

### Likely additions (to simplify engine logic)

- Parse inspect JSON into a typed struct:
  - `ContainerInfo` with `Name`, `ID`, `Config`, and `Mounts []Mount{ Name, Source, Destination, Type, ReadOnly }`
- Expose a helper: `ParseContainerInfo(inspectJSON []byte) (ContainerInfo, error)`

### Design notes

- Use `exec.CommandContext` to ensure commands respect context cancellation.
- Capture stderr; wrap errors with command + stderr output for debuggability.
- Avoid logging secrets. Log command invoked and runtime, not full outputs.

### Acceptance Criteria

- Commands terminate when context is cancelled
- `InspectContainer` and `ExportContainerFilesystem` work against a local Docker daemon
- `ListVolumes` returns non-empty list on systems with volumes

### Checklist

- [ ] Implement `InspectContainer`
- [ ] Implement `ExportContainerFilesystem`
- [ ] Implement `ListVolumes`
- [ ] Add `ParseContainerInfo` helper and typed structs
- [ ] Add lightweight integration tests guarded by env var (e.g., `DOCKER_INTEGRATION=1`)

---

## M3: Implement `backup.DefaultBackupEngine` (Container Backup)

### Target files

- `pkg/backup/engine.go`
- `pkg/docker/types.go` (if not created in M2)

### Workflow (aligned with README)

1. Validate container ID exists (via `InspectContainer`).
2. Create working directory structure:
   - `work/<container>/container.json`
   - `work/<container>/filesystem.tar`
   - `work/<container>/volumes/`
   - `work/<container>/metadata.json`
3. Write `container.json` (raw inspect JSON)
4. Export container filesystem to `filesystem.tar`
5. Discover mounted volumes from parsed inspect data
6. For each named volume mount:
   - Create `volumes/<volumeName>.tar.gz`
   - Use `archive.CreateArchive` targeting the mount source directory
7. Build final backup archive at `Options.OutputPath` (default `<container>_backup.tar.gz`):
   - Use `archive.CreateArchive` on `container.json`, `filesystem.tar`, `volumes/`, and `metadata.json`
8. Cleanup working directory (best-effort)

### Metadata

- `metadata.json` fields (v0):
  - `version` (e.g., `1`)
  - `createdAt` (RFC3339)
  - `containerID`, `containerName`
  - `engine` (e.g., `default`)

### Error handling

- Wrap operations with typed errors (`OperationError`)
- Best-effort cleanup on failure
- Log progress and key steps (use `logger`)

### Acceptance Criteria

- Produces archive structure matching README
- Re-run safe: output path conflict yields error unless `--output` overridden
- Works for containers with and without volumes

### Checklist

- [ ] Define metadata struct + write helper
- [ ] Implement backup happy path
- [ ] Add unit tests with mocked `docker.DockerClient` and real `archive` implementation (using temp FS)

---

## M4: Implement `backup.DefaultBackupEngine` (Container Restore)

### Workflow (aligned with README)

1. Extract backup archive to a temp directory
2. Read `container.json` and parse into `ContainerInfo`
3. Create image from `filesystem.tar` via `docker import` (may be added to `docker.CLIClient` later)
4. Recreate named volumes and restore content from `volumes/*.tar.gz`
5. Create container using original config from inspect, applying overrides:
   - New container name if provided
6. Optionally start container when `--start` is set

### Acceptance Criteria

- Restores a simple container (no complex capabilities) end-to-end
- Honors `--name` and `--start`

### Checklist

- [ ] Add `docker import` support to `docker.CLIClient`
- [ ] Implement restore happy path
- [ ] Add unit tests with mocks and limited integration test (guarded by `DOCKER_INTEGRATION`)

---

## Testing Strategy

- Unit tests for archive (no Docker dependency)
- Mock-based tests for engine logic
- Optional integration tests requiring local Docker (opt-in via env var)

---

## Follow-ups (post v0)

- Compose project workflows (backup/restore)
- Parallelization of volume archiving
- Streaming to reduce disk usage during packaging
- Rich validation (`Validate` API)
- Progress reporting and metrics
- Robust permission/owner mapping across OSes

---

## Ready-to-Implement TODOs (short list)

- [ ] `pkg/archive/tar.go`: fill in `CreateArchive`, `ExtractArchive`, `ListArchive`
- [ ] `pkg/docker/client.go`: implement `InspectContainer`, `ExportContainerFilesystem`, `ListVolumes`
- [ ] `pkg/docker/types.go`: add `ContainerInfo`, `Mount`, and `ParseContainerInfo`
- [ ] `pkg/backup/engine.go`: implement backup (container) happy path
- [ ] Tests: `pkg/archive/*_test.go`, `pkg/backup/*_test.go`

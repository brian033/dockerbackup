# DockerBackup

A CLI tool written in Go that can backup running Docker containers and restore them on another machine.

## Features

- Backup running or stopped Docker containers
- Backup entire Docker Compose projects
- Include container filesystem, configuration, and volume data
- Generate portable compressed backup files
- Support cross-machine container restoration

## Installation

```bash
go install github.com/brian033/dockerbackup@latest
```

Or build from source:

```bash
git clone https://github.com/brian033/dockerbackup
cd dockerbackup
go build -o dockerbackup
```

## Usage

### Backup Container

```bash
# Backup specified container
dockerbackup backup <container_name_or_id> [options]

# Examples
dockerbackup backup my-app
dockerbackup backup a1b2c3d4e5f6
```

#### Backup Options

- `--output, -o`: Specify output file path (default: `<container_name>_backup.tar.gz`)
- `--compress, -c`: Compression level (1-9, default: 6)

### Restore Container

```bash
# Restore container from backup file
dockerbackup restore <backup_file> [options]

# Example
dockerbackup restore my-app_backup.tar.gz
```

#### Restore Options (portability and safety)

- `--name, -n`: Specify new container name (default: original container name)
- `--start`: Start container immediately after restore
- `--wait-healthy`: Wait for HEALTHCHECK to report healthy (auto-skips if no healthcheck)
- `--wait-timeout <seconds>`: Max seconds to wait with `--wait-healthy` (default ~120)
- `--replace`: Stop/remove existing container with the same name before restoring
- `--bind-restore-root <path>`: If a bind mount source path doesn't exist on the host, restore it under `<path>/<basename>`
- `--network-map old:new`: Map network names from backup to target (repeatable)
- `--parent-map net:parentIf`: Override macvlan/ipvlan parent interface per network (repeatable)
- `--fallback-bridge`: If macvlan/ipvlan parent isn't available, use the bridge driver
- `--drop-host-ips`: Ignore HostIp in port bindings if that IP isn't present on the host (bind to all interfaces)
- `--reassign-ips`: Ignore saved static container IPs and let Docker assign dynamically
- `--auto-relax-ips`: If a static IPv4 conflicts with a host subnet, automatically drop the static IP so Docker assigns
- `--force-bind-ip <ip>`: Force all port bindings to use a specific host IP
- `--bind-interface <name>`: Prefer this interface's primary IPv4 for port bindings when HostIp is missing
- Safe mode:
  - `--drop-devices`: Drop `HostConfig.Devices`
  - `--drop-caps`: Drop `CapAdd/CapDrop`
  - `--drop-seccomp`: Drop `SecurityOpt` seccomp profile
  - `--drop-apparmor`: Drop `SecurityOpt` apparmor profile

### Backup Docker Compose Project

```bash
# Backup entire compose project
dockerbackup backup-compose [project_path] [options]

# Examples
dockerbackup backup-compose .                    # Backup current directory
dockerbackup backup-compose /path/to/project    # Backup specific project
```

#### Compose Backup Options

- `--output, -o`: Specify output file path (default: `<project_name>_compose_backup.tar.gz`)
- `--project-name, -p`: Override project name detection

### Restore Docker Compose Project

```bash
# Restore compose project from backup
dockerbackup restore-compose <backup_file> [options]

# Example
dockerbackup restore-compose my-project_compose_backup.tar.gz
```

#### Compose Restore Options

- Inherits all container restore portability/safety options (applied per service)
- Starts services in dependency order (from `depends_on` when present)

### Validate and Dry-Run

```bash
# Validate a backup archive structure
dockerbackup validate <backup_file>

# Show a plan of what would be restored (no changes)
dockerbackup dry-run-restore <backup_file>
```

#### Dry-run detail levels

- **Basic (default)**: plan + summary counts extracted from `container.json` and a list of volume archives.

  - Shows number of env vars, port bindings, mounts, attached networks.
  - Example:

  ```bash
  dockerbackup dry-run-restore /tmp/my_backup.tar.gz
  ```

- **Verbose**: enable verbose logs for more context (steps, file paths) via environment variable:

  ```bash
  DOCKERBACKUP_DEBUG=1 dockerbackup dry-run-restore /tmp/my_backup.tar.gz
  ```

  This will include additional INFO logs for extraction and planning steps. Future versions may add a `--diff` mode to print full mapping previews (ports/networks/mounts/env) line-by-line.

## Single Container Backup Process

1. **Container Check**: Verify container exists and is accessible
2. **Configuration Backup**: Save complete ContainerJSON configuration
3. **Filesystem Backup**: Export container filesystem using `docker export`
4. **Volume Backup**: Backup all mounted volume data
5. **Package**: Compress all data into tar.gz file

## Single Container Restore Process

1. **Extract Backup**: Decompress backup file
2. **Load Filesystem**: Prefer `docker load image.tar`, fallback to `docker import filesystem.tar`
3. **Restore Volumes**: Recreate volumes and data
4. **Create Container**: Create new container based on original configuration and portability/safety flags
5. **Start Container**: (Optional) Start the restored container and optionally wait for healthy

## Compose Project Backup Process

1. **Project Discovery**: Parse `docker-compose.yml` and identify services
2. **Per-Service Backup**: Backup each service container individually
3. **Networks/Volumes**: Capture network/volume configs
4. **Project Files**: Backup compose files and `.env`
5. **Package**: Compress into tar.gz

## Compose Project Restore Process

1. **Extract Backup**
2. **Networks & Volumes**: Ensure project networks and volumes exist (driver/options/IPAM)
3. **Per-Service Restore**: Restore each service container; apply portability/safety flags
4. **Order & Startup**: Start services in dependency order (from `depends_on`)

## Backup File Structure

### Single Container Backup

```
container_backup.tar.gz
├── container.json          # Complete container configuration
├── filesystem.tar          # Container filesystem (docker export)
├── volumes/                # Volume data
│   ├── volume1.tar.gz
│   └── volume2.tar.gz
├── networks/               # Network configs (optional)
│   └── network_configs.json
├── image.tar               # Original image (optional)
└── metadata.json          # Backup information and version
```

### Compose Project Backup

```
project_compose_backup.tar.gz
├── compose-files/          # Project configuration files
│   ├── docker-compose.yml
│   ├── .env
│   ├── docker-compose.override.yml
│   └── other-configs/
├── containers/             # Per-service container backups
│   ├── service1/
│   │   └── container.tar.gz
│   └── service2/
│       └── ...
├── networks/               # Network configurations
│   └── network_configs.json
├── volumes/                # Volume configurations
│   └── volume_configs.json
└── metadata.json          # Project backup information
```

## Requirements

- Go 1.19+
- Docker Engine API v1.41+
- Sufficient disk space for backup files

## Notes

- Backing up large containers may take considerable time
- Ensure sufficient disk space is available
- Volume data will be completely copied, mind file permissions
- Network settings may need adjustment in different environments

## Development

```bash
# Run tests
go test ./...

# Build
go build -o dockerbackup

# Run
./dockerbackup --help
```

## Verbose Logs

Set `DOCKERBACKUP_DEBUG=1` to enable verbose logs across commands (including dry-run) for more detail.

## Contributing

Issues and Pull Requests are welcome.

## License

MIT License

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

#### Restore Options
- `--name, -n`: Specify new container name (default: original container name)
- `--start`: Start container immediately after restore

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
- `--project-name, -p`: Specify new project name (default: original project name)
- `--start`: Start all services immediately after restore

### List Backup Contents

```bash
# View backup file contents
dockerbackup list <backup_file>
```

## Single Container Backup Process

1. **Container Check**: Verify container exists and is accessible
2. **Configuration Backup**: Save complete ContainerJSON configuration
3. **Filesystem Backup**: Export container filesystem using `docker export`
4. **Volume Backup**: Backup all mounted volume data
5. **Package**: Compress all data into tar.gz file

## Single Container Restore Process

1. **Extract Backup**: Decompress backup file
2. **Load Filesystem**: Create new image using `docker import`
3. **Restore Volumes**: Recreate volumes and data
4. **Create Container**: Create new container based on original configuration
5. **Start Container**: (Optional) Start the restored container

## Compose Project Backup Process

1. **Project Discovery**: Parse `docker-compose.yml` and identify all services
2. **Container Mapping**: Map services to running containers
3. **Shared Resources**: Identify shared volumes and networks
4. **Project Files**: Backup compose files, .env, and related configurations
5. **Multi-Container Backup**: Execute backup process for each service container
6. **Dependencies**: Record service startup order and dependencies
7. **Package**: Compress all project data into tar.gz file

## Compose Project Restore Process

1. **Extract Backup**: Decompress project backup file
2. **Project Setup**: Recreate project directory and compose files
3. **Networks & Volumes**: Recreate shared networks and volumes
4. **Container Restore**: Restore each service container individually
5. **Dependencies**: Respect original service dependencies and startup order
6. **Start Services**: (Optional) Start all services with `docker-compose up`

## Backup File Structure

### Single Container Backup
```
container_backup.tar.gz
├── container.json          # Complete container configuration
├── filesystem.tar          # Container filesystem (docker export)
├── volumes/                # Volume data
│   ├── volume1.tar.gz
│   └── volume2.tar.gz
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
├── containers/             # Individual container backups
│   ├── service1/
│   │   ├── container.json
│   │   ├── filesystem.tar
│   │   └── volumes/
│   └── service2/
│       └── ...
├── shared-volumes/         # Shared volumes across services
│   ├── shared_vol1.tar.gz
│   └── shared_vol2.tar.gz
├── networks/               # Network configurations
│   └── network_configs.json
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

## Contributing

Issues and Pull Requests are welcome.

## License

MIT License
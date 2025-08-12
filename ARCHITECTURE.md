# DockerBackup - Technical Architecture

## Overview

DockerBackup is designed with a modular, layered architecture that emphasizes separation of concerns and reusability. The core principle is that container backup is the atomic unit, and compose project backup builds upon this foundation.

## Architecture Principles

1. **Single Responsibility**: Each module handles one specific aspect of backup/restore
2. **Dependency Injection**: Components are loosely coupled through interfaces
3. **Strategy Pattern**: Different backup/restore strategies for different scenarios
4. **Command Pattern**: CLI commands are encapsulated as executable objects
5. **Template Method**: Common backup/restore workflows with customizable steps

## Core Architecture

```
┌─────────────────────────────────────────────────────┐
│                  CLI Layer                          │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  │
│  │   backup    │  │   restore   │  │    list     │  │
│  │  commands   │  │  commands   │  │  commands   │  │
│  └─────────────┘  └─────────────┘  └─────────────┘  │
└─────────────────────────────────────────────────────┘
                           │
┌─────────────────────────────────────────────────────┐
│              Service Layer                          │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  │
│  │ Container   │  │  Compose    │  │   Backup    │  │
│  │  Service    │  │  Service    │  │   Manager   │  │
│  └─────────────┘  └─────────────┘  └─────────────┘  │
└─────────────────────────────────────────────────────┘
                           │
┌─────────────────────────────────────────────────────┐
│               Core Layer                            │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  │
│  │   Backup    │  │   Archive   │  │   Docker    │  │
│  │  Engine     │  │  Handler    │  │   Client    │  │
│  └─────────────┘  └─────────────┘  └─────────────┘  │
└─────────────────────────────────────────────────────┘
                           │
┌─────────────────────────────────────────────────────┐
│             Infrastructure Layer                    │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  │
│  │ FileSystem  │  │   Logger    │  │   Config    │  │
│  │  Handler    │  │             │  │   Manager   │  │
│  └─────────────┘  └─────────────┘  └─────────────┘  │
└─────────────────────────────────────────────────────┘
```

## Core Interfaces

### Backup Engine Interface
```go
type BackupEngine interface {
    Backup(ctx context.Context, request BackupRequest) (*BackupResult, error)
    Restore(ctx context.Context, request RestoreRequest) (*RestoreResult, error)
    Validate(ctx context.Context, backupPath string) (*ValidationResult, error)
}
```

### Container Backup Interface
```go
type ContainerBackupper interface {
    BackupContainer(ctx context.Context, containerID string, opts BackupOptions) (*ContainerBackup, error)
    RestoreContainer(ctx context.Context, backup *ContainerBackup, opts RestoreOptions) (*Container, error)
}
```

### Archive Handler Interface
```go
type ArchiveHandler interface {
    CreateArchive(ctx context.Context, sources []ArchiveSource, dest string) error
    ExtractArchive(ctx context.Context, archivePath, destDir string) error
    ListArchive(ctx context.Context, archivePath string) ([]ArchiveEntry, error)
}
```

## Module Structure

```
cmd/
├── root.go              # Root command and global flags
├── backup.go           # Single container backup command
├── restore.go          # Single container restore command
├── backup_compose.go   # Compose project backup command
├── restore_compose.go  # Compose project restore command
└── list.go            # List backup contents command

pkg/
├── backup/
│   ├── engine.go       # Backup engine implementation
│   ├── container.go    # Container backup logic
│   ├── compose.go      # Compose project backup logic
│   └── strategy.go     # Different backup strategies
├── archive/
│   ├── tar.go          # Tar archive implementation
│   ├── compression.go  # Compression handling
│   └── validator.go    # Archive validation
├── docker/
│   ├── client.go       # Docker API client wrapper
│   ├── container.go    # Container operations
│   ├── volume.go       # Volume operations
│   └── network.go      # Network operations
├── compose/
│   ├── parser.go       # docker-compose.yml parser
│   ├── project.go      # Project management
│   └── discovery.go    # Service discovery
├── filesystem/
│   ├── handler.go      # File system operations
│   ├── permissions.go  # Permission handling
│   └── watcher.go      # File system watching
└── config/
    ├── manager.go      # Configuration management
    └── validation.go   # Config validation

internal/
├── logger/
│   └── logger.go       # Structured logging
├── errors/
│   └── types.go        # Custom error types
└── utils/
    ├── progress.go     # Progress tracking
    └── helpers.go      # Utility functions
```

## Design Patterns Implementation

### 1. Strategy Pattern - Backup Strategies
```go
type BackupStrategy interface {
    Execute(ctx context.Context, target BackupTarget) (*BackupResult, error)
    Validate(target BackupTarget) error
}

type ContainerBackupStrategy struct{}
type ComposeBackupStrategy struct {
    containerStrategy ContainerBackupStrategy
}
```

### 2. Command Pattern - CLI Commands
```go
type Command interface {
    Execute(ctx context.Context, args []string) error
    Validate(args []string) error
}

type BackupCommand struct {
    backupEngine BackupEngine
    logger       Logger
}
```

### 3. Template Method Pattern - Backup Workflow
```go
type BackupTemplate struct{}

func (bt *BackupTemplate) Execute(ctx context.Context, req BackupRequest) error {
    // Template method defining the workflow
    if err := bt.PreBackup(ctx, req); err != nil {
        return err
    }
    
    if err := bt.DoBackup(ctx, req); err != nil {
        return err
    }
    
    return bt.PostBackup(ctx, req)
}

// Hook methods to be implemented by concrete types
func (bt *BackupTemplate) PreBackup(ctx context.Context, req BackupRequest) error
func (bt *BackupTemplate) DoBackup(ctx context.Context, req BackupRequest) error  
func (bt *BackupTemplate) PostBackup(ctx context.Context, req BackupRequest) error
```

### 4. Builder Pattern - Backup Configuration
```go
type BackupOptionsBuilder struct {
    options BackupOptions
}

func NewBackupOptionsBuilder() *BackupOptionsBuilder {
    return &BackupOptionsBuilder{
        options: BackupOptions{
            CompressionLevel: DefaultCompressionLevel,
        },
    }
}

func (b *BackupOptionsBuilder) WithOutput(path string) *BackupOptionsBuilder {
    b.options.OutputPath = path
    return b
}

func (b *BackupOptionsBuilder) WithCompression(level int) *BackupOptionsBuilder {
    b.options.CompressionLevel = level
    return b
}

func (b *BackupOptionsBuilder) Build() BackupOptions {
    return b.options
}
```

## Data Flow

### Single Container Backup Flow
```
User Input → CLI Command → Container Service → Backup Engine
    ↓
Docker Client ← Archive Handler ← Filesystem Handler
    ↓
Container Inspection → Filesystem Export → Volume Backup
    ↓
Archive Creation → Compression → Output File
```

### Compose Project Backup Flow
```
User Input → CLI Command → Compose Service
    ↓
Compose Parser → Project Discovery → Service Mapping
    ↓
For Each Service: Container Service → Backup Engine
    ↓
Shared Resource Collection → Project File Backup
    ↓
Archive Aggregation → Final Package Creation
```

## Error Handling Strategy

1. **Typed Errors**: Custom error types for different failure scenarios
2. **Error Wrapping**: Context-rich error messages with stack traces
3. **Graceful Degradation**: Partial success handling where possible
4. **Rollback Capability**: Cleanup on failure scenarios

## Configuration Management

- **Environment Variables**: For runtime configuration
- **Config Files**: For complex default settings
- **CLI Flags**: For per-command overrides
- **Validation**: Comprehensive config validation at startup

## Logging and Observability

- **Structured Logging**: JSON-formatted logs with context
- **Progress Tracking**: Real-time progress for long operations
- **Metrics**: Optional metrics collection for monitoring
- **Debug Mode**: Verbose logging for troubleshooting

## Testing Strategy

- **Unit Tests**: Individual component testing
- **Integration Tests**: Docker integration testing
- **E2E Tests**: Full backup/restore cycle testing
- **Mock Framework**: Docker API mocking for testing

## Performance Considerations

- **Streaming**: Stream processing for large files
- **Parallel Processing**: Concurrent backup of independent resources
- **Memory Management**: Efficient memory usage for large containers
- **Progress Feedback**: User feedback for long-running operations

This architecture ensures maintainability, testability, and extensibility while following Go best practices and design patterns.
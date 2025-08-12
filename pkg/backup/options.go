package backup

import "github.com/brian033/dockerbackup/pkg/archive"

type BackupOptions struct {
	OutputPath       string
	CompressionLevel int
}

type RestoreOptions struct {
	ContainerName      string
	Start              bool
	// Portability and mapping
	NetworkMap         map[string]string
	ParentMap          map[string]string
	DropHostIPs        bool
	ReassignIPs        bool
	FallbackBridge     bool
	// Health / readiness
	WaitHealthy        bool
	WaitTimeoutSeconds int
}

type BackupOptionsBuilder struct {
	options BackupOptions
}

func NewBackupOptionsBuilder() *BackupOptionsBuilder {
	return &BackupOptionsBuilder{
		options: BackupOptions{
			CompressionLevel: archive.DefaultCompressionLevel,
		},
	}
}

func (b *BackupOptionsBuilder) WithOutput(path string) *BackupOptionsBuilder {
	b.options.OutputPath = path
	return b
}

func (b *BackupOptionsBuilder) WithCompression(level int) *BackupOptionsBuilder {
	if level > 0 {
		b.options.CompressionLevel = level
	}
	return b
}

func (b *BackupOptionsBuilder) Build() BackupOptions {
	return b.options
}

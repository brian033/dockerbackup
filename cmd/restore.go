package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/brian033/dockerbackup/internal/logger"
	"github.com/brian033/dockerbackup/pkg/backup"
	"github.com/spf13/pflag"
)

type RestoreCmd struct {
	log    logger.Logger
	engine backup.BackupEngine
}

func (c *RestoreCmd) Name() string { return "restore" }

func (c *RestoreCmd) Help() string {
	return `
Restore a container from a backup file.

Usage:
  dockerbackup restore <backup_file> [options]

Options:
  -n, --name string   New container name (default: original)
  --start             Start container after restore
`
}

func (c *RestoreCmd) Validate(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing backup file path")
	}
	return nil
}

func (c *RestoreCmd) Execute(ctx context.Context, args []string) error {
	fs := pflag.NewFlagSet(c.Name(), pflag.ContinueOnError)
	var name string
	var start bool
	var netMaps []string
	var parentMaps []string
	var dropHostIPs bool
	var reassignIPs bool
	var fallbackBridge bool
	var waitHealthy bool
	var waitTimeout int
	var replace bool
	var bindRestoreRoot string
	var forceBindIP string
	var bindInterface string
	var dropDevices bool
	var dropCaps bool
	var dropSeccomp bool
	var dropAppArmor bool
	var autoRelaxIPs bool
	fs.StringVarP(&name, "name", "n", "", "New container name")
	fs.BoolVar(&start, "start", false, "Start container after restore")
	fs.StringArrayVar(&netMaps, "network-map", nil, "Map networks old:new (repeatable)")
	fs.StringArrayVar(&parentMaps, "parent-map", nil, "Override macvlan/ipvlan parent: network:parentIf (repeatable)")
	fs.BoolVar(&dropHostIPs, "drop-host-ips", false, "Ignore HostIp in port bindings if not present on host")
	fs.BoolVar(&reassignIPs, "reassign-ips", false, "Ignore saved static container IPs; let Docker assign")
	fs.BoolVar(&fallbackBridge, "fallback-bridge", false, "If macvlan/ipvlan parent missing, use bridge network")
	fs.BoolVar(&waitHealthy, "wait-healthy", false, "Wait until container healthcheck reports healthy before returning")
	fs.IntVar(&waitTimeout, "wait-timeout", int((2 * time.Minute).Seconds()), "Max seconds to wait when --wait-healthy is set")
	fs.BoolVar(&replace, "replace", false, "Stop and remove existing container with the same name before restore")
	fs.StringVar(&bindRestoreRoot, "bind-restore-root", "", "If bind source missing, relocate under this root (e.g., /srv/restored)")
	fs.StringVar(&forceBindIP, "force-bind-ip", "", "Force all port bindings to use this host IP")
	fs.StringVar(&bindInterface, "bind-interface", "", "Prefer this interface's primary IP for port bindings if HostIp missing")
	fs.BoolVar(&dropDevices, "drop-devices", false, "Drop HostConfig.Devices on restore (safe mode)")
	fs.BoolVar(&dropCaps, "drop-caps", false, "Drop HostConfig.CapAdd/CapDrop on restore (safe mode)")
	fs.BoolVar(&dropSeccomp, "drop-seccomp", false, "Drop HostConfig.SecurityOpt seccomp profile (safe mode)")
	fs.BoolVar(&dropAppArmor, "drop-apparmor", false, "Drop HostConfig.SecurityOpt apparmor profile (safe mode)")
	fs.BoolVar(&autoRelaxIPs, "auto-relax-ips", false, "If container has static IPs conflicting with host networks, drop IPAM to let Docker assign")
	if err := fs.Parse(args); err != nil {
		return err
	}
	remaining := fs.Args()
	if len(remaining) == 0 {
		return fmt.Errorf("missing backup file path")
	}
	backupFile := remaining[0]

	parseMap := func(items []string) map[string]string {
		m := map[string]string{}
		for _, it := range items {
			parts := strings.SplitN(it, ":", 2)
			if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
				m[parts[0]] = parts[1]
			}
		}
		return m
	}

	req := backup.RestoreRequest{
		BackupPath: backupFile,
		Options: backup.RestoreOptions{
			ContainerName:      name,
			Start:              start,
			NetworkMap:         parseMap(netMaps),
			ParentMap:          parseMap(parentMaps),
			DropHostIPs:        dropHostIPs,
			ReassignIPs:        reassignIPs,
			FallbackBridge:     fallbackBridge,
			WaitHealthy:        waitHealthy,
			WaitTimeoutSeconds: waitTimeout,
			ReplaceExisting:    replace,
			BindRestoreRoot:    bindRestoreRoot,
			ForceBindIP:        forceBindIP,
			BindInterface:      bindInterface,
			DropDevices:        dropDevices,
			DropCaps:           dropCaps,
			DropSeccomp:        dropSeccomp,
			DropAppArmor:       dropAppArmor,
			AutoRelaxIPs:      autoRelaxIPs,
		},
		TargetType: backup.TargetContainer,
	}
	if c.engine == nil {
		c.engine = newDefaultEngine(c.log)
	}
	_, err := c.engine.Restore(ctx, req)
	return err
}

func init() {
	cmd := &RestoreCmd{
		log:    logger.New(),
		engine: nil,
	}
	RegisterCommand(cmd)
}

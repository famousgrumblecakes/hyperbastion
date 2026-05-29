package main

import (
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/tg123/sshpiper/libplugin"
	"github.com/urfave/cli/v2"
)

// containerMetaKey is the key used in the inter-plugin metadata map to carry
// the target container name from the selection plugin into this one. Any plugin
// that precedes this one in the chain must call:
//
//	libplugin.CreateNextPluginAuth(map[string]string{containerMetaKey: "<name>"})
const containerMetaKey = "container"

func main() {
	libplugin.CreateAndRunPluginTemplate(&libplugin.PluginTemplate{
		Name:  "nspawn",
		Usage: "sshpiperd nspawn plugin: starts a systemd-nspawn container and routes the SSH connection into it",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "machines-dir",
				Usage:   "base directory containing container root filesystems; the container name is appended to form the --directory argument passed to systemd-nspawn",
				Value:   "/var/lib/machines",
				EnvVars: []string{"SSHPIPERD_NSPAWN_MACHINES_DIR"},
			},
			&cli.StringFlag{
				Name:    "meta-key",
				Usage:   "metadata key from which to read the container name forwarded by the preceding plugin",
				Value:   containerMetaKey,
				EnvVars: []string{"SSHPIPERD_NSPAWN_META_KEY"},
			},
			&cli.IntFlag{
				Name:    "upstream-port",
				Usage:   "SSH port the container's sshd listens on; the container hostname is resolved via nss-mymachines",
				Value:   22,
				EnvVars: []string{"SSHPIPERD_NSPAWN_UPSTREAM_PORT"},
			},
			&cli.DurationFlag{
				Name:    "startup-timeout",
				Usage:   "how long to poll the container's SSH port before giving up after a launch",
				Value:   30 * time.Second,
				EnvVars: []string{"SSHPIPERD_NSPAWN_STARTUP_TIMEOUT"},
			},
		},
		CreateConfig: func(c *cli.Context) (*libplugin.SshPiperPluginConfig, error) {
			machinesDir := c.String("machines-dir")
			metaKey := c.String("meta-key")
			upstreamPort := int32(c.Int("upstream-port"))
			startupTimeout := c.Duration("startup-timeout")

			return &libplugin.SshPiperPluginConfig{
				PasswordCallback: func(conn libplugin.ConnMetadata, password []byte) (*libplugin.Upstream, error) {
					containerName := conn.GetMeta(metaKey)
					if containerName == "" {
						return nil, fmt.Errorf("nspawn: meta key %q not set — a container-selection plugin must precede this one in the chain", metaKey)
					}

					log.Infof("nspawn: user=%q container=%q", conn.User(), containerName)

					if err := ensureRunning(containerName, machinesDir); err != nil {
						return nil, err
					}

					if err := waitForSSH(containerName, upstreamPort, startupTimeout); err != nil {
						return nil, err
					}

					// The container hostname resolves via nss-mymachines once the machine
					// is registered with systemd-machined, so no static IP mapping is needed.
					// Upstream credentials are whatever the user typed; the container's sshd
					// is expected to accept them. This will be revisited when auth is hardened.
					return &libplugin.Upstream{
						Host:          containerName,
						Port:          upstreamPort,
						UserName:      conn.User(),
						IgnoreHostKey: true,
						Auth:          libplugin.CreatePasswordAuth(password),
					}, nil
				},
			}, nil
		},
	})
}

// ensureRunning starts the named container if it is not already in the "running"
// state according to systemd-machined. The launch command is intentionally minimal
// and will be extended with additional systemd-nspawn options as requirements are
// defined.
func ensureRunning(name, machinesDir string) error {
	if isMachineRunning(name) {
		log.Infof("nspawn: machine %q is already running", name)
		return nil
	}

	log.Infof("nspawn: starting machine %q (rootfs: %s)", name, filepath.Join(machinesDir, name))

	//nolint:gosec // name and machinesDir are operator-supplied configuration values, not user input.
	cmd := exec.Command(
		"systemd-run",
		"--collect",
		"--unit=sshpiper-nspawn@"+name,
		"--",
		"systemd-nspawn",
		"--ephemeral",
		"--machine="+name,
		"--boot",
		"--quiet",
		"--directory="+filepath.Join(machinesDir, "ubuntu-2604"),
	)

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("nspawn: failed to start machine %q: %w\n%s", name, err, strings.TrimSpace(string(out)))
	}

	log.Infof("nspawn: machine %q started", name)

	return nil
}

// isMachineRunning returns true if systemd-machined reports the machine as running.
// This covers containers started by any mechanism (machinectl, systemd-run, etc.).
func isMachineRunning(name string) bool {
	//nolint:gosec // name is operator-supplied configuration.
	out, err := exec.Command("machinectl", "show", "--property=State", name).Output()
	if err != nil {
		return false
	}

	return strings.Contains(string(out), "State=running")
}

// waitForSSH dials the container's SSH port on a half-second interval until it
// responds or the timeout elapses. Because nss-mymachines provides name
// resolution once the machine is registered, the container name is used directly
// as the dial address.
func waitForSSH(host string, port int32, timeout time.Duration) error {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err == nil {
			_ = c.Close()
			log.Infof("nspawn: SSH reachable on %s", addr)

			return nil
		}

		log.Debugf("nspawn: waiting for SSH on %s: %v", addr, err)
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("nspawn: timed out after %v waiting for SSH on %s", timeout, addr)
}

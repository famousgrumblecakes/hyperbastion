package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/tg123/sshpiper/libplugin"
	"github.com/urfave/cli/v2"
)

// nextPluginContainerKey is the metadata key under which the selected container
// name is forwarded to the next plugin (e.g. the nspawn plugin).  It must match
// the --meta-key value configured on that plugin (default: "container").
const nextPluginContainerKey = "container"

func main() {
	libplugin.CreateAndRunPluginTemplate(&libplugin.PluginTemplate{
		Name:  "container-select",
		Usage: "sshpiperd container-select plugin: presents the authenticated user with their available containers and forwards the selection to the next plugin",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "containers-dir",
				Usage:   "directory holding per-user container lists; each file is named <username>.json and contains a JSON array of container name strings",
				Value:   "/etc/sshpiper/containers",
				EnvVars: []string{"SSHPIPERD_CONTAINER_SELECT_DIR"},
			},
		},
		CreateConfig: func(c *cli.Context) (*libplugin.SshPiperPluginConfig, error) {
			containersDir := c.String("containers-dir")

			return &libplugin.SshPiperPluginConfig{
				KeyboardInteractiveCallback: func(conn libplugin.ConnMetadata, challenge libplugin.KeyboardInteractiveChallenge) (*libplugin.Upstream, error) {
					username := conn.User()

					containers, err := loadContainers(containersDir, username)
					if err != nil {
						return nil, err
					}

					if len(containers) == 0 {
						return nil, fmt.Errorf("container-select: no containers available for user %q", username)
					}

					// Build the display list once; reuse it on every re-prompt.
					listing := buildListing(containers)

					for {
						answer, err := challenge("", listing, "Enter container name or number: ", true)
						if err != nil {
							return nil, fmt.Errorf("container-select: challenge error for user %q: %w", username, err)
						}

						selected, ok := resolve(containers, strings.TrimSpace(answer))
						if !ok {
							// Re-prompt; the instruction field on the next call will
							// show the list again alongside the error.
							_, _ = challenge(
								"",
								fmt.Sprintf("Invalid selection %q — please choose from the list below.\n\n%s", answer, listing),
								"Enter container name or number: ",
								true,
							)
							continue
						}

						log.Infof("container-select: user %q selected container %q", username, selected)

						return &libplugin.Upstream{
							Auth: libplugin.CreateNextPluginAuth(map[string]string{
								nextPluginContainerKey: selected,
							}),
						}, nil
					}
				},
			}, nil
		},
	})
}

// buildListing formats the container slice into a numbered display string
// suitable for the SSH keyboard-interactive instruction field.
func buildListing(containers []string) string {
	var sb strings.Builder

	sb.WriteString("Available containers:\n\n")

	for i, name := range containers {
		fmt.Fprintf(&sb, "  [%d] %s\n", i+1, name)
	}

	sb.WriteString("\n")

	return sb.String()
}

// resolve accepts either a 1-based index ("1", "2", …) or the container name
// verbatim and returns the matching container name.
func resolve(containers []string, input string) (string, bool) {
	for i, name := range containers {
		if input == name || input == fmt.Sprintf("%d", i+1) {
			return name, true
		}
	}

	return "", false
}

// loadContainers returns the list of container names available to username.
//
// TODO: Replace this file-based lookup with a database query once the schema is
// defined.  The expected query is roughly:
//
//	SELECT name FROM containers WHERE owner = $username ORDER BY name
//
// The calling code only depends on the returned []string, so this function is
// the sole change point.
func loadContainers(containersDir, username string) ([]string, error) {
	path := filepath.Join(containersDir, username+".json")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("container-select: no container list found for user %q (looked in %s)", username, path)
		}

		return nil, fmt.Errorf("container-select: could not read container list for user %q: %w", username, err)
	}

	var containers []string
	if err := json.Unmarshal(data, &containers); err != nil {
		return nil, fmt.Errorf("container-select: malformed container list for user %q in %s: %w", username, path, err)
	}

	return containers, nil
}

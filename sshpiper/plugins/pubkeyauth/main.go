package main

import (
	"crypto/subtle"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/tg123/sshpiper/libplugin"
	"github.com/urfave/cli/v2"
	"golang.org/x/crypto/ssh"
)

func main() {
	libplugin.CreateAndRunPluginTemplate(&libplugin.PluginTemplate{
		Name:  "pubkeyauth",
		Usage: "sshpiperd public key auth plugin: checks the client's public key against per-user key files and passes to the next plugin on success",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "authorized-users-dir",
				Usage:    "path to a directory that contains one subdirectory per username; each subdirectory holds individual public key files in authorized_keys format",
				EnvVars:  []string{"SSHPIPERD_PUBKEYAUTH_AUTHORIZED_USERS_DIR"},
				Required: true,
			},
		},
		CreateConfig: func(c *cli.Context) (*libplugin.SshPiperPluginConfig, error) {
			authorizedUsersDir := c.String("authorized-users-dir")

			return &libplugin.SshPiperPluginConfig{
				PublicKeyCallback: func(conn libplugin.ConnMetadata, key []byte) (*libplugin.Upstream, error) {
					username := conn.User()

					// Reject usernames that could escape the base directory.
					if strings.ContainsAny(username, "/\\") || username == ".." || username == "." || username == "" {
						return nil, fmt.Errorf("pubkeyauth: invalid username %q", username)
					}

					userDir := filepath.Join(authorizedUsersDir, username)
					log.Infof("pubkeyauth: checking authorized keys for user %q in %s", username, userDir)

					entries, err := os.ReadDir(userDir)
					if err != nil {
						if os.IsNotExist(err) {
							return nil, fmt.Errorf("pubkeyauth: no authorized keys directory found for user %q in %s", username, userDir)
						}
						return nil, fmt.Errorf("pubkeyauth: error reading authorized keys directory for user %q: %w", username, err)
					}

					for _, entry := range entries {
						if entry.IsDir() {
							continue
						}

						keyFilePath := filepath.Join(userDir, entry.Name())
						data, err := os.ReadFile(keyFilePath)
						if err != nil {
							log.Warnf("pubkeyauth: skipping unreadable key file %s: %v", keyFilePath, err)
							continue
						}

						rest := data
						for len(rest) > 0 {
							var authorizedKey ssh.PublicKey
							authorizedKey, _, _, rest, err = ssh.ParseAuthorizedKey(rest)
							if err != nil {
								log.Warnf("pubkeyauth: could not parse key file %s: %v", keyFilePath, err)
								break
							}

							if subtle.ConstantTimeCompare(authorizedKey.Marshal(), key) == 1 {
								log.Infof("pubkeyauth: public key accepted for user %q (matched %s)", username, keyFilePath)
								return &libplugin.Upstream{
									Auth: libplugin.CreateNextPluginAuth(nil),
								}, nil
							}
						}
					}

					log.Warnf("pubkeyauth: no authorized key matched for user %q", username)
					return nil, fmt.Errorf("pubkeyauth: public key not authorized for user %q", username)
				},
			}, nil
		},
	})
}

// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/PurpleSentinel/cyber-city/internal/config"
	"github.com/PurpleSentinel/cyber-city/internal/privdrop"
)

// dropPrivileges switches away from root once the capture handle, listener
// and output files are open (docs/CONFIG.md "Startup order", D22).
func dropPrivileges(cfg config.Config) error {
	target, warn := privdrop.Plan(os.Geteuid(), cfg.User)
	if warn != "" {
		slog.Warn(warn)
	}
	if target == "" {
		return nil
	}
	acct, err := privdrop.Resolve(target)
	if err != nil {
		return fmt.Errorf("privilege drop: %w", err)
	}
	if err := privdrop.Drop(acct); err != nil {
		return err
	}
	slog.Info("dropped root privileges", "user", acct.Name, "uid", acct.UID)
	return nil
}

// giveToSudoUser hands a file created as root back to the user who ran sudo,
// so they can read the URL file without sudo. A no-op unless running as root
// under sudo.
func giveToSudoUser(path string) {
	if os.Geteuid() != 0 {
		return
	}
	uid, uerr := strconv.Atoi(os.Getenv("SUDO_UID"))
	gid, gerr := strconv.Atoi(os.Getenv("SUDO_GID"))
	if uerr != nil || gerr != nil {
		return
	}
	if err := os.Chown(path, uid, gid); err != nil {
		slog.Warn("could not hand url file to sudo user", "path", path, "err", err)
	}
}

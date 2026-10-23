// SPDX-License-Identifier: MIT

// Package privdrop gives up root after the capture handle and listener are
// open (docs/CONFIG.md "Startup order", DECISIONS D22).
package privdrop

import (
	"errors"
	"fmt"
	"os/user"
	"strconv"
)

// DefaultUser is the account used when running as root without --user.
const DefaultUser = "nobody"

// Account is a resolved user to switch to.
type Account struct {
	Name     string
	UID, GID int
}

// Resolve looks up name in the user database.
func Resolve(name string) (Account, error) {
	if name == "" {
		return Account{}, errors.New("empty user name")
	}
	u, err := user.Lookup(name)
	if err != nil {
		return Account{}, fmt.Errorf("user %q: %w", name, err)
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return Account{}, fmt.Errorf("user %q: uid %q: %w", name, u.Uid, err)
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return Account{}, fmt.Errorf("user %q: gid %q: %w", name, u.Gid, err)
	}
	return Account{Name: name, UID: uid, GID: gid}, nil
}

// Plan decides what to do for the effective uid and requested user:
// target is the account to switch to (nil: stay), warn is a message to log.
func Plan(euid int, requested string) (target string, warn string) {
	switch {
	case euid != 0 && requested != "":
		return "", "--user ignored: not running as root"
	case euid != 0:
		return "", ""
	case requested == "root":
		return "", "running as root (--user root): the server keeps full privileges"
	case requested == "":
		return DefaultUser, ""
	default:
		return requested, ""
	}
}

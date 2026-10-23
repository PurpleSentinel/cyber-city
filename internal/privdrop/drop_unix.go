// SPDX-License-Identifier: MIT

//go:build unix

package privdrop

import (
	"fmt"
	"os"
	"syscall"
)

// Drop switches the whole process (all threads, Go ≥ 1.16) to a: supplementary
// groups cleared, then gid, then uid. It verifies the switch and that root
// cannot be regained.
func Drop(a Account) error {
	if err := syscall.Setgroups([]int{}); err != nil {
		return fmt.Errorf("setgroups: %w", err)
	}
	if err := syscall.Setgid(a.GID); err != nil {
		return fmt.Errorf("setgid %d: %w", a.GID, err)
	}
	if err := syscall.Setuid(a.UID); err != nil {
		return fmt.Errorf("setuid %d: %w", a.UID, err)
	}
	if os.Getuid() != a.UID || os.Geteuid() != a.UID || os.Getgid() != a.GID || os.Getegid() != a.GID {
		return fmt.Errorf("privilege drop incomplete: uid=%d euid=%d gid=%d egid=%d", os.Getuid(), os.Geteuid(), os.Getgid(), os.Getegid())
	}
	if a.UID != 0 && syscall.Setuid(0) == nil {
		return fmt.Errorf("privilege drop failed: root could be regained")
	}
	return nil
}

//go:build !windows

package cenciwatch

import (
	"os"
	"syscall"
)

// dirOwnedByCurrentUser reports whether the directory described by info is
// owned by the current user. A socket directory owned by someone else could
// hold a socket planted by that user, which the board would then trust as the
// daemon, so secureSocketDir skips such a tier.
func dirOwnedByCurrentUser(info os.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == os.Getuid()
}

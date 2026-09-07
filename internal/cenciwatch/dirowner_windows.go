//go:build windows

package cenciwatch

import "os"

// dirOwnedByCurrentUser reports whether the directory described by info is
// owned by the current user.
//
// Windows has no uid to compare against: os.Getuid returns -1 and
// os.FileInfo carries no ACL, so ownership is not expressible here without
// pulling in golang.org/x/sys/windows. This returns true so the tier is
// judged on secureSocketDir's remaining, portable checks (it must exist as a
// real directory and not be a symlink). The permission-bit check is likewise
// meaningless on Windows, where Go synthesizes mode bits rather than reading
// them from an ACL.
func dirOwnedByCurrentUser(os.FileInfo) bool { return true }

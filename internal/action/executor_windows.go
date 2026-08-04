//go:build windows

package action

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// openBrowser opens url in the system browser via ShellExecuteW, never a
// shell -- see docs/shell-and-url-safety.md for the vulnerability this
// avoids (a repo-local keymap action URL containing shell metacharacters
// reaching `cmd /c start`).
func openBrowser(url string) error {
	return shellExecuteOpen(url)
}

// shellExecuteOpen is a package-level seam so tests can swap in a recorder
// without invoking a real ShellExecuteW call.
var shellExecuteOpen = defaultShellExecuteOpen

// defaultShellExecuteOpen opens url via the Windows ShellExecuteW API.
func defaultShellExecuteOpen(url string) error {
	verbPtr, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return fmt.Errorf("ShellExecuteW %q: %w", url, err)
	}
	urlPtr, err := windows.UTF16PtrFromString(url)
	if err != nil {
		return fmt.Errorf("ShellExecuteW %q: %w", url, err)
	}
	if err := windows.ShellExecute(0, verbPtr, urlPtr, nil, nil, windows.SW_SHOWNORMAL); err != nil {
		return fmt.Errorf("ShellExecuteW %q: %w", url, err)
	}
	return nil
}

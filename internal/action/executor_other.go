//go:build !windows && !darwin

package action

import "os/exec"

// openBrowser opens url in the system browser via the `xdg-open` command.
func openBrowser(url string) error {
	return exec.Command("xdg-open", url).Start()
}

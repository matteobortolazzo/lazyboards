//go:build darwin

package action

import "os/exec"

// openBrowser opens url in the system browser via the `open` command.
func openBrowser(url string) error {
	return exec.Command("open", url).Start()
}

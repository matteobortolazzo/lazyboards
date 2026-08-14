package action

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// --- #624: window: actions ---
//
// TmuxNewWindow and WithDir assemble shell command lines out of values that
// come from provider data (card titles, branch names, worktree paths), so
// the property that matters is not "the string looks right" but "the shell
// splits it into exactly the arguments we intended, whatever the values
// contain". Every test below therefore runs the built command through a real
// `sh -c` with a recording stub standing in for tmux, and asserts the argv
// the stub actually received. A quoting bug shows up as a wrong argv, not as
// a string mismatch a reviewer has to eyeball.

// recordArgv runs command through `sh -c` with a fake `tmux` first on PATH
// that records its own argv, and returns that argv. The stub separates
// arguments with NUL bytes so an argument containing a newline is still
// recorded faithfully.
func recordArgv(t *testing.T, command string) []string {
	t.Helper()
	dir := t.TempDir()
	argvFile := filepath.Join(dir, "argv")
	stub := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\0' \"$a\" >> \"$ARGV_FILE\"; done\n"
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(stub), 0o755); err != nil {
		t.Fatalf("failed to write tmux stub: %v", err)
	}

	cmd := exec.Command("sh", "-c", command)
	cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "ARGV_FILE="+argvFile)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("running %q failed: %v (stderr: %s)", command, err, stderr.String())
	}

	data, err := os.ReadFile(argvFile)
	if err != nil {
		t.Fatalf("tmux stub recorded no argv for %q: %v", command, err)
	}
	recorded := strings.Split(string(data), "\x00")
	return recorded[:len(recorded)-1] // trailing empty element after the last NUL
}

func assertArgv(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("tmux received %d args %q, want %d args %q", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("arg %d = %q, want %q (full argv: %q)", i, got[i], want[i], got)
		}
	}
}

func TestTmuxNewWindow_DetachedByDefault(t *testing.T) {
	argv := recordArgv(t, TmuxNewWindow("pr-42", "", "go run .", false))
	assertArgv(t, argv, []string{"new-window", "-d", "-n", "pr-42", "go run ."})
}

func TestTmuxNewWindow_FocusDropsDetachFlag(t *testing.T) {
	argv := recordArgv(t, TmuxNewWindow("pr-42", "", "go run .", true))
	assertArgv(t, argv, []string{"new-window", "-n", "pr-42", "go run ."})
}

func TestTmuxNewWindow_CwdBecomesStartDirectory(t *testing.T) {
	argv := recordArgv(t, TmuxNewWindow("pr-42", "/home/dev/wt", "go run .", false))
	assertArgv(t, argv, []string{"new-window", "-d", "-n", "pr-42", "-c", "/home/dev/wt", "go run ."})
}

// An empty command is a real action ("just open a window there"): tmux opens
// the window running the default shell, so no command argument is passed.
func TestTmuxNewWindow_EmptyCommandPassesNoCommandArgument(t *testing.T) {
	argv := recordArgv(t, TmuxNewWindow("pr-42", "/home/dev/wt", "", false))
	assertArgv(t, argv, []string{"new-window", "-d", "-n", "pr-42", "-c", "/home/dev/wt"})
}

// --- Escaping: the values below are what a hostile card title, branch name,
// or worktree path can contain. Each must arrive as exactly one argument. ---

func TestTmuxNewWindow_HostileValuesStayOneArgumentEach(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"command substitution", "$(touch /tmp/lazyboards-pwned)"},
		{"backticks", "`touch /tmp/lazyboards-pwned`"},
		{"statement separator", "x'; touch /tmp/lazyboards-pwned; echo '"},
		{"quote and space", "my 'window' name"},
		{"newline", "line1\nline2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			argv := recordArgv(t, TmuxNewWindow(tc.value, tc.value, tc.value, false))
			assertArgv(t, argv, []string{"new-window", "-d", "-n", tc.value, "-c", tc.value, tc.value})
		})
	}
}

// TestTmuxNewWindow_HostileValueDoesNotExecute is the companion to the argv
// assertions: it proves the injected payload never ran, rather than only
// that the arguments came out grouped as expected.
func TestTmuxNewWindow_HostileValueDoesNotExecute(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "pwned")
	payload := "x'; touch " + marker + "; echo '"

	recordArgv(t, TmuxNewWindow(payload, payload, payload, false))

	if _, err := os.Stat(marker); err == nil {
		t.Fatalf("injected payload executed: %s was created", marker)
	}
}

// --- WithDir: the cwd: field for non-windowed actions ---

func TestWithDir_RunsCommandInDirectory(t *testing.T) {
	dir := t.TempDir()
	out, err := exec.Command("sh", "-c", WithDir(dir, "pwd")).Output()
	if err != nil {
		t.Fatalf("running the composed command failed: %v", err)
	}
	// macOS resolves TempDir through /private; compare the resolved forms.
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", dir, err)
	}
	got, err := filepath.EvalSymlinks(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", strings.TrimSpace(string(out)), err)
	}
	if got != want {
		t.Errorf("command ran in %q, want %q", got, want)
	}
}

func TestWithDir_EmptyDirLeavesCommandUnchanged(t *testing.T) {
	if got := WithDir("", "go test ./..."); got != "go test ./..." {
		t.Errorf("WithDir(\"\", cmd) = %q, want the command unchanged", got)
	}
}

// A directory whose name contains shell metacharacters must be handled as
// one path, not re-parsed by the shell.
func TestWithDir_HostileDirectoryNameIsOnePath(t *testing.T) {
	parent := t.TempDir()
	hostile := filepath.Join(parent, "dir'; touch pwned; echo '")
	if err := os.Mkdir(hostile, 0o755); err != nil {
		t.Fatalf("failed to create hostile dir: %v", err)
	}

	out, err := exec.Command("sh", "-c", WithDir(hostile, "pwd")).Output()
	if err != nil {
		t.Fatalf("running the composed command failed: %v", err)
	}
	if strings.TrimSpace(string(out)) != hostile {
		t.Errorf("command ran in %q, want %q", strings.TrimSpace(string(out)), hostile)
	}
	if _, err := os.Stat(filepath.Join(parent, "pwned")); err == nil {
		t.Error("injected payload executed: the hostile directory name was re-parsed by the shell")
	}
}

// TestWithDir_FailedChdirDoesNotRunCommand pins the && (not ;) composition:
// if the directory is gone, the command must not run in whatever directory
// lazyboards happens to be in.
func TestWithDir_FailedChdirDoesNotRunCommand(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran")
	missing := filepath.Join(dir, "does-not-exist")

	cmd := exec.Command("sh", "-c", WithDir(missing, "touch "+marker))
	cmd.Stderr = nil
	if err := cmd.Run(); err == nil {
		t.Error("composed command exited 0 with a missing directory, want a non-zero exit")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("command ran even though the chdir failed")
	}
}

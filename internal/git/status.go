package git

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// gitCommandTimeout bounds how long a single git subprocess may run before
// it is killed. This runs on a background poll (see gitStatusPollInterval),
// on every board refresh, and after every successful action; without a
// timeout a hung git process (e.g. lock contention, a stuck credential
// helper) would block indefinitely and leak a blocked goroutine per cycle.
// 3s is generous for local, read-only plumbing commands that should never
// legitimately take that long.
const gitCommandTimeout = 3 * time.Second

// Status represents live git repository state: current branch, staged and
// unstaged file counts, and commits ahead/behind the configured upstream.
type Status struct {
	Branch      string
	Staged      int
	Unstaged    int
	Ahead       int
	Behind      int
	HasUpstream bool
}

// Reader reads live git status from a repository directory.
type Reader interface {
	Read(dir string) (Status, error)
}

// ExecReader reads live git status by shelling out to the git binary.
// All commands use fixed argv slices (no string interpolation of any
// user-controlled input) with cmd.Dir set to the target repository
// directory, per this project's injection-prevention rules.
type ExecReader struct{}

// Read gathers the current branch, staged/unstaged counts, and ahead/behind
// counts for the repository rooted at dir. A missing upstream is treated as
// a graceful partial state (HasUpstream=false), not an error. Failure to
// read the branch name or status (e.g. dir is not a git repository) is a
// hard error.
func (ExecReader) Read(dir string) (Status, error) {
	branch, err := runGitCommand(dir, "branch", "--show-current")
	if err != nil {
		return Status{}, err
	}

	porcelain, err := runGitCommand(dir, "status", "--porcelain")
	if err != nil {
		return Status{}, err
	}

	staged, unstaged := countPorcelainChanges(porcelain)

	status := Status{
		Branch:   strings.TrimSpace(branch),
		Staged:   staged,
		Unstaged: unstaged,
	}

	// A missing upstream causes this command to fail; that's a graceful
	// partial state, not a hard read failure, so ignore the error here.
	revList, err := runGitCommand(dir, "rev-list", "--left-right", "--count", "@{u}...HEAD")
	if err == nil {
		// The command succeeded, so an upstream genuinely exists, regardless
		// of whether we can parse the counts out of its output. Treat a
		// parse failure here as "upstream exists but counts are unknown",
		// not the same as "no upstream configured" (which is the err != nil
		// case above).
		status.HasUpstream = true
		if behind, ahead, parseErr := parseAheadBehind(revList); parseErr == nil {
			status.Behind = behind
			status.Ahead = ahead
		}
	}

	return status, nil
}

// runGitCommand runs git with the given fixed argv in dir and returns stdout.
// The command is bounded by gitCommandTimeout; a timeout surfaces as an
// ordinary error (context.DeadlineExceeded) through the same path as any
// other command failure.
func runGitCommand(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", errors.New(msg)
		}
		return "", err
	}

	return stdout.String(), nil
}

// countPorcelainChanges counts staged and unstaged changes from
// `git status --porcelain` output. Each line's first two columns (XY)
// describe index (staged) and worktree (unstaged) status respectively;
// untracked files ("??") count as unstaged only.
func countPorcelainChanges(porcelain string) (staged, unstaged int) {
	for line := range strings.SplitSeq(porcelain, "\n") {
		if len(line) < 2 {
			continue
		}
		x, y := line[0], line[1]

		if x == '?' && y == '?' {
			unstaged++
			continue
		}
		if x != ' ' {
			staged++
		}
		if y != ' ' {
			unstaged++
		}
	}
	return staged, unstaged
}

// parseAheadBehind parses `git rev-list --left-right --count @{u}...HEAD`
// output ("<behind>\t<ahead>") into behind/ahead counts.
func parseAheadBehind(out string) (behind, ahead int, err error) {
	fields := strings.Fields(out)
	if len(fields) != 2 {
		return 0, 0, errors.New("unexpected rev-list output")
	}

	behind, err = strconv.Atoi(fields[0])
	if err != nil {
		return 0, 0, err
	}
	ahead, err = strconv.Atoi(fields[1])
	if err != nil {
		return 0, 0, err
	}

	return behind, ahead, nil
}

// FakeReader is a test double for Reader that returns a fixed Status/error
// without shelling out.
type FakeReader struct {
	Status Status
	Err    error
}

func (f FakeReader) Read(dir string) (Status, error) {
	return f.Status, f.Err
}

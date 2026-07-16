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

// Status represents live git repository state: current branch, total
// inserted/deleted line counts across the working tree (staged, unstaged,
// and untracked files combined), and commits ahead/behind the configured
// upstream.
type Status struct {
	Branch      string
	Insertions  int
	Deletions   int
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

// Read gathers the current branch, total inserted/deleted line counts, and
// ahead/behind counts for the repository rooted at dir. A missing upstream
// is treated as a graceful partial state (HasUpstream=false), not an error.
// Failure to read the branch name or diff a tracked file is a hard error;
// a single untracked file that can't be diffed (e.g. it vanished mid-poll)
// is skipped rather than failing the whole read.
func (ExecReader) Read(dir string) (Status, error) {
	branch, err := runGitCommand(dir, "branch", "--show-current")
	if err != nil {
		return Status{}, err
	}

	stagedOut, err := runGitCommand(dir, "diff", "--cached", "--numstat")
	if err != nil {
		return Status{}, err
	}
	unstagedOut, err := runGitCommand(dir, "diff", "--numstat")
	if err != nil {
		return Status{}, err
	}

	insertions, deletions := sumNumstat(stagedOut)
	unstagedInsertions, unstagedDeletions := sumNumstat(unstagedOut)
	insertions += unstagedInsertions
	deletions += unstagedDeletions

	// -z: NUL-separated, unquoted paths. Without it, git C-quotes any path
	// containing non-ASCII bytes (e.g. "日本語.txt" becomes the literal
	// string `"\346\227\245..."`), which then fails to resolve as a real
	// file when passed to runGitDiffNoIndex below.
	untrackedOut, err := runGitCommand(dir, "ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return Status{}, err
	}
	for path := range strings.SplitSeq(untrackedOut, "\x00") {
		if path == "" {
			continue
		}
		diffOut, derr := runGitDiffNoIndex(dir, path)
		if derr != nil {
			// Best-effort: a file that vanished or became unreadable between
			// the ls-files listing above and this diff (a rare race, e.g. a
			// fast save-and-delete) shouldn't fail the whole status read;
			// it's simply undercounted for this poll cycle.
			continue
		}
		fileInsertions, _ := sumNumstat(diffOut)
		insertions += fileInsertions
	}

	status := Status{
		Branch:     strings.TrimSpace(branch),
		Insertions: insertions,
		Deletions:  deletions,
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

// sumNumstat sums the insertion/deletion columns of `git diff --numstat`
// (or `--no-index --numstat`) output, one "<added>\t<deleted>\t<path>" line
// per changed file. Binary files report "-" in both numeric columns; those
// lines contribute 0, since a line count is meaningless for binary content.
func sumNumstat(output string) (insertions, deletions int) {
	for line := range strings.SplitSeq(output, "\n") {
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) < 2 {
			continue
		}
		if n, err := strconv.Atoi(fields[0]); err == nil {
			insertions += n
		}
		if n, err := strconv.Atoi(fields[1]); err == nil {
			deletions += n
		}
	}
	return insertions, deletions
}

// runGitDiffNoIndex diffs an untracked file at path (relative to dir)
// against /dev/null via `git diff --no-index`, giving its full content as
// pure insertions without mutating the index (unlike `git add -N`, which
// would stage an empty placeholder entry and be visible to a concurrently
// running `git status`/`git add`). `--no-index` exits 1 (not 0) whenever
// the two sides differ — the expected, common outcome here, not a failure
// — so unlike runGitCommand, exit code 1 with no stderr output is treated
// as success. Anything else (a real exit-code-1 error, e.g. an unreadable
// path, which still prints a message; a killed/timed-out process; any other
// exit code) is a genuine error, matching this file's rule against
// collapsing "expected absence" and "unexpected failure" into one state.
func runGitDiffNoIndex(dir, path string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "diff", "--no-index", "--numstat", "/dev/null", path)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return stdout.String(), nil
	}

	var exitErr *exec.ExitError
	stderrMsg := strings.TrimSpace(stderr.String())
	if ctx.Err() == nil && errors.As(err, &exitErr) && exitErr.ExitCode() == 1 && stderrMsg == "" {
		return stdout.String(), nil
	}

	if stderrMsg != "" {
		return "", errors.New(stderrMsg)
	}
	return "", err
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

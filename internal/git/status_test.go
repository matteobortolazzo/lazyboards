package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runGit runs a git command in dir, failing the test on error.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\noutput: %s", args, err, out)
	}
	return string(out)
}

// initRepo creates a fresh git repo in a new temp dir, on branch "main",
// with a single initial commit so HEAD is valid.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main", ".")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")

	readmePath := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readmePath, []byte("hello\n"), 0644); err != nil {
		t.Fatalf("failed to write initial file: %v", err)
	}
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-q", "-m", "init")

	return dir
}

func TestExecReader_CleanRepo_NoChanges(t *testing.T) {
	dir := initRepo(t)

	status, err := ExecReader{}.Read(dir)
	if err != nil {
		t.Fatalf("Read() returned unexpected error: %v", err)
	}

	if status.Branch != "main" {
		t.Errorf("Branch = %q, want %q", status.Branch, "main")
	}
	if status.Insertions != 0 {
		t.Errorf("Insertions = %d, want 0 for a clean repo", status.Insertions)
	}
	if status.Deletions != 0 {
		t.Errorf("Deletions = %d, want 0 for a clean repo", status.Deletions)
	}
}

func TestExecReader_InsertionsDeletions_StagedUnstagedAndUntracked(t *testing.T) {
	dir := initRepo(t)

	// One staged new file (1 inserted line).
	stagedPath := filepath.Join(dir, "staged.txt")
	if err := os.WriteFile(stagedPath, []byte("staged\n"), 0644); err != nil {
		t.Fatalf("failed to write staged.txt: %v", err)
	}
	runGit(t, dir, "add", "staged.txt")

	// One unstaged modification to the tracked README (1 line replaced: 1
	// insertion + 1 deletion).
	readmePath := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readmePath, []byte("hello again\n"), 0644); err != nil {
		t.Fatalf("failed to modify README.md: %v", err)
	}

	// One untracked file (2 inserted lines, 0 deletions).
	untrackedPath := filepath.Join(dir, "untracked.txt")
	if err := os.WriteFile(untrackedPath, []byte("line one\nline two\n"), 0644); err != nil {
		t.Fatalf("failed to write untracked.txt: %v", err)
	}

	status, err := ExecReader{}.Read(dir)
	if err != nil {
		t.Fatalf("Read() returned unexpected error: %v", err)
	}

	// Total insertions: staged.txt (1) + README.md's replaced line (1) +
	// untracked.txt (2) = 4. Total deletions: README.md's replaced line (1).
	if status.Insertions != 4 {
		t.Errorf("Insertions = %d, want 4 (1 staged + 1 unstaged + 2 untracked)", status.Insertions)
	}
	if status.Deletions != 1 {
		t.Errorf("Deletions = %d, want 1 (README.md's replaced line)", status.Deletions)
	}
}

func TestExecReader_UntrackedFile_CountsAsInsertionsOnlyAndStaysUntracked(t *testing.T) {
	dir := initRepo(t)

	untrackedPath := filepath.Join(dir, "untracked.txt")
	if err := os.WriteFile(untrackedPath, []byte("one\ntwo\nthree\n"), 0644); err != nil {
		t.Fatalf("failed to write untracked.txt: %v", err)
	}

	status, err := ExecReader{}.Read(dir)
	if err != nil {
		t.Fatalf("Read() returned unexpected error: %v", err)
	}

	if status.Insertions != 3 {
		t.Errorf("Insertions = %d, want 3 (every line of the untracked file counts as inserted)", status.Insertions)
	}
	if status.Deletions != 0 {
		t.Errorf("Deletions = %d, want 0 for a brand new untracked file", status.Deletions)
	}

	// Computing the diff must not mutate the index (e.g. via `git add -N`):
	// the file should still show as untracked ("??") afterward.
	porcelain := runGit(t, dir, "status", "--porcelain")
	if !strings.Contains(porcelain, "?? untracked.txt") {
		t.Errorf("git status --porcelain = %q, want untracked.txt to remain untracked after Read()", porcelain)
	}
}

// TestExecReader_UntrackedFile_NonASCIIName_CountsInsertions guards against
// `git ls-files` C-quoting non-ASCII paths (e.g. "日本語.txt" becomes the
// literal string `"\346\227\245..."` without -z), which would then fail to
// resolve as a real file and silently undercount the file's insertions.
func TestExecReader_UntrackedFile_NonASCIIName_CountsInsertions(t *testing.T) {
	dir := initRepo(t)

	untrackedPath := filepath.Join(dir, "日本語.txt")
	if err := os.WriteFile(untrackedPath, []byte("one\ntwo\n"), 0644); err != nil {
		t.Fatalf("failed to write 日本語.txt: %v", err)
	}

	status, err := ExecReader{}.Read(dir)
	if err != nil {
		t.Fatalf("Read() returned unexpected error: %v", err)
	}

	if status.Insertions != 2 {
		t.Errorf("Insertions = %d, want 2 (both lines of the non-ASCII-named untracked file)", status.Insertions)
	}
}

// --- runGitDiffNoIndex: exit-code-1-means-diff-found vs. a genuine error ---
// `--no-index` exits 1 both when the two sides differ (the expected, common
// case) and on a real failure (e.g. an unreadable path); these must not be
// collapsed into the same outcome.

func TestRunGitDiffNoIndex_RealFile_ReturnsNumstatNoError(t *testing.T) {
	dir := initRepo(t)
	path := filepath.Join(dir, "untracked.txt")
	if err := os.WriteFile(path, []byte("one\ntwo\n"), 0644); err != nil {
		t.Fatalf("failed to write untracked.txt: %v", err)
	}

	out, err := runGitDiffNoIndex(dir, "untracked.txt")
	if err != nil {
		t.Fatalf("runGitDiffNoIndex() returned unexpected error: %v", err)
	}
	if insertions, _ := sumNumstat(out); insertions != 2 {
		t.Errorf("sumNumstat(runGitDiffNoIndex output) insertions = %d, want 2", insertions)
	}
}

func TestRunGitDiffNoIndex_UnreadablePath_ReturnsError(t *testing.T) {
	dir := initRepo(t)
	// A directory (not a plain file) makes `git diff --no-index` fail with a
	// real "Could not access" stderr message, still under exit code 1 -- the
	// same exit code as the ordinary diff-found case.
	if err := os.Mkdir(filepath.Join(dir, "adir"), 0755); err != nil {
		t.Fatalf("failed to create adir: %v", err)
	}

	_, err := runGitDiffNoIndex(dir, "adir")
	if err == nil {
		t.Fatal("runGitDiffNoIndex() on a directory path returned nil error, want a genuine error")
	}
}

func TestExecReader_AheadBehind_ViaLocalUpstream(t *testing.T) {
	dir := initRepo(t)

	// Create "feature" branch, diverging from main with one commit each side.
	runGit(t, dir, "checkout", "-q", "-b", "feature")
	featureFile := filepath.Join(dir, "feature.txt")
	if err := os.WriteFile(featureFile, []byte("feature\n"), 0644); err != nil {
		t.Fatalf("failed to write feature.txt: %v", err)
	}
	runGit(t, dir, "add", "feature.txt")
	runGit(t, dir, "commit", "-q", "-m", "feature commit")

	runGit(t, dir, "checkout", "-q", "main")
	mainFile := filepath.Join(dir, "main.txt")
	if err := os.WriteFile(mainFile, []byte("main\n"), 0644); err != nil {
		t.Fatalf("failed to write main.txt: %v", err)
	}
	runGit(t, dir, "add", "main.txt")
	runGit(t, dir, "commit", "-q", "-m", "main commit")

	runGit(t, dir, "checkout", "-q", "feature")
	// Track "main" as feature's upstream (a purely local ref, no network needed).
	runGit(t, dir, "branch", "--set-upstream-to=main", "feature")

	status, err := ExecReader{}.Read(dir)
	if err != nil {
		t.Fatalf("Read() returned unexpected error: %v", err)
	}

	if status.Branch != "feature" {
		t.Errorf("Branch = %q, want %q", status.Branch, "feature")
	}
	if !status.HasUpstream {
		t.Errorf("HasUpstream = false, want true after --set-upstream-to")
	}
	if status.Ahead != 1 {
		t.Errorf("Ahead = %d, want 1 (feature has 1 commit main doesn't)", status.Ahead)
	}
	if status.Behind != 1 {
		t.Errorf("Behind = %d, want 1 (main has 1 commit feature doesn't)", status.Behind)
	}
}

func TestExecReader_NoUpstream_GracefulState(t *testing.T) {
	dir := initRepo(t)

	status, err := ExecReader{}.Read(dir)
	if err != nil {
		t.Fatalf("Read() returned unexpected error: %v, want no-upstream to be a graceful non-error state", err)
	}

	if status.HasUpstream {
		t.Errorf("HasUpstream = true, want false when no upstream is configured")
	}
	if status.Branch != "main" {
		t.Errorf("Branch = %q, want %q even without an upstream", status.Branch, "main")
	}
}

func TestExecReader_NonRepoDir_HardError(t *testing.T) {
	dir := t.TempDir() // not a git repo

	_, err := ExecReader{}.Read(dir)
	if err == nil {
		t.Fatal("Read() on a non-repo directory returned nil error, want a hard error")
	}
}

func TestFakeReader_ReturnsConfiguredStatusAndError(t *testing.T) {
	want := Status{Branch: "main", Insertions: 2, Deletions: 1, Ahead: 3, Behind: 0, HasUpstream: true}
	fr := FakeReader{Status: want}

	got, err := fr.Read("irrelevant")
	if err != nil {
		t.Fatalf("Read() returned unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("Read() = %+v, want %+v", got, want)
	}
}

func TestFakeReader_ReturnsConfiguredError(t *testing.T) {
	fr := FakeReader{Err: os.ErrPermission}

	_, err := fr.Read("irrelevant")
	if err == nil {
		t.Fatal("Read() returned nil error, want the configured error")
	}
}

func TestParseAheadBehind(t *testing.T) {
	tests := []struct {
		name       string
		in         string
		wantBehind int
		wantAhead  int
		wantErr    bool
	}{
		{
			name:       "well-formed tab-separated counts",
			in:         "2\t3",
			wantBehind: 2,
			wantAhead:  3,
		},
		{
			name:       "well-formed with surrounding and extra whitespace",
			in:         "  1   4  \n",
			wantBehind: 1,
			wantAhead:  4,
		},
		{
			name:    "empty output",
			in:      "",
			wantErr: true,
		},
		{
			name:    "single number only",
			in:      "2",
			wantErr: true,
		},
		{
			name:    "non-numeric fields",
			in:      "abc\tdef",
			wantErr: true,
		},
		{
			name:    "too many fields",
			in:      "1 2 3",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			behind, ahead, err := parseAheadBehind(tt.in)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseAheadBehind(%q) returned nil error, want an error", tt.in)
				}
				return
			}

			if err != nil {
				t.Fatalf("parseAheadBehind(%q) returned unexpected error: %v", tt.in, err)
			}
			if behind != tt.wantBehind {
				t.Errorf("parseAheadBehind(%q) behind = %d, want %d", tt.in, behind, tt.wantBehind)
			}
			if ahead != tt.wantAhead {
				t.Errorf("parseAheadBehind(%q) ahead = %d, want %d", tt.in, ahead, tt.wantAhead)
			}
		})
	}
}

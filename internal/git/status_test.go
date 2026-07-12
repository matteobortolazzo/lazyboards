package git

import (
	"os"
	"os/exec"
	"path/filepath"
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
	if status.Staged != 0 {
		t.Errorf("Staged = %d, want 0 for a clean repo", status.Staged)
	}
	if status.Unstaged != 0 {
		t.Errorf("Unstaged = %d, want 0 for a clean repo", status.Unstaged)
	}
}

func TestExecReader_StagedUnstagedUntrackedCounts(t *testing.T) {
	dir := initRepo(t)

	// One staged new file.
	stagedPath := filepath.Join(dir, "staged.txt")
	if err := os.WriteFile(stagedPath, []byte("staged\n"), 0644); err != nil {
		t.Fatalf("failed to write staged.txt: %v", err)
	}
	runGit(t, dir, "add", "staged.txt")

	// One unstaged modification to the tracked README.
	readmePath := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readmePath, []byte("hello again\n"), 0644); err != nil {
		t.Fatalf("failed to modify README.md: %v", err)
	}

	// One untracked file.
	untrackedPath := filepath.Join(dir, "untracked.txt")
	if err := os.WriteFile(untrackedPath, []byte("untracked\n"), 0644); err != nil {
		t.Fatalf("failed to write untracked.txt: %v", err)
	}

	status, err := ExecReader{}.Read(dir)
	if err != nil {
		t.Fatalf("Read() returned unexpected error: %v", err)
	}

	// staged.txt (staged add) + untracked.txt (untracked, counted as staged-worthy
	// change only if the implementation treats "??" separately) -- assert the two
	// distinct categories independently below rather than guessing combined totals.
	if status.Staged != 1 {
		t.Errorf("Staged = %d, want 1 (staged.txt added to index)", status.Staged)
	}
	// Unstaged should count both the modified tracked file and the untracked file
	// as "not yet staged" changes.
	if status.Unstaged != 2 {
		t.Errorf("Unstaged = %d, want 2 (modified README.md + untracked.txt)", status.Unstaged)
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
	want := Status{Branch: "main", Staged: 2, Unstaged: 1, Ahead: 3, Behind: 0, HasUpstream: true}
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

package action

// This file asserts a structural absence, not observable behavior: whether
// the Windows-only URL opener avoids os/exec (and os) cannot be exercised by
// any behavioral test running on this Linux CI runner, because a
// windows-only file's code only compiles/executes under GOOS=windows. The
// only way to guard the "no shell launderer on Windows" invariant here is
// to inspect the source structurally via go/build and go/parser. This is a
// deliberate, documented exception to .claude/rules/testing.md's "assert
// behavior not implementation" rule (see docs/shell-and-url-safety.md for
// the underlying vulnerability this guards against: a repo-local keymap
// action URL containing shell metacharacters reaching `cmd /c start`).

import (
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWindowsOnlyFiles_DoNotImportOSExec walks the internal/action package
// directory, selects every file that is included when GOOS=windows but
// excluded under both GOOS=linux and GOOS=darwin (via go/build's own build
// constraint matching -- no hardcoded filename), and asserts none of them
// imports os/exec or os. The Windows opener must use
// golang.org/x/sys/windows.ShellExecute instead of shelling out.
func TestWindowsOnlyFiles_DoNotImportOSExec(t *testing.T) {
	const pkgDir = "."

	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		t.Fatalf("ReadDir(%q) error = %v", pkgDir, err)
	}

	var windowsOnlyFiles []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			continue
		}
		name := entry.Name()
		// Test files are a separate concern from the production opener this
		// guard targets; excluding them also prevents this guard's own
		// windows-only companion test file from ever satisfying the
		// "windows-only file exists" requirement in place of the real
		// production implementation.
		if strings.HasSuffix(name, "_test.go") {
			continue
		}

		includedOnWindows := matchesGOOS(t, pkgDir, name, "windows")
		includedOnLinux := matchesGOOS(t, pkgDir, name, "linux")
		includedOnDarwin := matchesGOOS(t, pkgDir, name, "darwin")

		if includedOnWindows && !includedOnLinux && !includedOnDarwin {
			windowsOnlyFiles = append(windowsOnlyFiles, name)
		}
	}

	if len(windowsOnlyFiles) == 0 {
		t.Fatal("no windows-only file found in internal/action -- expected a build-tag-separated Windows URL opener (e.g. executor_windows.go, excluded on linux/darwin) implementing OpenURL via golang.org/x/sys/windows.ShellExecute; if this legitimately changed, update this guard")
	}

	for _, name := range windowsOnlyFiles {
		path := filepath.Join(pkgDir, name)
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("ParseFile(%q) error = %v", path, err)
		}
		for _, imp := range f.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			switch importPath {
			case "os/exec":
				t.Errorf("%s imports os/exec -- the Windows URL opener must not shell out; use golang.org/x/sys/windows.ShellExecute instead", name)
			case "os":
				t.Errorf("%s imports os -- the Windows URL opener must not use the os package", name)
			}
		}
	}
}

// matchesGOOS reports whether the named file would be included in a build
// for the given GOOS, per go/build's own filename- and build-constraint-tag
// matching rules.
func matchesGOOS(t *testing.T, dir, name, goos string) bool {
	t.Helper()
	ctx := build.Default
	ctx.GOOS = goos
	match, err := ctx.MatchFile(dir, name)
	if err != nil {
		t.Fatalf("MatchFile(dir=%q, name=%q, GOOS=%q) error = %v", dir, name, goos, err)
	}
	return match
}

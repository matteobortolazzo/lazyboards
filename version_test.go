package main

import (
	"strings"
	"testing"
)

func TestAppVersion_UsesInjectedVersion(t *testing.T) {
	saved := version
	t.Cleanup(func() { version = saved })

	version = "1.2.3"
	if got := appVersion(); got != "1.2.3" {
		t.Errorf("appVersion() = %q, want %q", got, "1.2.3")
	}
}

func TestAppVersion_FallsBackToDev(t *testing.T) {
	saved := version
	t.Cleanup(func() { version = saved })

	// With no injected version, the test binary's build info reports
	// "(devel)"/empty, so appVersion() falls back to "dev".
	version = ""
	if got := appVersion(); got != "dev" {
		t.Errorf("appVersion() = %q, want %q", got, "dev")
	}
}

func TestVersionRequested(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"long flag", []string{"lazyboards", "--version"}, true},
		{"short flag", []string{"lazyboards", "-v"}, true},
		{"subcommand", []string{"lazyboards", "version"}, true},
		{"no args", []string{"lazyboards"}, false},
		{"unrelated arg", []string{"lazyboards", "--help"}, false},
		{"empty", []string{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := versionRequested(tt.args); got != tt.want {
				t.Errorf("versionRequested(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestBuildHelpContent_IncludesVersion(t *testing.T) {
	b := newLoadedTestBoard(t)
	content := b.buildHelpContent()
	if !strings.Contains(content, "lazyboards") {
		t.Errorf("buildHelpContent() should mention %q, got:\n%s", "lazyboards", content)
	}
	if !strings.Contains(content, appVersion()) {
		t.Errorf("buildHelpContent() should include version %q, got:\n%s", appVersion(), content)
	}
}

// --- versionNewer (#444) ---
//
// versionNewer parses each version string into numeric dot-separated
// components (stripping a leading v/V from both operands first) and compares
// them numerically, not lexically -- so e.g. 1.9.0 is correctly older than
// 1.10.0 despite "1.10.0" < "1.9.0" as a plain string.

func TestVersionNewer_VPrefixNormalizedOnBothOperands(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
	}{
		{"both v-prefixed", "v1.0.0", "v1.1.0"},
		{"current bare, latest v-prefixed", "1.0.0", "v1.1.0"},
		{"current v-prefixed, latest bare", "v1.0.0", "1.1.0"},
		{"both bare", "1.0.0", "1.1.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !versionNewer(tt.current, tt.latest) {
				t.Errorf("versionNewer(%q, %q) = false, want true", tt.current, tt.latest)
			}
		})
	}
}

func TestVersionNewer_NumericNotLexicalComparison(t *testing.T) {
	// Lexical string comparison would say "1.10.0" < "1.9.0" (since '1' < '9'
	// byte-wise), incorrectly treating 1.10.0 as older. Numeric comparison of
	// the parsed minor component (10 > 9) must get this right.
	if !versionNewer("1.9.0", "1.10.0") {
		t.Error("versionNewer(\"1.9.0\", \"1.10.0\") = false, want true (numeric minor-version comparison, not lexical)")
	}
	if versionNewer("1.10.0", "1.9.0") {
		t.Error("versionNewer(\"1.10.0\", \"1.9.0\") = true, want false (1.9.0 is older than 1.10.0)")
	}
}

func TestVersionNewer_EqualVersionsReturnsFalse(t *testing.T) {
	if versionNewer("v1.2.3", "v1.2.3") {
		t.Error("versionNewer(equal versions) = true, want false")
	}
}

func TestVersionNewer_OlderLatestReturnsFalse(t *testing.T) {
	if versionNewer("v1.2.3", "v1.2.2") {
		t.Error("versionNewer(current newer than latest) = true, want false")
	}
}

func TestVersionNewer_MalformedComponent_FailsSafeToFalse(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
	}{
		{"non-numeric patch in latest", "v1.2.3", "v1.2.x"},
		{"completely non-version latest", "v1.2.3", "not-a-version"},
		{"non-numeric current", "not-a-version", "v1.2.3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if versionNewer(tt.current, tt.latest) {
				t.Errorf("versionNewer(%q, %q) = true, want false (fail-safe on malformed component)", tt.current, tt.latest)
			}
		})
	}
}

// --- shouldCheckForUpdate (#444) ---

func TestShouldCheckForUpdate_DevVersionSkipsCheck(t *testing.T) {
	if shouldCheckForUpdate("dev", true) {
		t.Error("shouldCheckForUpdate(\"dev\", true) = true, want false (dev builds skip the check entirely)")
	}
}

func TestShouldCheckForUpdate_DisabledSkipsCheck(t *testing.T) {
	if shouldCheckForUpdate("v1.0.0", false) {
		t.Error("shouldCheckForUpdate(\"v1.0.0\", false) = true, want false (config disables the check)")
	}
}

func TestShouldCheckForUpdate_EnabledNonDevVersionRunsCheck(t *testing.T) {
	if !shouldCheckForUpdate("v1.0.0", true) {
		t.Error("shouldCheckForUpdate(\"v1.0.0\", true) = false, want true")
	}
}

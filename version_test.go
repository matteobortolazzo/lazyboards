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

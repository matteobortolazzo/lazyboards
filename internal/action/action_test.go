package action

import (
	"testing"
)

// --- Slugify ---

func TestSlugify_SpacesToHyphens(t *testing.T) {
	got := Slugify("Hello World")
	want := "hello-world"
	if got != want {
		t.Errorf("Slugify(%q) = %q, want %q", "Hello World", got, want)
	}
}

func TestSlugify_SpecialCharsRemoved(t *testing.T) {
	got := Slugify("Fix bug #42!")
	want := "fix-bug-42"
	if got != want {
		t.Errorf("Slugify(%q) = %q, want %q", "Fix bug #42!", got, want)
	}
}

func TestSlugify_ConsecutiveHyphensCollapsed(t *testing.T) {
	got := Slugify("a--b")
	want := "a-b"
	if got != want {
		t.Errorf("Slugify(%q) = %q, want %q", "a--b", got, want)
	}
}

func TestSlugify_LeadingTrailingHyphensTrimmed(t *testing.T) {
	got := Slugify("-hello-")
	want := "hello"
	if got != want {
		t.Errorf("Slugify(%q) = %q, want %q", "-hello-", got, want)
	}
}

func TestSlugify_UnicodeAlphanumericPreserved(t *testing.T) {
	input := "Cafe123"
	got := Slugify(input)
	want := "cafe123"
	if got != want {
		t.Errorf("Slugify(%q) = %q, want %q", input, got, want)
	}
}

func TestSlugify_EmptyString(t *testing.T) {
	got := Slugify("")
	if got != "" {
		t.Errorf("Slugify(%q) = %q, want empty string", "", got)
	}
}

func TestSlugify_AlreadySlugified(t *testing.T) {
	input := "hello-world"
	got := Slugify(input)
	if got != input {
		t.Errorf("Slugify(%q) = %q, want %q (unchanged)", input, got, input)
	}
}

// --- ExpandTemplate ---

func TestExpandTemplate_AllVariablesExpanded(t *testing.T) {
	template := "{number}-{title}-{tags}-{repo_owner}-{repo_name}-{provider}-{session}-{window}"
	vars := map[string]string{
		"number":     "42",
		"title":      "add-actions",
		"tags":       "bug,feature",
		"repo_owner": "matteobortolazzo",
		"repo_name":  "lazyboards",
		"provider":   "github",
		"session":    "42-add-actions",
		"window":     "42-refine",
	}
	got := ExpandTemplate(template, vars)
	want := "42-add-actions-bug,feature-matteobortolazzo-lazyboards-github-42-add-actions-42-refine"
	if got != want {
		t.Errorf("ExpandTemplate() = %q, want %q", got, want)
	}
}

func TestExpandTemplate_UnknownPlaceholdersLeftAsIs(t *testing.T) {
	template := "hello {unknown} world"
	vars := map[string]string{}
	got := ExpandTemplate(template, vars)
	want := "hello {unknown} world"
	if got != want {
		t.Errorf("ExpandTemplate() = %q, want %q (unknown placeholders should remain)", got, want)
	}
}

func TestExpandTemplate_EmptyValueExpandsToEmpty(t *testing.T) {
	template := "prefix-{tags}-suffix"
	vars := map[string]string{
		"tags": "",
	}
	got := ExpandTemplate(template, vars)
	want := "prefix--suffix"
	if got != want {
		t.Errorf("ExpandTemplate() = %q, want %q (empty value should expand to empty)", got, want)
	}
}

func TestExpandTemplate_MultipleOccurrencesExpanded(t *testing.T) {
	template := "{number}/{number}/{number}"
	vars := map[string]string{
		"number": "42",
	}
	got := ExpandTemplate(template, vars)
	want := "42/42/42"
	if got != want {
		t.Errorf("ExpandTemplate() = %q, want %q (all occurrences should expand)", got, want)
	}
}

func TestExpandTemplate_NoVariables(t *testing.T) {
	template := "plain string with no vars"
	vars := map[string]string{
		"number": "42",
	}
	got := ExpandTemplate(template, vars)
	if got != template {
		t.Errorf("ExpandTemplate() = %q, want %q (unchanged)", got, template)
	}
}

// --- ShellEscape ---

func TestShellEscape_SimpleString(t *testing.T) {
	got := ShellEscape("hello")
	want := "'hello'"
	if got != want {
		t.Errorf("ShellEscape(%q) = %q, want %q", "hello", got, want)
	}
}

func TestShellEscape_SingleQuote(t *testing.T) {
	got := ShellEscape("it's")
	want := "'it'\\''s'"
	if got != want {
		t.Errorf("ShellEscape(%q) = %q, want %q", "it's", got, want)
	}
}

func TestShellEscape_ShellMetachars(t *testing.T) {
	input := "; rm -rf / #"
	got := ShellEscape(input)
	want := "'; rm -rf / #'"
	if got != want {
		t.Errorf("ShellEscape(%q) = %q, want %q", input, got, want)
	}
}

func TestShellEscape_EmptyString(t *testing.T) {
	got := ShellEscape("")
	want := "''"
	if got != want {
		t.Errorf("ShellEscape(%q) = %q, want %q", "", got, want)
	}
}

func TestBuildShellSafeVars_EscapesAllValues(t *testing.T) {
	vars := map[string]string{
		"number": "42",
		"tags":   "bug,feature",
	}
	safe := BuildShellSafeVars(vars)
	if safe["number"] != "'42'" {
		t.Errorf("number = %q, want %q", safe["number"], "'42'")
	}
	if safe["tags"] != "'bug,feature'" {
		t.Errorf("tags = %q, want %q", safe["tags"], "'bug,feature'")
	}
}

// --- BuildSessionName ---

func TestBuildSessionName_NoTruncation(t *testing.T) {
	got := BuildSessionName(80, "New parameter to pass", 32)
	want := "80-new-parameter-to-pass"
	if got != want {
		t.Errorf("BuildSessionName() = %q, want %q", got, want)
	}
}

func TestBuildSessionName_TruncatesAtWordBoundary(t *testing.T) {
	got := BuildSessionName(99, "Implement user authentication flow for OAuth", 32)
	want := "99-implement-user-authentication"
	if got != want {
		t.Errorf("BuildSessionName() = %q, want %q", got, want)
	}
}

func TestBuildSessionName_EmptyTitle(t *testing.T) {
	got := BuildSessionName(42, "", 32)
	want := "42"
	if got != want {
		t.Errorf("BuildSessionName() = %q, want %q", got, want)
	}
}

func TestBuildSessionName_TitleSlugsToEmpty(t *testing.T) {
	got := BuildSessionName(42, "!!!", 32)
	want := "42"
	if got != want {
		t.Errorf("BuildSessionName() = %q, want %q", got, want)
	}
}

func TestBuildSessionName_LongNumber(t *testing.T) {
	// Number + hyphen already leaves only 1 char of room — hard cut mid-slug.
	got := BuildSessionName(123456789012345678, "title", 20)
	want := "123456789012345678-t"
	if got != want {
		t.Errorf("BuildSessionName() = %q, want %q", got, want)
	}
}

func TestBuildSessionName_SingleLongWord(t *testing.T) {
	// Title is a single word with no hyphen after the number prefix — hard
	// cut still applies mid-word, it does not back off to the bare prefix.
	got := BuildSessionName(99, "superlongwordwithnobreakpoints", 32)
	want := "99-superlongwordwithnobreakpoint"
	if got != want {
		t.Errorf("BuildSessionName() = %q, want %q", got, want)
	}
}

func TestBuildSessionName_CustomMaxLen(t *testing.T) {
	// "1-short-title" is 13 chars; with maxLen=10, hard cut at 10 runes.
	got := BuildSessionName(1, "short title", 10)
	want := "1-short-ti"
	if got != want {
		t.Errorf("BuildSessionName() = %q, want %q", got, want)
	}
}

// cenci's own window-name truncation (cenci/watch internal/run/slug.go:
// capName) does a hard cut at maxLen runes and trims only trailing hyphens —
// it never backs off to the last complete segment. lazyboards must produce
// byte-for-byte the same join key the daemon broadcasts as WindowName, or the
// exact-equality lookup in agentStatusForNumber silently never matches and no
// badge is shown. This reproduces a real title (#270) whose 40-char cutoff
// does not land on a hyphen boundary.
func TestBuildSessionName_MatchesCenciHardCutTruncation(t *testing.T) {
	title := "Lazygit-style git integration: live git status display (1/2)"
	got := BuildSessionName(270, title, 40)
	want := "270-lazygit-style-git-integration-live-g"
	if got != want {
		t.Errorf("BuildSessionName() = %q, want %q (must match cenci's capName truncation)", got, want)
	}
}

// The following BuildSessionName tests are regression tests against
// cenci v1.12.0's own window-name algorithm (internal/run/slug.go:
// slugify + capName). cenci's slugify keeps only ASCII a-z0-9, maps
// space/underscore/hyphen to a single dash, and DROPS all other runes
// (punctuation, non-ASCII) without inserting a hyphen. capName hard-cuts at
// 40 runes and trims only trailing dashes. lazyboards' BuildSessionName must
// produce byte-for-byte the same join key the daemon broadcasts, or the
// exact-equality lookup silently never matches and no agent badge is shown.

// Reproduces the original bug report: punctuation like "(", "/", ")" must be
// dropped entirely, not hyphenated. The old (wrong) behavior would turn
// "(2/4)" into "2-4" via hyphen-insertion; cenci's real algorithm drops
// each punctuation rune with nothing in its place, yielding "24".
func TestBuildSessionName_PunctuationDroppedNotHyphenated(t *testing.T) {
	got := BuildSessionName(1, "(2/4)", 40)
	want := "1-24"
	if got != want {
		t.Errorf("BuildSessionName() = %q, want %q (cenci drops punctuation, does not hyphenate it)", got, want)
	}
}

// Apostrophe and colon touching adjacent letters must be dropped without
// inserting a separator, while the real word-boundary spaces still become
// single dashes.
func TestBuildSessionName_ApostropheColonTouchingWord(t *testing.T) {
	got := BuildSessionName(5, "Don't crash: retry", 40)
	want := "5-dont-crash-retry"
	if got != want {
		t.Errorf("BuildSessionName() = %q, want %q (apostrophe/colon must be dropped, not hyphenated)", got, want)
	}
}

// Non-ASCII letters (accented Latin here) must be dropped entirely, not
// transliterated or kept as Unicode letters. "café münchen" -> "é" and "ü"
// are dropped -> "caf" + dash (from the space) + "mnchen".
func TestBuildSessionName_NonASCIIDroppedEntirely(t *testing.T) {
	got := BuildSessionName(8, "café münchen", 40)
	want := "8-caf-mnchen"
	if got != want {
		t.Errorf("BuildSessionName() = %q, want %q (non-ASCII runes must be dropped entirely, not preserved)", got, want)
	}
}

// Truncation must hard-cut at exactly the new 40-rune cap (not the old
// 32-rune cap), with the cut landing mid-word past a dropped-punctuation
// segment. The title below has a "parser(core)" segment whose parentheses
// are dropped (not hyphenated), producing a slug that only crosses the
// 40-rune total length several words later, inside "punctuation". The hard
// cut at rune 40 lands mid-word ("punctuatio"), and since the 40th rune is
// not a hyphen, no trailing-dash trim occurs.
func TestBuildSessionName_HardCutAtNew40RuneCap(t *testing.T) {
	title := "Update parser(core) to handle punctuation edge cases in titles"
	got := BuildSessionName(9, title, 40)
	want := "9-update-parsercore-to-handle-punctuatio"
	if got != want {
		t.Errorf("BuildSessionName() = %q, want %q (must hard-cut at the 40-rune cap, matching cenci's capName)", got, want)
	}
	if len([]rune(got)) != 40 {
		t.Errorf("BuildSessionName() result has %d runes, want exactly 40", len([]rune(got)))
	}
}

// Control case: an ordinary title with only spaces as separators and no
// punctuation or truncation should be unaffected by the algorithm change.
func TestBuildSessionName_ControlCaseNoTruncation(t *testing.T) {
	got := BuildSessionName(80, "New parameter to pass", 40)
	want := "80-new-parameter-to-pass"
	if got != want {
		t.Errorf("BuildSessionName() = %q, want %q", got, want)
	}
}

// --- URLEscape ---

func TestURLEscape_SimpleString(t *testing.T) {
	got := URLEscape("hello")
	want := "hello"
	if got != want {
		t.Errorf("URLEscape(%q) = %q, want %q", "hello", got, want)
	}
}

func TestURLEscape_SpecialChars(t *testing.T) {
	input := "bug&fix?v2=yes#top 100%"
	got := URLEscape(input)
	want := "bug%26fix%3Fv2%3Dyes%23top+100%25"
	if got != want {
		t.Errorf("URLEscape(%q) = %q, want %q", input, got, want)
	}
}

func TestURLEscape_EmptyString(t *testing.T) {
	got := URLEscape("")
	want := ""
	if got != want {
		t.Errorf("URLEscape(%q) = %q, want %q", "", got, want)
	}
}

func TestBuildURLSafeVars_EscapesAllValues(t *testing.T) {
	vars := map[string]string{
		"number": "42",
		"tags":   "bug&fix,feature?v2",
	}
	safe := BuildURLSafeVars(vars)
	if safe["number"] != "42" {
		t.Errorf("number = %q, want %q", safe["number"], "42")
	}
	expectedTags := "bug%26fix%2Cfeature%3Fv2"
	if safe["tags"] != expectedTags {
		t.Errorf("tags = %q, want %q", safe["tags"], expectedTags)
	}
}

// --- BuildBoardTemplateVars ---

func TestBuildBoardTemplateVars_ReturnsOnlyBoardVars(t *testing.T) {
	vars := BuildBoardTemplateVars("matteobortolazzo", "lazyboards", "github", "")

	// Should contain exactly 4 keys: repo_owner, repo_name, provider, comment.
	expectedKeys := []string{"repo_owner", "repo_name", "provider", "comment"}
	if len(vars) != len(expectedKeys) {
		t.Fatalf("BuildBoardTemplateVars() returned %d keys, want %d", len(vars), len(expectedKeys))
	}
	for _, key := range expectedKeys {
		if _, ok := vars[key]; !ok {
			t.Errorf("BuildBoardTemplateVars() missing key %q", key)
		}
	}

	// Verify values.
	if vars["repo_owner"] != "matteobortolazzo" {
		t.Errorf("vars[repo_owner] = %q, want %q", vars["repo_owner"], "matteobortolazzo")
	}
	if vars["repo_name"] != "lazyboards" {
		t.Errorf("vars[repo_name] = %q, want %q", vars["repo_name"], "lazyboards")
	}
	if vars["provider"] != "github" {
		t.Errorf("vars[provider] = %q, want %q", vars["provider"], "github")
	}
}

// --- BuildTemplateVars comment parameter ---

func TestBuildTemplateVars_IncludesCommentVariable(t *testing.T) {
	comment := "this is my comment"
	window := "42-refine"
	vars := BuildTemplateVars(42, "Test Title", []string{"bug"}, "owner", "repo", "github", 32, comment, window)

	got, ok := vars["comment"]
	if !ok {
		t.Fatal("BuildTemplateVars() missing key \"comment\"")
	}
	if got != comment {
		t.Errorf("vars[comment] = %q, want %q", got, comment)
	}
}

// --- BuildTemplateVars window parameter (#309) ---

// {window} must expand to whatever live cenci window name the caller
// resolved (e.g. "1-refine"), not something BuildTemplateVars derives itself.
// BuildTemplateVars is a pure passthrough here: resolution (live window vs.
// {session} fallback) is the caller's responsibility (see resolveWindowName
// in update.go).
func TestBuildTemplateVars_IncludesWindowVariable(t *testing.T) {
	window := "1-refine"
	vars := BuildTemplateVars(1, "Setup CI", []string{"infra"}, "owner", "repo", "github", 32, "", window)

	got, ok := vars["window"]
	if !ok {
		t.Fatal("BuildTemplateVars() missing key \"window\"")
	}
	if got != window {
		t.Errorf("vars[window] = %q, want %q", got, window)
	}
}

func TestBuildBoardTemplateVars_IncludesCommentVariable(t *testing.T) {
	comment := "board-level comment"
	vars := BuildBoardTemplateVars("owner", "repo", "github", comment)

	got, ok := vars["comment"]
	if !ok {
		t.Fatal("BuildBoardTemplateVars() missing key \"comment\"")
	}
	if got != comment {
		t.Errorf("vars[comment] = %q, want %q", got, comment)
	}
}

// --- BuildPRTemplateVars (#340) ---

func TestBuildPRTemplateVars_MergesBaseAndPRVars(t *testing.T) {
	base := map[string]string{
		"number": "42",
		"title":  "add-actions",
	}
	got := BuildPRTemplateVars(base, 10, "Feat: add PR support", "https://github.com/owner/repo/pull/10", "feature/add-pr-support", "/repo/.worktrees/add-pr-support")

	expectedKeys := []string{"number", "title", "pr_branch", "pr_number", "pr_url", "pr_title", "pr_worktree"}
	if len(got) != len(expectedKeys) {
		t.Fatalf("BuildPRTemplateVars() returned %d keys, want %d", len(got), len(expectedKeys))
	}
	for _, key := range expectedKeys {
		if _, ok := got[key]; !ok {
			t.Errorf("BuildPRTemplateVars() missing key %q", key)
		}
	}
	if got["number"] != "42" {
		t.Errorf("vars[number] = %q, want %q (base value preserved)", got["number"], "42")
	}
	if got["title"] != "add-actions" {
		t.Errorf("vars[title] = %q, want %q (base value preserved)", got["title"], "add-actions")
	}
}

func TestBuildPRTemplateVars_DoesNotMutateBaseMap(t *testing.T) {
	base := map[string]string{"number": "42"}
	_ = BuildPRTemplateVars(base, 10, "Some title", "https://example.com/pull/10", "some-branch", "/repo/worktree")

	if len(base) != 1 {
		t.Errorf("BuildPRTemplateVars() mutated the caller's base map: got %d keys, want 1", len(base))
	}
	if _, ok := base["pr_number"]; ok {
		t.Error("BuildPRTemplateVars() must return a copy, not add pr_number to the caller's base map")
	}
}

func TestBuildPRTemplateVars_PRTitleSlugified(t *testing.T) {
	base := map[string]string{}
	got := BuildPRTemplateVars(base, 1, "Fix Bug #42!", "https://example.com", "some-branch", "/repo/worktree")

	want := Slugify("Fix Bug #42!")
	if got["pr_title"] != want {
		t.Errorf("vars[pr_title] = %q, want %q (slugified)", got["pr_title"], want)
	}
}

func TestBuildPRTemplateVars_PRNumberFormatted(t *testing.T) {
	base := map[string]string{}
	got := BuildPRTemplateVars(base, 99, "Some title", "https://example.com", "some-branch", "/repo/worktree")

	if got["pr_number"] != "99" {
		t.Errorf("vars[pr_number] = %q, want %q", got["pr_number"], "99")
	}
}

func TestBuildPRTemplateVars_BranchAndURLPassThroughRaw(t *testing.T) {
	base := map[string]string{}
	branch := "feature/my-branch"
	url := "https://github.com/owner/repo/pull/10"
	worktree := "/repo/.worktrees/my-branch"
	got := BuildPRTemplateVars(base, 10, "Some title", url, branch, worktree)

	if got["pr_branch"] != branch {
		t.Errorf("vars[pr_branch] = %q, want %q (raw passthrough; escaping happens later)", got["pr_branch"], branch)
	}
	if got["pr_url"] != url {
		t.Errorf("vars[pr_url] = %q, want %q (raw passthrough; escaping happens later)", got["pr_url"], url)
	}
	if got["pr_worktree"] != worktree {
		t.Errorf("vars[pr_worktree] = %q, want %q (raw passthrough; escaping happens later)", got["pr_worktree"], worktree)
	}
}

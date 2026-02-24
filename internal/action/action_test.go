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
	template := "{number}-{title}-{tags}-{repo_owner}-{repo_name}-{provider}-{session}"
	vars := map[string]string{
		"number":     "42",
		"title":      "add-actions",
		"tags":       "bug,feature",
		"repo_owner": "matteobortolazzo",
		"repo_name":  "lazyboards",
		"provider":   "github",
		"session":    "42-add-actions",
	}
	got := ExpandTemplate(template, vars)
	want := "42-add-actions-bug,feature-matteobortolazzo-lazyboards-github-42-add-actions"
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
	// Number + hyphen already exceeds maxLen — return number only.
	got := BuildSessionName(123456789012345678, "title", 20)
	want := "123456789012345678"
	if got != want {
		t.Errorf("BuildSessionName() = %q, want %q", got, want)
	}
}

func TestBuildSessionName_SingleLongWord(t *testing.T) {
	// Title is a single word that can't be split at hyphens after number prefix.
	got := BuildSessionName(99, "superlongwordwithnobreakpoints", 32)
	want := "99"
	if got != want {
		t.Errorf("BuildSessionName() = %q, want %q", got, want)
	}
}

func TestBuildSessionName_CustomMaxLen(t *testing.T) {
	// "1-short-title" is 13 chars; with maxLen=10, should truncate to "1-short"
	got := BuildSessionName(1, "short title", 10)
	want := "1-short"
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
	vars := BuildBoardTemplateVars("matteobortolazzo", "lazyboards", "github")

	// Should contain exactly 3 keys.
	expectedKeys := []string{"repo_owner", "repo_name", "provider"}
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

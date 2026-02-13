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
	template := "{number}-{title}-{tags}-{repo_owner}-{repo_name}-{provider}"
	vars := map[string]string{
		"number":     "42",
		"title":      "add-actions",
		"tags":       "bug,feature",
		"repo_owner": "matteobortolazzo",
		"repo_name":  "lazyboards",
		"provider":   "github",
	}
	got := ExpandTemplate(template, vars)
	want := "42-add-actions-bug,feature-matteobortolazzo-lazyboards-github"
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

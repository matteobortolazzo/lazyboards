package action

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

// consecutiveHyphens matches one or more consecutive hyphens.
var consecutiveHyphens = regexp.MustCompile(`-+`)

// Slugify converts a string to a URL-friendly slug.
// Lowercase, alphanumeric and hyphens only, no consecutive/leading/trailing hyphens.
func Slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	result := consecutiveHyphens.ReplaceAllString(b.String(), "-")
	return strings.Trim(result, "-")
}

// ShellEscape wraps a string in single quotes for safe use in shell commands.
// Any embedded single quotes are escaped with the '\'' idiom.
func ShellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// BuildShellSafeVars creates a variable map with all values shell-escaped.
func BuildShellSafeVars(vars map[string]string) map[string]string {
	safe := make(map[string]string, len(vars))
	for k, v := range vars {
		safe[k] = ShellEscape(v)
	}
	return safe
}

// URLEscape percent-encodes a string for safe use in URLs.
func URLEscape(s string) string {
	return url.QueryEscape(s)
}

// BuildURLSafeVars creates a variable map with all values URL-encoded.
func BuildURLSafeVars(vars map[string]string) map[string]string {
	safe := make(map[string]string, len(vars))
	for k, v := range vars {
		safe[k] = URLEscape(v)
	}
	return safe
}

// ExpandTemplate replaces {key} placeholders in template with values from vars.
// Unknown placeholders are left as-is.
func ExpandTemplate(template string, vars map[string]string) string {
	result := template
	for key, value := range vars {
		result = strings.ReplaceAll(result, "{"+key+"}", value)
	}
	return result
}

// BuildSessionName creates a session identifier from a card number and title.
// Format: {number}-{slugified-title}, capped at maxLen characters.
// Truncation snaps to the last complete hyphen-delimited segment.
func BuildSessionName(number int, title string, maxLen int) string {
	prefix := fmt.Sprintf("%d", number)
	slug := Slugify(title)
	if slug == "" {
		return prefix
	}
	full := prefix + "-" + slug
	if len(full) <= maxLen {
		return full
	}
	// If truncation lands exactly on a segment boundary, keep it.
	if full[maxLen] == '-' {
		return full[:maxLen]
	}
	truncated := full[:maxLen]
	lastHyphen := strings.LastIndex(truncated, "-")
	if lastHyphen <= len(prefix) {
		return prefix
	}
	return strings.TrimRight(truncated[:lastHyphen], "-")
}

// BuildTemplateVars creates the variable map for template expansion.
func BuildTemplateVars(cardNumber int, cardTitle string, cardLabels []string, repoOwner, repoName, providerName string, sessionMaxLen int) map[string]string {
	return map[string]string{
		"number":     fmt.Sprintf("%d", cardNumber),
		"title":      Slugify(cardTitle),
		// tags expands to a single comma-separated string of all card labels,
		// e.g., "bug,feature,urgent". After shell escaping via BuildShellSafeVars,
		// this becomes one quoted token: 'bug,feature,urgent'.
		// Shell iteration like `for tag in {tags}` will NOT split into individual
		// tags — the entire string is one argument. To iterate, users must split
		// the value themselves, e.g.: echo {tags} | tr ',' '\n'
		"tags": strings.Join(cardLabels, ","),
		"repo_owner": repoOwner,
		"repo_name":  repoName,
		"provider":   providerName,
		"session":    BuildSessionName(cardNumber, cardTitle, sessionMaxLen),
	}
}

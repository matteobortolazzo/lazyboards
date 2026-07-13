package action

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
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

// sessionSlug slugifies a title: lowercase, keep only ASCII a-z0-9, map
// space/underscore/hyphen to a single dash separator, and drop every other
// rune (punctuation, non-ASCII) entirely rather than hyphenating it.
// Consecutive separators are collapsed and leading/trailing dashes trimmed.
//
// It backs the {session} template variable (see BuildSessionName), which
// user-defined shell actions may reference. Agent-status matching no longer
// depends on this: cards join agentwatch windows by ticket-number prefix (see
// agentStatusForNumber in model.go), not by reproducing the window name, so
// this slug no longer has to match agentwatch byte-for-byte.
func sessionSlug(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '_' || r == '-':
			b.WriteByte('-')
		default:
			// drop
		}
	}
	parts := strings.FieldsFunc(b.String(), func(r rune) bool { return r == '-' })
	return strings.Join(parts, "-")
}

// BuildSessionName creates a session identifier from a card number and title.
// Format: {number}-{slugified-title}, capped at maxLen characters.
// Truncation is a hard cut at maxLen runes with trailing hyphens trimmed.
//
// It backs the {session} template variable, which user-defined shell actions
// may reference (e.g. a cleanup hook). It is no longer the agent-status join
// key: cards match agentwatch windows by ticket-number prefix (see
// agentStatusForNumber in model.go), which is independent of this name.
func BuildSessionName(number int, title string, maxLen int) string {
	prefix := fmt.Sprintf("%d", number)
	slug := sessionSlug(title)
	if slug == "" {
		return prefix
	}
	full := prefix + "-" + slug
	if utf8.RuneCountInString(full) <= maxLen {
		return full
	}
	r := []rune(full)
	return strings.TrimRight(string(r[:maxLen]), "-")
}

// BuildBoardTemplateVars creates the variable map for board-scope template expansion.
// Only includes board-level variables (no card-specific variables).
func BuildBoardTemplateVars(repoOwner, repoName, providerName, comment string) map[string]string {
	return map[string]string{
		"repo_owner": repoOwner,
		"repo_name":  repoName,
		"provider":   providerName,
		"comment":    comment,
	}
}

// BuildTemplateVars creates the variable map for template expansion.
func BuildTemplateVars(cardNumber int, cardTitle string, cardLabels []string, repoOwner, repoName, providerName string, sessionMaxLen int, comment string) map[string]string {
	return map[string]string{
		"number":     fmt.Sprintf("%d", cardNumber),
		"title":      Slugify(cardTitle),
		// tags expands to a single comma-separated string of all card labels,
		// e.g., "bug,feature,urgent". After shell escaping via BuildShellSafeVars,
		// this becomes one quoted token: 'bug,feature,urgent'.
		// Shell iteration like `for tag in {tags}` will NOT split into individual
		// tags — the entire string is one argument. To iterate, users must split
		// the value themselves, e.g.: echo {tags} | tr ',' '\n'
		"tags":       strings.Join(cardLabels, ","),
		"repo_owner": repoOwner,
		"repo_name":  repoName,
		"provider":   providerName,
		"session":    BuildSessionName(cardNumber, cardTitle, sessionMaxLen),
		"comment":    comment,
	}
}

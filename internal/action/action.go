package action

import (
	"fmt"
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

// ExpandTemplate replaces {key} placeholders in template with values from vars.
// Unknown placeholders are left as-is.
func ExpandTemplate(template string, vars map[string]string) string {
	result := template
	for key, value := range vars {
		result = strings.ReplaceAll(result, "{"+key+"}", value)
	}
	return result
}

// BuildTemplateVars creates the variable map for template expansion.
func BuildTemplateVars(cardNumber int, cardTitle string, cardLabels []string, repoOwner, repoName, providerName string) map[string]string {
	return map[string]string{
		"number":     fmt.Sprintf("%d", cardNumber),
		"title":      Slugify(cardTitle),
		"tags":       strings.Join(cardLabels, ","),
		"repo_owner": repoOwner,
		"repo_name":  repoName,
		"provider":   providerName,
	}
}

package auth

import (
	"fmt"
	"strings"
)

// validPrefixes lists the known GitHub token prefixes.
var validPrefixes = []string{
	"ghp_",
	"gho_",
	"ghs_",
	"ghu_",
	"github_pat_",
}

// ValidateGitHubToken checks that the token starts with a known GitHub prefix.
// Returns nil if valid, or an error describing the problem.
func ValidateGitHubToken(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("token is empty")
	}

	for _, prefix := range validPrefixes {
		if strings.HasPrefix(token, prefix) {
			return nil
		}
	}

	return fmt.Errorf("token does not start with a recognized GitHub prefix (ghp_, gho_, ghs_, ghu_, github_pat_)")
}

package provider

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/google/go-github/v68/github"
)

// statusDescriptions maps HTTP status codes to user-friendly descriptions.
var statusDescriptions = map[int]string{
	http.StatusUnauthorized:        "unauthorized",
	http.StatusForbidden:           "forbidden",
	http.StatusNotFound:            "not found",
	http.StatusUnprocessableEntity: "validation failed",
	http.StatusInternalServerError: "internal server error",
}

// SanitizeError converts provider errors into user-friendly messages.
// GitHub API errors are mapped to readable descriptions; other errors pass through unchanged.
func SanitizeError(err error) string {
	var rateLimitErr *github.RateLimitError
	if errors.As(err, &rateLimitErr) {
		return "API rate limit exceeded"
	}

	var abuseRateLimitErr *github.AbuseRateLimitError
	if errors.As(err, &abuseRateLimitErr) {
		return "API rate limit exceeded"
	}

	var ghErr *github.ErrorResponse
	if errors.As(err, &ghErr) {
		if ghErr.Response == nil {
			return "API error: unknown"
		}
		code := ghErr.Response.StatusCode
		desc, ok := statusDescriptions[code]
		if !ok {
			desc = "request failed"
		}
		return fmt.Sprintf("API error (%d): %s", code, desc)
	}

	return err.Error()
}

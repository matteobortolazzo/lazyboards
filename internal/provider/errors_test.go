package provider

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-github/v68/github"
)

func TestSanitizeError_GitHubErrorResponse_MapsStatusToDescription(t *testing.T) {
	cases := []struct {
		code     int
		wantDesc string
	}{
		{http.StatusUnauthorized, "unauthorized"},
		{http.StatusForbidden, "forbidden"},
		{http.StatusNotFound, "not found"},
		{http.StatusUnprocessableEntity, "validation failed"},
		{http.StatusInternalServerError, "internal server error"},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("status_%d", tc.code), func(t *testing.T) {
			err := &github.ErrorResponse{
				Response: &http.Response{StatusCode: tc.code},
			}

			result := SanitizeError(err)

			want := fmt.Sprintf("API error (%d): %s", tc.code, tc.wantDesc)
			if result != want {
				t.Errorf("SanitizeError() = %q, want %q", result, want)
			}
		})
	}
}

func TestSanitizeError_GitHubErrorResponse_UnmappedStatus_ReturnsFallback(t *testing.T) {
	// Use a status code that is not in the explicit mapping.
	unmappedStatusCode := http.StatusServiceUnavailable
	err := &github.ErrorResponse{
		Response: &http.Response{StatusCode: unmappedStatusCode},
	}

	result := SanitizeError(err)

	expectedPrefix := fmt.Sprintf("API error (%d)", unmappedStatusCode)
	if len(result) < len(expectedPrefix) || result[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("SanitizeError() = %q, want it to start with %q", result, expectedPrefix)
	}
	if !strings.Contains(result, "request failed") {
		t.Errorf("SanitizeError() = %q, want it to contain fallback description %q", result, "request failed")
	}
}

func TestSanitizeError_GitHubRateLimitError_ReturnsRateLimitMessage(t *testing.T) {
	err := &github.RateLimitError{}

	result := SanitizeError(err)

	if !strings.Contains(result, "rate limit") {
		t.Errorf("SanitizeError() = %q, want it to mention rate limit", result)
	}
}

func TestSanitizeError_GitHubAbuseRateLimitError_ReturnsRateLimitMessage(t *testing.T) {
	err := &github.AbuseRateLimitError{}

	result := SanitizeError(err)

	if !strings.Contains(result, "rate limit") {
		t.Errorf("SanitizeError() = %q, want it to mention rate limit", result)
	}
}

func TestSanitizeError_NonGitHubError_ReturnsOriginalMessage(t *testing.T) {
	originalMsg := "connection refused to api.example.com"
	err := errors.New(originalMsg)

	result := SanitizeError(err)

	if result != originalMsg {
		t.Errorf("SanitizeError() = %q, want original error message %q", result, originalMsg)
	}
}

func TestSanitizeError_GitHubErrorResponse_NilResponse_DoesNotPanic(t *testing.T) {
	err := &github.ErrorResponse{
		Response: nil,
	}

	// The function must not panic when Response is nil.
	// We use a deferred recover to catch panics and fail the test explicitly.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SanitizeError() panicked with nil Response: %v", r)
		}
	}()

	result := SanitizeError(err)

	// Result should be a non-empty safe message (not a zero-value or crash).
	if result == "" {
		t.Error("SanitizeError() returned empty string for nil Response, want a safe message")
	}
}

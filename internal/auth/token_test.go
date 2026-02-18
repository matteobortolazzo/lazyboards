package auth

import "testing"

func TestValidateGitHubToken(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		wantErr bool
	}{
		// Valid tokens — each known GitHub prefix followed by characters.
		{
			name:    "ClassicPAT",
			token:   "ghp_abc123XYZ",
			wantErr: false,
		},
		{
			name:    "OAuthToken",
			token:   "gho_tokenvalue456",
			wantErr: false,
		},
		{
			name:    "AppInstallationToken",
			token:   "ghs_installtoken789",
			wantErr: false,
		},
		{
			name:    "FineGrainedPAT",
			token:   "github_pat_longertoken123abc",
			wantErr: false,
		},
		{
			name:    "AppUserToServerToken",
			token:   "ghu_usertoken456",
			wantErr: false,
		},

		// Invalid tokens — should all return an error.
		{
			name:    "EmptyString",
			token:   "",
			wantErr: true,
		},
		{
			name:    "RandomString",
			token:   "not-a-token",
			wantErr: true,
		},
		{
			name:    "GitLabToken",
			token:   "glpat-abc123",
			wantErr: true,
		},
		{
			name:    "PrefixNotAtStart",
			token:   "xghp_abc123",
			wantErr: true,
		},
		{
			name:    "WhitespaceOnly",
			token:   "   ",
			wantErr: true,
		},
		{
			name:    "PartialPrefix",
			token:   "gh",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGitHubToken(tt.token)

			if tt.wantErr && err == nil {
				t.Errorf("ValidateGitHubToken(%q) = nil, want error", tt.token)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidateGitHubToken(%q) = %v, want nil", tt.token, err)
			}
		})
	}
}

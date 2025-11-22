package auth

import (
	"os"
	"testing"
)

func TestExtractToken(t *testing.T) {
	tests := []struct {
		name        string
		authHeader  string
		wantToken   string
		wantErr     bool
		errContains string
	}{
		{
			name:       "valid bearer token",
			authHeader: "Bearer test-token-123",
			wantToken:  "test-token-123",
			wantErr:    false,
		},
		{
			name:       "valid bearer token with spaces",
			authHeader: "Bearer token-with-spaces",
			wantToken:  "token-with-spaces",
			wantErr:    false,
		},
		{
			name:        "missing authorization header",
			authHeader:  "",
			wantToken:   "",
			wantErr:     true,
			errContains: "missing Authorization header",
		},
		{
			name:        "invalid format - no bearer",
			authHeader:  "test-token-123",
			wantToken:   "",
			wantErr:     true,
			errContains: "invalid Authorization header format",
		},
		{
			name:        "invalid format - no space",
			authHeader:  "Bearertoken",
			wantToken:   "",
			wantErr:     true,
			errContains: "invalid Authorization header format",
		},
		{
			name:        "wrong scheme - not bearer",
			authHeader:  "Basic dGVzdDp0ZXN0",
			wantToken:   "",
			wantErr:     true,
			errContains: "authorization header must use Bearer scheme",
		},
		{
			name:       "case insensitive bearer",
			authHeader: "bearer test-token-123",
			wantToken:  "test-token-123",
			wantErr:    false,
		},
		{
			name:       "uppercase bearer",
			authHeader: "BEARER test-token-123",
			wantToken:  "test-token-123",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := ExtractToken(tt.authHeader)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errContains != "" {
				if err.Error() != tt.errContains {
					t.Errorf("ExtractToken() error = %v, want error containing %v", err, tt.errContains)
				}
			}
			if token != tt.wantToken {
				t.Errorf("ExtractToken() token = %v, want %v", token, tt.wantToken)
			}
		})
	}
}

func TestValidateToken(t *testing.T) {
	// Save original token
	originalToken := os.Getenv("BFM_API_TOKEN")
	defer func() {
		if originalToken != "" {
			os.Setenv("BFM_API_TOKEN", originalToken)
		} else {
			os.Unsetenv("BFM_API_TOKEN")
		}
	}()

	tests := []struct {
		name        string
		envToken    string
		inputToken  string
		wantErr     bool
		errContains string
	}{
		{
			name:       "valid token match",
			envToken:   "test-token-123",
			inputToken: "test-token-123",
			wantErr:    false,
		},
		{
			name:        "invalid token - mismatch",
			envToken:    "test-token-123",
			inputToken:  "wrong-token",
			wantErr:     true,
			errContains: "invalid API token",
		},
		{
			name:        "missing env token",
			envToken:    "",
			inputToken:  "any-token",
			wantErr:     true,
			errContains: "BFM_API_TOKEN not configured",
		},
		{
			name:        "empty input token",
			envToken:    "test-token-123",
			inputToken:  "",
			wantErr:     true,
			errContains: "invalid API token",
		},
		{
			name:       "token with special characters",
			envToken:   "token-with-special-chars-!@#$%",
			inputToken: "token-with-special-chars-!@#$%",
			wantErr:    false,
		},
		{
			name:        "case sensitive token",
			envToken:    "Test-Token",
			inputToken:  "test-token",
			wantErr:     true,
			errContains: "invalid API token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable
			if tt.envToken != "" {
				os.Setenv("BFM_API_TOKEN", tt.envToken)
			} else {
				os.Unsetenv("BFM_API_TOKEN")
			}

			err := ValidateToken(tt.inputToken)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errContains != "" {
				if err.Error() != tt.errContains {
					t.Errorf("ValidateToken() error = %v, want error containing %v", err, tt.errContains)
				}
			}
		})
	}
}

func TestExtractAndValidateToken(t *testing.T) {
	// Save original token
	originalToken := os.Getenv("BFM_API_TOKEN")
	defer func() {
		if originalToken != "" {
			os.Setenv("BFM_API_TOKEN", originalToken)
		} else {
			os.Unsetenv("BFM_API_TOKEN")
		}
	}()

	os.Setenv("BFM_API_TOKEN", "test-token-123")

	tests := []struct {
		name        string
		authHeader  string
		wantErr     bool
		errContains string
	}{
		{
			name:       "valid flow",
			authHeader: "Bearer test-token-123",
			wantErr:    false,
		},
		{
			name:        "invalid header format",
			authHeader:  "invalid",
			wantErr:     true,
			errContains: "invalid Authorization header format",
		},
		{
			name:        "invalid token after extraction",
			authHeader:  "Bearer wrong-token",
			wantErr:     true,
			errContains: "invalid API token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := ExtractToken(tt.authHeader)
			if err != nil {
				if !tt.wantErr {
					t.Errorf("ExtractToken() unexpected error = %v", err)
				}
				return
			}

			err = ValidateToken(token)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errContains != "" {
				if err.Error() != tt.errContains {
					t.Errorf("ValidateToken() error = %v, want error containing %v", err, tt.errContains)
				}
			}
		})
	}
}


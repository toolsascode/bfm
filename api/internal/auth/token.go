package auth

import (
	"errors"
	"os"
	"strings"
)

// ValidateToken validates an API token
func ValidateToken(token string) error {
	expectedToken := os.Getenv("BFM_API_TOKEN")
	if expectedToken == "" {
		return errors.New("BFM_API_TOKEN not configured")
	}

	if token != expectedToken {
		return errors.New("invalid API token")
	}

	return nil
}

// ExtractToken extracts the token from an Authorization header
func ExtractToken(authHeader string) (string, error) {
	if authHeader == "" {
		return "", errors.New("missing Authorization header")
	}

	// Support "Bearer {token}" format
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 {
		return "", errors.New("invalid Authorization header format")
	}

	if strings.ToLower(parts[0]) != "bearer" {
		return "", errors.New("Authorization header must use Bearer scheme")
	}

	return parts[1], nil
}


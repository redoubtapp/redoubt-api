package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

const (
	// RefreshTokenLength is the length of refresh tokens in bytes.
	RefreshTokenLength = 32
	// VerificationTokenLength is the length of email verification tokens in bytes.
	VerificationTokenLength = 32
	// PasswordResetTokenLength is the length of password reset tokens in bytes.
	PasswordResetTokenLength = 32
)

// GenerateRefreshToken creates a cryptographically secure refresh token.
func GenerateRefreshToken() (string, error) {
	return generateSecureToken(RefreshTokenLength)
}

// GenerateVerificationToken creates a cryptographically secure email verification token.
func GenerateVerificationToken() (string, error) {
	return generateSecureToken(VerificationTokenLength)
}

// GeneratePasswordResetToken creates a cryptographically secure password reset token.
func GeneratePasswordResetToken() (string, error) {
	return generateSecureToken(PasswordResetTokenLength)
}

func generateSecureToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate secure token: %w", err)
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

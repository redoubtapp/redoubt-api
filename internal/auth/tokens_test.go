package auth

import (
	"encoding/base64"
	"testing"
)

func TestGenerateRefreshToken(t *testing.T) {
	tests := []struct {
		name    string
		wantLen int // expected decoded byte length
		wantErr bool
	}{
		{
			name:    "generates token of correct length",
			wantLen: RefreshTokenLength,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := GenerateRefreshToken()
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateRefreshToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify it's valid base64
				decoded, err := base64.URLEncoding.DecodeString(token)
				if err != nil {
					t.Errorf("GenerateRefreshToken() produced invalid base64: %v", err)
					return
				}

				if len(decoded) != tt.wantLen {
					t.Errorf("GenerateRefreshToken() decoded length = %v, want %v", len(decoded), tt.wantLen)
				}
			}
		})
	}
}

func TestGenerateVerificationToken(t *testing.T) {
	tests := []struct {
		name    string
		wantLen int
		wantErr bool
	}{
		{
			name:    "generates token of correct length",
			wantLen: VerificationTokenLength,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := GenerateVerificationToken()
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateVerificationToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				decoded, err := base64.URLEncoding.DecodeString(token)
				if err != nil {
					t.Errorf("GenerateVerificationToken() produced invalid base64: %v", err)
					return
				}

				if len(decoded) != tt.wantLen {
					t.Errorf("GenerateVerificationToken() decoded length = %v, want %v", len(decoded), tt.wantLen)
				}
			}
		})
	}
}

func TestGeneratePasswordResetToken(t *testing.T) {
	tests := []struct {
		name    string
		wantLen int
		wantErr bool
	}{
		{
			name:    "generates token of correct length",
			wantLen: PasswordResetTokenLength,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := GeneratePasswordResetToken()
			if (err != nil) != tt.wantErr {
				t.Errorf("GeneratePasswordResetToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				decoded, err := base64.URLEncoding.DecodeString(token)
				if err != nil {
					t.Errorf("GeneratePasswordResetToken() produced invalid base64: %v", err)
					return
				}

				if len(decoded) != tt.wantLen {
					t.Errorf("GeneratePasswordResetToken() decoded length = %v, want %v", len(decoded), tt.wantLen)
				}
			}
		})
	}
}

func TestTokenUniqueness(t *testing.T) {
	tokenGenerators := []struct {
		name     string
		generate func() (string, error)
	}{
		{"RefreshToken", GenerateRefreshToken},
		{"VerificationToken", GenerateVerificationToken},
		{"PasswordResetToken", GeneratePasswordResetToken},
	}

	for _, tg := range tokenGenerators {
		t.Run(tg.name+"_uniqueness", func(t *testing.T) {
			const numTokens = 100
			tokens := make(map[string]bool)

			for i := 0; i < numTokens; i++ {
				token, err := tg.generate()
				if err != nil {
					t.Fatalf("Failed to generate token: %v", err)
				}

				if tokens[token] {
					t.Errorf("Duplicate token generated: %v", token)
				}
				tokens[token] = true
			}
		})
	}
}

func TestTokenFormat(t *testing.T) {
	tokenGenerators := []struct {
		name     string
		generate func() (string, error)
		length   int
	}{
		{"RefreshToken", GenerateRefreshToken, RefreshTokenLength},
		{"VerificationToken", GenerateVerificationToken, VerificationTokenLength},
		{"PasswordResetToken", GeneratePasswordResetToken, PasswordResetTokenLength},
	}

	for _, tg := range tokenGenerators {
		t.Run(tg.name+"_format", func(t *testing.T) {
			token, err := tg.generate()
			if err != nil {
				t.Fatalf("Failed to generate token: %v", err)
			}

			// Token should not be empty
			if token == "" {
				t.Error("Token should not be empty")
			}

			// Token should be URL-safe base64
			decoded, err := base64.URLEncoding.DecodeString(token)
			if err != nil {
				t.Errorf("Token is not valid URL-safe base64: %v", err)
			}

			// Decoded length should match expected
			if len(decoded) != tg.length {
				t.Errorf("Decoded token length = %v, want %v", len(decoded), tg.length)
			}
		})
	}
}

func TestTokenConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant int
		wantMin  int
	}{
		{
			name:     "RefreshTokenLength",
			constant: RefreshTokenLength,
			wantMin:  32,
		},
		{
			name:     "VerificationTokenLength",
			constant: VerificationTokenLength,
			wantMin:  32,
		},
		{
			name:     "PasswordResetTokenLength",
			constant: PasswordResetTokenLength,
			wantMin:  32,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant < tt.wantMin {
				t.Errorf("%s = %v, should be at least %v for security", tt.name, tt.constant, tt.wantMin)
			}
		})
	}
}

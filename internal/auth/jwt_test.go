package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestNewJWTManager(t *testing.T) {
	tests := []struct {
		name   string
		secret string
		expiry time.Duration
	}{
		{
			name:   "standard configuration",
			secret: "test-secret-key",
			expiry: 15 * time.Minute,
		},
		{
			name:   "long expiry",
			secret: "another-secret",
			expiry: 24 * time.Hour,
		},
		{
			name:   "empty secret",
			secret: "",
			expiry: time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewJWTManager(tt.secret, tt.expiry)
			if manager == nil {
				t.Fatal("NewJWTManager() returned nil")
			}
			if string(manager.secret) != tt.secret {
				t.Errorf("secret = %v, want %v", string(manager.secret), tt.secret)
			}
			if manager.expiry != tt.expiry {
				t.Errorf("expiry = %v, want %v", manager.expiry, tt.expiry)
			}
			if manager.issuer != "redoubt" {
				t.Errorf("issuer = %v, want redoubt", manager.issuer)
			}
		})
	}
}

func TestGenerateToken(t *testing.T) {
	manager := NewJWTManager("test-secret", 15*time.Minute)

	tests := []struct {
		name    string
		userID  uuid.UUID
		isAdmin bool
		wantErr bool
	}{
		{
			name:    "regular user",
			userID:  uuid.MustParse("123e4567-e89b-12d3-a456-426614174000"),
			isAdmin: false,
			wantErr: false,
		},
		{
			name:    "admin user",
			userID:  uuid.MustParse("123e4567-e89b-12d3-a456-426614174001"),
			isAdmin: true,
			wantErr: false,
		},
		{
			name:    "nil uuid",
			userID:  uuid.Nil,
			isAdmin: false,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := manager.GenerateToken(tt.userID, tt.isAdmin, uuid.New())
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && token == "" {
				t.Error("GenerateToken() returned empty token")
			}
		})
	}
}

func TestValidateToken(t *testing.T) {
	secret := "test-secret-key"
	manager := NewJWTManager(secret, 15*time.Minute)
	userID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")

	// Generate a valid token
	validToken, _ := manager.GenerateToken(userID, true, uuid.New())

	// Generate an expired token manually
	expiredClaims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
			ID:        uuid.New().String(),
			Issuer:    "redoubt",
		},
		Admin: false,
	}
	expiredToken, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, expiredClaims).SignedString([]byte(secret))

	// Generate a token with wrong secret
	wrongSecretManager := NewJWTManager("wrong-secret", 15*time.Minute)
	wrongSecretToken, _ := wrongSecretManager.GenerateToken(userID, false, uuid.New())

	// Token claiming to use RS256 but without proper signature
	nonHMACTokenStr := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.invalid-signature" //nolint:gosec // test token, not real credentials

	tests := []struct {
		name      string
		token     string
		wantErr   error
		wantAdmin bool
	}{
		{
			name:      "valid token",
			token:     validToken,
			wantErr:   nil,
			wantAdmin: true,
		},
		{
			name:    "expired token",
			token:   expiredToken,
			wantErr: ErrTokenExpired,
		},
		{
			name:    "wrong secret",
			token:   wrongSecretToken,
			wantErr: ErrTokenInvalid,
		},
		{
			name:    "malformed token",
			token:   "not.a.valid.token",
			wantErr: ErrTokenInvalid,
		},
		{
			name:    "empty token",
			token:   "",
			wantErr: ErrTokenInvalid,
		},
		{
			name:    "non-HMAC signing method",
			token:   nonHMACTokenStr,
			wantErr: ErrTokenInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims, err := manager.ValidateToken(tt.token)
			if err != tt.wantErr {
				t.Errorf("ValidateToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr == nil {
				if claims == nil {
					t.Error("ValidateToken() returned nil claims for valid token")
					return
				}
				if claims.Admin != tt.wantAdmin {
					t.Errorf("claims.Admin = %v, want %v", claims.Admin, tt.wantAdmin)
				}
			}
		})
	}
}

func TestClaimsGetUserID(t *testing.T) {
	validUUID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")

	tests := []struct {
		name    string
		subject string
		want    uuid.UUID
		wantErr bool
	}{
		{
			name:    "valid uuid",
			subject: validUUID.String(),
			want:    validUUID,
			wantErr: false,
		},
		{
			name:    "invalid uuid",
			subject: "not-a-uuid",
			want:    uuid.Nil,
			wantErr: true,
		},
		{
			name:    "empty subject",
			subject: "",
			want:    uuid.Nil,
			wantErr: true,
		},
		{
			name:    "nil uuid string",
			subject: uuid.Nil.String(),
			want:    uuid.Nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &Claims{
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: tt.subject,
				},
			}
			got, err := claims.GetUserID()
			if (err != nil) != tt.wantErr {
				t.Errorf("GetUserID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetUserID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTokenRoundTrip(t *testing.T) {
	manager := NewJWTManager("round-trip-secret", 15*time.Minute)

	tests := []struct {
		name    string
		userID  uuid.UUID
		isAdmin bool
	}{
		{
			name:    "regular user round trip",
			userID:  uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			isAdmin: false,
		},
		{
			name:    "admin user round trip",
			userID:  uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			isAdmin: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Generate token
			token, err := manager.GenerateToken(tt.userID, tt.isAdmin, uuid.New())
			if err != nil {
				t.Fatalf("GenerateToken() error = %v", err)
			}

			// Validate token
			claims, err := manager.ValidateToken(token)
			if err != nil {
				t.Fatalf("ValidateToken() error = %v", err)
			}

			// Check user ID
			gotUserID, err := claims.GetUserID()
			if err != nil {
				t.Fatalf("GetUserID() error = %v", err)
			}
			if gotUserID != tt.userID {
				t.Errorf("userID = %v, want %v", gotUserID, tt.userID)
			}

			// Check admin status
			if claims.Admin != tt.isAdmin {
				t.Errorf("isAdmin = %v, want %v", claims.Admin, tt.isAdmin)
			}

			// Check issuer
			if claims.Issuer != "redoubt" {
				t.Errorf("issuer = %v, want redoubt", claims.Issuer)
			}
		})
	}
}

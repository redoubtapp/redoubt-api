package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	ErrTokenExpired = errors.New("token expired")
	ErrTokenInvalid = errors.New("token invalid")
)

// Claims represents the JWT claims for access tokens.
type Claims struct {
	jwt.RegisteredClaims
	Admin     bool   `json:"admin,omitempty"`
	SessionID string `json:"sid,omitempty"` // Session ID for session management
}

// JWTManager handles JWT token generation and validation.
type JWTManager struct {
	secret []byte
	expiry time.Duration
	issuer string
}

// NewJWTManager creates a new JWT manager.
func NewJWTManager(secret string, expiry time.Duration) *JWTManager {
	return &JWTManager{
		secret: []byte(secret),
		expiry: expiry,
		issuer: "redoubt",
	}
}

// GenerateToken creates a new JWT access token for the given user.
func (m *JWTManager) GenerateToken(userID uuid.UUID, isAdmin bool, sessionID uuid.UUID) (string, error) {
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.expiry)),
			ID:        uuid.New().String(),
			Issuer:    m.issuer,
		},
		Admin:     isAdmin,
		SessionID: sessionID.String(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

// ValidateToken validates a JWT token and returns the claims.
func (m *JWTManager) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return m.secret, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrTokenInvalid
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrTokenInvalid
	}

	return claims, nil
}

// GetUserID extracts the user ID from the claims.
func (c *Claims) GetUserID() (uuid.UUID, error) {
	return uuid.Parse(c.Subject)
}

// GetSessionID extracts the session ID from the claims.
func (c *Claims) GetSessionID() (uuid.UUID, error) {
	if c.SessionID == "" {
		return uuid.Nil, nil
	}
	return uuid.Parse(c.SessionID)
}

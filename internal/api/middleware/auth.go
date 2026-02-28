package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/redoubtapp/redoubt-api/internal/auth"
	apperrors "github.com/redoubtapp/redoubt-api/internal/errors"
)

// Context keys for auth values.
type contextKey string

const (
	userIDKey    contextKey = "user_id"
	isAdminKey   contextKey = "is_admin"
	claimsKey    contextKey = "claims"
	sessionIDKey contextKey = "session_id"
)

// Auth is middleware that validates JWT tokens.
func Auth(jwtManager *auth.JWTManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract token from Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				apperrors.Unauthorized(w, r)
				return
			}

			// Expect "Bearer <token>"
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				apperrors.Unauthorized(w, r)
				return
			}

			tokenString := parts[1]

			// Validate token
			claims, err := jwtManager.ValidateToken(tokenString)
			if err != nil {
				apperrors.Unauthorized(w, r)
				return
			}

			// Extract user ID
			userID, err := claims.GetUserID()
			if err != nil {
				apperrors.Unauthorized(w, r)
				return
			}

			// Add user info to context
			ctx := r.Context()
			ctx = context.WithValue(ctx, userIDKey, userID)
			ctx = context.WithValue(ctx, isAdminKey, claims.Admin)
			ctx = context.WithValue(ctx, claimsKey, claims)

			// Extract session ID if present
			if sessionID, err := claims.GetSessionID(); err == nil && sessionID != uuid.Nil {
				ctx = context.WithValue(ctx, sessionIDKey, sessionID)
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAuth is middleware that ensures the request has a valid JWT.
// Use this for routes that require authentication.
func RequireAuth(jwtManager *auth.JWTManager) func(http.Handler) http.Handler {
	return Auth(jwtManager)
}

// RequireInstanceAdmin is middleware that ensures the user is an instance admin.
func RequireInstanceAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isAdmin := GetIsAdmin(r.Context())
		if !isAdmin {
			apperrors.Forbidden(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// GetUserID retrieves the authenticated user's ID from the context.
func GetUserID(ctx context.Context) (uuid.UUID, bool) {
	userID, ok := ctx.Value(userIDKey).(uuid.UUID)
	return userID, ok
}

// GetIsAdmin retrieves whether the user is an instance admin from the context.
func GetIsAdmin(ctx context.Context) bool {
	isAdmin, ok := ctx.Value(isAdminKey).(bool)
	return ok && isAdmin
}

// GetClaims retrieves the JWT claims from the context.
func GetClaims(ctx context.Context) (*auth.Claims, bool) {
	claims, ok := ctx.Value(claimsKey).(*auth.Claims)
	return claims, ok
}

// GetSessionID retrieves the current session ID from the context.
func GetSessionID(ctx context.Context) (uuid.UUID, bool) {
	sessionID, ok := ctx.Value(sessionIDKey).(uuid.UUID)
	return sessionID, ok
}

// OptionalAuth is middleware that validates JWT tokens if present, but doesn't require them.
func OptionalAuth(jwtManager *auth.JWTManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				next.ServeHTTP(w, r)
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				next.ServeHTTP(w, r)
				return
			}

			tokenString := parts[1]
			claims, err := jwtManager.ValidateToken(tokenString)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			userID, err := claims.GetUserID()
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			ctx := r.Context()
			ctx = context.WithValue(ctx, userIDKey, userID)
			ctx = context.WithValue(ctx, isAdminKey, claims.Admin)
			ctx = context.WithValue(ctx, claimsKey, claims)

			// Extract session ID if present
			if sessionID, err := claims.GetSessionID(); err == nil && sessionID != uuid.Nil {
				ctx = context.WithValue(ctx, sessionIDKey, sessionID)
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

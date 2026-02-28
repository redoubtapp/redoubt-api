package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	apperrors "github.com/redoubtapp/redoubt-api/internal/errors"
	"github.com/redoubtapp/redoubt-api/internal/ratelimit"
)

// IdentifierFunc extracts the rate limit identifier from a request.
type IdentifierFunc func(r *http.Request) string

// RateLimitConfig holds configuration for the rate limit middleware.
type RateLimitConfig struct {
	Limiter    *ratelimit.Limiter
	Scope      string
	Identifier IdentifierFunc
}

// RateLimit creates rate limiting middleware with configurable scope and identifier.
func RateLimit(cfg RateLimitConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identifier := cfg.Identifier(r)

			result, err := cfg.Limiter.Check(r.Context(), cfg.Scope, identifier)
			if err != nil {
				// Log error but don't block request on rate limiter failure
				// Fail open to avoid blocking legitimate requests
				next.ServeHTTP(w, r)
				return
			}

			// Set IETF draft rate limit headers
			setRateLimitHeaders(w, result)

			if !result.Allowed {
				retryAfter := int(time.Until(result.ResetAt).Seconds())
				if retryAfter < 1 {
					retryAfter = 1
				}
				apperrors.RateLimited(w, r, retryAfter)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// LoginRateLimit is specialized middleware for login that extracts email from request body.
// It uses IP+Email as the rate limit identifier for more precise limiting.
func LoginRateLimit(limiter *ratelimit.Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Read and buffer the body
			body, err := io.ReadAll(r.Body)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))

			// Parse email from body
			var req struct {
				Email string `json:"email"`
			}

			identifier := GetClientIP(r)
			if err := json.Unmarshal(body, &req); err == nil && req.Email != "" {
				// Use IP:Email as identifier
				identifier = fmt.Sprintf("%s:%s", identifier, strings.ToLower(req.Email))
			}

			result, err := limiter.Check(r.Context(), ratelimit.ScopeLogin, identifier)
			if err != nil {
				// Restore body and continue on error
				r.Body = io.NopCloser(bytes.NewReader(body))
				next.ServeHTTP(w, r)
				return
			}

			setRateLimitHeaders(w, result)

			if !result.Allowed {
				retryAfter := int(time.Until(result.ResetAt).Seconds())
				if retryAfter < 1 {
					retryAfter = 1
				}
				apperrors.RateLimited(w, r, retryAfter)
				return
			}

			// Restore body for handler
			r.Body = io.NopCloser(bytes.NewReader(body))
			next.ServeHTTP(w, r)
		})
	}
}

// setRateLimitHeaders sets IETF draft rate limit headers.
func setRateLimitHeaders(w http.ResponseWriter, result *ratelimit.Result) {
	w.Header().Set("RateLimit-Limit", strconv.Itoa(result.Limit))
	w.Header().Set("RateLimit-Remaining", strconv.Itoa(result.Remaining))

	resetSeconds := int(time.Until(result.ResetAt).Seconds())
	if resetSeconds < 0 {
		resetSeconds = 0
	}
	w.Header().Set("RateLimit-Reset", strconv.Itoa(resetSeconds))
}

// --- Identifier Extraction Functions ---

// IPIdentifier extracts the client IP address for rate limiting.
func IPIdentifier(r *http.Request) string {
	return GetClientIP(r)
}

// UserIdentifier extracts the authenticated user ID for rate limiting.
// Falls back to IP if no user is authenticated.
func UserIdentifier(r *http.Request) string {
	userID, ok := GetUserID(r.Context())
	if !ok {
		return GetClientIP(r)
	}
	return userID.String()
}

// GetClientIP extracts the client IP from the request.
// Handles X-Forwarded-For and X-Real-IP headers for reverse proxies.
func GetClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for reverse proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the list (client IP)
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// RemoteAddr might not have a port
		return r.RemoteAddr
	}
	return ip
}

// Context key for email (used in IP+Email rate limiting).
type emailContextKey struct{}

// SetEmailInContext stores email in context for rate limiting.
func SetEmailInContext(ctx context.Context, email string) context.Context {
	return context.WithValue(ctx, emailContextKey{}, email)
}

// GetEmailFromContext retrieves email from context.
func GetEmailFromContext(ctx context.Context) string {
	if email, ok := ctx.Value(emailContextKey{}).(string); ok {
		return email
	}
	return ""
}

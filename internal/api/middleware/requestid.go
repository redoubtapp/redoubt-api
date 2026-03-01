package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	apperrors "github.com/redoubtapp/redoubt-api/internal/errors"
)

const (
	// RequestIDHeader is the HTTP header for request ID.
	RequestIDHeader = "X-Request-ID"
)

// RequestID is middleware that ensures each request has a unique ID.
// If the client provides an X-Request-ID header, it is used; otherwise, a new UUID is generated.
// The request ID is added to the request context and the response header.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get(RequestIDHeader)
		if requestID == "" {
			requestID = uuid.New().String()
		}

		// Add to response header
		w.Header().Set(RequestIDHeader, requestID)

		// Add to request context
		ctx := context.WithValue(r.Context(), apperrors.RequestIDKey(), requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestID retrieves the request ID from the context.
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(apperrors.RequestIDKey()).(string); ok {
		return id
	}
	return ""
}

package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	apperrors "github.com/redoubtapp/redoubt-api/internal/errors"
)

// Recover is middleware that recovers from panics and returns a 500 error.
func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					requestID := GetRequestID(r.Context())

					logger.Error("panic recovered",
						slog.String("request_id", requestID),
						slog.Any("error", err),
						slog.String("stack", string(debug.Stack())),
					)

					apperrors.InternalError(w, r)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

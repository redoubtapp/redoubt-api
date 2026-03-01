package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/redoubtapp/redoubt-api/internal/config"
)

// CORS is middleware that handles Cross-Origin Resource Sharing.
func CORS(cfg config.CORSConfig) func(http.Handler) http.Handler {
	allowedOrigins := make(map[string]bool)
	for _, origin := range cfg.AllowedOrigins {
		allowedOrigins[origin] = true
	}

	allowedMethods := strings.Join(cfg.AllowedMethods, ", ")
	allowedHeaders := strings.Join(cfg.AllowedHeaders, ", ")
	maxAge := strconv.Itoa(cfg.MaxAge)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if origin is allowed
			if origin != "" && allowedOrigins[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", allowedMethods)
				w.Header().Set("Access-Control-Allow-Headers", allowedHeaders)
				w.Header().Set("Access-Control-Max-Age", maxAge)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			// Handle preflight requests
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

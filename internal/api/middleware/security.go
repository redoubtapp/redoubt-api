package middleware

import "net/http"

// SecurityHeaders is middleware that adds security-related HTTP headers.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent MIME type sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Prevent clickjacking
		w.Header().Set("X-Frame-Options", "DENY")

		// Enable XSS filter
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Control referrer information
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Content Security Policy (basic)
		w.Header().Set("Content-Security-Policy", "default-src 'self'")

		next.ServeHTTP(w, r)
	})
}

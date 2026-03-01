package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeaders(t *testing.T) {
	tests := []struct {
		name      string
		header    string
		wantValue string
	}{
		{
			name:      "X-Content-Type-Options",
			header:    "X-Content-Type-Options",
			wantValue: "nosniff",
		},
		{
			name:      "X-Frame-Options",
			header:    "X-Frame-Options",
			wantValue: "DENY",
		},
		{
			name:      "X-XSS-Protection",
			header:    "X-XSS-Protection",
			wantValue: "1; mode=block",
		},
		{
			name:      "Referrer-Policy",
			header:    "Referrer-Policy",
			wantValue: "strict-origin-when-cross-origin",
		},
		{
			name:      "Content-Security-Policy",
			header:    "Content-Security-Policy",
			wantValue: "default-src 'self'",
		},
	}

	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rec.Header().Get(tt.header)
			if got != tt.wantValue {
				t.Errorf("%s = %v, want %v", tt.header, got, tt.wantValue)
			}
		})
	}
}

func TestSecurityHeadersAllMethods(t *testing.T) {
	methods := []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodOptions,
		http.MethodHead,
	}

	expectedHeaders := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"X-XSS-Protection":        "1; mode=block",
		"Referrer-Policy":         "strict-origin-when-cross-origin",
		"Content-Security-Policy": "default-src 'self'",
	}

	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/test", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			for header, wantValue := range expectedHeaders {
				got := rec.Header().Get(header)
				if got != wantValue {
					t.Errorf("%s: %s = %v, want %v", method, header, got, wantValue)
				}
			}
		})
	}
}

func TestSecurityHeadersNextHandlerCalled(t *testing.T) {
	nextCalled := false

	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !nextCalled {
		t.Error("next handler was not called")
	}
}

func TestSecurityHeadersPreserveExistingHeaders(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Header", "custom-value")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Check custom headers are preserved
	if rec.Header().Get("X-Custom-Header") != "custom-value" {
		t.Error("custom header should be preserved")
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Error("content-type should be preserved")
	}

	// Check security headers are still set
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("security header should still be set")
	}
}

func TestSecurityHeadersStatusCode(t *testing.T) {
	statusCodes := []int{
		http.StatusOK,
		http.StatusCreated,
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusInternalServerError,
	}

	for _, statusCode := range statusCodes {
		t.Run(http.StatusText(statusCode), func(t *testing.T) {
			handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(statusCode)
			}))

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			// Status code should pass through unchanged
			if rec.Code != statusCode {
				t.Errorf("status code = %v, want %v", rec.Code, statusCode)
			}

			// Security headers should still be present
			if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Error("security headers should be set regardless of status code")
			}
		})
	}
}

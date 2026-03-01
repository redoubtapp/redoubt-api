package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		remoteAddr string
		want       string
	}{
		{
			name:       "X-Forwarded-For single IP",
			headers:    map[string]string{"X-Forwarded-For": "192.168.1.1"},
			remoteAddr: "10.0.0.1:12345",
			want:       "192.168.1.1",
		},
		{
			name:       "X-Forwarded-For multiple IPs",
			headers:    map[string]string{"X-Forwarded-For": "192.168.1.1, 10.0.0.2, 172.16.0.1"},
			remoteAddr: "10.0.0.1:12345",
			want:       "192.168.1.1",
		},
		{
			name:       "X-Real-IP",
			headers:    map[string]string{"X-Real-IP": "192.168.2.1"},
			remoteAddr: "10.0.0.1:12345",
			want:       "192.168.2.1",
		},
		{
			name:       "RemoteAddr with port",
			headers:    map[string]string{},
			remoteAddr: "192.168.3.1:54321",
			want:       "192.168.3.1",
		},
		{
			name:       "RemoteAddr without port",
			headers:    map[string]string{},
			remoteAddr: "192.168.4.1",
			want:       "192.168.4.1",
		},
		{
			name:       "X-Forwarded-For takes priority over X-Real-IP",
			headers:    map[string]string{"X-Forwarded-For": "192.168.1.1", "X-Real-IP": "192.168.2.1"},
			remoteAddr: "10.0.0.1:12345",
			want:       "192.168.1.1",
		},
		{
			name:       "X-Forwarded-For with whitespace",
			headers:    map[string]string{"X-Forwarded-For": "  192.168.1.1  "},
			remoteAddr: "10.0.0.1:12345",
			want:       "192.168.1.1",
		},
		{
			name:       "X-Real-IP with whitespace",
			headers:    map[string]string{"X-Real-IP": "  192.168.2.1  "},
			remoteAddr: "10.0.0.1:12345",
			want:       "192.168.2.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			got := GetClientIP(req)
			if got != tt.want {
				t.Errorf("GetClientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIPIdentifier(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	got := IPIdentifier(req)
	want := "192.168.1.1"

	if got != want {
		t.Errorf("IPIdentifier() = %q, want %q", got, want)
	}
}

func TestUserIdentifierFallback(t *testing.T) {
	// Without a user ID in context, should fall back to IP
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	got := UserIdentifier(req)
	want := "192.168.1.1"

	if got != want {
		t.Errorf("UserIdentifier() without user = %q, want %q", got, want)
	}
}

func TestSetEmailInContext(t *testing.T) {
	ctx := context.Background()
	email := "test@example.com"

	ctx = SetEmailInContext(ctx, email)
	got := GetEmailFromContext(ctx)

	if got != email {
		t.Errorf("GetEmailFromContext() = %q, want %q", got, email)
	}
}

func TestGetEmailFromContextEmpty(t *testing.T) {
	ctx := context.Background()
	got := GetEmailFromContext(ctx)

	if got != "" {
		t.Errorf("GetEmailFromContext() = %q, want empty string", got)
	}
}

func TestGetEmailFromContextWrongType(t *testing.T) {
	ctx := context.WithValue(context.Background(), emailContextKey{}, 123)
	got := GetEmailFromContext(ctx)

	if got != "" {
		t.Errorf("GetEmailFromContext() = %q, want empty string", got)
	}
}

func TestRateLimitConfigStruct(t *testing.T) {
	// Verify RateLimitConfig struct fields
	cfg := RateLimitConfig{
		Limiter:    nil,
		Scope:      "test-scope",
		Identifier: IPIdentifier,
	}

	if cfg.Scope != "test-scope" {
		t.Errorf("Scope = %q, want %q", cfg.Scope, "test-scope")
	}

	if cfg.Identifier == nil {
		t.Error("Identifier should not be nil")
	}
}

func TestIdentifierFuncType(t *testing.T) {
	// Verify IPIdentifier and UserIdentifier satisfy IdentifierFunc
	var _ IdentifierFunc = IPIdentifier
	var _ IdentifierFunc = UserIdentifier

	// Custom identifier function
	customIdentifier := func(r *http.Request) string {
		return "custom"
	}
	var _ IdentifierFunc = customIdentifier

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if customIdentifier(req) != "custom" {
		t.Error("custom identifier should return 'custom'")
	}
}

func TestIPIdentifierWithXForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "10.20.30.40")
	req.RemoteAddr = "192.168.1.1:12345"

	got := IPIdentifier(req)
	want := "10.20.30.40"

	if got != want {
		t.Errorf("IPIdentifier() with X-Forwarded-For = %q, want %q", got, want)
	}
}

func TestIPIdentifierWithXRealIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", "10.20.30.50")
	req.RemoteAddr = "192.168.1.1:12345"

	got := IPIdentifier(req)
	want := "10.20.30.50"

	if got != want {
		t.Errorf("IPIdentifier() with X-Real-IP = %q, want %q", got, want)
	}
}

func TestEmailContextRoundTrip(t *testing.T) {
	emails := []string{
		"user@example.com",
		"user+tag@example.com",
		"test.user@sub.domain.com",
		"",
	}

	for _, email := range emails {
		t.Run(email, func(t *testing.T) {
			ctx := SetEmailInContext(context.Background(), email)
			got := GetEmailFromContext(ctx)
			if got != email {
				t.Errorf("email roundtrip: got %q, want %q", got, email)
			}
		})
	}
}

func TestIPExtractionEdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		want       string
	}{
		{
			name:       "IPv4 with port",
			remoteAddr: "127.0.0.1:8080",
			want:       "127.0.0.1",
		},
		{
			name:       "IPv4 without port",
			remoteAddr: "127.0.0.1",
			want:       "127.0.0.1",
		},
		{
			name:       "IPv6 loopback with port",
			remoteAddr: "[::1]:8080",
			want:       "::1",
		},
		{
			name:       "IPv6 full address with port",
			remoteAddr: "[2001:db8::1]:8080",
			want:       "2001:db8::1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr

			got := GetClientIP(req)
			if got != tt.want {
				t.Errorf("GetClientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

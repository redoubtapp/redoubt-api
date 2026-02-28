package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	ut "github.com/go-playground/universal-translator"
)

func TestValidationErrors(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		endpoint string
		wantCode int
	}{
		{
			name:     "register - empty body",
			body:     `{}`,
			endpoint: "/register",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "register - invalid email",
			body:     `{"username":"testuser","email":"notanemail","password":"password123456","invite_code":"ABC123"}`,
			endpoint: "/register",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "register - short password",
			body:     `{"username":"testuser","email":"test@example.com","password":"short","invite_code":"ABC123"}`,
			endpoint: "/register",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "register - short username",
			body:     `{"username":"ab","email":"test@example.com","password":"password123456","invite_code":"ABC123"}`,
			endpoint: "/register",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "login - empty body",
			body:     `{}`,
			endpoint: "/login",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "login - invalid email",
			body:     `{"email":"notanemail","password":"password123456"}`,
			endpoint: "/login",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "login - missing password",
			body:     `{"email":"test@example.com"}`,
			endpoint: "/login",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create handler with nil auth service (will fail after validation)
			handler := NewAuthHandler(nil)

			req := httptest.NewRequest(http.MethodPost, tt.endpoint, bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			switch tt.endpoint {
			case "/register":
				handler.Register(rr, req)
			case "/login":
				handler.Login(rr, req)
			}

			if rr.Code != tt.wantCode {
				t.Errorf("got status %d, want %d", rr.Code, tt.wantCode)
			}

			// Verify response is valid JSON
			var resp map[string]any
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Errorf("response is not valid JSON: %v", err)
			}
		})
	}
}

func TestValidationMessage(t *testing.T) {
	tests := []struct {
		tag      string
		param    string
		expected string
	}{
		{"required", "", "This field is required"},
		{"email", "", "Must be a valid email address"},
		{"min", "12", "Must be at least 12 characters"},
		{"max", "32", "Must be at most 32 characters"},
		{"alphanum", "", "Must contain only letters and numbers"},
		{"unknown", "", "Invalid value"},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			// Create a mock FieldError
			fe := mockFieldError{tag: tt.tag, param: tt.param}
			got := validationMessage(fe)
			if got != tt.expected {
				t.Errorf("validationMessage(%s) = %q, want %q", tt.tag, got, tt.expected)
			}
		})
	}
}

// mockFieldError implements validator.FieldError for testing.
type mockFieldError struct {
	tag   string
	param string
}

func (m mockFieldError) Tag() string                      { return m.tag }
func (m mockFieldError) ActualTag() string                { return m.tag }
func (m mockFieldError) Namespace() string                { return "" }
func (m mockFieldError) StructNamespace() string          { return "" }
func (m mockFieldError) Field() string                    { return "TestField" }
func (m mockFieldError) StructField() string              { return "TestField" }
func (m mockFieldError) Value() interface{}               { return nil }
func (m mockFieldError) Param() string                    { return m.param }
func (m mockFieldError) Kind() reflect.Kind               { return reflect.String }
func (m mockFieldError) Type() reflect.Type               { return reflect.TypeOf("") }
func (m mockFieldError) Translate(_ ut.Translator) string { return "" }
func (m mockFieldError) Error() string                    { return "" }

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
			name:       "IPv6 RemoteAddr",
			headers:    map[string]string{},
			remoteAddr: "[::1]:12345",
			want:       "[::1]",
		},
		{
			name:       "X-Forwarded-For takes priority over X-Real-IP",
			headers:    map[string]string{"X-Forwarded-For": "192.168.1.1", "X-Real-IP": "192.168.2.1"},
			remoteAddr: "10.0.0.1:12345",
			want:       "192.168.1.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			got := getClientIP(req)
			if got != tt.want {
				t.Errorf("getClientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInvalidRequestBody(t *testing.T) {
	handler := NewAuthHandler(nil)

	tests := []struct {
		name    string
		body    string
		handler func(http.ResponseWriter, *http.Request)
	}{
		{"register invalid json", "not json", handler.Register},
		{"login invalid json", "not json", handler.Login},
		{"refresh invalid json", "not json", handler.Refresh},
		{"logout invalid json", "not json", handler.Logout},
		{"verify email invalid json", "not json", handler.VerifyEmail},
		{"forgot password invalid json", "not json", handler.ForgotPassword},
		{"reset password invalid json", "not json", handler.ResetPassword},
		{"resend verification invalid json", "not json", handler.ResendVerification},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			tt.handler(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("got status %d, want %d", rr.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestWriteJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	data := map[string]string{"message": "test"}

	writeJSON(rr, http.StatusOK, data)

	if rr.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusOK)
	}

	contentType := rr.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["message"] != "test" {
		t.Errorf("message = %q, want %q", resp["message"], "test")
	}
}

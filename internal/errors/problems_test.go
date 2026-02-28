package errors

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetRequestID(t *testing.T) {
	tests := []struct {
		name     string
		setupCtx func() *http.Request
		wantID   string
	}{
		{
			name: "request with ID in context",
			setupCtx: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/test", nil)
				ctx := context.WithValue(r.Context(), RequestIDKey(), "test-request-id")
				return r.WithContext(ctx)
			},
			wantID: "test-request-id",
		},
		{
			name: "request without ID",
			setupCtx: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/test", nil)
			},
			wantID: "",
		},
		{
			name: "request with empty ID",
			setupCtx: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/test", nil)
				ctx := context.WithValue(r.Context(), RequestIDKey(), "")
				return r.WithContext(ctx)
			},
			wantID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := tt.setupCtx()
			got := GetRequestID(r)
			if got != tt.wantID {
				t.Errorf("GetRequestID() = %v, want %v", got, tt.wantID)
			}
		})
	}
}

func TestRequestIDKey(t *testing.T) {
	key1 := RequestIDKey()
	key2 := RequestIDKey()

	// Keys should be equal (same type)
	if key1 != key2 {
		t.Error("RequestIDKey() should return consistent key")
	}
}

func TestValidationError(t *testing.T) {
	tests := []struct {
		name        string
		fieldErrors []FieldError
		wantStatus  int
		wantType    string
	}{
		{
			name: "single field error",
			fieldErrors: []FieldError{
				{Field: "email", Message: "invalid email format"},
			},
			wantStatus: http.StatusBadRequest,
			wantType:   ProblemBaseURI + "validation-error",
		},
		{
			name: "multiple field errors",
			fieldErrors: []FieldError{
				{Field: "email", Message: "required"},
				{Field: "password", Message: "too short"},
			},
			wantStatus: http.StatusBadRequest,
			wantType:   ProblemBaseURI + "validation-error",
		},
		{
			name:        "empty field errors",
			fieldErrors: []FieldError{},
			wantStatus:  http.StatusBadRequest,
			wantType:    ProblemBaseURI + "validation-error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/api/register", nil)

			ValidationError(w, r, tt.fieldErrors)

			if w.Code != tt.wantStatus {
				t.Errorf("ValidationError() status = %v, want %v", w.Code, tt.wantStatus)
			}

			var response map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatalf("Failed to parse response: %v", err)
			}

			if response["type"] != tt.wantType {
				t.Errorf("type = %v, want %v", response["type"], tt.wantType)
			}

			if response["title"] != "Validation Failed" {
				t.Errorf("title = %v, want Validation Failed", response["title"])
			}
		})
	}
}

func TestInvalidCredentials(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/login", nil)

	InvalidCredentials(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("InvalidCredentials() status = %v, want %v", w.Code, http.StatusUnauthorized)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["type"] != ProblemBaseURI+"invalid-credentials" {
		t.Errorf("type = %v, want %v", response["type"], ProblemBaseURI+"invalid-credentials")
	}
}

func TestUnauthorized(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/protected", nil)

	Unauthorized(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Unauthorized() status = %v, want %v", w.Code, http.StatusUnauthorized)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["type"] != ProblemBaseURI+"unauthorized" {
		t.Errorf("type = %v, want %v", response["type"], ProblemBaseURI+"unauthorized")
	}
}

func TestEmailNotVerified(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/protected", nil)

	EmailNotVerified(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("EmailNotVerified() status = %v, want %v", w.Code, http.StatusForbidden)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["type"] != ProblemBaseURI+"email-not-verified" {
		t.Errorf("type = %v, want %v", response["type"], ProblemBaseURI+"email-not-verified")
	}
}

func TestForbidden(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodDelete, "/api/admin/users/1", nil)

	Forbidden(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("Forbidden() status = %v, want %v", w.Code, http.StatusForbidden)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["type"] != ProblemBaseURI+"forbidden" {
		t.Errorf("type = %v, want %v", response["type"], ProblemBaseURI+"forbidden")
	}
}

func TestNotFound(t *testing.T) {
	tests := []struct {
		name       string
		resource   string
		wantDetail string
	}{
		{
			name:       "user not found",
			resource:   "User",
			wantDetail: "User not found",
		},
		{
			name:       "space not found",
			resource:   "Space",
			wantDetail: "Space not found",
		},
		{
			name:       "empty resource",
			resource:   "",
			wantDetail: " not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/api/resource/123", nil)

			NotFound(w, r, tt.resource)

			if w.Code != http.StatusNotFound {
				t.Errorf("NotFound() status = %v, want %v", w.Code, http.StatusNotFound)
			}

			var response map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatalf("Failed to parse response: %v", err)
			}

			if response["detail"] != tt.wantDetail {
				t.Errorf("detail = %v, want %v", response["detail"], tt.wantDetail)
			}
		})
	}
}

func TestConflict(t *testing.T) {
	tests := []struct {
		name   string
		detail string
	}{
		{
			name:   "email exists",
			detail: "A user with this email already exists",
		},
		{
			name:   "username taken",
			detail: "Username is already taken",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/api/register", nil)

			Conflict(w, r, tt.detail)

			if w.Code != http.StatusConflict {
				t.Errorf("Conflict() status = %v, want %v", w.Code, http.StatusConflict)
			}

			var response map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatalf("Failed to parse response: %v", err)
			}

			if response["detail"] != tt.detail {
				t.Errorf("detail = %v, want %v", response["detail"], tt.detail)
			}
		})
	}
}

func TestRateLimited(t *testing.T) {
	tests := []struct {
		name       string
		retryAfter int
	}{
		{
			name:       "60 seconds",
			retryAfter: 60,
		},
		{
			name:       "5 seconds",
			retryAfter: 5,
		},
		{
			name:       "zero seconds",
			retryAfter: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/api/login", nil)

			RateLimited(w, r, tt.retryAfter)

			if w.Code != http.StatusTooManyRequests {
				t.Errorf("RateLimited() status = %v, want %v", w.Code, http.StatusTooManyRequests)
			}

			retryHeader := w.Header().Get("Retry-After")
			if retryHeader == "" {
				t.Error("Retry-After header should be set")
			}
		})
	}
}

func TestAccountLocked(t *testing.T) {
	tests := []struct {
		name             string
		remainingMinutes int
		wantContains     string
	}{
		{
			name:             "15 minutes",
			remainingMinutes: 15,
			wantContains:     "15 minutes",
		},
		{
			name:             "1 minute",
			remainingMinutes: 1,
			wantContains:     "1 minutes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/api/login", nil)

			AccountLocked(w, r, tt.remainingMinutes)

			if w.Code != http.StatusLocked {
				t.Errorf("AccountLocked() status = %v, want %v", w.Code, http.StatusLocked)
			}

			var response map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatalf("Failed to parse response: %v", err)
			}

			detail := response["detail"].(string)
			if !strings.Contains(detail, tt.wantContains) {
				t.Errorf("detail = %v, want to contain %v", detail, tt.wantContains)
			}
		})
	}
}

func TestInternalError(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/something", nil)

	InternalError(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("InternalError() status = %v, want %v", w.Code, http.StatusInternalServerError)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["type"] != ProblemBaseURI+"internal-error" {
		t.Errorf("type = %v, want %v", response["type"], ProblemBaseURI+"internal-error")
	}

	// Internal error should not expose sensitive details
	detail := response["detail"].(string)
	if strings.Contains(detail, "panic") || strings.Contains(detail, "stack") {
		t.Error("Internal error should not expose sensitive information")
	}
}

func TestBadRequest(t *testing.T) {
	tests := []struct {
		name   string
		detail string
	}{
		{
			name:   "invalid json",
			detail: "Invalid JSON in request body",
		},
		{
			name:   "missing field",
			detail: "Missing required field: email",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/api/register", nil)

			BadRequest(w, r, tt.detail)

			if w.Code != http.StatusBadRequest {
				t.Errorf("BadRequest() status = %v, want %v", w.Code, http.StatusBadRequest)
			}

			var response map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatalf("Failed to parse response: %v", err)
			}

			if response["detail"] != tt.detail {
				t.Errorf("detail = %v, want %v", response["detail"], tt.detail)
			}
		})
	}
}

func TestInviteInvalid(t *testing.T) {
	tests := []struct {
		name   string
		reason string
	}{
		{
			name:   "expired",
			reason: "Invite has expired",
		},
		{
			name:   "revoked",
			reason: "Invite has been revoked",
		},
		{
			name:   "max uses",
			reason: "Invite has reached maximum uses",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/api/invite/use", nil)

			InviteInvalid(w, r, tt.reason)

			if w.Code != http.StatusBadRequest {
				t.Errorf("InviteInvalid() status = %v, want %v", w.Code, http.StatusBadRequest)
			}

			var response map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatalf("Failed to parse response: %v", err)
			}

			if response["detail"] != tt.reason {
				t.Errorf("detail = %v, want %v", response["detail"], tt.reason)
			}

			if response["type"] != ProblemBaseURI+"invite-invalid" {
				t.Errorf("type = %v, want %v", response["type"], ProblemBaseURI+"invite-invalid")
			}
		})
	}
}

func TestProblemBaseURI(t *testing.T) {
	if ProblemBaseURI != "https://redoubt.app/problems/" {
		t.Errorf("ProblemBaseURI = %v, want https://redoubt.app/problems/", ProblemBaseURI)
	}
}

func TestResponseContentType(t *testing.T) {
	// All error responses should have application/problem+json content type
	errorFunctions := []struct {
		name string
		call func(w http.ResponseWriter, r *http.Request)
	}{
		{"InvalidCredentials", func(w http.ResponseWriter, r *http.Request) { InvalidCredentials(w, r) }},
		{"Unauthorized", func(w http.ResponseWriter, r *http.Request) { Unauthorized(w, r) }},
		{"Forbidden", func(w http.ResponseWriter, r *http.Request) { Forbidden(w, r) }},
		{"InternalError", func(w http.ResponseWriter, r *http.Request) { InternalError(w, r) }},
	}

	for _, ef := range errorFunctions {
		t.Run(ef.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/test", nil)

			ef.call(w, r)

			ct := w.Header().Get("Content-Type")
			if !strings.Contains(ct, "application/problem+json") {
				t.Errorf("%s Content-Type = %v, want application/problem+json", ef.name, ct)
			}
		})
	}
}

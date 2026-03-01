package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	apperrors "github.com/redoubtapp/redoubt-api/internal/errors"
)

func TestRequestID(t *testing.T) {
	tests := []struct {
		name             string
		inputRequestID   string
		wantSameID       bool // whether output should match input
		wantIDInResponse bool
		wantIDInContext  bool
	}{
		{
			name:             "client provides request ID",
			inputRequestID:   "client-request-id-123",
			wantSameID:       true,
			wantIDInResponse: true,
			wantIDInContext:  true,
		},
		{
			name:             "no request ID provided - generates new",
			inputRequestID:   "",
			wantSameID:       false,
			wantIDInResponse: true,
			wantIDInContext:  true,
		},
		{
			name:             "uuid format request ID",
			inputRequestID:   "550e8400-e29b-41d4-a716-446655440000",
			wantSameID:       true,
			wantIDInResponse: true,
			wantIDInContext:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ctxRequestID string

			handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctxRequestID = GetRequestID(r.Context())
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.inputRequestID != "" {
				req.Header.Set(RequestIDHeader, tt.inputRequestID)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			responseID := rec.Header().Get(RequestIDHeader)

			if tt.wantIDInResponse && responseID == "" {
				t.Error("expected request ID in response header")
			}

			if tt.wantIDInContext && ctxRequestID == "" {
				t.Error("expected request ID in context")
			}

			if tt.wantSameID && responseID != tt.inputRequestID {
				t.Errorf("response ID = %v, want %v", responseID, tt.inputRequestID)
			}

			if tt.wantSameID && ctxRequestID != tt.inputRequestID {
				t.Errorf("context ID = %v, want %v", ctxRequestID, tt.inputRequestID)
			}

			// Response and context should always match
			if responseID != ctxRequestID {
				t.Errorf("response ID (%v) != context ID (%v)", responseID, ctxRequestID)
			}

			// If no input, should be a valid UUID
			if !tt.wantSameID && responseID != "" {
				if _, err := uuid.Parse(responseID); err != nil {
					t.Errorf("generated ID is not a valid UUID: %v", responseID)
				}
			}
		})
	}
}

func TestGetRequestIDFromContext(t *testing.T) {
	tests := []struct {
		name   string
		ctx    context.Context
		wantID string
	}{
		{
			name:   "request ID present",
			ctx:    context.WithValue(context.Background(), apperrors.RequestIDKey(), "test-id-123"),
			wantID: "test-id-123",
		},
		{
			name:   "request ID missing",
			ctx:    context.Background(),
			wantID: "",
		},
		{
			name:   "wrong type in context",
			ctx:    context.WithValue(context.Background(), apperrors.RequestIDKey(), 12345),
			wantID: "",
		},
		{
			name:   "empty string ID",
			ctx:    context.WithValue(context.Background(), apperrors.RequestIDKey(), ""),
			wantID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetRequestID(tt.ctx)
			if got != tt.wantID {
				t.Errorf("GetRequestID() = %v, want %v", got, tt.wantID)
			}
		})
	}
}

func TestRequestIDHeader(t *testing.T) {
	if RequestIDHeader != "X-Request-ID" {
		t.Errorf("RequestIDHeader = %v, want X-Request-ID", RequestIDHeader)
	}
}

func TestRequestIDUniqueness(t *testing.T) {
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ids := make(map[string]bool)
	const numRequests = 100

	for i := 0; i < numRequests; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		id := rec.Header().Get(RequestIDHeader)
		if ids[id] {
			t.Errorf("Duplicate request ID generated: %v", id)
		}
		ids[id] = true
	}
}

func TestRequestIDMiddlewareChain(t *testing.T) {
	var requestIDInHandler string
	var requestIDInDownstream string

	// Simulate middleware chain
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestIDInHandler = GetRequestID(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	// Add another middleware after RequestID
	afterRequestID := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestIDInDownstream = GetRequestID(r.Context())
			next.ServeHTTP(w, r)
		})
	}

	handler := RequestID(afterRequestID(finalHandler))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	responseID := rec.Header().Get(RequestIDHeader)

	// All should have the same request ID
	if requestIDInHandler == "" {
		t.Error("request ID not available in handler")
	}
	if requestIDInDownstream == "" {
		t.Error("request ID not available in downstream middleware")
	}
	if requestIDInHandler != requestIDInDownstream {
		t.Error("request ID should be same throughout chain")
	}
	if requestIDInHandler != responseID {
		t.Error("request ID in context should match response header")
	}
}

package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteJSONHealth(t *testing.T) {
	rr := httptest.NewRecorder()
	data := map[string]string{"status": "ok"}

	writeJSONHealth(rr, http.StatusOK, data)

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

	if resp["status"] != "ok" {
		t.Errorf("status = %q, want %q", resp["status"], "ok")
	}
}

func TestWriteJSONHealthStatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"OK", http.StatusOK},
		{"Service Unavailable", http.StatusServiceUnavailable},
		{"Created", http.StatusCreated},
		{"Bad Request", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			writeJSONHealth(rr, tt.statusCode, map[string]string{"test": "value"})

			if rr.Code != tt.statusCode {
				t.Errorf("got status %d, want %d", rr.Code, tt.statusCode)
			}
		})
	}
}

func TestComponentStatus(t *testing.T) {
	// Test that ComponentStatus marshals correctly
	latency := int64(42)
	tests := []struct {
		name     string
		status   ComponentStatus
		wantJSON string
	}{
		{
			name: "healthy with latency",
			status: ComponentStatus{
				Status:    "healthy",
				LatencyMs: &latency,
			},
			wantJSON: `{"status":"healthy","latency_ms":42}`,
		},
		{
			name: "unhealthy with error",
			status: ComponentStatus{
				Status:    "unhealthy",
				LatencyMs: &latency,
				Error:     "connection refused",
			},
			wantJSON: `{"status":"unhealthy","latency_ms":42,"error":"connection refused"}`,
		},
		{
			name: "healthy without latency",
			status: ComponentStatus{
				Status: "healthy",
			},
			wantJSON: `{"status":"healthy"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.status)
			if err != nil {
				t.Fatalf("failed to marshal: %v", err)
			}

			// Compare by unmarshaling both to handle field ordering
			var gotMap, wantMap map[string]any
			if err := json.Unmarshal(got, &gotMap); err != nil {
				t.Fatalf("failed to unmarshal got: %v", err)
			}
			if err := json.Unmarshal([]byte(tt.wantJSON), &wantMap); err != nil {
				t.Fatalf("failed to unmarshal want: %v", err)
			}

			// Check status field
			if gotMap["status"] != wantMap["status"] {
				t.Errorf("status = %v, want %v", gotMap["status"], wantMap["status"])
			}
		})
	}
}

func TestHealthResponse(t *testing.T) {
	latency := int64(5)
	response := HealthResponse{
		Status:  "healthy",
		Version: "1.0.0",
		Components: map[string]ComponentStatus{
			"database": {
				Status:    "healthy",
				LatencyMs: &latency,
			},
			"redis": {
				Status:    "healthy",
				LatencyMs: &latency,
			},
		},
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var got HealthResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if got.Status != "healthy" {
		t.Errorf("Status = %q, want %q", got.Status, "healthy")
	}

	if got.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", got.Version, "1.0.0")
	}

	if len(got.Components) != 2 {
		t.Errorf("len(Components) = %d, want %d", len(got.Components), 2)
	}
}

func TestNewHealthHandler(t *testing.T) {
	deps := &HealthDependencies{
		Version: "test-version",
	}

	handler := NewHealthHandler(deps)
	if handler == nil {
		t.Fatal("NewHealthHandler returned nil")
	}

	if handler.deps != deps {
		t.Error("handler.deps does not match input")
	}
}

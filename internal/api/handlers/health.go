package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/redoubtapp/redoubt-api/internal/cache"
)

// writeJSONHealth writes a JSON response for health endpoints.
func writeJSONHealth(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode JSON response", slog.String("error", err.Error()))
	}
}

// HealthDependencies holds dependencies for health checks.
type HealthDependencies struct {
	DB      *pgxpool.Pool
	Redis   *cache.Client
	Version string
}

// HealthHandler handles health check endpoints.
type HealthHandler struct {
	deps *HealthDependencies
}

// NewHealthHandler creates a new health handler.
func NewHealthHandler(deps *HealthDependencies) *HealthHandler {
	return &HealthHandler{deps: deps}
}

// ComponentStatus represents the health of a single component.
type ComponentStatus struct {
	Status    string `json:"status"`
	LatencyMs *int64 `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

// HealthResponse is the response for the /health endpoint.
type HealthResponse struct {
	Status     string                     `json:"status"`
	Version    string                     `json:"version"`
	Components map[string]ComponentStatus `json:"components"`
}

// Health returns detailed health information about all components.
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	response := HealthResponse{
		Status:     "healthy",
		Version:    h.deps.Version,
		Components: make(map[string]ComponentStatus),
	}

	// Check database health
	dbStatus := h.checkDatabase(ctx)
	response.Components["database"] = dbStatus
	if dbStatus.Status != "healthy" {
		response.Status = "unhealthy"
	}

	// Check Redis health
	redisStatus := h.checkRedis(ctx)
	response.Components["redis"] = redisStatus
	if redisStatus.Status != "healthy" {
		response.Status = "unhealthy"
	}

	// Set appropriate status code
	statusCode := http.StatusOK
	if response.Status != "healthy" {
		statusCode = http.StatusServiceUnavailable
	}

	writeJSONHealth(w, statusCode, response)
}

// Live is a simple liveness probe that returns 200 if the process is running.
func (h *HealthHandler) Live(w http.ResponseWriter, r *http.Request) {
	writeJSONHealth(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Ready checks if the service is ready to accept traffic.
func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Check database
	if err := h.deps.DB.Ping(ctx); err != nil {
		writeJSONHealth(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not ready",
			"reason": "database unavailable",
		})
		return
	}

	// Check Redis
	if err := h.deps.Redis.HealthCheck(ctx); err != nil {
		writeJSONHealth(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not ready",
			"reason": "redis unavailable",
		})
		return
	}

	writeJSONHealth(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (h *HealthHandler) checkDatabase(ctx context.Context) ComponentStatus {
	start := time.Now()
	err := h.deps.DB.Ping(ctx)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return ComponentStatus{
			Status:    "unhealthy",
			LatencyMs: &latency,
			Error:     err.Error(),
		}
	}

	return ComponentStatus{
		Status:    "healthy",
		LatencyMs: &latency,
	}
}

func (h *HealthHandler) checkRedis(ctx context.Context) ComponentStatus {
	start := time.Now()
	err := h.deps.Redis.HealthCheck(ctx)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return ComponentStatus{
			Status:    "unhealthy",
			LatencyMs: &latency,
			Error:     err.Error(),
		}
	}

	return ComponentStatus{
		Status:    "healthy",
		LatencyMs: &latency,
	}
}

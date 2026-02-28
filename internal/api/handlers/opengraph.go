package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"

	apperrors "github.com/redoubtapp/redoubt-api/internal/errors"
	"github.com/redoubtapp/redoubt-api/internal/opengraph"
)

// OpenGraphHandler handles OpenGraph metadata endpoints.
type OpenGraphHandler struct {
	service *opengraph.Service
}

// NewOpenGraphHandler creates a new OpenGraph handler.
func NewOpenGraphHandler(service *opengraph.Service) *OpenGraphHandler {
	return &OpenGraphHandler{
		service: service,
	}
}

// FetchMetadataRequest is the request body for fetching metadata.
type FetchMetadataRequest struct {
	URL string `json:"url"`
}

// FetchMetadata fetches OpenGraph metadata for a URL.
func (h *OpenGraphHandler) FetchMetadata(w http.ResponseWriter, r *http.Request) {
	var req FetchMetadataRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.BadRequest(w, r, "Invalid request body")
		return
	}

	if req.URL == "" {
		apperrors.BadRequest(w, r, "URL is required")
		return
	}

	// Validate URL
	parsedURL, err := url.Parse(req.URL)
	if err != nil {
		apperrors.BadRequest(w, r, "Invalid URL")
		return
	}

	// Only allow http/https
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		apperrors.BadRequest(w, r, "Only HTTP/HTTPS URLs are supported")
		return
	}

	metadata, err := h.service.Fetch(r.Context(), req.URL)
	if err != nil {
		// Return empty metadata rather than error for fetch failures
		metadata = &opengraph.Metadata{URL: req.URL}
	}

	writeJSON(w, http.StatusOK, metadata)
}

package handlers

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/redoubtapp/redoubt-api/internal/api/middleware"
	"github.com/redoubtapp/redoubt-api/internal/db/generated"
	apperrors "github.com/redoubtapp/redoubt-api/internal/errors"
	"github.com/redoubtapp/redoubt-api/internal/storage"
)

// UserHandler handles user endpoints.
type UserHandler struct {
	queries        *generated.Queries
	storageService *storage.Service
	validate       *validator.Validate
}

// NewUserHandler creates a new user handler.
func NewUserHandler(queries *generated.Queries, storageService *storage.Service) *UserHandler {
	return &UserHandler{
		queries:        queries,
		storageService: storageService,
		validate:       validator.New(),
	}
}

// GetCurrentUser returns the current authenticated user.
func (h *UserHandler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	user, err := h.queries.GetUserByID(r.Context(), pgtype.UUID{Bytes: userID, Valid: true})
	if err != nil {
		apperrors.NotFound(w, r, "User")
		return
	}

	writeJSON(w, http.StatusOK, userToResponse(user))
}

// UpdateUserRequest is the request body for updating a user.
type UpdateUserRequest struct {
	Username *string `json:"username" validate:"omitempty,min=3,max=32,alphanum"`
}

// UpdateCurrentUser updates the current authenticated user.
func (h *UserHandler) UpdateCurrentUser(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	var req UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.BadRequest(w, r, "Invalid request body")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		fieldErrors := validationErrors(err)
		apperrors.ValidationError(w, r, fieldErrors)
		return
	}

	// Check if username is already taken
	if req.Username != nil {
		existing, err := h.queries.GetUserByUsername(r.Context(), *req.Username)
		if err == nil && uuidToString(existing.ID) != userID.String() {
			apperrors.Conflict(w, r, "Username is already taken")
			return
		}
	}

	// Build update params
	params := generated.UpdateUserParams{
		ID: pgtype.UUID{Bytes: userID, Valid: true},
	}
	if req.Username != nil {
		params.Username = pgtype.Text{String: *req.Username, Valid: true}
	}

	user, err := h.queries.UpdateUser(r.Context(), params)
	if err != nil {
		apperrors.InternalError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, userToResponse(user))
}

// DeleteCurrentUser soft deletes the current authenticated user.
func (h *UserHandler) DeleteCurrentUser(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	// Delete avatar if exists
	if h.storageService != nil {
		_ = h.storageService.DeleteAvatar(r.Context(), userID)
	}

	// Soft delete user
	if err := h.queries.SoftDeleteUser(r.Context(), pgtype.UUID{Bytes: userID, Valid: true}); err != nil {
		apperrors.InternalError(w, r)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// UploadAvatar uploads an avatar for the current user.
func (h *UserHandler) UploadAvatar(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	// Parse multipart form with max file size
	if err := r.ParseMultipartForm(storage.MaxFileSize); err != nil {
		apperrors.FileTooLarge(w, r)
		return
	}

	file, header, err := r.FormFile("avatar")
	if err != nil {
		apperrors.BadRequest(w, r, "No avatar file provided")
		return
	}
	defer func() { _ = file.Close() }()

	// Read file data
	data, err := io.ReadAll(file)
	if err != nil {
		apperrors.InternalError(w, r)
		return
	}

	// Upload avatar
	mediaFile, err := h.storageService.UploadAvatar(r.Context(), userID, data, header.Filename)
	if err != nil {
		handleStorageError(w, r, err)
		return
	}

	// Update user's avatar URL
	avatarURL := "/api/v1/users/" + userID.String() + "/avatar"
	if err := h.queries.UpdateUserAvatar(r.Context(), generated.UpdateUserAvatarParams{
		ID:        pgtype.UUID{Bytes: userID, Valid: true},
		AvatarUrl: pgtype.Text{String: avatarURL, Valid: true},
	}); err != nil {
		apperrors.InternalError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"avatar_url":   avatarURL,
		"content_type": mediaFile.ContentType,
		"size_bytes":   mediaFile.SizeBytes,
	})
}

// DeleteAvatar removes the avatar for the current user.
func (h *UserHandler) DeleteAvatar(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	// Delete avatar from storage
	if err := h.storageService.DeleteAvatar(r.Context(), userID); err != nil {
		if err == apperrors.ErrAvatarNotFound {
			apperrors.NotFound(w, r, "Avatar")
			return
		}
		apperrors.InternalError(w, r)
		return
	}

	// Remove avatar URL from user
	if err := h.queries.RemoveUserAvatar(r.Context(), pgtype.UUID{Bytes: userID, Valid: true}); err != nil {
		apperrors.InternalError(w, r)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetAvatar retrieves an avatar by user ID.
func (h *UserHandler) GetAvatar(w http.ResponseWriter, r *http.Request) {
	userIDStr := mux.Vars(r)["id"]
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		apperrors.BadRequest(w, r, "Invalid user ID")
		return
	}

	// Get avatar data
	data, contentType, err := h.storageService.GetAvatar(r.Context(), userID)
	if err != nil {
		if err == apperrors.ErrAvatarNotFound {
			apperrors.NotFound(w, r, "Avatar")
			return
		}
		apperrors.InternalError(w, r)
		return
	}

	// Set headers and write response
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400") // Cache for 1 day
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// userToResponse converts a database user to API response.
func userToResponse(user generated.User) UserResponse {
	var avatarURL *string
	if user.AvatarUrl.Valid {
		avatarURL = &user.AvatarUrl.String
	}

	return UserResponse{
		ID:              uuidToString(user.ID),
		Username:        user.Username,
		Email:           user.Email,
		AvatarURL:       avatarURL,
		IsInstanceAdmin: user.IsInstanceAdmin,
		EmailVerified:   user.EmailVerified,
		CreatedAt:       user.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:       user.UpdatedAt.Time.Format("2006-01-02T15:04:05Z"),
	}
}

// uuidToString converts a pgtype.UUID to string.
func uuidToString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}

// handleStorageError maps storage errors to HTTP responses.
func handleStorageError(w http.ResponseWriter, r *http.Request, err error) {
	switch err {
	case apperrors.ErrFileTooLarge:
		apperrors.FileTooLarge(w, r)
	case apperrors.ErrInvalidFileType:
		apperrors.InvalidFileType(w, r)
	case apperrors.ErrAvatarNotFound:
		apperrors.NotFound(w, r, "Avatar")
	default:
		apperrors.InternalError(w, r)
	}
}

package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/redoubtapp/redoubt-api/internal/api/middleware"
	"github.com/redoubtapp/redoubt-api/internal/audit"
	"github.com/redoubtapp/redoubt-api/internal/db/generated"
	apperrors "github.com/redoubtapp/redoubt-api/internal/errors"
	"github.com/redoubtapp/redoubt-api/internal/spaces"
)

// SpaceHandler handles space endpoints.
type SpaceHandler struct {
	spaceService *spaces.Service
	auditService *audit.Service
	validate     *validator.Validate
}

// NewSpaceHandler creates a new space handler.
func NewSpaceHandler(spaceService *spaces.Service, auditService *audit.Service) *SpaceHandler {
	return &SpaceHandler{
		spaceService: spaceService,
		auditService: auditService,
		validate:     validator.New(),
	}
}

// SpaceResponse is the response for a space.
type SpaceResponse struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	IconURL   *string `json:"icon_url"`
	OwnerID   string  `json:"owner_id"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

// ChannelResponse is the response for a channel.
type ChannelResponse struct {
	ID        string `json:"id"`
	SpaceID   string `json:"space_id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Position  int32  `json:"position"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// ListSpacesResponse is the response for listing spaces.
type ListSpacesResponse struct {
	Spaces []SpaceResponse `json:"spaces"`
}

// CreateSpaceRequest is the request body for creating a space.
type CreateSpaceRequest struct {
	Name    string  `json:"name" validate:"required,min=1,max=100"`
	IconURL *string `json:"icon_url" validate:"omitempty,url"`
}

// CreateSpaceResponse is the response for creating a space.
type CreateSpaceResponse struct {
	Space    SpaceResponse     `json:"space"`
	Channels []ChannelResponse `json:"channels"`
}

// UpdateSpaceRequest is the request body for updating a space.
type UpdateSpaceRequest struct {
	Name    *string `json:"name" validate:"omitempty,min=1,max=100"`
	IconURL *string `json:"icon_url" validate:"omitempty,url"`
}

// MemberResponse is the response for a space member.
type MemberResponse struct {
	UserID    string  `json:"user_id"`
	Username  string  `json:"username"`
	Email     string  `json:"email"`
	AvatarURL *string `json:"avatar_url"`
	Role      string  `json:"role"`
	JoinedAt  string  `json:"joined_at"`
}

// ListMembersResponse is the response for listing members.
type ListMembersResponse struct {
	Members []MemberResponse `json:"members"`
}

// ChangeMemberRoleRequest is the request body for changing a member's role.
type ChangeMemberRoleRequest struct {
	Role string `json:"role" validate:"required,oneof=admin member"`
}

// ListSpaces lists all spaces the user is a member of.
func (h *SpaceHandler) ListSpaces(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	userSpaces, err := h.spaceService.ListUserSpaces(r.Context(), userID)
	if err != nil {
		apperrors.InternalError(w, r)
		return
	}

	response := ListSpacesResponse{
		Spaces: make([]SpaceResponse, 0, len(userSpaces)),
	}

	for _, space := range userSpaces {
		response.Spaces = append(response.Spaces, spaceToResponse(space))
	}

	writeJSON(w, http.StatusOK, response)
}

// CreateSpace creates a new space.
func (h *SpaceHandler) CreateSpace(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	// Only instance admins can create spaces
	if !middleware.GetIsAdmin(r.Context()) {
		apperrors.Forbidden(w, r)
		return
	}

	var req CreateSpaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.BadRequest(w, r, "Invalid request body")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		fieldErrors := validationErrors(err)
		apperrors.ValidationError(w, r, fieldErrors)
		return
	}

	result, err := h.spaceService.CreateSpace(r.Context(), spaces.CreateSpaceRequest{
		Name:    req.Name,
		IconURL: req.IconURL,
		OwnerID: userID,
	})
	if err != nil {
		apperrors.InternalError(w, r)
		return
	}

	// Audit log
	if h.auditService != nil {
		spaceID := spaces.UUIDFromPgtype(result.Space.ID)
		_ = h.auditService.LogSpaceCreate(r.Context(), userID, spaceID, result.Space.Name, getClientIP(r))
	}

	channels := make([]ChannelResponse, 0, len(result.Channels))
	for _, ch := range result.Channels {
		channels = append(channels, channelToResponse(ch))
	}

	writeJSON(w, http.StatusCreated, CreateSpaceResponse{
		Space:    spaceToResponse(result.Space),
		Channels: channels,
	})
}

// GetSpace returns a space by ID.
func (h *SpaceHandler) GetSpace(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	spaceID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		apperrors.BadRequest(w, r, "Invalid space ID")
		return
	}

	isAdmin := middleware.GetIsAdmin(r.Context())
	space, err := h.spaceService.GetSpace(r.Context(), spaceID, userID, isAdmin)
	if err != nil {
		handleSpaceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, spaceToResponse(*space))
}

// UpdateSpace updates a space.
func (h *SpaceHandler) UpdateSpace(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	spaceID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		apperrors.BadRequest(w, r, "Invalid space ID")
		return
	}

	var req UpdateSpaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.BadRequest(w, r, "Invalid request body")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		fieldErrors := validationErrors(err)
		apperrors.ValidationError(w, r, fieldErrors)
		return
	}

	isAdmin := middleware.GetIsAdmin(r.Context())
	space, err := h.spaceService.UpdateSpace(r.Context(), spaces.UpdateSpaceRequest{
		SpaceID: spaceID,
		Name:    req.Name,
		IconURL: req.IconURL,
	}, userID, isAdmin)
	if err != nil {
		handleSpaceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, spaceToResponse(*space))
}

// DeleteSpace soft-deletes a space.
func (h *SpaceHandler) DeleteSpace(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	spaceID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		apperrors.BadRequest(w, r, "Invalid space ID")
		return
	}

	isAdmin := middleware.GetIsAdmin(r.Context())
	if err := h.spaceService.DeleteSpace(r.Context(), spaceID, userID, isAdmin); err != nil {
		handleSpaceError(w, r, err)
		return
	}

	// Audit log
	if h.auditService != nil {
		_ = h.auditService.LogSpaceDelete(r.Context(), userID, spaceID, "", getClientIP(r))
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListMembers lists all members of a space.
func (h *SpaceHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	spaceID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		apperrors.BadRequest(w, r, "Invalid space ID")
		return
	}

	isAdmin := middleware.GetIsAdmin(r.Context())
	members, err := h.spaceService.ListSpaceMembers(r.Context(), spaceID, userID, isAdmin)
	if err != nil {
		handleSpaceError(w, r, err)
		return
	}

	response := ListMembersResponse{
		Members: make([]MemberResponse, 0, len(members)),
	}

	for _, member := range members {
		var avatarURL *string
		if member.AvatarUrl.Valid {
			avatarURL = &member.AvatarUrl.String
		}

		response.Members = append(response.Members, MemberResponse{
			UserID:    spaces.UUIDFromPgtype(member.UserID).String(),
			Username:  member.Username,
			Email:     member.Email,
			AvatarURL: avatarURL,
			Role:      string(member.Role),
			JoinedAt:  member.JoinedAt.Time.Format("2006-01-02T15:04:05Z"),
		})
	}

	writeJSON(w, http.StatusOK, response)
}

// KickMember removes a member from a space.
func (h *SpaceHandler) KickMember(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	vars := mux.Vars(r)
	spaceID, err := uuid.Parse(vars["id"])
	if err != nil {
		apperrors.BadRequest(w, r, "Invalid space ID")
		return
	}

	targetUserID, err := uuid.Parse(vars["userId"])
	if err != nil {
		apperrors.BadRequest(w, r, "Invalid user ID")
		return
	}

	isAdmin := middleware.GetIsAdmin(r.Context())
	if err := h.spaceService.KickMember(r.Context(), spaceID, targetUserID, userID, isAdmin); err != nil {
		handleSpaceError(w, r, err)
		return
	}

	// Audit log
	if h.auditService != nil {
		_ = h.auditService.LogMemberKick(r.Context(), userID, targetUserID, spaceID, getClientIP(r))
	}

	w.WriteHeader(http.StatusNoContent)
}

// ChangeMemberRole changes a member's role.
func (h *SpaceHandler) ChangeMemberRole(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	vars := mux.Vars(r)
	spaceID, err := uuid.Parse(vars["id"])
	if err != nil {
		apperrors.BadRequest(w, r, "Invalid space ID")
		return
	}

	targetUserID, err := uuid.Parse(vars["userId"])
	if err != nil {
		apperrors.BadRequest(w, r, "Invalid user ID")
		return
	}

	var req ChangeMemberRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.BadRequest(w, r, "Invalid request body")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		fieldErrors := validationErrors(err)
		apperrors.ValidationError(w, r, fieldErrors)
		return
	}

	newRole := generated.MembershipRole(req.Role)
	isAdmin := middleware.GetIsAdmin(r.Context())
	if err := h.spaceService.ChangeMemberRole(r.Context(), spaceID, targetUserID, userID, newRole, isAdmin); err != nil {
		handleSpaceError(w, r, err)
		return
	}

	// Audit log
	if h.auditService != nil {
		_ = h.auditService.LogMemberRoleChange(r.Context(), userID, targetUserID, spaceID, "", req.Role, getClientIP(r))
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Role updated successfully"})
}

// spaceToResponse converts a Space to SpaceResponse.
func spaceToResponse(space generated.Space) SpaceResponse {
	var iconURL *string
	if space.IconUrl.Valid {
		iconURL = &space.IconUrl.String
	}

	return SpaceResponse{
		ID:        spaces.UUIDFromPgtype(space.ID).String(),
		Name:      space.Name,
		IconURL:   iconURL,
		OwnerID:   spaces.UUIDFromPgtype(space.OwnerID).String(),
		CreatedAt: space.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: space.UpdatedAt.Time.Format("2006-01-02T15:04:05Z"),
	}
}

// channelToResponse converts a Channel to ChannelResponse.
func channelToResponse(channel generated.Channel) ChannelResponse {
	return ChannelResponse{
		ID:        spaces.UUIDFromPgtype(channel.ID).String(),
		SpaceID:   spaces.UUIDFromPgtype(channel.SpaceID).String(),
		Name:      channel.Name,
		Type:      string(channel.Type),
		Position:  channel.Position,
		CreatedAt: channel.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: channel.UpdatedAt.Time.Format("2006-01-02T15:04:05Z"),
	}
}

// handleSpaceError maps space errors to HTTP responses.
func handleSpaceError(w http.ResponseWriter, r *http.Request, err error) {
	switch err {
	case apperrors.ErrSpaceNotFound:
		apperrors.NotFound(w, r, "Space")
	case apperrors.ErrForbidden, apperrors.ErrInsufficientRole:
		apperrors.Forbidden(w, r)
	case apperrors.ErrMembershipNotFound:
		apperrors.NotFound(w, r, "Member")
	case apperrors.ErrCannotKickOwner:
		apperrors.BadRequest(w, r, "Cannot kick the space owner")
	case apperrors.ErrCannotChangeOwner:
		apperrors.BadRequest(w, r, "Cannot change owner's role")
	default:
		apperrors.InternalError(w, r)
	}
}

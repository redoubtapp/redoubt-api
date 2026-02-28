package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/redoubtapp/redoubt-api/internal/api/middleware"
	"github.com/redoubtapp/redoubt-api/internal/audit"
	"github.com/redoubtapp/redoubt-api/internal/db/generated"
	apperrors "github.com/redoubtapp/redoubt-api/internal/errors"
	"github.com/redoubtapp/redoubt-api/internal/invites"
)

// InviteHandler handles invite endpoints.
type InviteHandler struct {
	inviteService *invites.Service
	auditService  *audit.Service
	validate      *validator.Validate
}

// NewInviteHandler creates a new invite handler.
func NewInviteHandler(inviteService *invites.Service, auditService *audit.Service) *InviteHandler {
	return &InviteHandler{
		inviteService: inviteService,
		auditService:  auditService,
		validate:      validator.New(),
	}
}

// InviteResponse is the response for an invite.
type InviteResponse struct {
	ID                string  `json:"id"`
	Code              string  `json:"code"`
	SpaceID           string  `json:"space_id"`
	CreatedBy         string  `json:"created_by"`
	CreatedByUsername string  `json:"created_by_username,omitempty"`
	Uses              int32   `json:"uses"`
	MaxUses           *int32  `json:"max_uses"`
	ExpiresAt         *string `json:"expires_at"`
	CreatedAt         string  `json:"created_at"`
}

// InviteInfoResponse is the public response for invite info.
type InviteInfoResponse struct {
	Code         string  `json:"code"`
	SpaceName    string  `json:"space_name"`
	SpaceIconURL *string `json:"space_icon_url"`
}

// ListInvitesResponse is the response for listing invites.
type ListInvitesResponse struct {
	Invites []InviteResponse `json:"invites"`
}

// CreateInviteRequest is the request body for creating an invite.
type CreateInviteRequest struct {
	MaxUses        *int32 `json:"max_uses" validate:"omitempty,min=1"`
	ExpiresInHours *int   `json:"expires_in_hours" validate:"omitempty,min=1,max=8760"` // max 1 year
}

// JoinSpaceResponse is the response for joining a space.
type JoinSpaceResponse struct {
	Space SpaceResponse `json:"space"`
}

// ListInvites lists all active invites for a space.
func (h *InviteHandler) ListInvites(w http.ResponseWriter, r *http.Request) {
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
	inviteList, err := h.inviteService.ListSpaceInvites(r.Context(), spaceID, userID, isAdmin)
	if err != nil {
		handleInviteError(w, r, err)
		return
	}

	response := ListInvitesResponse{
		Invites: make([]InviteResponse, 0, len(inviteList)),
	}

	for _, inv := range inviteList {
		response.Invites = append(response.Invites, inviteRowToResponse(inv))
	}

	writeJSON(w, http.StatusOK, response)
}

// CreateInvite creates a new invite for a space.
func (h *InviteHandler) CreateInvite(w http.ResponseWriter, r *http.Request) {
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

	var req CreateInviteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Allow empty body for default invite
		req = CreateInviteRequest{}
	}

	if err := h.validate.Struct(req); err != nil {
		fieldErrors := validationErrors(err)
		apperrors.ValidationError(w, r, fieldErrors)
		return
	}

	createReq := invites.CreateInviteRequest{
		SpaceID:   spaceID,
		CreatedBy: userID,
		MaxUses:   req.MaxUses,
	}

	if req.ExpiresInHours != nil {
		expiresIn := time.Duration(*req.ExpiresInHours) * time.Hour
		createReq.ExpiresIn = &expiresIn
	}

	isAdmin := middleware.GetIsAdmin(r.Context())
	invite, err := h.inviteService.CreateInvite(r.Context(), createReq, userID, isAdmin)
	if err != nil {
		handleInviteError(w, r, err)
		return
	}

	// Audit log
	if h.auditService != nil {
		inviteID := invites.UUIDFromPgtype(invite.ID)
		_ = h.auditService.LogInviteCreate(r.Context(), userID, inviteID, spaceID, invite.Code, getClientIP(r))
	}

	var maxUses *int32
	if invite.MaxUses.Valid {
		maxUses = &invite.MaxUses.Int32
	}

	var expiresAt *string
	if invite.ExpiresAt.Valid {
		t := invite.ExpiresAt.Time.Format("2006-01-02T15:04:05Z")
		expiresAt = &t
	}

	writeJSON(w, http.StatusCreated, InviteResponse{
		ID:        invites.UUIDFromPgtype(invite.ID).String(),
		Code:      invite.Code,
		SpaceID:   invites.UUIDFromPgtype(invite.SpaceID).String(),
		CreatedBy: invites.UUIDFromPgtype(invite.CreatedBy).String(),
		Uses:      invite.Uses,
		MaxUses:   maxUses,
		ExpiresAt: expiresAt,
		CreatedAt: invite.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
	})
}

// RevokeInvite revokes an invite.
func (h *InviteHandler) RevokeInvite(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	inviteID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		apperrors.BadRequest(w, r, "Invalid invite ID")
		return
	}

	isAdmin := middleware.GetIsAdmin(r.Context())

	// Get invite info for audit log before revoking
	var inviteCode string
	var spaceID uuid.UUID
	if h.auditService != nil {
		if invite, err := h.inviteService.GetInviteByID(r.Context(), inviteID, userID, isAdmin); err == nil {
			inviteCode = invite.Code
			spaceID = invites.UUIDFromPgtype(invite.SpaceID)
		}
	}

	if err := h.inviteService.RevokeInvite(r.Context(), inviteID, userID, isAdmin); err != nil {
		handleInviteError(w, r, err)
		return
	}

	// Audit log
	if h.auditService != nil {
		_ = h.auditService.LogInviteRevoke(r.Context(), userID, inviteID, spaceID, inviteCode, getClientIP(r))
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetInviteInfo returns public information about an invite (no auth required).
func (h *InviteHandler) GetInviteInfo(w http.ResponseWriter, r *http.Request) {
	code := mux.Vars(r)["code"]
	if code == "" {
		apperrors.BadRequest(w, r, "Invite code is required")
		return
	}

	info, err := h.inviteService.GetInviteInfo(r.Context(), code)
	if err != nil {
		handleInviteError(w, r, err)
		return
	}

	var iconURL *string
	if info.SpaceIconUrl.Valid {
		iconURL = &info.SpaceIconUrl.String
	}

	writeJSON(w, http.StatusOK, InviteInfoResponse{
		Code:         info.Code,
		SpaceName:    info.SpaceName,
		SpaceIconURL: iconURL,
	})
}

// JoinViaInvite allows a user to join a space using an invite code.
func (h *InviteHandler) JoinViaInvite(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	code := mux.Vars(r)["code"]
	if code == "" {
		apperrors.BadRequest(w, r, "Invite code is required")
		return
	}

	space, err := h.inviteService.JoinSpaceViaInvite(r.Context(), code, userID)
	if err != nil {
		handleInviteError(w, r, err)
		return
	}

	var iconURL *string
	if space.IconUrl.Valid {
		iconURL = &space.IconUrl.String
	}

	writeJSON(w, http.StatusOK, JoinSpaceResponse{
		Space: SpaceResponse{
			ID:        invites.UUIDFromPgtype(space.ID).String(),
			Name:      space.Name,
			IconURL:   iconURL,
			OwnerID:   invites.UUIDFromPgtype(space.OwnerID).String(),
			CreatedAt: space.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
			UpdatedAt: space.UpdatedAt.Time.Format("2006-01-02T15:04:05Z"),
		},
	})
}

// inviteRowToResponse converts a ListSpaceInvitesRow to InviteResponse.
func inviteRowToResponse(inv generated.ListSpaceInvitesRow) InviteResponse {
	var maxUses *int32
	if inv.MaxUses.Valid {
		maxUses = &inv.MaxUses.Int32
	}

	var expiresAt *string
	if inv.ExpiresAt.Valid {
		t := inv.ExpiresAt.Time.Format("2006-01-02T15:04:05Z")
		expiresAt = &t
	}

	return InviteResponse{
		ID:                invites.UUIDFromPgtype(inv.ID).String(),
		Code:              inv.Code,
		SpaceID:           invites.UUIDFromPgtype(inv.SpaceID).String(),
		CreatedBy:         invites.UUIDFromPgtype(inv.CreatedBy).String(),
		CreatedByUsername: inv.CreatedByUsername,
		Uses:              inv.Uses,
		MaxUses:           maxUses,
		ExpiresAt:         expiresAt,
		CreatedAt:         inv.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
	}
}

// handleInviteError maps invite errors to HTTP responses.
func handleInviteError(w http.ResponseWriter, r *http.Request, err error) {
	switch err {
	case apperrors.ErrInviteNotFound:
		apperrors.NotFound(w, r, "Invite")
	case apperrors.ErrInviteExpired:
		apperrors.InviteInvalid(w, r, "This invite has expired")
	case apperrors.ErrInviteRevoked:
		apperrors.InviteInvalid(w, r, "This invite has been revoked")
	case apperrors.ErrInviteExhausted:
		apperrors.InviteInvalid(w, r, "This invite has reached its maximum uses")
	case apperrors.ErrAlreadyMember:
		apperrors.Conflict(w, r, "You are already a member of this space")
	case apperrors.ErrSpaceNotFound:
		apperrors.NotFound(w, r, "Space")
	case apperrors.ErrForbidden, apperrors.ErrInsufficientRole:
		apperrors.Forbidden(w, r)
	default:
		apperrors.InternalError(w, r)
	}
}

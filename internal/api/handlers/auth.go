package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-playground/validator/v10"

	"github.com/redoubtapp/redoubt-api/internal/auth"
	apperrors "github.com/redoubtapp/redoubt-api/internal/errors"
)

// writeJSON writes a JSON response. Errors are logged but not returned
// since headers have already been written at this point.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode JSON response", slog.String("error", err.Error()))
	}
}

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	authService *auth.Service
	validate    *validator.Validate
}

// NewAuthHandler creates a new auth handler.
func NewAuthHandler(authService *auth.Service) *AuthHandler {
	return &AuthHandler{
		authService: authService,
		validate:    validator.New(),
	}
}

// RegisterRequest is the request body for registration.
type RegisterRequest struct {
	Username   string `json:"username" validate:"required,min=3,max=32,alphanum"`
	Email      string `json:"email" validate:"required,email"`
	Password   string `json:"password" validate:"required,min=12"`
	InviteCode string `json:"invite_code" validate:"required"`
}

// RegisterResponse is the response for successful registration.
type RegisterResponse struct {
	Message string `json:"message"`
}

// Register handles user registration.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.BadRequest(w, r, "Invalid request body")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		fieldErrors := validationErrors(err)
		apperrors.ValidationError(w, r, fieldErrors)
		return
	}

	_, err := h.authService.Register(r.Context(), auth.RegisterRequest{
		Username:   req.Username,
		Email:      req.Email,
		Password:   req.Password,
		InviteCode: req.InviteCode,
	})
	if err != nil {
		handleAuthError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, RegisterResponse{
		Message: "Registration successful. Please check your email to verify your account.",
	})
}

// LoginRequest is the request body for login.
type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// LoginResponse is the response for successful login.
type LoginResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	ExpiresIn    int          `json:"expires_in"`
	ExpiresAt    string       `json:"expires_at"`
	User         UserResponse `json:"user"`
}

// UserResponse is the user data in API responses.
type UserResponse struct {
	ID              string  `json:"id"`
	Username        string  `json:"username"`
	Email           string  `json:"email"`
	AvatarURL       *string `json:"avatar_url"`
	IsInstanceAdmin bool    `json:"is_instance_admin"`
	EmailVerified   bool    `json:"email_verified,omitempty"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at,omitempty"`
}

// Login handles user login.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.BadRequest(w, r, "Invalid request body")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		fieldErrors := validationErrors(err)
		apperrors.ValidationError(w, r, fieldErrors)
		return
	}

	result, err := h.authService.Login(r.Context(), auth.LoginRequest{
		Email:     req.Email,
		Password:  req.Password,
		UserAgent: r.UserAgent(),
		IPAddress: getClientIP(r),
	})
	if err != nil {
		handleAuthError(w, r, err)
		return
	}

	var avatarURL *string
	if result.User.AvatarUrl.Valid {
		avatarURL = &result.User.AvatarUrl.String
	}

	expiresAt := time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)

	writeJSON(w, http.StatusOK, LoginResponse{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresIn:    result.ExpiresIn,
		ExpiresAt:    expiresAt.Format(time.RFC3339),
		User: UserResponse{
			ID:              auth.UUIDFromPgtype(result.User.ID).String(),
			Username:        result.User.Username,
			Email:           result.User.Email,
			AvatarURL:       avatarURL,
			IsInstanceAdmin: result.User.IsInstanceAdmin,
			CreatedAt:       result.User.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
		},
	})
}

// RefreshRequest is the request body for token refresh.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// Refresh handles token refresh.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.BadRequest(w, r, "Invalid request body")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		fieldErrors := validationErrors(err)
		apperrors.ValidationError(w, r, fieldErrors)
		return
	}

	result, err := h.authService.RefreshTokens(r.Context(), req.RefreshToken)
	if err != nil {
		handleAuthError(w, r, err)
		return
	}

	var avatarURL *string
	if result.User.AvatarUrl.Valid {
		avatarURL = &result.User.AvatarUrl.String
	}

	refreshExpiresAt := time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)

	writeJSON(w, http.StatusOK, LoginResponse{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresIn:    result.ExpiresIn,
		ExpiresAt:    refreshExpiresAt.Format(time.RFC3339),
		User: UserResponse{
			ID:              auth.UUIDFromPgtype(result.User.ID).String(),
			Username:        result.User.Username,
			Email:           result.User.Email,
			AvatarURL:       avatarURL,
			IsInstanceAdmin: result.User.IsInstanceAdmin,
			CreatedAt:       result.User.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
		},
	})
}

// LogoutRequest is the request body for logout.
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// Logout handles user logout.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req LogoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.BadRequest(w, r, "Invalid request body")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		fieldErrors := validationErrors(err)
		apperrors.ValidationError(w, r, fieldErrors)
		return
	}

	_ = h.authService.Logout(r.Context(), req.RefreshToken)

	writeJSON(w, http.StatusOK, map[string]string{"message": "Logged out successfully"})
}

// VerifyEmailRequest is the request body for email verification.
type VerifyEmailRequest struct {
	Token string `json:"token" validate:"required"`
}

// VerifyEmail handles email verification via POST (JSON body) or GET (query param).
func (h *AuthHandler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	var token string

	if r.Method == http.MethodGet {
		token = r.URL.Query().Get("token")
		if token == "" {
			writeVerifyHTML(w, http.StatusBadRequest, false, "Missing verification token.")
			return
		}

		if err := h.authService.VerifyEmail(r.Context(), token); err != nil {
			writeVerifyHTML(w, http.StatusBadRequest, false, "Verification failed: "+err.Error())
			return
		}

		writeVerifyHTML(w, http.StatusOK, true, "Your email has been verified. You can close this page and log in.")
		return
	}

	// POST: JSON body (for API/desktop clients)
	var req VerifyEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.BadRequest(w, r, "Invalid request body")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		fieldErrors := validationErrors(err)
		apperrors.ValidationError(w, r, fieldErrors)
		return
	}

	if err := h.authService.VerifyEmail(r.Context(), req.Token); err != nil {
		handleAuthError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Email verified successfully"})
}

// writeVerifyHTML writes a simple HTML page for email verification results.
func writeVerifyHTML(w http.ResponseWriter, status int, success bool, message string) {
	title := "Verification Failed"
	color := "#dc3545"
	if success {
		title = "Email Verified"
		color = "#28a745"
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>%s - Redoubt</title></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; display: flex; justify-content: center; align-items: center; min-height: 100vh; margin: 0; background: #f5f5f5;">
<div style="text-align: center; padding: 40px; background: white; border-radius: 12px; box-shadow: 0 2px 8px rgba(0,0,0,0.1); max-width: 400px;">
<h1 style="color: %s;">%s</h1>
<p style="color: #666; font-size: 16px;">%s</p>
</div>
</body>
</html>`, title, color, title, message)
}

// ForgotPasswordRequest is the request body for password reset request.
type ForgotPasswordRequest struct {
	Email string `json:"email" validate:"required,email"`
}

// ForgotPassword handles password reset request.
func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req ForgotPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.BadRequest(w, r, "Invalid request body")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		fieldErrors := validationErrors(err)
		apperrors.ValidationError(w, r, fieldErrors)
		return
	}

	// Always return success to not reveal if email exists
	_ = h.authService.RequestPasswordReset(r.Context(), req.Email)

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "If an account exists with this email, you will receive a password reset link.",
	})
}

// ResetPasswordRequest is the request body for password reset.
type ResetPasswordRequest struct {
	Token       string `json:"token" validate:"required"`
	NewPassword string `json:"new_password" validate:"required,min=12"`
}

// ResetPassword handles password reset.
func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req ResetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.BadRequest(w, r, "Invalid request body")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		fieldErrors := validationErrors(err)
		apperrors.ValidationError(w, r, fieldErrors)
		return
	}

	if err := h.authService.ResetPassword(r.Context(), req.Token, req.NewPassword); err != nil {
		handleAuthError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Password reset successfully"})
}

// ResendVerificationRequest is the request body for resending verification email.
type ResendVerificationRequest struct {
	Email string `json:"email" validate:"required,email"`
}

// ResendVerification handles resending verification email.
func (h *AuthHandler) ResendVerification(w http.ResponseWriter, r *http.Request) {
	var req ResendVerificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.BadRequest(w, r, "Invalid request body")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		fieldErrors := validationErrors(err)
		apperrors.ValidationError(w, r, fieldErrors)
		return
	}

	// Always return success to not reveal if email exists
	_ = h.authService.ResendVerificationEmail(r.Context(), req.Email)

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "If an unverified account exists with this email, you will receive a verification link.",
	})
}

// validationErrors converts validator errors to FieldError slice.
func validationErrors(err error) []apperrors.FieldError {
	var fieldErrors []apperrors.FieldError

	if validationErrs, ok := err.(validator.ValidationErrors); ok {
		for _, e := range validationErrs {
			fieldErrors = append(fieldErrors, apperrors.FieldError{
				Field:   e.Field(),
				Message: validationMessage(e),
			})
		}
	}

	return fieldErrors
}

// validationMessage returns a human-readable validation message.
func validationMessage(e validator.FieldError) string {
	switch e.Tag() {
	case "required":
		return "This field is required"
	case "email":
		return "Must be a valid email address"
	case "min":
		return "Must be at least " + e.Param() + " characters"
	case "max":
		return "Must be at most " + e.Param() + " characters"
	case "alphanum":
		return "Must contain only letters and numbers"
	default:
		return "Invalid value"
	}
}

// handleAuthError maps auth errors to HTTP responses.
func handleAuthError(w http.ResponseWriter, r *http.Request, err error) {
	switch err {
	case apperrors.ErrInvalidCredentials:
		apperrors.InvalidCredentials(w, r)
	case apperrors.ErrEmailNotVerified:
		apperrors.EmailNotVerified(w, r)
	case apperrors.ErrAccountLocked:
		apperrors.AccountLocked(w, r, 15)
	case apperrors.ErrInvalidToken, apperrors.ErrTokenExpired:
		apperrors.BadRequest(w, r, "Invalid or expired token")
	case apperrors.ErrPasswordTooWeak:
		apperrors.ValidationError(w, r, []apperrors.FieldError{
			{Field: "password", Message: "Password must be at least 12 characters"},
		})
	case apperrors.ErrUserAlreadyExists:
		apperrors.BadRequest(w, r, "Registration failed")
	case apperrors.ErrInvalidInvite, apperrors.ErrInviteExpired, apperrors.ErrInviteRevoked, apperrors.ErrInviteExhausted:
		apperrors.InviteInvalid(w, r, "Invalid or expired invite code")
	case apperrors.ErrUserDeleted:
		apperrors.BadRequest(w, r, "Account no longer exists")
	default:
		apperrors.InternalError(w, r)
	}
}

// getClientIP extracts the client IP address from the request.
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for reverse proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the list
		if idx := len(xff); idx > 0 {
			for i := 0; i < len(xff); i++ {
				if xff[i] == ',' {
					return xff[:i]
				}
			}
			return xff
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	// Remove port if present
	for i := len(ip) - 1; i >= 0; i-- {
		if ip[i] == ':' {
			return ip[:i]
		}
		if ip[i] == ']' {
			// IPv6 address
			break
		}
	}
	return ip
}

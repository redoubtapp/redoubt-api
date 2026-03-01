package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/redoubtapp/redoubt-api/internal/config"
	"github.com/redoubtapp/redoubt-api/internal/db/generated"
	"github.com/redoubtapp/redoubt-api/internal/email"
	apperrors "github.com/redoubtapp/redoubt-api/internal/errors"
)

// Service handles authentication business logic.
type Service struct {
	queries            *generated.Queries
	jwtManager         *JWTManager
	sessionManager     *SessionManager
	emailClient        *email.Client
	logger             *slog.Logger
	passwordMinLength  int
	lockoutThreshold   int
	lockoutDuration    time.Duration
	verificationExpiry time.Duration
	resetExpiry        time.Duration
}

// NewService creates a new auth service.
func NewService(
	queries *generated.Queries,
	jwtManager *JWTManager,
	sessionManager *SessionManager,
	emailClient *email.Client,
	logger *slog.Logger,
	cfg config.AuthConfig,
	emailCfg config.EmailConfig,
) *Service {
	return &Service{
		queries:            queries,
		jwtManager:         jwtManager,
		sessionManager:     sessionManager,
		emailClient:        emailClient,
		logger:             logger,
		passwordMinLength:  cfg.PasswordMinLength,
		lockoutThreshold:   cfg.LockoutThreshold,
		lockoutDuration:    cfg.LockoutDuration,
		verificationExpiry: emailCfg.VerificationExpiry,
		resetExpiry:        emailCfg.ResetExpiry,
	}
}

// RegisterRequest contains the registration request data.
type RegisterRequest struct {
	Username   string
	Email      string
	Password   string
	InviteCode string
}

// RegisterResult contains the registration result.
type RegisterResult struct {
	UserID uuid.UUID
}

// Register creates a new user account.
func (s *Service) Register(ctx context.Context, req RegisterRequest) (*RegisterResult, error) {
	// First check if this is the bootstrap invite code
	isBootstrapInvite := false
	bootstrapState, err := s.queries.GetBootstrapState(ctx)
	if err == nil && bootstrapState.InviteCode.Valid {
		isBootstrapInvite = req.InviteCode == bootstrapState.InviteCode.String
	}

	// If not a bootstrap code, validate against regular invites
	var invite generated.Invite
	if !isBootstrapInvite {
		invite, err = s.queries.GetInviteByCode(ctx, req.InviteCode)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, apperrors.ErrInvalidInvite
			}
			return nil, err
		}

		// Check invite validity
		if invite.RevokedAt.Valid {
			return nil, apperrors.ErrInviteRevoked
		}
		if invite.ExpiresAt.Valid && invite.ExpiresAt.Time.Before(time.Now()) {
			return nil, apperrors.ErrInviteExpired
		}
		if invite.MaxUses.Valid && invite.Uses >= invite.MaxUses.Int32 {
			return nil, apperrors.ErrInviteExhausted
		}
	}

	// Validate password
	if len(req.Password) < s.passwordMinLength {
		return nil, apperrors.ErrPasswordTooWeak
	}

	// Normalize email to lowercase
	emailAddr := strings.ToLower(req.Email)

	// Hash password
	passwordHash, err := HashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	// Check if this is the first user (make them instance admin)
	userCount, err := s.queries.CountUsers(ctx)
	if err != nil {
		return nil, err
	}
	isFirstUser := userCount == 0

	// Create user
	user, err := s.queries.CreateUser(ctx, generated.CreateUserParams{
		Username:        req.Username,
		Email:           emailAddr,
		PasswordHash:    passwordHash,
		IsInstanceAdmin: isFirstUser,
		EmailVerified:   false,
	})
	if err != nil {
		// Check for unique constraint violation (user already exists)
		// Return generic error to not reveal existence
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			return nil, apperrors.ErrUserAlreadyExists
		}
		return nil, err
	}

	// Increment invite uses (only for non-bootstrap invites)
	if !isBootstrapInvite {
		if err := s.queries.IncrementInviteUses(ctx, invite.ID); err != nil {
			s.logger.Error("failed to increment invite uses", slog.String("error", err.Error()))
		}
	}

	// Generate verification token
	verificationToken, err := GenerateVerificationToken()
	if err != nil {
		return nil, err
	}

	// Store verification token
	_, err = s.queries.CreateEmailVerification(ctx, generated.CreateEmailVerificationParams{
		UserID:    user.ID,
		Token:     verificationToken,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(s.verificationExpiry), Valid: true},
	})
	if err != nil {
		return nil, err
	}

	// Send verification email
	if err := s.emailClient.SendVerificationEmail(ctx, user.Email, user.Username, verificationToken); err != nil {
		s.logger.Error("failed to send verification email",
			slog.String("error", err.Error()),
			slog.String("user_id", uuidToString(user.ID)),
		)
		// Don't fail registration if email fails - user can request a new one
	}

	return &RegisterResult{UserID: UUIDFromPgtype(user.ID)}, nil
}

// LoginRequest contains the login request data.
type LoginRequest struct {
	Email     string
	Password  string
	UserAgent string
	IPAddress string
}

// LoginResult contains the login result.
type LoginResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
	User         *generated.User
}

// Login authenticates a user and returns tokens.
func (s *Service) Login(ctx context.Context, req LoginRequest) (*LoginResult, error) {
	emailAddr := strings.ToLower(req.Email)

	// Check for account lockout
	locked, err := s.isAccountLocked(ctx, emailAddr, req.IPAddress)
	if err != nil {
		return nil, err
	}
	if locked {
		return nil, apperrors.ErrAccountLocked
	}

	// Find user
	user, err := s.queries.GetUserByEmail(ctx, emailAddr)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.recordLoginAttempt(ctx, emailAddr, req.IPAddress, false)
			return nil, apperrors.ErrInvalidCredentials
		}
		return nil, err
	}

	// Check if user is deleted
	if user.DeletedAt.Valid {
		s.recordLoginAttempt(ctx, emailAddr, req.IPAddress, false)
		return nil, apperrors.ErrInvalidCredentials
	}

	// Check if user is disabled
	if user.DisabledAt.Valid {
		s.recordLoginAttempt(ctx, emailAddr, req.IPAddress, false)
		return nil, apperrors.ErrAccountDisabled
	}

	// Verify password
	valid, err := VerifyPassword(req.Password, user.PasswordHash)
	if err != nil || !valid {
		s.recordLoginAttempt(ctx, emailAddr, req.IPAddress, false)
		return nil, apperrors.ErrInvalidCredentials
	}

	// Check email verification
	if !user.EmailVerified {
		return nil, apperrors.ErrEmailNotVerified
	}

	// Record successful login
	s.recordLoginAttempt(ctx, emailAddr, req.IPAddress, true)

	// Create session with refresh token first (so we have session ID for JWT)
	userID := UUIDFromPgtype(user.ID)
	session, err := s.sessionManager.CreateSession(ctx, userID, req.UserAgent, req.IPAddress)
	if err != nil {
		return nil, err
	}

	// Generate access token with session ID
	sessionID := UUIDFromPgtype(session.ID)
	accessToken, err := s.jwtManager.GenerateToken(userID, user.IsInstanceAdmin, sessionID)
	if err != nil {
		return nil, err
	}

	return &LoginResult{
		AccessToken:  accessToken,
		RefreshToken: session.RefreshToken,
		ExpiresIn:    900, // 15 minutes in seconds
		User:         &user,
	}, nil
}

// RefreshTokens generates new tokens from a refresh token.
func (s *Service) RefreshTokens(ctx context.Context, refreshToken string) (*LoginResult, error) {
	session, err := s.sessionManager.ValidateRefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, err
	}

	// Get user
	user, err := s.queries.GetUserByID(ctx, session.UserID)
	if err != nil {
		return nil, err
	}

	// Check if user is deleted
	if user.DeletedAt.Valid {
		return nil, apperrors.ErrUserDeleted
	}

	// Check if user is disabled
	if user.DisabledAt.Valid {
		return nil, apperrors.ErrAccountDisabled
	}

	// Generate new access token with session ID
	userID := UUIDFromPgtype(user.ID)
	sessionID := UUIDFromPgtype(session.ID)
	accessToken, err := s.jwtManager.GenerateToken(userID, user.IsInstanceAdmin, sessionID)
	if err != nil {
		return nil, err
	}

	return &LoginResult{
		AccessToken:  accessToken,
		RefreshToken: refreshToken, // Keep same refresh token
		ExpiresIn:    900,
		User:         &user,
	}, nil
}

// Logout revokes the current session.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	session, err := s.sessionManager.ValidateRefreshToken(ctx, refreshToken)
	if err != nil {
		// If token is invalid, consider it already logged out
		return nil
	}
	sessionID := UUIDFromPgtype(session.ID)
	return s.sessionManager.RevokeSession(ctx, sessionID)
}

// VerifyEmail verifies a user's email address.
func (s *Service) VerifyEmail(ctx context.Context, token string) error {
	// Find verification record
	verification, err := s.queries.GetEmailVerificationByToken(ctx, token)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrInvalidToken
		}
		return err
	}

	// Check if expired
	if verification.ExpiresAt.Valid && verification.ExpiresAt.Time.Before(time.Now()) {
		return apperrors.ErrTokenExpired
	}

	// Mark user as verified
	if err := s.queries.VerifyUserEmail(ctx, verification.UserID); err != nil {
		return err
	}

	// Delete verification token
	if err := s.queries.DeleteEmailVerification(ctx, verification.ID); err != nil {
		s.logger.Error("failed to delete verification token", slog.String("error", err.Error()))
	}

	// Get user for welcome email
	user, err := s.queries.GetUserByID(ctx, verification.UserID)
	if err != nil {
		s.logger.Error("failed to get user for welcome email", slog.String("error", err.Error()))
		return nil // Don't fail verification if welcome email fails
	}

	// Send welcome email
	if err := s.emailClient.SendWelcomeEmail(ctx, user.Email, user.Username); err != nil {
		s.logger.Error("failed to send welcome email", slog.String("error", err.Error()))
	}

	return nil
}

// RequestPasswordReset initiates a password reset.
func (s *Service) RequestPasswordReset(ctx context.Context, emailAddr string) error {
	emailAddr = strings.ToLower(emailAddr)

	// Find user (don't reveal if exists)
	user, err := s.queries.GetUserByEmail(ctx, emailAddr)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // Don't reveal if user doesn't exist
		}
		return err
	}

	// Only allow reset for verified emails
	if !user.EmailVerified {
		return nil
	}

	// Generate reset token
	resetToken, err := GeneratePasswordResetToken()
	if err != nil {
		return err
	}

	// Store reset token
	_, err = s.queries.CreatePasswordReset(ctx, generated.CreatePasswordResetParams{
		UserID:    user.ID,
		Token:     resetToken,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(s.resetExpiry), Valid: true},
	})
	if err != nil {
		return err
	}

	// Send reset email
	if err := s.emailClient.SendPasswordResetEmail(ctx, user.Email, user.Username, resetToken); err != nil {
		s.logger.Error("failed to send password reset email", slog.String("error", err.Error()))
	}

	return nil
}

// ResetPassword resets a user's password.
func (s *Service) ResetPassword(ctx context.Context, token, newPassword string) error {
	// Validate password
	if len(newPassword) < s.passwordMinLength {
		return apperrors.ErrPasswordTooWeak
	}

	// Find reset record
	reset, err := s.queries.GetPasswordResetByToken(ctx, token)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrInvalidToken
		}
		return err
	}

	// Check if expired
	if reset.ExpiresAt.Valid && reset.ExpiresAt.Time.Before(time.Now()) {
		return apperrors.ErrTokenExpired
	}

	// Check if already used
	if reset.UsedAt.Valid {
		return apperrors.ErrInvalidToken
	}

	// Hash new password
	passwordHash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}

	// Update password
	if err := s.queries.UpdateUserPassword(ctx, generated.UpdateUserPasswordParams{
		ID:           reset.UserID,
		PasswordHash: passwordHash,
	}); err != nil {
		return err
	}

	// Mark reset token as used
	if err := s.queries.MarkPasswordResetUsed(ctx, reset.ID); err != nil {
		s.logger.Error("failed to mark reset token as used", slog.String("error", err.Error()))
	}

	// Revoke all sessions (force re-login)
	userID := UUIDFromPgtype(reset.UserID)
	if err := s.sessionManager.RevokeAllUserSessions(ctx, userID); err != nil {
		s.logger.Error("failed to revoke sessions after password reset", slog.String("error", err.Error()))
	}

	return nil
}

// ResendVerificationEmail resends the verification email.
func (s *Service) ResendVerificationEmail(ctx context.Context, emailAddr string) error {
	emailAddr = strings.ToLower(emailAddr)

	// Find user
	user, err := s.queries.GetUserByEmail(ctx, emailAddr)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // Don't reveal if user doesn't exist
		}
		return err
	}

	// Already verified
	if user.EmailVerified {
		return nil
	}

	// Delete existing verification tokens
	if err := s.queries.DeleteUserEmailVerifications(ctx, user.ID); err != nil {
		s.logger.Error("failed to delete existing verifications", slog.String("error", err.Error()))
	}

	// Generate new token
	verificationToken, err := GenerateVerificationToken()
	if err != nil {
		return err
	}

	// Store verification token
	_, err = s.queries.CreateEmailVerification(ctx, generated.CreateEmailVerificationParams{
		UserID:    user.ID,
		Token:     verificationToken,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(s.verificationExpiry), Valid: true},
	})
	if err != nil {
		return err
	}

	// Send verification email
	return s.emailClient.SendVerificationEmail(ctx, user.Email, user.Username, verificationToken)
}

func (s *Service) isAccountLocked(ctx context.Context, email, ipAddress string) (bool, error) {
	// Count recent failed attempts
	since := time.Now().Add(-s.lockoutDuration)

	// Parse IP
	ipAddr, err := netip.ParseAddr(ipAddress)
	if err != nil {
		// If IP is invalid, use a zero address
		ipAddr = netip.IPv4Unspecified()
	}

	count, err := s.queries.CountRecentFailedLoginAttempts(ctx, generated.CountRecentFailedLoginAttemptsParams{
		Email:     email,
		IpAddress: ipAddr,
		CreatedAt: pgtype.Timestamptz{Time: since, Valid: true},
	})
	if err != nil {
		return false, err
	}

	return count >= int64(s.lockoutThreshold), nil
}

func (s *Service) recordLoginAttempt(ctx context.Context, email, ipAddress string, success bool) {
	ipAddr, err := netip.ParseAddr(ipAddress)
	if err != nil {
		// If IP is invalid, use a zero address
		ipAddr = netip.IPv4Unspecified()
	}

	if err := s.queries.CreateLoginAttempt(ctx, generated.CreateLoginAttemptParams{
		Email:     email,
		IpAddress: ipAddr,
		Success:   success,
	}); err != nil {
		s.logger.Error("failed to record login attempt", slog.String("error", err.Error()))
	}
}

// uuidToString converts a pgtype.UUID to a string.
func uuidToString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}

// GetUserSessions returns all active sessions for a user.
func (s *Service) GetUserSessions(ctx context.Context, userID uuid.UUID) ([]generated.Session, error) {
	return s.sessionManager.GetUserSessions(ctx, userID)
}

// RevokeSession revokes a specific session.
// Returns an error if the session doesn't exist or doesn't belong to the user.
func (s *Service) RevokeSession(ctx context.Context, userID, sessionID uuid.UUID) error {
	// Verify the session belongs to the user
	pgSessionID := pgtype.UUID{Bytes: sessionID, Valid: true}
	session, err := s.queries.GetSessionByID(ctx, pgSessionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrSessionNotFound
		}
		return err
	}

	// Check ownership
	if UUIDFromPgtype(session.UserID) != userID {
		return apperrors.ErrForbidden
	}

	return s.sessionManager.RevokeSession(ctx, sessionID)
}

// RevokeAllSessions revokes all sessions for a user except the current one.
func (s *Service) RevokeAllSessions(ctx context.Context, userID uuid.UUID, currentSessionID *uuid.UUID) error {
	if currentSessionID != nil {
		// Revoke all except current session
		return s.queries.RevokeOtherUserSessions(ctx, generated.RevokeOtherUserSessionsParams{
			UserID: pgtype.UUID{Bytes: userID, Valid: true},
			ID:     pgtype.UUID{Bytes: *currentSessionID, Valid: true},
		})
	}
	// Revoke all sessions
	return s.sessionManager.RevokeAllUserSessions(ctx, userID)
}

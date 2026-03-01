package auth

import (
	"context"
	"net/netip"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/redoubtapp/redoubt-api/internal/db/generated"
)

// SessionManager handles session creation and management.
type SessionManager struct {
	queries       *generated.Queries
	refreshExpiry time.Duration
}

// NewSessionManager creates a new session manager.
func NewSessionManager(queries *generated.Queries, refreshExpiry time.Duration) *SessionManager {
	return &SessionManager{
		queries:       queries,
		refreshExpiry: refreshExpiry,
	}
}

// CreateSession creates a new session for the user.
func (m *SessionManager) CreateSession(ctx context.Context, userID uuid.UUID, userAgent, ipAddress string) (*generated.Session, error) {
	refreshToken, err := GenerateRefreshToken()
	if err != nil {
		return nil, err
	}

	// Parse IP address
	var ipAddr *netip.Addr
	if addr, err := netip.ParseAddr(ipAddress); err == nil {
		ipAddr = &addr
	}

	// Convert uuid.UUID to pgtype.UUID
	pgUserID := pgtype.UUID{
		Bytes: userID,
		Valid: true,
	}

	session, err := m.queries.CreateSession(ctx, generated.CreateSessionParams{
		UserID:       pgUserID,
		RefreshToken: refreshToken,
		UserAgent:    pgtype.Text{String: userAgent, Valid: userAgent != ""},
		IpAddress:    ipAddr,
		ExpiresAt:    pgtype.Timestamptz{Time: time.Now().Add(m.refreshExpiry), Valid: true},
	})
	if err != nil {
		return nil, err
	}

	return &session, nil
}

// ValidateRefreshToken validates a refresh token and returns the session.
func (m *SessionManager) ValidateRefreshToken(ctx context.Context, refreshToken string) (*generated.Session, error) {
	session, err := m.queries.GetSessionByRefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, err
	}

	// Check if session is expired
	if session.ExpiresAt.Valid && session.ExpiresAt.Time.Before(time.Now()) {
		return nil, ErrTokenExpired
	}

	// Check if session is revoked
	if session.RevokedAt.Valid {
		return nil, ErrTokenInvalid
	}

	// Update last used timestamp
	if err := m.queries.UpdateSessionLastUsed(ctx, session.ID); err != nil {
		return nil, err
	}

	return &session, nil
}

// RevokeSession revokes a specific session.
func (m *SessionManager) RevokeSession(ctx context.Context, sessionID uuid.UUID) error {
	pgSessionID := pgtype.UUID{Bytes: sessionID, Valid: true}
	return m.queries.RevokeSession(ctx, pgSessionID)
}

// RevokeAllUserSessions revokes all sessions for a user.
func (m *SessionManager) RevokeAllUserSessions(ctx context.Context, userID uuid.UUID) error {
	pgUserID := pgtype.UUID{Bytes: userID, Valid: true}
	return m.queries.RevokeAllUserSessions(ctx, pgUserID)
}

// GetUserSessions returns all active sessions for a user.
func (m *SessionManager) GetUserSessions(ctx context.Context, userID uuid.UUID) ([]generated.Session, error) {
	pgUserID := pgtype.UUID{Bytes: userID, Valid: true}
	return m.queries.ListUserSessions(ctx, pgUserID)
}

// CleanupExpiredSessions removes expired sessions from the database.
func (m *SessionManager) CleanupExpiredSessions(ctx context.Context) error {
	return m.queries.DeleteExpiredSessions(ctx)
}

// UUIDFromPgtype converts pgtype.UUID to uuid.UUID.
func UUIDFromPgtype(id pgtype.UUID) uuid.UUID {
	return uuid.UUID(id.Bytes)
}

// UUIDToPgtype converts uuid.UUID to pgtype.UUID.
func UUIDToPgtype(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

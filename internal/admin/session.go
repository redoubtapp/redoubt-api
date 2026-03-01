package admin

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"log/slog"
	"net/http"
	"net/netip"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/redoubtapp/redoubt-api/internal/auth"
	"github.com/redoubtapp/redoubt-api/internal/db/generated"
)

type contextKey int

const adminUserKey contextKey = iota

const sessionCookieName = "redoubt_admin_session"

func generateSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func (s *Server) generateCSRFToken(sessionToken string) string {
	mac := hmac.New(sha256.New, []byte(s.config.SessionSecret))
	mac.Write([]byte(sessionToken))
	return base64.URLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Server) validateCSRFToken(sessionToken, token string) bool {
	expected := s.generateCSRFToken(sessionToken)
	return hmac.Equal([]byte(expected), []byte(token))
}

func (s *Server) createSession(ctx context.Context, userID uuid.UUID, ip, userAgent string) (string, error) {
	token, err := generateSessionToken()
	if err != nil {
		return "", err
	}

	ipAddr := parseIP(ip)

	_, err = s.queries.CreateAdminSession(ctx, generated.CreateAdminSessionParams{
		UserID:    pgtype.UUID{Bytes: userID, Valid: true},
		Token:     token,
		IpAddress: ipAddr,
		UserAgent: pgtype.Text{String: userAgent, Valid: userAgent != ""},
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(s.config.SessionExpiry), Valid: true},
	})
	if err != nil {
		return "", err
	}

	return token, nil
}

func (s *Server) deleteSession(ctx context.Context, token string) error {
	return s.queries.DeleteAdminSession(ctx, token)
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil || cookie.Value == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		session, err := s.queries.GetAdminSession(r.Context(), cookie.Value)
		if err != nil {
			http.SetCookie(w, &http.Cookie{
				Name:     sessionCookieName,
				Value:    "",
				Path:     "/",
				MaxAge:   -1,
				HttpOnly: true,
				SameSite: http.SameSiteStrictMode,
			})
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		user, err := s.queries.AdminGetUser(r.Context(), session.UserID)
		if err != nil || !user.IsInstanceAdmin {
			_ = s.deleteSession(r.Context(), cookie.Value)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		ctx := context.WithValue(r.Context(), adminUserKey, &user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func getAdminUser(ctx context.Context) *generated.User {
	u, _ := ctx.Value(adminUserKey).(*generated.User)
	return u
}

func (s *Server) csrfMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			cookie, err := r.Cookie(sessionCookieName)
			if err != nil {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			token := r.FormValue("csrf_token")
			if !s.validateCSRFToken(cookie.Value, token) {
				http.Error(w, "Forbidden - invalid CSRF token", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) startCleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	go func() {
		for {
			select {
			case <-ticker.C:
				if err := s.queries.DeleteExpiredAdminSessions(ctx); err != nil {
					s.logger.Error("failed to cleanup expired admin sessions",
						slog.String("error", err.Error()),
					)
				}
			case <-ctx.Done():
				ticker.Stop()
				return
			}
		}
	}()
}

func getSessionToken(r *http.Request) string {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func parseIP(ip string) *netip.Addr {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return nil
	}
	return &addr
}

func uuidFromPgtype(id pgtype.UUID) uuid.UUID {
	return auth.UUIDFromPgtype(id)
}

package admin

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/redoubtapp/redoubt-api/internal/auth"
	"github.com/redoubtapp/redoubt-api/internal/db/generated"
)

// --- Login ---

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	// If already logged in, redirect to dashboard
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		if _, err := s.queries.GetAdminSession(r.Context(), cookie.Value); err == nil {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
	}

	s.templates.render(w, r, "login", TemplateData{
		Title: "Admin Login",
		Flash: r.URL.Query().Get("flash"),
	})
}

func (s *Server) loginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.templates.render(w, r, "login", TemplateData{
			Title: "Admin Login",
			Flash: "Invalid form submission.",
		})
		return
	}

	email := strings.TrimSpace(strings.ToLower(r.FormValue("email")))
	password := r.FormValue("password")

	if email == "" || password == "" {
		s.templates.render(w, r, "login", TemplateData{
			Title: "Admin Login",
			Flash: "Email and password are required.",
		})
		return
	}

	user, err := s.queries.GetUserByEmail(r.Context(), email)
	if err != nil {
		s.templates.render(w, r, "login", TemplateData{
			Title: "Admin Login",
			Flash: "Invalid email or password.",
		})
		return
	}

	valid, err := auth.VerifyPassword(password, user.PasswordHash)
	if err != nil || !valid {
		s.templates.render(w, r, "login", TemplateData{
			Title: "Admin Login",
			Flash: "Invalid email or password.",
		})
		return
	}

	if !user.IsInstanceAdmin {
		s.templates.render(w, r, "login", TemplateData{
			Title: "Admin Login",
			Flash: "You do not have admin access.",
		})
		return
	}

	if user.DisabledAt.Valid {
		s.templates.render(w, r, "login", TemplateData{
			Title: "Admin Login",
			Flash: "This account has been disabled.",
		})
		return
	}

	userID := uuidFromPgtype(user.ID)
	token, err := s.createSession(r.Context(), userID, getClientIP(r), r.UserAgent())
	if err != nil {
		s.logger.Error("failed to create admin session", slog.String("error", err.Error()))
		s.templates.render(w, r, "login", TemplateData{
			Title: "Admin Login",
			Flash: "Internal error. Please try again.",
		})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(s.config.SessionExpiry.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// --- Logout ---

func (s *Server) logoutSubmit(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		_ = s.deleteSession(r.Context(), cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// --- Dashboard ---

type dashboardData struct {
	UserCount      int64
	SpaceCount     int64
	ChannelCount   int64
	MessagesToday  int64
	OnlineCount    int
	Uptime         string
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getAdminUser(ctx)
	csrfToken := s.generateCSRFToken(getSessionToken(r))

	users, _ := s.queries.AdminCountUsers(ctx)
	spaces, _ := s.queries.AdminCountSpaces(ctx)
	channels, _ := s.queries.AdminCountChannels(ctx)
	messages, _ := s.queries.AdminCountMessagesToday(ctx)
	online := s.hub.GetConnectionCount()
	uptime := formatDuration(time.Since(s.startTime))

	s.templates.render(w, r, "dashboard", TemplateData{
		Title:     "Dashboard",
		Nav:       "dashboard",
		CSRFToken: csrfToken,
		User:      user,
		Content: dashboardData{
			UserCount:     users,
			SpaceCount:    spaces,
			ChannelCount:  channels,
			MessagesToday: messages,
			OnlineCount:   online,
			Uptime:        uptime,
		},
	})
}

func (s *Server) statsPartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	users, _ := s.queries.AdminCountUsers(ctx)
	spaces, _ := s.queries.AdminCountSpaces(ctx)
	channels, _ := s.queries.AdminCountChannels(ctx)
	messages, _ := s.queries.AdminCountMessagesToday(ctx)
	online := s.hub.GetConnectionCount()
	uptime := formatDuration(time.Since(s.startTime))

	data := dashboardData{
		UserCount:     users,
		SpaceCount:    spaces,
		ChannelCount:  channels,
		MessagesToday: messages,
		OnlineCount:   online,
		Uptime:        uptime,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.renderPartial(w, "stats", data); err != nil {
		s.logger.Error("failed to render stats partial", slog.String("error", err.Error()))
	}
}

// --- Users ---

type userListData struct {
	Users       []generated.AdminListUsersRow
	Page        int
	TotalPages  int
	TotalUsers  int64
}

func (s *Server) userList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getAdminUser(ctx)
	csrfToken := s.generateCSRFToken(getSessionToken(r))
	page := parsePage(r)
	const perPage = 25

	total, _ := s.queries.AdminCountUsers(ctx)
	totalPages := int((total + int64(perPage) - 1) / int64(perPage))
	if totalPages < 1 {
		totalPages = 1
	}

	users, err := s.queries.AdminListUsers(ctx, generated.AdminListUsersParams{
		Limit:  perPage,
		Offset: int32((page - 1) * perPage),
	})
	if err != nil {
		s.logger.Error("failed to list users", slog.String("error", err.Error()))
		users = []generated.AdminListUsersRow{}
	}

	s.templates.render(w, r, "users", TemplateData{
		Title:     "Users",
		Nav:       "users",
		CSRFToken: csrfToken,
		User:      user,
		Content: userListData{
			Users:      users,
			Page:       page,
			TotalPages: totalPages,
			TotalUsers: total,
		},
	})
}

type userDetailData struct {
	TargetUser generated.User
	Spaces     []generated.AdminGetUserSpacesRow
	Sessions   []generated.AdminGetUserSessionsRow
}

func (s *Server) userDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getAdminUser(ctx)
	csrfToken := s.generateCSRFToken(getSessionToken(r))

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		http.NotFound(w, r)
		return
	}

	targetUser, err := s.queries.AdminGetUser(ctx, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	spaces, _ := s.queries.AdminGetUserSpaces(ctx, id)
	sessions, _ := s.queries.AdminGetUserSessions(ctx, id)

	if spaces == nil {
		spaces = []generated.AdminGetUserSpacesRow{}
	}
	if sessions == nil {
		sessions = []generated.AdminGetUserSessionsRow{}
	}

	s.templates.render(w, r, "user_detail", TemplateData{
		Title:     fmt.Sprintf("User: %s", targetUser.Username),
		Nav:       "users",
		CSRFToken: csrfToken,
		User:      user,
		Content: userDetailData{
			TargetUser: targetUser,
			Spaces:     spaces,
			Sessions:   sessions,
		},
	})
}

func (s *Server) disableUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := s.queries.AdminDisableUser(ctx, id); err != nil {
		s.logger.Error("failed to disable user", slog.String("error", err.Error()))
		redirectWithFlash(w, r, fmt.Sprintf("/users/%s", mux.Vars(r)["id"]), "Failed to disable user.")
		return
	}

	// Revoke all sessions for the disabled user
	if err := s.queries.RevokeAllUserSessions(ctx, id); err != nil {
		s.logger.Error("failed to revoke sessions for disabled user", slog.String("error", err.Error()))
	}

	redirectWithFlash(w, r, fmt.Sprintf("/users/%s", mux.Vars(r)["id"]), "User has been disabled.")
}

func (s *Server) enableUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := s.queries.AdminEnableUser(ctx, id); err != nil {
		s.logger.Error("failed to enable user", slog.String("error", err.Error()))
		redirectWithFlash(w, r, fmt.Sprintf("/users/%s", mux.Vars(r)["id"]), "Failed to enable user.")
		return
	}

	redirectWithFlash(w, r, fmt.Sprintf("/users/%s", mux.Vars(r)["id"]), "User has been enabled.")
}

func (s *Server) resetUserPassword(w http.ResponseWriter, r *http.Request) {
	// Password reset requires email service integration.
	// For now, log the request and inform the admin.
	idStr := mux.Vars(r)["id"]
	s.logger.Info("admin password reset requested", slog.String("target_user_id", idStr))
	redirectWithFlash(w, r, fmt.Sprintf("/users/%s", idStr), "Password reset is not yet implemented. Use the email-based flow instead.")
}

func (s *Server) revokeUserSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := s.queries.RevokeAllUserSessions(ctx, id); err != nil {
		s.logger.Error("failed to revoke user sessions", slog.String("error", err.Error()))
		redirectWithFlash(w, r, fmt.Sprintf("/users/%s", mux.Vars(r)["id"]), "Failed to revoke sessions.")
		return
	}

	redirectWithFlash(w, r, fmt.Sprintf("/users/%s", mux.Vars(r)["id"]), "All user sessions have been revoked.")
}

// --- Spaces ---

type spaceListData struct {
	Spaces      []generated.AdminListSpacesRow
	Page        int
	TotalPages  int
	TotalSpaces int64
}

func (s *Server) spaceList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getAdminUser(ctx)
	csrfToken := s.generateCSRFToken(getSessionToken(r))
	page := parsePage(r)
	const perPage = 25

	total, _ := s.queries.AdminCountSpaces(ctx)
	totalPages := int((total + int64(perPage) - 1) / int64(perPage))
	if totalPages < 1 {
		totalPages = 1
	}

	spaces, err := s.queries.AdminListSpaces(ctx, generated.AdminListSpacesParams{
		Limit:  perPage,
		Offset: int32((page - 1) * perPage),
	})
	if err != nil {
		s.logger.Error("failed to list spaces", slog.String("error", err.Error()))
		spaces = []generated.AdminListSpacesRow{}
	}

	s.templates.render(w, r, "spaces", TemplateData{
		Title:     "Spaces",
		Nav:       "spaces",
		CSRFToken: csrfToken,
		User:      user,
		Content: spaceListData{
			Spaces:      spaces,
			Page:        page,
			TotalPages:  totalPages,
			TotalSpaces: total,
		},
	})
}

type spaceDetailData struct {
	Space    generated.AdminGetSpaceDetailRow
	Members  []generated.AdminGetSpaceMembersRow
	Channels []generated.AdminGetSpaceChannelsRow
	Invites  []generated.AdminGetSpaceInvitesRow
}

func (s *Server) spaceDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getAdminUser(ctx)
	csrfToken := s.generateCSRFToken(getSessionToken(r))

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		http.NotFound(w, r)
		return
	}

	space, err := s.queries.AdminGetSpaceDetail(ctx, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	members, _ := s.queries.AdminGetSpaceMembers(ctx, id)
	channels, _ := s.queries.AdminGetSpaceChannels(ctx, id)
	invites, _ := s.queries.AdminGetSpaceInvites(ctx, id)

	if members == nil {
		members = []generated.AdminGetSpaceMembersRow{}
	}
	if channels == nil {
		channels = []generated.AdminGetSpaceChannelsRow{}
	}
	if invites == nil {
		invites = []generated.AdminGetSpaceInvitesRow{}
	}

	s.templates.render(w, r, "space_detail", TemplateData{
		Title:     fmt.Sprintf("Space: %s", space.Name),
		Nav:       "spaces",
		CSRFToken: csrfToken,
		User:      user,
		Content: spaceDetailData{
			Space:    space,
			Members:  members,
			Channels: channels,
			Invites:  invites,
		},
	})
}

func (s *Server) deleteSpace(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := s.queries.SoftDeleteSpace(ctx, id); err != nil {
		s.logger.Error("failed to delete space", slog.String("error", err.Error()))
		redirectWithFlash(w, r, fmt.Sprintf("/spaces/%s", mux.Vars(r)["id"]), "Failed to delete space.")
		return
	}

	redirectWithFlash(w, r, "/spaces", "Space has been deleted.")
}

func (s *Server) revokeInvite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	spaceIDStr := vars["id"]

	inviteID, err := parseUUIDParam(r, "inviteId")
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := s.queries.RevokeInvite(ctx, inviteID); err != nil {
		s.logger.Error("failed to revoke invite", slog.String("error", err.Error()))
		redirectWithFlash(w, r, fmt.Sprintf("/spaces/%s", spaceIDStr), "Failed to revoke invite.")
		return
	}

	redirectWithFlash(w, r, fmt.Sprintf("/spaces/%s", spaceIDStr), "Invite has been revoked.")
}

// --- Audit ---

type auditListData struct {
	Entries    []generated.AdminListAuditLogsRow
	Page       int
	TotalPages int
	TotalLogs  int64
}

func (s *Server) auditList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getAdminUser(ctx)
	csrfToken := s.generateCSRFToken(getSessionToken(r))
	page := parsePage(r)
	const perPage = 50

	total, _ := s.queries.AdminCountAuditLogs(ctx)
	totalPages := int((total + int64(perPage) - 1) / int64(perPage))
	if totalPages < 1 {
		totalPages = 1
	}

	entries, err := s.queries.AdminListAuditLogs(ctx, generated.AdminListAuditLogsParams{
		Limit:  perPage,
		Offset: int32((page - 1) * perPage),
	})
	if err != nil {
		s.logger.Error("failed to list audit logs", slog.String("error", err.Error()))
		entries = []generated.AdminListAuditLogsRow{}
	}

	s.templates.render(w, r, "audit", TemplateData{
		Title:     "Audit Log",
		Nav:       "audit",
		CSRFToken: csrfToken,
		User:      user,
		Content: auditListData{
			Entries:    entries,
			Page:       page,
			TotalPages: totalPages,
			TotalLogs:  total,
		},
	})
}

// --- Helpers ---

func parsePage(r *http.Request) int {
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		return 1
	}
	return page
}

func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func redirectWithFlash(w http.ResponseWriter, r *http.Request, url, msg string) {
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	http.Redirect(w, r, fmt.Sprintf("%s%sflash=%s", url, sep, msg), http.StatusSeeOther)
}

func parseUUIDParam(r *http.Request, key string) (pgtype.UUID, error) {
	idStr := mux.Vars(r)[key]
	parsed, err := uuid.Parse(idStr)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}

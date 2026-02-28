package admin

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"github.com/redoubtapp/redoubt-api/internal/config"
	"github.com/redoubtapp/redoubt-api/internal/db/generated"
	"github.com/redoubtapp/redoubt-api/internal/presence"
)

type Server struct {
	queries    *generated.Queries
	hub        *presence.Hub
	logger     *slog.Logger
	config     config.AdminConfig
	httpServer *http.Server
	templates  *templateRegistry
	startTime  time.Time
}

func NewServer(
	queries *generated.Queries,
	hub *presence.Hub,
	logger *slog.Logger,
	cfg config.AdminConfig,
) (*Server, error) {
	tmpl, err := loadTemplates()
	if err != nil {
		return nil, fmt.Errorf("loading admin templates: %w", err)
	}

	s := &Server{
		queries:   queries,
		hub:       hub,
		logger:    logger.With(slog.String("component", "admin")),
		config:    cfg,
		templates: tmpl,
		startTime: time.Now(),
	}

	router := s.setupRoutes()

	s.httpServer = &http.Server{
		Addr:         cfg.Address(),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s, nil
}

func (s *Server) setupRoutes() *mux.Router {
	r := mux.NewRouter()

	// Static files
	r.PathPrefix("/static/").Handler(http.FileServer(http.FS(staticFiles)))

	// Public routes (login)
	r.HandleFunc("/login", s.loginPage).Methods(http.MethodGet)
	r.HandleFunc("/login", s.loginSubmit).Methods(http.MethodPost)

	// Authenticated routes
	auth := r.PathPrefix("").Subrouter()
	auth.Use(s.requireAdmin)
	auth.Use(s.csrfMiddleware)
	auth.Use(s.requestLogger)

	auth.HandleFunc("/logout", s.logoutSubmit).Methods(http.MethodPost)

	// Dashboard
	auth.HandleFunc("/", s.dashboard).Methods(http.MethodGet)
	auth.HandleFunc("/partials/stats", s.statsPartial).Methods(http.MethodGet)

	// Users
	auth.HandleFunc("/users", s.userList).Methods(http.MethodGet)
	auth.HandleFunc("/users/{id}", s.userDetail).Methods(http.MethodGet)
	auth.HandleFunc("/users/{id}/disable", s.disableUser).Methods(http.MethodPost)
	auth.HandleFunc("/users/{id}/enable", s.enableUser).Methods(http.MethodPost)
	auth.HandleFunc("/users/{id}/reset-password", s.resetUserPassword).Methods(http.MethodPost)
	auth.HandleFunc("/users/{id}/revoke-sessions", s.revokeUserSessions).Methods(http.MethodPost)

	// Spaces
	auth.HandleFunc("/spaces", s.spaceList).Methods(http.MethodGet)
	auth.HandleFunc("/spaces/{id}", s.spaceDetail).Methods(http.MethodGet)
	auth.HandleFunc("/spaces/{id}/delete", s.deleteSpace).Methods(http.MethodPost)
	auth.HandleFunc("/spaces/{id}/invites/{inviteId}/revoke", s.revokeInvite).Methods(http.MethodPost)

	// Audit log
	auth.HandleFunc("/audit", s.auditList).Methods(http.MethodGet)

	return r
}

func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.logger.Debug("admin request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Duration("duration", time.Since(start)),
		)
	})
}

func (s *Server) Start(ctx context.Context) error {
	s.startCleanupLoop(ctx)
	s.logger.Info("admin panel starting", slog.String("address", s.config.Address()))
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("admin panel server error", slog.String("error", err.Error()))
		}
	}()
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("admin panel shutting down")
	return s.httpServer.Shutdown(ctx)
}

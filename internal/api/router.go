package api

import (
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/redoubtapp/redoubt-api/internal/api/handlers"
	"github.com/redoubtapp/redoubt-api/internal/api/middleware"
	"github.com/redoubtapp/redoubt-api/internal/auth"
	"github.com/redoubtapp/redoubt-api/internal/config"
	"github.com/redoubtapp/redoubt-api/internal/ratelimit"
)

// RouterConfig holds dependencies for the router.
type RouterConfig struct {
	Logger                *slog.Logger
	Config                *config.Config
	HealthDeps            *handlers.HealthDependencies
	AuthHandler           *handlers.AuthHandler
	SessionHandler        *handlers.SessionHandler
	UserHandler           *handlers.UserHandler
	SpaceHandler          *handlers.SpaceHandler
	ChannelHandler        *handlers.ChannelHandler
	InviteHandler         *handlers.InviteHandler
	VoiceHandler          *handlers.VoiceHandler
	MessageHandler        *handlers.MessageHandler
	ReactionHandler       *handlers.ReactionHandler
	EmojiHandler          *handlers.EmojiHandler
	OpenGraphHandler      *handlers.OpenGraphHandler
	AttachmentHandler     *handlers.AttachmentHandler
	WebSocketHandler      *handlers.WebSocketHandler
	LiveKitWebhookHandler http.Handler
	JWTManager            *auth.JWTManager
	RateLimiter           *ratelimit.Limiter
}

// NewRouter creates a new HTTP router with all routes and middleware.
func NewRouter(cfg RouterConfig) http.Handler {
	r := mux.NewRouter()

	// Health check handlers (no auth required)
	healthHandler := handlers.NewHealthHandler(cfg.HealthDeps)

	// Health routes - outside /api/v1 for infrastructure probes
	r.HandleFunc("/health", healthHandler.Health).Methods(http.MethodGet)
	r.HandleFunc("/health/live", healthHandler.Live).Methods(http.MethodGet)
	r.HandleFunc("/health/ready", healthHandler.Ready).Methods(http.MethodGet)

	// API v1 subrouter
	apiV1 := r.PathPrefix("/api/v1").Subrouter()

	// API v1 health endpoint (mirrors root health)
	apiV1.HandleFunc("/health", healthHandler.Health).Methods(http.MethodGet)

	// Auth routes (no auth required)
	// Apply rate limiting to auth endpoints if enabled
	if cfg.Config.RateLimit.Enabled && cfg.RateLimiter != nil {
		// Register: 5 requests per hour per IP
		apiV1.Handle("/auth/register", middleware.RateLimit(middleware.RateLimitConfig{
			Limiter:    cfg.RateLimiter,
			Scope:      ratelimit.ScopeRegister,
			Identifier: middleware.IPIdentifier,
		})(http.HandlerFunc(cfg.AuthHandler.Register))).Methods(http.MethodPost)

		// Login: 10 requests per 15 min per IP+Email
		apiV1.Handle("/auth/login", middleware.LoginRateLimit(cfg.RateLimiter)(
			http.HandlerFunc(cfg.AuthHandler.Login))).Methods(http.MethodPost)

		// Forgot password: 3 requests per hour per IP
		apiV1.Handle("/auth/forgot-password", middleware.RateLimit(middleware.RateLimitConfig{
			Limiter:    cfg.RateLimiter,
			Scope:      ratelimit.ScopeForgotPassword,
			Identifier: middleware.IPIdentifier,
		})(http.HandlerFunc(cfg.AuthHandler.ForgotPassword))).Methods(http.MethodPost)

		// Verify email: 10 requests per hour per IP
		apiV1.Handle("/auth/verify-email", middleware.RateLimit(middleware.RateLimitConfig{
			Limiter:    cfg.RateLimiter,
			Scope:      ratelimit.ScopeVerifyEmail,
			Identifier: middleware.IPIdentifier,
		})(http.HandlerFunc(cfg.AuthHandler.VerifyEmail))).Methods(http.MethodPost)

		// Other auth routes without specific rate limits
		apiV1.HandleFunc("/auth/refresh", cfg.AuthHandler.Refresh).Methods(http.MethodPost)
		apiV1.HandleFunc("/auth/logout", cfg.AuthHandler.Logout).Methods(http.MethodPost)
		apiV1.HandleFunc("/auth/reset-password", cfg.AuthHandler.ResetPassword).Methods(http.MethodPost)
		apiV1.HandleFunc("/auth/resend-verification", cfg.AuthHandler.ResendVerification).Methods(http.MethodPost)
	} else {
		// Rate limiting disabled - register routes without rate limiting
		apiV1.HandleFunc("/auth/register", cfg.AuthHandler.Register).Methods(http.MethodPost)
		apiV1.HandleFunc("/auth/login", cfg.AuthHandler.Login).Methods(http.MethodPost)
		apiV1.HandleFunc("/auth/refresh", cfg.AuthHandler.Refresh).Methods(http.MethodPost)
		apiV1.HandleFunc("/auth/logout", cfg.AuthHandler.Logout).Methods(http.MethodPost)
		apiV1.HandleFunc("/auth/verify-email", cfg.AuthHandler.VerifyEmail).Methods(http.MethodPost)
		apiV1.HandleFunc("/auth/forgot-password", cfg.AuthHandler.ForgotPassword).Methods(http.MethodPost)
		apiV1.HandleFunc("/auth/reset-password", cfg.AuthHandler.ResetPassword).Methods(http.MethodPost)
		apiV1.HandleFunc("/auth/resend-verification", cfg.AuthHandler.ResendVerification).Methods(http.MethodPost)
	}

	// Session routes (auth required)
	if cfg.SessionHandler != nil && cfg.JWTManager != nil {
		sessionsRouter := apiV1.PathPrefix("/sessions").Subrouter()
		sessionsRouter.Use(middleware.RequireAuth(cfg.JWTManager))
		sessionsRouter.HandleFunc("", cfg.SessionHandler.ListSessions).Methods(http.MethodGet)
		sessionsRouter.HandleFunc("", cfg.SessionHandler.RevokeAllSessions).Methods(http.MethodDelete)
		sessionsRouter.HandleFunc("/{id}", cfg.SessionHandler.RevokeSession).Methods(http.MethodDelete)
	}

	// User routes (auth required)
	if cfg.UserHandler != nil && cfg.JWTManager != nil {
		usersRouter := apiV1.PathPrefix("/users").Subrouter()
		usersRouter.Use(middleware.RequireAuth(cfg.JWTManager))
		usersRouter.HandleFunc("/me", cfg.UserHandler.GetCurrentUser).Methods(http.MethodGet)
		usersRouter.HandleFunc("/me", cfg.UserHandler.UpdateCurrentUser).Methods(http.MethodPatch)
		usersRouter.HandleFunc("/me", cfg.UserHandler.DeleteCurrentUser).Methods(http.MethodDelete)
		usersRouter.HandleFunc("/me/avatar", cfg.UserHandler.UploadAvatar).Methods(http.MethodPut)
		usersRouter.HandleFunc("/me/avatar", cfg.UserHandler.DeleteAvatar).Methods(http.MethodDelete)
		usersRouter.HandleFunc("/{id}/avatar", cfg.UserHandler.GetAvatar).Methods(http.MethodGet)
	}

	// Space routes (auth required)
	if cfg.SpaceHandler != nil && cfg.JWTManager != nil {
		spacesRouter := apiV1.PathPrefix("/spaces").Subrouter()
		spacesRouter.Use(middleware.RequireAuth(cfg.JWTManager))
		spacesRouter.HandleFunc("", cfg.SpaceHandler.ListSpaces).Methods(http.MethodGet)
		spacesRouter.HandleFunc("", cfg.SpaceHandler.CreateSpace).Methods(http.MethodPost)
		spacesRouter.HandleFunc("/{id}", cfg.SpaceHandler.GetSpace).Methods(http.MethodGet)
		spacesRouter.HandleFunc("/{id}", cfg.SpaceHandler.UpdateSpace).Methods(http.MethodPatch)
		spacesRouter.HandleFunc("/{id}", cfg.SpaceHandler.DeleteSpace).Methods(http.MethodDelete)
		spacesRouter.HandleFunc("/{id}/members", cfg.SpaceHandler.ListMembers).Methods(http.MethodGet)
		spacesRouter.HandleFunc("/{id}/members/{userId}", cfg.SpaceHandler.KickMember).Methods(http.MethodDelete)
		spacesRouter.HandleFunc("/{id}/members/{userId}", cfg.SpaceHandler.ChangeMemberRole).Methods(http.MethodPatch)

		// Channel routes nested under spaces
		if cfg.ChannelHandler != nil {
			spacesRouter.HandleFunc("/{id}/channels", cfg.ChannelHandler.ListChannels).Methods(http.MethodGet)
			spacesRouter.HandleFunc("/{id}/channels", cfg.ChannelHandler.CreateChannel).Methods(http.MethodPost)
			spacesRouter.HandleFunc("/{id}/channels/reorder", cfg.ChannelHandler.ReorderChannels).Methods(http.MethodPatch)
		}
	}

	// Channel routes (auth required) - for direct channel access
	if cfg.ChannelHandler != nil && cfg.JWTManager != nil {
		channelsRouter := apiV1.PathPrefix("/channels").Subrouter()
		channelsRouter.Use(middleware.RequireAuth(cfg.JWTManager))
		channelsRouter.HandleFunc("/{id}", cfg.ChannelHandler.GetChannel).Methods(http.MethodGet)
		channelsRouter.HandleFunc("/{id}", cfg.ChannelHandler.UpdateChannel).Methods(http.MethodPatch)
		channelsRouter.HandleFunc("/{id}", cfg.ChannelHandler.DeleteChannel).Methods(http.MethodDelete)

		// Voice routes nested under channels
		if cfg.VoiceHandler != nil {
			channelsRouter.HandleFunc("/{id}/voice/join", cfg.VoiceHandler.JoinVoiceChannel).Methods(http.MethodPost)
			channelsRouter.HandleFunc("/{id}/voice/participants", cfg.VoiceHandler.GetChannelParticipants).Methods(http.MethodGet)
		}

		// Message routes nested under channels
		if cfg.MessageHandler != nil {
			channelsRouter.HandleFunc("/{id}/messages", cfg.MessageHandler.SendMessage).Methods(http.MethodPost)
			channelsRouter.HandleFunc("/{id}/messages", cfg.MessageHandler.ListMessages).Methods(http.MethodGet)
			channelsRouter.HandleFunc("/{id}/read", cfg.MessageHandler.MarkChannelAsRead).Methods(http.MethodPut)
			channelsRouter.HandleFunc("/{id}/unread", cfg.MessageHandler.GetUnreadCount).Methods(http.MethodGet)
		}
	}

	// Message routes (auth required) - for direct message access
	if cfg.MessageHandler != nil && cfg.JWTManager != nil {
		messagesRouter := apiV1.PathPrefix("/messages").Subrouter()
		messagesRouter.Use(middleware.RequireAuth(cfg.JWTManager))
		messagesRouter.HandleFunc("/{id}", cfg.MessageHandler.GetMessage).Methods(http.MethodGet)
		messagesRouter.HandleFunc("/{id}", cfg.MessageHandler.EditMessage).Methods(http.MethodPatch)
		messagesRouter.HandleFunc("/{id}", cfg.MessageHandler.DeleteMessage).Methods(http.MethodDelete)
		messagesRouter.HandleFunc("/{id}/edits", cfg.MessageHandler.GetEditHistory).Methods(http.MethodGet)
		messagesRouter.HandleFunc("/{id}/thread", cfg.MessageHandler.GetThreadReplies).Methods(http.MethodGet)
		messagesRouter.HandleFunc("/{id}/thread", cfg.MessageHandler.ReplyToThread).Methods(http.MethodPost)

		// Reaction routes nested under messages
		if cfg.ReactionHandler != nil {
			messagesRouter.HandleFunc("/{id}/reactions", cfg.ReactionHandler.AddReaction).Methods(http.MethodPost)
			messagesRouter.HandleFunc("/{id}/reactions", cfg.ReactionHandler.GetMessageReactions).Methods(http.MethodGet)
			messagesRouter.HandleFunc("/{id}/reactions/toggle", cfg.ReactionHandler.ToggleReaction).Methods(http.MethodPost)
			messagesRouter.HandleFunc("/{id}/reactions/{emoji}", cfg.ReactionHandler.RemoveReaction).Methods(http.MethodDelete)
		}

		// Attachment routes nested under messages
		if cfg.AttachmentHandler != nil {
			messagesRouter.HandleFunc("/{id}/attachments", cfg.AttachmentHandler.UploadAttachment).Methods(http.MethodPost)
			messagesRouter.HandleFunc("/{id}/attachments", cfg.AttachmentHandler.GetMessageAttachments).Methods(http.MethodGet)
		}
	}

	// Attachment routes (auth required) - for direct attachment access
	if cfg.AttachmentHandler != nil && cfg.JWTManager != nil {
		attachmentsRouter := apiV1.PathPrefix("/attachments").Subrouter()
		attachmentsRouter.Use(middleware.RequireAuth(cfg.JWTManager))
		attachmentsRouter.HandleFunc("/{id}", cfg.AttachmentHandler.GetAttachment).Methods(http.MethodGet)
		attachmentsRouter.HandleFunc("/{id}/download", cfg.AttachmentHandler.DownloadAttachment).Methods(http.MethodGet)
		attachmentsRouter.HandleFunc("/{id}", cfg.AttachmentHandler.DeleteAttachment).Methods(http.MethodDelete)
	}

	// Emoji routes (auth required)
	if cfg.EmojiHandler != nil && cfg.JWTManager != nil {
		emojiRouter := apiV1.PathPrefix("/emoji").Subrouter()
		emojiRouter.Use(middleware.RequireAuth(cfg.JWTManager))
		emojiRouter.HandleFunc("", cfg.EmojiHandler.ListEmoji).Methods(http.MethodGet)
	}

	// OpenGraph routes (auth required)
	if cfg.OpenGraphHandler != nil && cfg.JWTManager != nil {
		ogRouter := apiV1.PathPrefix("/opengraph").Subrouter()
		ogRouter.Use(middleware.RequireAuth(cfg.JWTManager))
		ogRouter.HandleFunc("/fetch", cfg.OpenGraphHandler.FetchMetadata).Methods(http.MethodPost)
	}

	// Voice routes (auth required) - for user-level voice operations
	if cfg.VoiceHandler != nil && cfg.JWTManager != nil {
		voiceRouter := apiV1.PathPrefix("/voice").Subrouter()
		voiceRouter.Use(middleware.RequireAuth(cfg.JWTManager))
		voiceRouter.HandleFunc("/leave", cfg.VoiceHandler.LeaveVoiceChannel).Methods(http.MethodPost)
		voiceRouter.HandleFunc("/mute", cfg.VoiceHandler.UpdateMuteState).Methods(http.MethodPatch)
		voiceRouter.HandleFunc("/state", cfg.VoiceHandler.GetVoiceState).Methods(http.MethodGet)
		voiceRouter.HandleFunc("/server-mute/{userId}", cfg.VoiceHandler.ServerMute).Methods(http.MethodPost)
	}

	// Invite routes nested under spaces (auth required)
	if cfg.InviteHandler != nil && cfg.SpaceHandler != nil && cfg.JWTManager != nil {
		spacesRouter := apiV1.PathPrefix("/spaces").Subrouter()
		spacesRouter.Use(middleware.RequireAuth(cfg.JWTManager))
		spacesRouter.HandleFunc("/{id}/invites", cfg.InviteHandler.ListInvites).Methods(http.MethodGet)
		spacesRouter.HandleFunc("/{id}/invites", cfg.InviteHandler.CreateInvite).Methods(http.MethodPost)
	}

	// Invite routes (mixed auth - some public, some require auth)
	if cfg.InviteHandler != nil && cfg.JWTManager != nil {
		// Public route - get invite info (no auth required)
		apiV1.HandleFunc("/invites/{code}", cfg.InviteHandler.GetInviteInfo).Methods(http.MethodGet)

		// Protected routes
		invitesRouter := apiV1.PathPrefix("/invites").Subrouter()
		invitesRouter.Use(middleware.RequireAuth(cfg.JWTManager))
		invitesRouter.HandleFunc("/{id}", cfg.InviteHandler.RevokeInvite).Methods(http.MethodDelete)
		invitesRouter.HandleFunc("/{code}/join", cfg.InviteHandler.JoinViaInvite).Methods(http.MethodPost)
	}

	// LiveKit webhook route (no auth required - LiveKit handles its own auth)
	if cfg.LiveKitWebhookHandler != nil {
		apiV1.Handle("/livekit/webhook", cfg.LiveKitWebhookHandler).Methods(http.MethodPost)
	}

	// Apply middleware stack (order matters - first applied is outermost)
	var handler http.Handler = r

	// CORS must be applied early
	handler = middleware.CORS(cfg.Config.CORS)(handler)

	// Security headers
	handler = middleware.SecurityHeaders(handler)

	// Request logging
	handler = middleware.Logging(cfg.Logger)(handler)

	// Request ID (should be early to ensure all logs have it)
	handler = middleware.RequestID(handler)

	// Panic recovery (outermost to catch all panics)
	handler = middleware.Recover(cfg.Logger)(handler)

	// OpenTelemetry HTTP instrumentation (outermost for full request tracing)
	handler = otelhttp.NewHandler(handler, "redoubt-api",
		otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string {
			return r.Method + " " + r.URL.Path
		}),
	)

	// WebSocket route needs to bypass otelhttp middleware because it wraps
	// the response writer in a way that doesn't support http.Hijacker.
	// We create a composite handler that routes /ws directly to the WebSocket
	// handler with only CORS applied, while all other routes go through the
	// full middleware stack.
	if cfg.WebSocketHandler != nil {
		wsHandler := middleware.CORS(cfg.Config.CORS)(cfg.WebSocketHandler)
		return &wsRouterHandler{
			wsHandler:   wsHandler,
			mainHandler: handler,
		}
	}

	return handler
}

// wsRouterHandler routes WebSocket requests directly to avoid otelhttp middleware
// which doesn't support http.Hijacker needed for WebSocket upgrades.
type wsRouterHandler struct {
	wsHandler   http.Handler
	mainHandler http.Handler
}

func (h *wsRouterHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/ws" {
		h.wsHandler.ServeHTTP(w, r)
		return
	}
	h.mainHandler.ServeHTTP(w, r)
}

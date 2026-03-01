package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/redoubtapp/redoubt-api/internal/admin"
	"github.com/redoubtapp/redoubt-api/internal/api"
	"github.com/redoubtapp/redoubt-api/internal/api/handlers"
	"github.com/redoubtapp/redoubt-api/internal/audit"
	"github.com/redoubtapp/redoubt-api/internal/auth"
	"github.com/redoubtapp/redoubt-api/internal/cache"
	"github.com/redoubtapp/redoubt-api/internal/channels"
	"github.com/redoubtapp/redoubt-api/internal/config"
	"github.com/redoubtapp/redoubt-api/internal/db"
	"github.com/redoubtapp/redoubt-api/internal/db/generated"
	"github.com/redoubtapp/redoubt-api/internal/email"
	"github.com/redoubtapp/redoubt-api/internal/invites"
	"github.com/redoubtapp/redoubt-api/internal/livekit"
	"github.com/redoubtapp/redoubt-api/internal/logging"
	"github.com/redoubtapp/redoubt-api/internal/messages"
	"github.com/redoubtapp/redoubt-api/internal/opengraph"
	"github.com/redoubtapp/redoubt-api/internal/presence"
	"github.com/redoubtapp/redoubt-api/internal/ratelimit"
	"github.com/redoubtapp/redoubt-api/internal/spaces"
	"github.com/redoubtapp/redoubt-api/internal/storage"
	"github.com/redoubtapp/redoubt-api/internal/telemetry"
	"github.com/redoubtapp/redoubt-api/internal/voice"
)

func main() {
	if err := run(); err != nil {
		slog.Error("application error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	// Set up structured logging early
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	slogLevel, err := logging.LogLevelToSlogLevel(logLevel)
	if err != nil {
		log.Fatalf("could not convert log level: %s", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slogLevel,
	}))
	slog.SetDefault(logger)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	slog.Info("starting redoubt-api",
		slog.String("version", cfg.Telemetry.ServiceVersion),
		slog.String("host", cfg.Server.Host),
		slog.Int("port", cfg.Server.Port),
	)

	// Initialize OpenTelemetry
	slog.Info("initializing OpenTelemetry")
	otelShutdown, err := telemetry.Initialize(ctx, cfg.Telemetry)
	if err != nil {
		return fmt.Errorf("failed to initialize OpenTelemetry: %w", err)
	}
	defer func() {
		if err := otelShutdown(ctx); err != nil {
			slog.Error("failed to shutdown OpenTelemetry", slog.String("error", err.Error()))
		}
	}()
	slog.Info("OpenTelemetry initialized",
		slog.Bool("tracing_enabled", cfg.Telemetry.Tracing.Enabled),
		slog.Bool("metrics_enabled", cfg.Telemetry.Metrics.Enabled),
		slog.Int("metrics_port", cfg.Telemetry.Metrics.Port),
	)

	// Run database migrations
	slog.Info("running database migrations")
	if err := db.RunMigrations(cfg.Database.ConnectionString()); err != nil {
		return fmt.Errorf("failed to run database migrations: %w", err)
	}
	slog.Info("database migrations completed")

	// Initialize database connection pool
	pool, err := db.NewPool(ctx, cfg.Database)
	if err != nil {
		return fmt.Errorf("failed to create database pool: %w", err)
	}
	defer pool.Close()
	slog.Info("database connection pool initialized")

	// Initialize Redis connection
	slog.Info("connecting to Redis")
	redisClient, err := cache.NewClient(ctx, cfg.Redis)
	if err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}
	defer func() {
		if err := redisClient.Close(); err != nil {
			slog.Error("failed to close Redis connection", slog.String("error", err.Error()))
		}
	}()
	slog.Info("Redis connection established")

	// Initialize rate limiter
	var rateLimiter *ratelimit.Limiter
	if cfg.RateLimit.Enabled {
		rateLimiter = ratelimit.NewLimiter(redisClient.Client, cfg.RateLimit)
		slog.Info("rate limiter initialized")
	} else {
		slog.Info("rate limiting disabled")
	}

	// Initialize sqlc queries
	queries := generated.New(pool)

	// Initialize auth components
	jwtManager := auth.NewJWTManager(cfg.Auth.JWTSecret, cfg.Auth.JWTExpiry)
	sessionManager := auth.NewSessionManager(queries, cfg.Auth.RefreshExpiry)

	// Initialize email client
	domain := os.Getenv("DOMAIN")
	baseURL := "https://localhost"
	if domain != "" {
		baseURL = "https://" + domain
	}
	emailClient := email.NewClient(cfg.Email, baseURL)

	// Initialize auth service
	authService := auth.NewService(
		queries,
		jwtManager,
		sessionManager,
		emailClient,
		logger,
		cfg.Auth,
		cfg.Email,
	)

	// Initialize S3 client and storage service
	var storageService *storage.Service
	if cfg.Storage.Endpoint != "" && cfg.Storage.AccessKey != "" {
		s3Client, err := storage.NewS3Client(ctx, cfg.Storage)
		if err != nil {
			slog.Warn("failed to create S3 client, storage features disabled", slog.String("error", err.Error()))
		} else {
			// Ensure bucket exists
			if err := s3Client.EnsureBucket(ctx); err != nil {
				slog.Warn("failed to ensure S3 bucket exists", slog.String("error", err.Error()))
			} else {
				// Initialize encryptor if master key is configured
				if cfg.Storage.Encryption.MasterKey != "" {
					encryptor, err := storage.NewEncryptor(cfg.Storage.Encryption.MasterKey)
					if err != nil {
						slog.Warn("failed to create encryptor, storage features disabled", slog.String("error", err.Error()))
					} else {
						storageService = storage.NewService(s3Client, encryptor, queries, logger)
						slog.Info("storage service initialized")
					}
				} else {
					slog.Warn("storage master key not configured, storage features disabled")
				}
			}
		}
	} else {
		slog.Info("storage not configured, avatar features disabled")
	}

	// Initialize audit service
	auditService := audit.NewService(queries, logger)

	// Initialize space, channel, and invite services
	spaceService := spaces.NewService(queries, logger)
	channelService := channels.NewService(queries, logger)
	inviteService := invites.NewService(queries, logger)

	// Create bootstrap invite if not initialized
	bootstrapCode, err := inviteService.CreateBootstrapInvite(ctx)
	if err != nil {
		slog.Warn("failed to create bootstrap invite", slog.String("error", err.Error()))
	} else if bootstrapCode != "" {
		slog.Info("bootstrap invite code ready", slog.String("code", bootstrapCode))
	}

	// Initialize presence hub for WebSocket connections
	presenceHub := presence.NewHub(queries, logger)
	go presenceHub.Run()
	slog.Info("presence hub started")

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(authService)
	sessionHandler := handlers.NewSessionHandler(authService)
	userHandler := handlers.NewUserHandler(queries, storageService)
	spaceHandler := handlers.NewSpaceHandler(spaceService, auditService)
	channelHandler := handlers.NewChannelHandler(channelService, auditService)
	inviteHandler := handlers.NewInviteHandler(inviteService, auditService)

	// Initialize LiveKit service, voice service, and webhook handler
	var livekitWebhookHandler http.Handler
	var voiceHandler *handlers.VoiceHandler
	if cfg.LiveKit.APIKey != "" && cfg.LiveKit.APISecret != "" {
		livekitService := livekit.NewService(cfg.LiveKit, logger)

		// Initialize voice service with presence hub for real-time events
		voiceService := voice.NewService(queries, livekitService, presenceHub, logger, cfg.Voice.DefaultMaxParticipants)
		voiceHandler = handlers.NewVoiceHandler(voiceService)

		// Webhook handler receives events from LiveKit server
		livekitWebhookHandler = livekit.NewWebhookHandler(
			cfg.LiveKit.APIKey,
			cfg.LiveKit.APISecret,
			presenceHub,
			logger,
		)
		slog.Info("LiveKit service initialized",
			slog.String("host", cfg.LiveKit.Host),
		)
	} else {
		slog.Info("LiveKit not configured, voice features disabled")
	}

	// Initialize WebSocket handler for real-time presence
	wsHandler := handlers.NewWebSocketHandler(presenceHub, jwtManager, queries, logger)
	slog.Info("WebSocket handler initialized")

	// Initialize message services
	messageService := messages.NewService(queries, logger, presenceHub, rateLimiter, cfg.Messages.EditWindow)
	reactionService := messages.NewReactionService(queries, logger, presenceHub, rateLimiter)
	readStateService := messages.NewReadStateService(queries, logger)

	// Initialize message handlers
	messageHandler := handlers.NewMessageHandler(messageService, readStateService, auditService)
	reactionHandler := handlers.NewReactionHandler(reactionService)
	emojiHandler := handlers.NewEmojiHandler(reactionService)
	slog.Info("message handlers initialized")

	// Initialize OpenGraph service and handler
	ogService := opengraph.NewService()
	ogHandler := handlers.NewOpenGraphHandler(ogService)
	slog.Info("OpenGraph handler initialized")

	// Initialize attachment handler (requires storage service)
	var attachmentHandler *handlers.AttachmentHandler
	if storageService != nil {
		attachmentHandler = handlers.NewAttachmentHandler(storageService, messageService)
		slog.Info("attachment handler initialized")
	}

	// Create router with all middleware and handlers
	router := api.NewRouter(api.RouterConfig{
		Logger: logger,
		Config: cfg,
		HealthDeps: &handlers.HealthDependencies{
			DB:      pool,
			Redis:   redisClient,
			Version: cfg.Telemetry.ServiceVersion,
		},
		AuthHandler:           authHandler,
		SessionHandler:        sessionHandler,
		UserHandler:           userHandler,
		SpaceHandler:          spaceHandler,
		ChannelHandler:        channelHandler,
		InviteHandler:         inviteHandler,
		VoiceHandler:          voiceHandler,
		MessageHandler:        messageHandler,
		ReactionHandler:       reactionHandler,
		EmojiHandler:          emojiHandler,
		OpenGraphHandler:      ogHandler,
		AttachmentHandler:     attachmentHandler,
		WebSocketHandler:      wsHandler,
		LiveKitWebhookHandler: livekitWebhookHandler,
		JWTManager:            jwtManager,
		RateLimiter:           rateLimiter,
	})

	// Create HTTP server
	server := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Channel to capture server errors
	serverErr := make(chan error, 2)

	// Start admin panel server (if enabled)
	var adminServer *admin.Server
	if cfg.Admin.Enabled {
		adminServer, err = admin.NewServer(queries, presenceHub, logger, cfg.Admin)
		if err != nil {
			return fmt.Errorf("failed to create admin server: %w", err)
		}
		if err := adminServer.Start(ctx); err != nil {
			return fmt.Errorf("failed to start admin server: %w", err)
		}
		slog.Info("admin panel listening", slog.String("address", cfg.Admin.Address()))
	}

	// Start server in goroutine
	go func() {
		slog.Info("HTTP server listening", slog.String("address", server.Addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// Wait for shutdown signal or server error
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		slog.Info("shutting down server...")
	case err := <-serverErr:
		return fmt.Errorf("HTTP server error: %w", err)
	}

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(ctx, cfg.Server.ShutdownTimeout)
	defer cancel()

	// Stop presence hub first (closes all WebSocket connections)
	slog.Info("stopping presence hub...")
	presenceHub.Stop()

	// Stop admin server
	if adminServer != nil {
		slog.Info("stopping admin panel...")
		if err := adminServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("admin server shutdown error", slog.String("error", err.Error()))
		}
	}

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server forced to shutdown: %w", err)
	}

	slog.Info("server shutdown complete")
	return nil
}

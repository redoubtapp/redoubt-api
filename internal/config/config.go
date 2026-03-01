package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the application.
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Redis     RedisConfig     `mapstructure:"redis"`
	Auth      AuthConfig      `mapstructure:"auth"`
	Email     EmailConfig     `mapstructure:"email"`
	Storage   StorageConfig   `mapstructure:"storage"`
	CORS      CORSConfig      `mapstructure:"cors"`
	Logging   LoggingConfig   `mapstructure:"logging"`
	Telemetry TelemetryConfig `mapstructure:"telemetry"`
	RateLimit RateLimitConfig `mapstructure:"ratelimit"`
	LiveKit   LiveKitConfig   `mapstructure:"livekit"`
	WebSocket WebSocketConfig `mapstructure:"websocket"`
	Presence  PresenceConfig  `mapstructure:"presence"`
	Voice     VoiceConfig     `mapstructure:"voice"`
	Messages  MessagesConfig  `mapstructure:"messages"`
	Admin     AdminConfig     `mapstructure:"admin"`
}

// AdminConfig holds admin panel settings.
type AdminConfig struct {
	Enabled       bool          `mapstructure:"enabled"`
	Port          int           `mapstructure:"port"`
	SessionSecret string        `mapstructure:"session_secret"`
	SessionExpiry time.Duration `mapstructure:"session_expiry"`
}

// Address returns the admin panel address in host:port format.
func (a AdminConfig) Address() string {
	return fmt.Sprintf("0.0.0.0:%d", a.Port)
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

// Address returns the server address in host:port format.
func (s ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

// DatabaseConfig holds PostgreSQL connection settings.
type DatabaseConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	Name            string        `mapstructure:"name"`
	User            string        `mapstructure:"user"`
	Password        string        `mapstructure:"password"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

// ConnectionString returns the PostgreSQL connection string.
func (d DatabaseConfig) ConnectionString() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		d.User, d.Password, d.Host, d.Port, d.Name,
	)
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// Address returns the Redis address in host:port format.
func (r RedisConfig) Address() string {
	return fmt.Sprintf("%s:%d", r.Host, r.Port)
}

// AuthConfig holds authentication settings.
type AuthConfig struct {
	JWTSecret         string        `mapstructure:"jwt_secret"`
	JWTExpiry         time.Duration `mapstructure:"jwt_expiry"`
	RefreshExpiry     time.Duration `mapstructure:"refresh_expiry"`
	PasswordMinLength int           `mapstructure:"password_min_length"`
	LockoutThreshold  int           `mapstructure:"lockout_threshold"`
	LockoutDuration   time.Duration `mapstructure:"lockout_duration"`
}

// EmailConfig holds email service settings.
type EmailConfig struct {
	Provider           string        `mapstructure:"provider"`
	APIKey             string        `mapstructure:"api_key"`
	FromAddress        string        `mapstructure:"from_address"`
	FromName           string        `mapstructure:"from_name"`
	VerificationExpiry time.Duration `mapstructure:"verification_expiry"`
	ResetExpiry        time.Duration `mapstructure:"reset_expiry"`
}

// StorageConfig holds S3-compatible storage settings.
type StorageConfig struct {
	Endpoint   string                  `mapstructure:"endpoint"`
	Bucket     string                  `mapstructure:"bucket"`
	Region     string                  `mapstructure:"region"`
	AccessKey  string                  `mapstructure:"access_key"`
	SecretKey  string                  `mapstructure:"secret_key"`
	Encryption StorageEncryptionConfig `mapstructure:"encryption"`
}

// StorageEncryptionConfig holds storage encryption settings.
type StorageEncryptionConfig struct {
	MasterKey string `mapstructure:"master_key"`
}

// CORSConfig holds CORS settings.
type CORSConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"`
	AllowedMethods []string `mapstructure:"allowed_methods"`
	AllowedHeaders []string `mapstructure:"allowed_headers"`
	MaxAge         int      `mapstructure:"max_age"`
}

// LoggingConfig holds logging settings.
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// TelemetryConfig holds OpenTelemetry settings.
type TelemetryConfig struct {
	ServiceName    string        `mapstructure:"service_name"`
	ServiceVersion string        `mapstructure:"service_version"`
	Tracing        TracingConfig `mapstructure:"tracing"`
	Metrics        MetricsConfig `mapstructure:"metrics"`
}

// TracingConfig holds tracing settings.
type TracingConfig struct {
	Enabled    bool    `mapstructure:"enabled"`
	SampleRate float64 `mapstructure:"sample_rate"`
}

// MetricsConfig holds metrics settings.
type MetricsConfig struct {
	Enabled bool `mapstructure:"enabled"`
	Port    int  `mapstructure:"port"`
}

// RateLimitConfig holds rate limiting settings.
type RateLimitConfig struct {
	Enabled bool                     `mapstructure:"enabled"`
	Rules   map[string]RateLimitRule `mapstructure:"rules"`
}

// RateLimitRule defines a rate limit for a specific scope.
type RateLimitRule struct {
	Limit  int           `mapstructure:"limit"`
	Window time.Duration `mapstructure:"window"`
}

// LiveKitConfig holds LiveKit SFU settings.
type LiveKitConfig struct {
	Host         string `mapstructure:"host"`
	APIKey       string `mapstructure:"api_key"`
	APISecret    string `mapstructure:"api_secret"`
	WebSocketURL string `mapstructure:"ws_url"`
	WebhookPath  string `mapstructure:"webhook_path"`
}

// WebSocketConfig holds WebSocket server settings.
type WebSocketConfig struct {
	ReadBufferSize  int           `mapstructure:"read_buffer_size"`
	WriteBufferSize int           `mapstructure:"write_buffer_size"`
	PingInterval    time.Duration `mapstructure:"ping_interval"`
	PongTimeout     time.Duration `mapstructure:"pong_timeout"`
	MaxMessageSize  int64         `mapstructure:"max_message_size"`
}

// PresenceConfig holds presence system settings.
type PresenceConfig struct {
	TypingTimeout time.Duration `mapstructure:"typing_timeout"`
	IdleTimeout   time.Duration `mapstructure:"idle_timeout"`
}

// VoiceConfig holds voice channel settings.
type VoiceConfig struct {
	DefaultMaxParticipants int `mapstructure:"default_max_participants"`
}

// MessagesConfig holds message system settings.
type MessagesConfig struct {
	EditWindow time.Duration `mapstructure:"edit_window"`
}

// Load reads configuration from file and environment variables.
func Load() (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Configuration file path
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config/config.yaml"
	}

	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		// Config file is optional; use defaults and env vars
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Environment variable overrides
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Bind specific env vars for secrets
	bindEnvVars(v)

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout", "30s")
	v.SetDefault("server.write_timeout", "30s")
	v.SetDefault("server.shutdown_timeout", "30s")

	// Database defaults
	v.SetDefault("database.host", "postgres")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.name", "redoubt")
	v.SetDefault("database.user", "redoubt")
	v.SetDefault("database.max_open_conns", 25)
	v.SetDefault("database.max_idle_conns", 5)
	v.SetDefault("database.conn_max_lifetime", "5m")

	// Redis defaults
	v.SetDefault("redis.host", "redis")
	v.SetDefault("redis.port", 6379)
	v.SetDefault("redis.db", 0)

	// Auth defaults
	v.SetDefault("auth.jwt_expiry", "15m")
	v.SetDefault("auth.refresh_expiry", "720h")
	v.SetDefault("auth.password_min_length", 12)
	v.SetDefault("auth.lockout_threshold", 5)
	v.SetDefault("auth.lockout_duration", "15m")

	// Email defaults
	v.SetDefault("email.provider", "resend")
	v.SetDefault("email.from_name", "Redoubt")
	v.SetDefault("email.verification_expiry", "24h")
	v.SetDefault("email.reset_expiry", "1h")

	// Storage defaults
	v.SetDefault("storage.endpoint", "http://localstack:4566")
	v.SetDefault("storage.bucket", "redoubt-media")
	v.SetDefault("storage.region", "us-east-1")

	// CORS defaults
	v.SetDefault("cors.allowed_methods", []string{"GET", "POST", "PUT", "PATCH", "DELETE"})
	v.SetDefault("cors.allowed_headers", []string{"Authorization", "Content-Type", "X-Request-ID"})
	v.SetDefault("cors.max_age", 86400)

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")

	// Telemetry defaults
	v.SetDefault("telemetry.service_name", "redoubt-api")
	v.SetDefault("telemetry.service_version", "0.1.0")
	v.SetDefault("telemetry.tracing.enabled", true)
	v.SetDefault("telemetry.tracing.sample_rate", 0.1)
	v.SetDefault("telemetry.metrics.enabled", true)
	v.SetDefault("telemetry.metrics.port", 9090)

	// Rate limit defaults
	v.SetDefault("ratelimit.enabled", true)
	v.SetDefault("ratelimit.rules.register.limit", 5)
	v.SetDefault("ratelimit.rules.register.window", "1h")
	v.SetDefault("ratelimit.rules.login.limit", 10)
	v.SetDefault("ratelimit.rules.login.window", "15m")
	v.SetDefault("ratelimit.rules.forgot_password.limit", 3)
	v.SetDefault("ratelimit.rules.forgot_password.window", "1h")
	v.SetDefault("ratelimit.rules.verify_email.limit", 10)
	v.SetDefault("ratelimit.rules.verify_email.window", "1h")
	v.SetDefault("ratelimit.rules.general.limit", 100)
	v.SetDefault("ratelimit.rules.general.window", "1m")
	v.SetDefault("ratelimit.rules.file_upload.limit", 10)
	v.SetDefault("ratelimit.rules.file_upload.window", "1h")
	v.SetDefault("ratelimit.rules.message_send.limit", 5)
	v.SetDefault("ratelimit.rules.message_send.window", "5s")
	v.SetDefault("ratelimit.rules.message_edit.limit", 3)
	v.SetDefault("ratelimit.rules.message_edit.window", "1m")
	v.SetDefault("ratelimit.rules.reaction_add.limit", 20)
	v.SetDefault("ratelimit.rules.reaction_add.window", "1m")

	// LiveKit defaults
	v.SetDefault("livekit.host", "http://livekit:7880")
	v.SetDefault("livekit.webhook_path", "/api/v1/livekit/webhook")

	// WebSocket defaults
	v.SetDefault("websocket.read_buffer_size", 1024)
	v.SetDefault("websocket.write_buffer_size", 1024)
	v.SetDefault("websocket.ping_interval", "30s")
	v.SetDefault("websocket.pong_timeout", "60s")
	v.SetDefault("websocket.max_message_size", 4096)

	// Presence defaults
	v.SetDefault("presence.typing_timeout", "5s")
	v.SetDefault("presence.idle_timeout", "5m")

	// Voice defaults
	v.SetDefault("voice.default_max_participants", 25)

	// Messages defaults
	v.SetDefault("messages.edit_window", "15m")

	// Admin panel defaults
	v.SetDefault("admin.enabled", true)
	v.SetDefault("admin.port", 9091)
	v.SetDefault("admin.session_expiry", "24h")
}

func bindEnvVars(v *viper.Viper) {
	// Bind environment variables for secrets
	_ = v.BindEnv("database.password", "POSTGRES_PASSWORD")
	_ = v.BindEnv("auth.jwt_secret", "JWT_SECRET")
	_ = v.BindEnv("email.api_key", "RESEND_API_KEY")
	_ = v.BindEnv("email.from_address", "EMAIL_FROM_ADDRESS")
	_ = v.BindEnv("storage.access_key", "S3_ACCESS_KEY")
	_ = v.BindEnv("storage.secret_key", "S3_SECRET_KEY")
	_ = v.BindEnv("storage.endpoint", "S3_ENDPOINT")
	_ = v.BindEnv("storage.bucket", "S3_BUCKET")
	_ = v.BindEnv("storage.region", "S3_REGION")
	_ = v.BindEnv("storage.encryption.master_key", "STORAGE_MASTER_KEY")
	_ = v.BindEnv("livekit.api_key", "LIVEKIT_API_KEY")
	_ = v.BindEnv("livekit.api_secret", "LIVEKIT_API_SECRET")
	_ = v.BindEnv("admin.session_secret", "ADMIN_SESSION_SECRET")
}

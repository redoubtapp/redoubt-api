package cache

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"

	"github.com/redoubtapp/redoubt-api/internal/config"
)

// Client wraps a Redis client with OpenTelemetry instrumentation.
type Client struct {
	*redis.Client
}

// NewClient creates a new Redis client with OpenTelemetry tracing and metrics.
func NewClient(ctx context.Context, cfg config.RedisConfig) (*Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Address(),
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	// Add OpenTelemetry tracing instrumentation
	if err := redisotel.InstrumentTracing(client); err != nil {
		return nil, fmt.Errorf("failed to instrument redis tracing: %w", err)
	}

	// Add OpenTelemetry metrics instrumentation
	if err := redisotel.InstrumentMetrics(client); err != nil {
		return nil, fmt.Errorf("failed to instrument redis metrics: %w", err)
	}

	// Verify connection
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &Client{Client: client}, nil
}

// Close closes the Redis connection.
func (c *Client) Close() error {
	return c.Client.Close()
}

// HealthCheck performs a health check on the Redis connection.
func (c *Client) HealthCheck(ctx context.Context) error {
	return c.Ping(ctx).Err()
}

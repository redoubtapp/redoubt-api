package db

import (
	"context"
	"fmt"
	"math"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redoubtapp/redoubt-api/internal/config"
)

// NewPool creates a new PostgreSQL connection pool with OpenTelemetry instrumentation.
func NewPool(ctx context.Context, cfg config.DatabaseConfig) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.ConnectionString())
	if err != nil {
		return nil, fmt.Errorf("failed to parse database config: %w", err)
	}

	// Set pool configuration with safe int32 conversion
	if cfg.MaxOpenConns > math.MaxInt32 || cfg.MaxOpenConns < 0 {
		return nil, fmt.Errorf("max_open_conns value %d is out of valid range", cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > math.MaxInt32 || cfg.MaxIdleConns < 0 {
		return nil, fmt.Errorf("max_idle_conns value %d is out of valid range", cfg.MaxIdleConns)
	}
	poolConfig.MaxConns = int32(cfg.MaxOpenConns) //nolint:gosec // bounds checked above
	poolConfig.MinConns = int32(cfg.MaxIdleConns) //nolint:gosec // bounds checked above
	poolConfig.MaxConnLifetime = cfg.ConnMaxLifetime

	// Add OpenTelemetry tracer for database instrumentation
	poolConfig.ConnConfig.Tracer = otelpgx.NewTracer()

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return pool, nil
}

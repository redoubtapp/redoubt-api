package db

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/redoubtapp/redoubt-api/internal/config"
)

func setupPostgresContainer(t *testing.T) (config.DatabaseConfig, func()) {
	t.Helper()

	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}

	host, err := pgContainer.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get postgres host: %v", err)
	}

	port, err := pgContainer.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("failed to get postgres port: %v", err)
	}

	cfg := config.DatabaseConfig{
		Host:            host,
		Port:            port.Int(),
		Name:            "testdb",
		User:            "testuser",
		Password:        "testpass",
		MaxOpenConns:    10,
		MaxIdleConns:    2,
		ConnMaxLifetime: 5 * time.Minute,
	}

	cleanup := func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Logf("failed to terminate postgres container: %v", err)
		}
	}

	return cfg, cleanup
}

func TestNewPool(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tests := []struct {
		name    string
		setup   func(t *testing.T) (config.DatabaseConfig, func())
		wantErr bool
	}{
		{
			name:    "successful connection",
			setup:   setupPostgresContainer,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, cleanup := tt.setup(t)
			defer cleanup()

			ctx := context.Background()
			pool, err := NewPool(ctx, cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewPool() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if pool == nil {
					t.Error("NewPool() returned nil pool")
					return
				}
				defer pool.Close()

				// Verify connection works
				err := pool.Ping(ctx)
				if err != nil {
					t.Errorf("pool.Ping() error = %v", err)
				}
			}
		})
	}
}

func TestNewPoolConnectionFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tests := []struct {
		name string
		cfg  config.DatabaseConfig
	}{
		{
			name: "invalid host",
			cfg: config.DatabaseConfig{
				Host:            "nonexistent.invalid",
				Port:            5432,
				Name:            "testdb",
				User:            "testuser",
				Password:        "testpass",
				MaxOpenConns:    10,
				MaxIdleConns:    2,
				ConnMaxLifetime: time.Minute,
			},
		},
		{
			name: "invalid port",
			cfg: config.DatabaseConfig{
				Host:            "localhost",
				Port:            1,
				Name:            "testdb",
				User:            "testuser",
				Password:        "testpass",
				MaxOpenConns:    10,
				MaxIdleConns:    2,
				ConnMaxLifetime: time.Minute,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			pool, err := NewPool(ctx, tt.cfg)
			if err == nil {
				pool.Close()
				t.Error("NewPool() expected error for invalid connection")
			}
		})
	}
}

func TestNewPoolInvalidConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tests := []struct {
		name string
		cfg  config.DatabaseConfig
	}{
		{
			name: "max_open_conns out of range",
			cfg: config.DatabaseConfig{
				Host:            "localhost",
				Port:            5432,
				Name:            "testdb",
				User:            "testuser",
				Password:        "testpass",
				MaxOpenConns:    -1,
				MaxIdleConns:    2,
				ConnMaxLifetime: time.Minute,
			},
		},
		{
			name: "max_idle_conns out of range",
			cfg: config.DatabaseConfig{
				Host:            "localhost",
				Port:            5432,
				Name:            "testdb",
				User:            "testuser",
				Password:        "testpass",
				MaxOpenConns:    10,
				MaxIdleConns:    -1,
				ConnMaxLifetime: time.Minute,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			pool, err := NewPool(ctx, tt.cfg)
			if err == nil {
				pool.Close()
				t.Error("NewPool() expected error for invalid config")
			}
		})
	}
}

func TestPoolOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg, cleanup := setupPostgresContainer(t)
	defer cleanup()

	ctx := context.Background()
	pool, err := NewPool(ctx, cfg)
	if err != nil {
		t.Fatalf("NewPool() error = %v", err)
	}
	defer pool.Close()

	tests := []struct {
		name      string
		operation func(t *testing.T)
	}{
		{
			name: "ping",
			operation: func(t *testing.T) {
				err := pool.Ping(ctx)
				if err != nil {
					t.Errorf("Ping() error = %v", err)
				}
			},
		},
		{
			name: "execute query",
			operation: func(t *testing.T) {
				var result int
				err := pool.QueryRow(ctx, "SELECT 1 + 1").Scan(&result)
				if err != nil {
					t.Fatalf("QueryRow() error = %v", err)
				}
				if result != 2 {
					t.Errorf("QueryRow() = %v, want 2", result)
				}
			},
		},
		{
			name: "execute query with now()",
			operation: func(t *testing.T) {
				var result time.Time
				err := pool.QueryRow(ctx, "SELECT NOW()").Scan(&result)
				if err != nil {
					t.Fatalf("QueryRow() error = %v", err)
				}
				// Result should be close to now
				if time.Since(result) > time.Minute {
					t.Errorf("NOW() result seems incorrect: %v", result)
				}
			},
		},
		{
			name: "create and query temp table",
			operation: func(t *testing.T) {
				// Create temp table
				_, err := pool.Exec(ctx, `
					CREATE TEMP TABLE test_items (
						id SERIAL PRIMARY KEY,
						name TEXT NOT NULL
					)
				`)
				if err != nil {
					t.Fatalf("CREATE TABLE error = %v", err)
				}

				// Insert data
				_, err = pool.Exec(ctx, "INSERT INTO test_items (name) VALUES ($1), ($2)", "item1", "item2")
				if err != nil {
					t.Fatalf("INSERT error = %v", err)
				}

				// Query data
				var count int
				err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM test_items").Scan(&count)
				if err != nil {
					t.Fatalf("SELECT COUNT error = %v", err)
				}
				if count != 2 {
					t.Errorf("count = %v, want 2", count)
				}
			},
		},
		{
			name: "transaction commit",
			operation: func(t *testing.T) {
				tx, err := pool.Begin(ctx)
				if err != nil {
					t.Fatalf("Begin() error = %v", err)
				}

				_, err = tx.Exec(ctx, `
					CREATE TEMP TABLE tx_test (
						id SERIAL PRIMARY KEY,
						value INT
					)
				`)
				if err != nil {
					_ = tx.Rollback(ctx)
					t.Fatalf("CREATE TABLE in tx error = %v", err)
				}

				_, err = tx.Exec(ctx, "INSERT INTO tx_test (value) VALUES ($1)", 42)
				if err != nil {
					_ = tx.Rollback(ctx)
					t.Fatalf("INSERT in tx error = %v", err)
				}

				err = tx.Commit(ctx)
				if err != nil {
					t.Fatalf("Commit() error = %v", err)
				}

				// Verify data was committed
				var value int
				err = pool.QueryRow(ctx, "SELECT value FROM tx_test WHERE value = 42").Scan(&value)
				if err != nil {
					t.Fatalf("SELECT after commit error = %v", err)
				}
				if value != 42 {
					t.Errorf("value = %v, want 42", value)
				}
			},
		},
		{
			name: "transaction rollback",
			operation: func(t *testing.T) {
				// Create a table first
				_, err := pool.Exec(ctx, `
					CREATE TEMP TABLE IF NOT EXISTS rollback_test (
						id SERIAL PRIMARY KEY,
						value INT
					)
				`)
				if err != nil {
					t.Fatalf("CREATE TABLE error = %v", err)
				}

				tx, err := pool.Begin(ctx)
				if err != nil {
					t.Fatalf("Begin() error = %v", err)
				}

				_, err = tx.Exec(ctx, "INSERT INTO rollback_test (value) VALUES ($1)", 999)
				if err != nil {
					_ = tx.Rollback(ctx)
					t.Fatalf("INSERT in tx error = %v", err)
				}

				err = tx.Rollback(ctx)
				if err != nil {
					t.Fatalf("Rollback() error = %v", err)
				}

				// Verify data was not committed
				var count int
				err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM rollback_test WHERE value = 999").Scan(&count)
				if err != nil {
					t.Fatalf("SELECT after rollback error = %v", err)
				}
				if count != 0 {
					t.Errorf("count = %v, want 0 (data should be rolled back)", count)
				}
			},
		},
		{
			name: "pool stats",
			operation: func(t *testing.T) {
				stats := pool.Stat()
				if stats == nil {
					t.Error("Stat() returned nil")
					return
				}

				// Pool should have some connections
				if stats.TotalConns() < 1 {
					t.Error("expected at least 1 connection in pool")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.operation(t)
		})
	}
}

func TestPoolClose(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg, cleanup := setupPostgresContainer(t)
	defer cleanup()

	ctx := context.Background()
	pool, err := NewPool(ctx, cfg)
	if err != nil {
		t.Fatalf("NewPool() error = %v", err)
	}

	// Verify pool is working
	err = pool.Ping(ctx)
	if err != nil {
		t.Fatalf("Ping() before close error = %v", err)
	}

	// Close the pool
	pool.Close()

	// Verify pool is closed
	err = pool.Ping(ctx)
	if err == nil {
		t.Error("Ping() after close should return error")
	}
}

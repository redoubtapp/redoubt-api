package cache

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/redoubtapp/redoubt-api/internal/config"
)

func setupRedisContainer(t *testing.T) (config.RedisConfig, func()) {
	t.Helper()

	ctx := context.Background()

	redisContainer, err := redis.Run(ctx,
		"redis:7-alpine",
	)
	if err != nil {
		t.Fatalf("failed to start redis container: %v", err)
	}

	host, err := redisContainer.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get redis host: %v", err)
	}

	port, err := redisContainer.MappedPort(ctx, "6379")
	if err != nil {
		t.Fatalf("failed to get redis port: %v", err)
	}

	cfg := config.RedisConfig{
		Host:     host,
		Port:     port.Int(),
		Password: "",
		DB:       0,
	}

	cleanup := func() {
		if err := redisContainer.Terminate(ctx); err != nil {
			t.Logf("failed to terminate redis container: %v", err)
		}
	}

	return cfg, cleanup
}

func TestNewClient(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tests := []struct {
		name    string
		setup   func(t *testing.T) (config.RedisConfig, func())
		wantErr bool
	}{
		{
			name:    "successful connection",
			setup:   setupRedisContainer,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, cleanup := tt.setup(t)
			defer cleanup()

			ctx := context.Background()
			client, err := NewClient(ctx, cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if client == nil {
					t.Error("NewClient() returned nil client")
					return
				}
				defer func() { _ = client.Close() }()
			}
		})
	}
}

func TestNewClientConnectionFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tests := []struct {
		name string
		cfg  config.RedisConfig
	}{
		{
			name: "invalid host",
			cfg: config.RedisConfig{
				Host:     "nonexistent.invalid",
				Port:     6379,
				Password: "",
				DB:       0,
			},
		},
		{
			name: "invalid port",
			cfg: config.RedisConfig{
				Host:     "localhost",
				Port:     1,
				Password: "",
				DB:       0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			client, err := NewClient(ctx, tt.cfg)
			if err == nil {
				_ = client.Close()
				t.Error("NewClient() expected error for invalid connection")
			}
		})
	}
}

func TestClientClose(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg, cleanup := setupRedisContainer(t)
	defer cleanup()

	ctx := context.Background()
	client, err := NewClient(ctx, cfg)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	err = client.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Verify connection is closed by trying to ping
	err = client.Ping(ctx).Err()
	if err == nil {
		t.Error("expected error when pinging closed connection")
	}
}

func TestClientHealthCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg, cleanup := setupRedisContainer(t)
	defer cleanup()

	ctx := context.Background()
	client, err := NewClient(ctx, cfg)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer func() { _ = client.Close() }()

	tests := []struct {
		name    string
		setup   func()
		wantErr bool
	}{
		{
			name:    "healthy connection",
			setup:   func() {},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			err := client.HealthCheck(ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("HealthCheck() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestClientOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg, cleanup := setupRedisContainer(t)
	defer cleanup()

	ctx := context.Background()
	client, err := NewClient(ctx, cfg)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer func() { _ = client.Close() }()

	tests := []struct {
		name      string
		operation func(t *testing.T)
	}{
		{
			name: "set and get string",
			operation: func(t *testing.T) {
				key := "test:string"
				value := "hello world"

				err := client.Set(ctx, key, value, time.Minute).Err()
				if err != nil {
					t.Fatalf("Set() error = %v", err)
				}

				got, err := client.Get(ctx, key).Result()
				if err != nil {
					t.Fatalf("Get() error = %v", err)
				}

				if got != value {
					t.Errorf("Get() = %v, want %v", got, value)
				}
			},
		},
		{
			name: "set with expiration",
			operation: func(t *testing.T) {
				key := "test:expiring"
				value := "temporary"

				err := client.Set(ctx, key, value, 100*time.Millisecond).Err()
				if err != nil {
					t.Fatalf("Set() error = %v", err)
				}

				// Key should exist initially
				exists, err := client.Exists(ctx, key).Result()
				if err != nil {
					t.Fatalf("Exists() error = %v", err)
				}
				if exists != 1 {
					t.Error("key should exist")
				}

				// Wait for expiration
				time.Sleep(150 * time.Millisecond)

				// Key should be gone
				exists, err = client.Exists(ctx, key).Result()
				if err != nil {
					t.Fatalf("Exists() error = %v", err)
				}
				if exists != 0 {
					t.Error("key should have expired")
				}
			},
		},
		{
			name: "delete key",
			operation: func(t *testing.T) {
				key := "test:delete"
				value := "to be deleted"

				err := client.Set(ctx, key, value, 0).Err()
				if err != nil {
					t.Fatalf("Set() error = %v", err)
				}

				deleted, err := client.Del(ctx, key).Result()
				if err != nil {
					t.Fatalf("Del() error = %v", err)
				}
				if deleted != 1 {
					t.Errorf("Del() = %v, want 1", deleted)
				}

				exists, err := client.Exists(ctx, key).Result()
				if err != nil {
					t.Fatalf("Exists() error = %v", err)
				}
				if exists != 0 {
					t.Error("key should not exist after deletion")
				}
			},
		},
		{
			name: "increment counter",
			operation: func(t *testing.T) {
				key := "test:counter"

				// Clean up first
				client.Del(ctx, key)

				val, err := client.Incr(ctx, key).Result()
				if err != nil {
					t.Fatalf("Incr() error = %v", err)
				}
				if val != 1 {
					t.Errorf("Incr() = %v, want 1", val)
				}

				val, err = client.Incr(ctx, key).Result()
				if err != nil {
					t.Fatalf("Incr() error = %v", err)
				}
				if val != 2 {
					t.Errorf("Incr() = %v, want 2", val)
				}
			},
		},
		{
			name: "hash operations",
			operation: func(t *testing.T) {
				key := "test:hash"
				field := "name"
				value := "redoubt"

				err := client.HSet(ctx, key, field, value).Err()
				if err != nil {
					t.Fatalf("HSet() error = %v", err)
				}

				got, err := client.HGet(ctx, key, field).Result()
				if err != nil {
					t.Fatalf("HGet() error = %v", err)
				}

				if got != value {
					t.Errorf("HGet() = %v, want %v", got, value)
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

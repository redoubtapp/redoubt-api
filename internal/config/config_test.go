package config

import (
	"testing"
)

func TestServerConfigAddress(t *testing.T) {
	tests := []struct {
		name   string
		config ServerConfig
		want   string
	}{
		{
			name: "default localhost",
			config: ServerConfig{
				Host: "localhost",
				Port: 8080,
			},
			want: "localhost:8080",
		},
		{
			name: "all interfaces",
			config: ServerConfig{
				Host: "0.0.0.0",
				Port: 3000,
			},
			want: "0.0.0.0:3000",
		},
		{
			name: "specific ip",
			config: ServerConfig{
				Host: "192.168.1.100",
				Port: 443,
			},
			want: "192.168.1.100:443",
		},
		{
			name: "empty host",
			config: ServerConfig{
				Host: "",
				Port: 8080,
			},
			want: ":8080",
		},
		{
			name: "port zero",
			config: ServerConfig{
				Host: "localhost",
				Port: 0,
			},
			want: "localhost:0",
		},
		{
			name: "high port",
			config: ServerConfig{
				Host: "localhost",
				Port: 65535,
			},
			want: "localhost:65535",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.Address()
			if got != tt.want {
				t.Errorf("Address() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDatabaseConfigConnectionString(t *testing.T) {
	tests := []struct {
		name   string
		config DatabaseConfig
		want   string
	}{
		{
			name: "standard configuration",
			config: DatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				Name:     "mydb",
				User:     "admin",
				Password: "secret",
			},
			want: "postgres://admin:secret@localhost:5432/mydb?sslmode=disable",
		},
		{
			name: "docker default",
			config: DatabaseConfig{
				Host:     "postgres",
				Port:     5432,
				Name:     "redoubt",
				User:     "redoubt",
				Password: "password123",
			},
			want: "postgres://redoubt:password123@postgres:5432/redoubt?sslmode=disable",
		},
		{
			name: "empty password",
			config: DatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				Name:     "testdb",
				User:     "testuser",
				Password: "",
			},
			want: "postgres://testuser:@localhost:5432/testdb?sslmode=disable",
		},
		{
			name: "custom port",
			config: DatabaseConfig{
				Host:     "db.example.com",
				Port:     5433,
				Name:     "production",
				User:     "app",
				Password: "p@ssw0rd!",
			},
			want: "postgres://app:p@ssw0rd!@db.example.com:5433/production?sslmode=disable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.ConnectionString()
			if got != tt.want {
				t.Errorf("ConnectionString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRedisConfigAddress(t *testing.T) {
	tests := []struct {
		name   string
		config RedisConfig
		want   string
	}{
		{
			name: "default redis",
			config: RedisConfig{
				Host: "localhost",
				Port: 6379,
			},
			want: "localhost:6379",
		},
		{
			name: "docker redis",
			config: RedisConfig{
				Host: "redis",
				Port: 6379,
			},
			want: "redis:6379",
		},
		{
			name: "custom port",
			config: RedisConfig{
				Host: "cache.example.com",
				Port: 6380,
			},
			want: "cache.example.com:6380",
		},
		{
			name: "empty host",
			config: RedisConfig{
				Host: "",
				Port: 6379,
			},
			want: ":6379",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.Address()
			if got != tt.want {
				t.Errorf("Address() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigStructs(t *testing.T) {
	// Test that all config structs can be instantiated with zero values
	t.Run("ServerConfig zero value", func(t *testing.T) {
		cfg := ServerConfig{}
		addr := cfg.Address()
		if addr != ":0" {
			t.Errorf("Zero value Address() = %v, want :0", addr)
		}
	})

	t.Run("DatabaseConfig zero value", func(t *testing.T) {
		cfg := DatabaseConfig{}
		connStr := cfg.ConnectionString()
		expected := "postgres://:@:0/?sslmode=disable"
		if connStr != expected {
			t.Errorf("Zero value ConnectionString() = %v, want %v", connStr, expected)
		}
	})

	t.Run("RedisConfig zero value", func(t *testing.T) {
		cfg := RedisConfig{}
		addr := cfg.Address()
		if addr != ":0" {
			t.Errorf("Zero value Address() = %v, want :0", addr)
		}
	})
}

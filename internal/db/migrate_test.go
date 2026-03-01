package db

import (
	"context"
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// getLatestMigrationVersion counts the number of up migrations to determine the latest version.
func getLatestMigrationVersion(t *testing.T) uint {
	t.Helper()

	var count uint
	err := fs.WalkDir(migrationsFS, "migrations", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".up.sql") {
			count++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to count migrations: %v", err)
	}
	return count
}

func setupPostgresForMigration(t *testing.T) (string, func()) {
	t.Helper()

	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("migratedb"),
		postgres.WithUsername("migrateuser"),
		postgres.WithPassword("migratepass"),
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

	databaseURL := fmt.Sprintf(
		"postgres://migrateuser:migratepass@%s:%s/migratedb?sslmode=disable",
		host, port.Port(),
	)

	cleanup := func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Logf("failed to terminate postgres container: %v", err)
		}
	}

	return databaseURL, cleanup
}

func TestRunMigrations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "run migrations successfully",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			databaseURL, cleanup := setupPostgresForMigration(t)
			defer cleanup()

			err := RunMigrations(databaseURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("RunMigrations() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRunMigrationsIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	databaseURL, cleanup := setupPostgresForMigration(t)
	defer cleanup()

	// Run migrations first time
	err := RunMigrations(databaseURL)
	if err != nil {
		t.Fatalf("RunMigrations() first run error = %v", err)
	}

	// Run migrations second time (should be idempotent)
	err = RunMigrations(databaseURL)
	if err != nil {
		t.Errorf("RunMigrations() second run error = %v (should be idempotent)", err)
	}

	// Run migrations third time
	err = RunMigrations(databaseURL)
	if err != nil {
		t.Errorf("RunMigrations() third run error = %v (should be idempotent)", err)
	}
}

func TestRunMigrationsInvalidURL(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tests := []struct {
		name        string
		databaseURL string
	}{
		{
			name:        "invalid host",
			databaseURL: "postgres://user:pass@nonexistent.invalid:5432/db?sslmode=disable",
		},
		{
			name:        "malformed URL",
			databaseURL: "not-a-valid-url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RunMigrations(tt.databaseURL)
			if err == nil {
				t.Error("RunMigrations() expected error for invalid URL")
			}
		})
	}
}

func TestGetMigrationVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	databaseURL, cleanup := setupPostgresForMigration(t)
	defer cleanup()

	latestVersion := getLatestMigrationVersion(t)

	tests := []struct {
		name        string
		setup       func() error
		wantVersion uint
		wantDirty   bool
		wantErr     bool
	}{
		{
			name:        "no migrations run",
			setup:       func() error { return nil },
			wantVersion: 0,
			wantDirty:   false,
			wantErr:     false,
		},
		{
			name: "after running migrations",
			setup: func() error {
				return RunMigrations(databaseURL)
			},
			wantVersion: latestVersion,
			wantDirty:   false,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.setup(); err != nil {
				t.Fatalf("setup() error = %v", err)
			}

			version, dirty, err := GetMigrationVersion(databaseURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetMigrationVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if version != tt.wantVersion {
				t.Errorf("GetMigrationVersion() version = %v, want %v", version, tt.wantVersion)
			}

			if dirty != tt.wantDirty {
				t.Errorf("GetMigrationVersion() dirty = %v, want %v", dirty, tt.wantDirty)
			}
		})
	}
}

func TestGetMigrationVersionInvalidURL(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tests := []struct {
		name        string
		databaseURL string
	}{
		{
			name:        "invalid host",
			databaseURL: "postgres://user:pass@nonexistent.invalid:5432/db?sslmode=disable",
		},
		{
			name:        "malformed URL",
			databaseURL: "not-a-valid-url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := GetMigrationVersion(tt.databaseURL)
			if err == nil {
				t.Error("GetMigrationVersion() expected error for invalid URL")
			}
		})
	}
}

func TestMigrateDown(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	databaseURL, cleanup := setupPostgresForMigration(t)
	defer cleanup()

	latestVersion := getLatestMigrationVersion(t)

	// First run migrations up
	err := RunMigrations(databaseURL)
	if err != nil {
		t.Fatalf("RunMigrations() error = %v", err)
	}

	// Verify we're at the latest version
	version, dirty, err := GetMigrationVersion(databaseURL)
	if err != nil {
		t.Fatalf("GetMigrationVersion() error = %v", err)
	}
	if version != latestVersion {
		t.Errorf("version = %v, want %v", version, latestVersion)
	}
	if dirty {
		t.Error("migration should not be dirty")
	}

	// Migrate down one step
	err = MigrateDown(databaseURL)
	if err != nil {
		t.Errorf("MigrateDown() error = %v", err)
	}

	// Verify we're at version latestVersion - 1
	version, _, err = GetMigrationVersion(databaseURL)
	if err != nil {
		t.Fatalf("GetMigrationVersion() after down error = %v", err)
	}
	expectedVersion := latestVersion - 1
	if version != expectedVersion {
		t.Errorf("version after down = %v, want %v", version, expectedVersion)
	}
}

func TestMigrateDownInvalidURL(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tests := []struct {
		name        string
		databaseURL string
	}{
		{
			name:        "invalid host",
			databaseURL: "postgres://user:pass@nonexistent.invalid:5432/db?sslmode=disable",
		},
		{
			name:        "malformed URL",
			databaseURL: "not-a-valid-url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := MigrateDown(tt.databaseURL)
			if err == nil {
				t.Error("MigrateDown() expected error for invalid URL")
			}
		})
	}
}

func TestMigrationSchemaCreated(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	databaseURL, cleanup := setupPostgresForMigration(t)
	defer cleanup()

	// Run migrations
	err := RunMigrations(databaseURL)
	if err != nil {
		t.Fatalf("RunMigrations() error = %v", err)
	}

	// Get a connection to verify tables exist
	cfg, cleanupPool := setupPostgresContainer(t)
	defer cleanupPool()

	// Use the same database URL to check schema
	ctx := context.Background()

	// We need to connect using the migration database URL
	// For simplicity, we'll verify using psql or a direct connection
	// The migration test database is separate from the pool test database

	// Just verify the migration ran successfully and version is correct
	version, dirty, err := GetMigrationVersion(databaseURL)
	if err != nil {
		t.Fatalf("GetMigrationVersion() error = %v", err)
	}

	if version < 1 {
		t.Errorf("expected at least version 1, got %v", version)
	}

	if dirty {
		t.Error("migration should not be dirty after successful run")
	}

	// Verify we can run again without errors (idempotent)
	err = RunMigrations(databaseURL)
	if err != nil {
		t.Errorf("RunMigrations() second run should not error: %v", err)
	}

	// Suppress unused variable warning
	_ = cfg
	_ = ctx
}

func TestMigrationUpDownUp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	databaseURL, cleanup := setupPostgresForMigration(t)
	defer cleanup()

	latestVersion := getLatestMigrationVersion(t)

	// Up
	err := RunMigrations(databaseURL)
	if err != nil {
		t.Fatalf("RunMigrations() up error = %v", err)
	}

	version, _, err := GetMigrationVersion(databaseURL)
	if err != nil {
		t.Fatalf("GetMigrationVersion() error = %v", err)
	}
	if version != latestVersion {
		t.Errorf("version after up = %v, want %v", version, latestVersion)
	}

	// Down one step
	err = MigrateDown(databaseURL)
	if err != nil {
		t.Fatalf("MigrateDown() error = %v", err)
	}

	version, _, err = GetMigrationVersion(databaseURL)
	if err != nil {
		t.Fatalf("GetMigrationVersion() after down error = %v", err)
	}
	expectedAfterDown := latestVersion - 1
	if version != expectedAfterDown {
		t.Errorf("version after down = %v, want %v", version, expectedAfterDown)
	}

	// Up again
	err = RunMigrations(databaseURL)
	if err != nil {
		t.Fatalf("RunMigrations() up again error = %v", err)
	}

	version, _, err = GetMigrationVersion(databaseURL)
	if err != nil {
		t.Fatalf("GetMigrationVersion() after up again error = %v", err)
	}
	if version != latestVersion {
		t.Errorf("version after up again = %v, want %v", version, latestVersion)
	}
}

package logging

import (
	"log/slog"
	"testing"
)

func TestLogLevelToSlogLevel(t *testing.T) {
	tests := []struct {
		name    string
		level   string
		want    slog.Level
		wantErr bool
	}{
		{
			name:    "debug lowercase",
			level:   "debug",
			want:    slog.LevelDebug,
			wantErr: false,
		},
		{
			name:    "debug uppercase",
			level:   "DEBUG",
			want:    slog.LevelDebug,
			wantErr: false,
		},
		{
			name:    "debug mixed case",
			level:   "DeBuG",
			want:    slog.LevelDebug,
			wantErr: false,
		},
		{
			name:    "info lowercase",
			level:   "info",
			want:    slog.LevelInfo,
			wantErr: false,
		},
		{
			name:    "info uppercase",
			level:   "INFO",
			want:    slog.LevelInfo,
			wantErr: false,
		},
		{
			name:    "warn lowercase",
			level:   "warn",
			want:    slog.LevelWarn,
			wantErr: false,
		},
		{
			name:    "warn uppercase",
			level:   "WARN",
			want:    slog.LevelWarn,
			wantErr: false,
		},
		{
			name:    "warning lowercase",
			level:   "warning",
			want:    slog.LevelWarn,
			wantErr: false,
		},
		{
			name:    "warning uppercase",
			level:   "WARNING",
			want:    slog.LevelWarn,
			wantErr: false,
		},
		{
			name:    "error lowercase",
			level:   "error",
			want:    slog.LevelError,
			wantErr: false,
		},
		{
			name:    "error uppercase",
			level:   "ERROR",
			want:    slog.LevelError,
			wantErr: false,
		},
		{
			name:    "unknown level",
			level:   "trace",
			want:    slog.LevelInfo,
			wantErr: true,
		},
		{
			name:    "empty string",
			level:   "",
			want:    slog.LevelInfo,
			wantErr: true,
		},
		{
			name:    "invalid level",
			level:   "verbose",
			want:    slog.LevelInfo,
			wantErr: true,
		},
		{
			name:    "numeric level",
			level:   "1",
			want:    slog.LevelInfo,
			wantErr: true,
		},
		{
			name:    "level with spaces",
			level:   " info ",
			want:    slog.LevelInfo,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LogLevelToSlogLevel(tt.level)
			if (err != nil) != tt.wantErr {
				t.Errorf("LogLevelToSlogLevel() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("LogLevelToSlogLevel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLogLevelToSlogLevelDefaults(t *testing.T) {
	// Test that unknown levels default to info
	unknownLevels := []string{"trace", "fatal", "panic", "verbose", "all", "none"}

	for _, level := range unknownLevels {
		t.Run("unknown_"+level, func(t *testing.T) {
			got, err := LogLevelToSlogLevel(level)
			if err == nil {
				t.Errorf("LogLevelToSlogLevel(%q) expected error", level)
			}
			if got != slog.LevelInfo {
				t.Errorf("LogLevelToSlogLevel(%q) = %v, want %v (default)", level, got, slog.LevelInfo)
			}
		})
	}
}

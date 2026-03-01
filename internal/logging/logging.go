package logging

import (
	"fmt"
	"log/slog"
	"strings"
)

// LogLevelToSlogLevel converts a string log level to a slog.Level.
// Supported levels: debug, info, warn, error.
func LogLevelToSlogLevel(level string) (slog.Level, error) {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unknown log level: %s", level)
	}
}

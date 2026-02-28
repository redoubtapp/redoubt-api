package telemetry

import (
	"context"
	"fmt"

	"alpineworks.io/ootel"

	"github.com/redoubtapp/redoubt-api/internal/config"
)

// Initialize sets up OpenTelemetry tracing and metrics using ootel.
// It returns a shutdown function that should be called on application exit.
func Initialize(ctx context.Context, cfg config.TelemetryConfig) (func(context.Context) error, error) {
	traceConfig := ootel.NewTraceConfig(
		cfg.Tracing.Enabled,
		cfg.Tracing.SampleRate,
		cfg.ServiceName,
		cfg.ServiceVersion,
	)

	metricConfig := ootel.NewMetricConfig(
		cfg.Metrics.Enabled,
		ootel.ExporterTypePrometheus,
		cfg.Metrics.Port,
	)

	client := ootel.NewOotelClient(
		ootel.WithTraceConfig(traceConfig),
		ootel.WithMetricConfig(metricConfig),
	)

	shutdown, err := client.Init(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize telemetry: %w", err)
	}

	return shutdown, nil
}

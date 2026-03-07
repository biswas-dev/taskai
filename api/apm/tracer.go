// Package apm initialises the OpenTelemetry SDK and exposes a shutdown
// function.  All instrumentation uses the OTel API, making the APM backend
// completely swappable at runtime via environment variables — no code changes
// needed to migrate between providers:
//
//	Provider      OTEL_EXPORTER_OTLP_ENDPOINT          OTEL_EXPORTER_OTLP_HEADERS
//	-----------   ----------------------------------   ---------------------------
//	Datadog       otel-collector:4317                  (none — collector handles auth)
//	New Relic     otlp.nr-data.net:4317                api-key=<ingest-license-key>
//	Honeycomb     api.honeycomb.io:443                 x-honeycomb-team=<api-key>
//	Jaeger        jaeger:4317                          (none)
package apm

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Config holds all configuration needed to connect to an OTLP-compatible
// APM backend.
type Config struct {
	ServiceName string
	Version     string
	Environment string
	// Endpoint is the OTLP gRPC endpoint, e.g. "otel-collector:4317".
	Endpoint string
	Enabled  bool
}

// ConfigFromEnv builds a Config from standard OpenTelemetry environment
// variables, falling back to sensible defaults.
//
// Relevant env vars:
//
//	APM_ENABLED                    "true" to enable (default: disabled)
//	OTEL_SERVICE_NAME              service name tag (default: "taskai")
//	OTEL_SERVICE_VERSION           service version tag (default: defaultVersion)
//	OTEL_ENVIRONMENT               deployment environment tag (default: "development")
//	OTEL_EXPORTER_OTLP_ENDPOINT   gRPC endpoint without scheme (default: "otel-collector:4317")
func ConfigFromEnv(defaultVersion string) Config {
	return Config{
		Enabled:     os.Getenv("APM_ENABLED") == "true",
		ServiceName: envOrDefault("OTEL_SERVICE_NAME", "taskai"),
		Version:     envOrDefault("OTEL_SERVICE_VERSION", defaultVersion),
		Environment: envOrDefault("OTEL_ENVIRONMENT", "development"),
		Endpoint:    envOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT", "otel-collector:4317"),
	}
}

// Init initialises the global OpenTelemetry tracer provider and returns a
// shutdown function that must be deferred in main().
//
// When APM_ENABLED != "true" the global tracer is left as its default noop
// implementation — all instrumentation calls compile and execute at zero cost.
func Init(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	if !cfg.Enabled {
		return noopShutdown, nil
	}

	exp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
		// TLS termination is handled by the collector, not the SDK.
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("apm: create otlp exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.Version),
			attribute.String("deployment.environment", cfg.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("apm: create resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	otel.SetTracerProvider(tp)

	return func(ctx context.Context) error {
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return tp.Shutdown(shutdownCtx)
	}, nil
}

func noopShutdown(_ context.Context) error { return nil }

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

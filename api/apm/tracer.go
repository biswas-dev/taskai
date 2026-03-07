// Package apm initialises the OpenTelemetry SDK — traces + metrics — and
// exposes a single shutdown function.  All instrumentation uses the OTel API,
// making the APM backend completely swappable at runtime via env vars:
//
//	Provider      OTEL_EXPORTER_OTLP_ENDPOINT
//	-----------   ----------------------------------
//	Datadog       otel-collector:4317
//	New Relic     otlp.nr-data.net:4317
//	Honeycomb     api.honeycomb.io:443
//	Jaeger        jaeger:4317
//
// Metrics emitted (when APM_ENABLED=true):
//   - process.runtime.go.goroutines     — live goroutine count
//   - process.runtime.go.gc.count       — GC cycles
//   - process.runtime.go.mem.heap_alloc — heap bytes allocated
//   - process.runtime.go.mem.heap_inuse — heap bytes in use
//   - http.server.request.duration      — HTTP latency histogram (otelhttp)
//   - db.client.connections.*           — PG pool stats (otelsql)
package apm

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Config holds all configuration needed to connect to an OTLP-compatible APM backend.
type Config struct {
	ServiceName string
	Version     string
	Environment string
	// Endpoint is the OTLP gRPC endpoint without scheme, e.g. "otel-collector:4317".
	Endpoint string
	Enabled  bool
}

// ConfigFromEnv builds a Config from standard OpenTelemetry environment variables.
//
//	APM_ENABLED                   "true" to enable (default: disabled)
//	OTEL_SERVICE_NAME             service name tag (default: "taskai")
//	OTEL_SERVICE_VERSION          version tag (default: defaultVersion)
//	OTEL_ENVIRONMENT              deployment env tag (default: "development")
//	OTEL_EXPORTER_OTLP_ENDPOINT  gRPC endpoint (default: "otel-collector:4317")
func ConfigFromEnv(defaultVersion string) Config {
	return Config{
		Enabled:     os.Getenv("APM_ENABLED") == "true",
		ServiceName: envOrDefault("OTEL_SERVICE_NAME", "taskai"),
		Version:     envOrDefault("OTEL_SERVICE_VERSION", defaultVersion),
		Environment: envOrDefault("OTEL_ENVIRONMENT", "development"),
		Endpoint:    envOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT", "otel-collector:4317"),
	}
}

// Init initialises the global OpenTelemetry TracerProvider and MeterProvider,
// starts Go runtime metrics collection, and returns a combined shutdown function.
//
// When APM_ENABLED != "true" all providers remain as their default noop
// implementations — zero overhead.
func Init(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	if !cfg.Enabled {
		return noopShutdown, nil
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

	// ── Traces ──────────────────────────────────────────────────────────────
	traceExp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("apm: create trace exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)

	// ── Metrics ─────────────────────────────────────────────────────────────
	metricExp, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(cfg.Endpoint),
		otlpmetricgrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("apm: create metric exporter: %w", err)
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp,
			sdkmetric.WithInterval(15*time.Second),
		)),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	// ── Go runtime metrics ───────────────────────────────────────────────────
	// Emits goroutines, GC duration/count, heap alloc/inuse every 15 s.
	if err := runtime.Start(
		runtime.WithMinimumReadMemStatsInterval(15 * time.Second),
	); err != nil {
		return nil, fmt.Errorf("apm: start runtime metrics: %w", err)
	}

	return func(ctx context.Context) error {
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := tp.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return mp.Shutdown(shutdownCtx)
	}, nil
}

func noopShutdown(_ context.Context) error { return nil }

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

package telemetryotel

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
)

// Mode describes how a node participates in telemetry topology.
//
// Edge nodes send telemetry to VM over the WireGuard tunnel.
// VM nodes send both local and aggregated Edge telemetry to a SaaS/Grafana stack.
type Mode string

const (
	// ModeEdge means this process is running on an Edge appliance.
	// Typically configured to send OTLP data to the VM.
	ModeEdge Mode = "edge"

	// ModeVM means this process is running on a VM (gateway).
	// Typically configured to send OTLP data to a SaaS / Grafana stack.
	ModeVM Mode = "vm"
)

// Config is a minimal, transport-agnostic configuration for telemetry initialisation.
//
// Mapping from YAML / env is done in the Edge and VM config packages; this package
// stays shared and dumb.
type Config struct {
	// ServiceName is the logical service identifier (e.g. "vm-orchestrator", "edge-orchestrator").
	ServiceName string

	// Environment (e.g. "dev", "staging", "prod").
	Environment string

	// Mode controls high-level semantics (VM vs Edge). It is NOT used to change
	// transport behaviour; both sides still speak OTLP to a collector endpoint.
	Mode Mode

	// OTLPEndpoint is the OpenTelemetry collector HTTP endpoint.
	// Example (Edge -> VM over WG): "http://vm-wireguard-ip:4318"
	// Example (VM -> SaaS): "http://grafana-agent:4318"
	OTLPEndpoint string

	// Insecure controls whether the OTLP HTTP connection uses TLS.
	// For WireGuard-internal or local deployments this can be true.
	Insecure bool

	// Timeout is the max time for exporter startup and shutdown.
	// If zero, a sensible default is used.
	Timeout time.Duration

	// ResourceAttributes adds additional attributes to every span/metric.
	// Keys should use OpenTelemetry semantic conventions where possible.
	ResourceAttributes map[string]string
}

// Provider holds OpenTelemetry SDK state and exposes helpers to application code.
type Provider struct {
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
}

// Init sets up global OpenTelemetry providers (traces + metrics) backed by OTLP HTTP exporters.
//
// Typical usage:
//
//	// in main():
//	ctx := context.Background()
//	tp, err := telemetryotel.Init(ctx, telemetryotel.Config{...})
//	if err != nil { ... }
//	defer tp.Shutdown(context.Background())
//
//	// elsewhere:
//	tr := telemetryotel.Tracer("edge-orchestrator")
//	m := telemetryotel.Meter("edge-orchestrator")
func Init(ctx context.Context, cfg Config) (*Provider, error) {
	if cfg.ServiceName == "" {
		return nil, fmt.Errorf("telemetry: ServiceName is required")
	}
	if cfg.OTLPEndpoint == "" {
		return nil, fmt.Errorf("telemetry: OTLPEndpoint is required (HTTP OTLP endpoint, e.g. http://host:4318)")
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	initCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Common options for HTTP exporters.
	traceOpts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(cfg.OTLPEndpoint),
	}
	metricOpts := []otlpmetrichttp.Option{
		otlpmetrichttp.WithEndpoint(cfg.OTLPEndpoint),
	}
	if cfg.Insecure {
		traceOpts = append(traceOpts, otlptracehttp.WithInsecure())
		metricOpts = append(metricOpts, otlpmetrichttp.WithInsecure())
	}

	// Trace exporter (HTTP OTLP).
	traceExporter, err := otlptracehttp.New(initCtx, traceOpts...)
	if err != nil {
		return nil, fmt.Errorf("telemetry: failed to create OTLP HTTP trace exporter: %w", err)
	}

	// Metric exporter (HTTP OTLP).
	metricExporter, err := otlpmetrichttp.New(initCtx, metricOpts...)
	if err != nil {
		_ = traceExporter.Shutdown(initCtx)
		return nil, fmt.Errorf("telemetry: failed to create OTLP HTTP metric exporter: %w", err)
	}

	res, err := buildResource(cfg)
	if err != nil {
		_ = metricExporter.Shutdown(initCtx)
		_ = traceExporter.Shutdown(initCtx)
		return nil, err
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(
			sdkmetric.NewPeriodicReader(
				metricExporter,
				sdkmetric.WithInterval(15*time.Second),
			),
		),
		sdkmetric.WithResource(res),
	)

	// Install as globals so that libraries can use them implicitly.
	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)

	return &Provider{
		tracerProvider: tracerProvider,
		meterProvider:  meterProvider,
	}, nil
}

// Shutdown flushes and releases OpenTelemetry resources created by Init.
func (p *Provider) Shutdown(ctx context.Context) error {
	if p == nil {
		return nil
	}

	var firstErr error

	// Shut down trace provider (flushes spans and exporter).
	if p.tracerProvider != nil {
		if err := p.tracerProvider.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("telemetry: tracer provider shutdown: %w", err)
		}
	}

	// Shut down metric provider (flushes metrics and exporter).
	if p.meterProvider != nil {
		if err := p.meterProvider.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("telemetry: meter provider shutdown: %w", err)
		}
	}

	return firstErr
}

// Tracer is a convenience wrapper around the global tracer provider.
func Tracer(instrumentationName string) trace.Tracer {
	return otel.Tracer(instrumentationName)
}

// Meter is a convenience wrapper around the global meter provider.
func Meter(instrumentationName string, opts ...metric.MeterOption) metric.Meter {
	return otel.Meter(instrumentationName, opts...)
}

// buildResource constructs the OpenTelemetry resource describing this process.
func buildResource(cfg Config) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{
		semconv.ServiceNameKey.String(cfg.ServiceName),
	}

	if cfg.Environment != "" {
		attrs = append(attrs, attribute.String("deployment.environment", cfg.Environment))
	}

	if cfg.Mode != "" {
		attrs = append(attrs, attribute.String("service.role", string(cfg.Mode)))
	}

	for k, v := range cfg.ResourceAttributes {
		if k == "" {
			continue
		}
		attrs = append(attrs, attribute.String(k, v))
	}

	// Merge default attributes (host, OS, etc.) with our custom ones.
	base, err := resource.New(context.Background(),
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithOS(),
		resource.WithHost(),
	)
	if err != nil {
		return nil, fmt.Errorf("telemetry: failed to build base resource: %w", err)
	}

	custom := resource.NewWithAttributes(semconv.SchemaURL, attrs...)
	res, err := resource.Merge(base, custom)
	if err != nil {
		return nil, fmt.Errorf("telemetry: failed to merge resources: %w", err)
	}
	return res, nil
}


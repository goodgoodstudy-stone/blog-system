package observ

import (
	"context"
	"fmt"
	"log/slog"
	"math"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// InitTracer creates request trace IDs even when no collector is configured.
// An empty endpoint disables export, which keeps the default Compose stack quiet.
func InitTracer(ctx context.Context, endpoint string, sampleRatio float64) (func(context.Context) error, error) {
	if math.IsNaN(sampleRatio) || math.IsInf(sampleRatio, 0) || sampleRatio < 0 || sampleRatio > 1 {
		return nil, fmt.Errorf("trace sample ratio must be between 0 and 1")
	}
	options := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName("blog-server"))),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(sampleRatio))),
	}
	if endpoint != "" {
		otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
			// Collector/export failures are infrastructure warnings, independent
			// of request success; they still increment the shared log counter.
			slog.Warn("telemetry export failed", "event", "telemetry_export", "error", err)
		}))
		exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpoint(endpoint), otlptracehttp.WithInsecure())
		if err != nil {
			return nil, fmt.Errorf("create OTLP exporter: %w", err)
		}
		options = append(options, sdktrace.WithBatcher(exporter))
	}
	provider := sdktrace.NewTracerProvider(options...)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return provider.Shutdown, nil
}

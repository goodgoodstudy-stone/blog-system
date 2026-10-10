package observ

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"
)

type requestIDKey struct{}

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// LogHandler counts emitted warning/error records, including derived loggers.
// IDs and messages remain in logs and never become metric labels.
type LogHandler struct {
	next   slog.Handler
	counts *prometheus.CounterVec
	bound  map[string]bool
	event  string
}

func NewLogHandler(next slog.Handler, registerer prometheus.Registerer) slog.Handler {
	counts := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "blog_log_entries_total",
		Help: "Warning and error log records emitted by the API process.",
	}, []string{"level", "event"})
	registerer.MustRegister(counts)
	for _, event := range logEvents {
		counts.WithLabelValues("warn", event)
		counts.WithLabelValues("error", event)
	}
	return &LogHandler{next: next, counts: counts}
}

func (h *LogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *LogHandler) Handle(ctx context.Context, record slog.Record) error {
	record = record.Clone()
	seen := make(map[string]bool, len(h.bound)+3)
	for key := range h.bound {
		seen[key] = true
	}
	record.Attrs(func(attr slog.Attr) bool { seen[attr.Key] = true; return true })
	if id, _ := ctx.Value(requestIDKey{}).(string); id != "" && !seen["requestId"] {
		record.AddAttrs(slog.String("requestId", id))
	}
	span := trace.SpanContextFromContext(ctx)
	if span.IsValid() {
		if !seen["traceId"] {
			record.AddAttrs(slog.String("traceId", span.TraceID().String()))
		}
		if !seen["spanId"] {
			record.AddAttrs(slog.String("spanId", span.SpanID().String()))
		}
	}
	if err := h.next.Handle(ctx, record); err != nil {
		return err
	}
	event := h.event
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == "event" {
			event = attr.Value.String()
		}
		return true
	})
	event = boundedLogEvent(event)
	switch {
	case record.Level >= slog.LevelError:
		h.counts.WithLabelValues("error", event).Inc()
	case record.Level >= slog.LevelWarn:
		h.counts.WithLabelValues("warn", event).Inc()
	}
	return nil
}

func (h *LogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	bound := make(map[string]bool, len(h.bound)+len(attrs))
	for key := range h.bound {
		bound[key] = true
	}
	event := h.event
	for _, attr := range attrs {
		bound[attr.Key] = true
		if attr.Key == "event" {
			event = attr.Value.String()
		}
	}
	return &LogHandler{next: h.next.WithAttrs(attrs), counts: h.counts, bound: bound, event: event}
}

func (h *LogHandler) WithGroup(name string) slog.Handler {
	return &LogHandler{next: h.next.WithGroup(name), counts: h.counts, bound: h.bound, event: h.event}
}

func FailureLevel(err error) slog.Level {
	var networkError net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) ||
		errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) ||
		(errors.As(err, &networkError) && networkError.Timeout()) {
		return slog.LevelWarn
	}
	return slog.LevelError
}

// LogFailure is used where an operation's error is handled, rather than logging
// the same propagated error at every layer.
func LogFailure(ctx context.Context, message string, err error, attrs ...any) {
	slog.Log(ctx, FailureLevel(err), message, append(attrs, "error", err)...)
}

// Only fixed event names may partition counters. Caller-supplied messages,
// SQL statements, IDs and arbitrary event values collapse to "other".
var logEvents = []string{
	"other", "json_response_encode", "json_request_decode", "invalid_request_json", "panic",
	"request_failure", "downstream_timeout", "request_canceled", "database_readiness",
	"telemetry_export", "response_write", "upload_write", "upload_cleanup", "maintenance", "tracer_shutdown",
}

func boundedLogEvent(event string) string {
	for _, allowed := range logEvents {
		if event == allowed {
			return event
		}
	}
	return "other"
}

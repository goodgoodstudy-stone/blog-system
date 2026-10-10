package observ

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"syscall"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"
)

func logCounts(t *testing.T, registry *prometheus.Registry) map[string]float64 {
	t.Helper()
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]float64{}
	for _, family := range families {
		if family.GetName() != "blog_log_entries_total" {
			continue
		}
		for _, metric := range family.Metric {
			if len(metric.Label) != 2 || metric.Label[0].GetName() != "event" || metric.Label[1].GetName() != "level" {
				t.Fatalf("unbounded metric labels: %v", metric.Label)
			}
			counts[metric.Label[1].GetValue()] += metric.Counter.GetValue()
		}
	}
	return counts
}

func TestLogHandlerSharesCountersAcrossConcurrentDerivedLoggers(t *testing.T) {
	registry := prometheus.NewRegistry()
	logger := slog.New(NewLogHandler(slog.NewJSONHandler(io.Discard, nil), registry))
	logger.Info("ordinary request")
	logger.Debug("filtered")
	var workers sync.WaitGroup
	for range 50 {
		workers.Go(func() {
			child := logger.With("operation", "read").WithGroup("details")
			child.Warn("timeout", "traceId", "never-a-metric-label")
			child.Error("unexpected data")
		})
	}
	workers.Wait()
	counts := logCounts(t, registry)
	if counts["warn"] != 50 || counts["error"] != 50 {
		t.Fatalf("log counts = %v", counts)
	}
}

func TestLogHandlerPreservesCorrelationAndRespectsFiltering(t *testing.T) {
	var output bytes.Buffer
	registry := prometheus.NewRegistry()
	logger := slog.New(NewLogHandler(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelError}), registry))
	traceID, _ := trace.TraceIDFromHex("0123456789abcdef0123456789abcdef")
	spanID, _ := trace.SpanIDFromHex("0123456789abcdef")
	ctx := trace.ContextWithSpanContext(WithRequestID(t.Context(), "request-1"), trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID}))
	logger.WarnContext(ctx, "filtered warning")
	logger.With("requestId", "bound-request").ErrorContext(ctx, "JSON encode failed")
	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["traceId"] != traceID.String() || entry["spanId"] != spanID.String() || entry["requestId"] != "bound-request" {
		t.Fatalf("correlation = %v", entry)
	}
	counts := logCounts(t, registry)
	if counts["warn"] != 0 || counts["error"] != 1 {
		t.Fatalf("filtered log counted: %v", counts)
	}
}

func TestFailureLevelRecognizesWrappedTimeouts(t *testing.T) {
	for _, err := range []error{
		fmt.Errorf("query: %w", context.DeadlineExceeded),
		fmt.Errorf("client: %w", context.Canceled),
		&net.OpError{Op: "read", Net: "tcp", Err: context.DeadlineExceeded},
		fmt.Errorf("write: %w", syscall.EPIPE),
	} {
		if level := FailureLevel(err); level != slog.LevelWarn {
			t.Fatalf("%v: level = %v", err, level)
		}
	}
	if level := FailureLevel(errors.New("invalid internal JSON")); level != slog.LevelError {
		t.Fatalf("unexpected error: level = %v", level)
	}
}

func TestLogEventsStayBoundedAndPreserveCategories(t *testing.T) {
	registry := prometheus.NewRegistry()
	logger := slog.New(NewLogHandler(slog.NewJSONHandler(io.Discard, nil), registry))
	logger.With("event", "json_response_encode").Error("internal JSON")
	logger.Warn("query timeout", "event", "downstream_timeout")
	logger.Error("arbitrary label", "event", "user-supplied-ID")
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]float64{}
	for _, family := range families {
		for _, metric := range family.Metric {
			event, level := "", ""
			for _, label := range metric.Label {
				if label.GetName() == "event" {
					event = label.GetValue()
				}
				if label.GetName() == "level" {
					level = label.GetValue()
				}
			}
			found[level+"/"+event] = metric.Counter.GetValue()
		}
	}
	if found["error/json_response_encode"] != 1 || found["warn/downstream_timeout"] != 1 || found["error/other"] != 1 {
		t.Fatalf("event categories = %v", found)
	}
	if _, leaked := found["error/user-supplied-ID"]; leaked {
		t.Fatal("unbounded event leaked into metrics")
	}
}

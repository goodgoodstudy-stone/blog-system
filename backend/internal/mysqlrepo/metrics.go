package mysqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"
)

var downstreamRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "blog_downstream_requests_total", Help: "Completed downstream operations, including result reading.",
}, []string{"dependency", "operation", "outcome"})
var downstreamDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name: "blog_downstream_duration_seconds", Help: "Downstream operation duration, including result reading.", Buckets: prometheus.DefBuckets,
}, []string{"dependency", "operation"})

func init() {
	prometheus.MustRegister(downstreamRequests, downstreamDuration)
	for _, operation := range []string{"query", "exec", "begin", "commit", "rollback"} {
		downstreamRequests.WithLabelValues("mysql", operation, "success")
		downstreamRequests.WithLabelValues("mysql", operation, "failure")
		downstreamDuration.WithLabelValues("mysql", operation)
	}
}

type downstreamCall struct {
	ctx              context.Context
	start            time.Time
	id               uint64
	operation, query string
	args             []any
	raw              bool
	once             sync.Once
	rows             int
	err              error
}

func newDownstreamCall(ctx context.Context, operation, query string, raw bool, args []any) *downstreamCall {
	return &downstreamCall{ctx: ctx, start: time.Now(), id: querySequence.Add(1), operation: operation, query: query, raw: raw, args: args}
}

func (c *downstreamCall) finish(err error, attrs ...any) {
	c.once.Do(func() {
		outcome := "success"
		// An empty result is a successful SQL read, not a downstream failure.
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			outcome = "failure"
		}
		duration := time.Since(c.start)
		downstreamRequests.WithLabelValues("mysql", c.operation, outcome).Inc()
		observer := downstreamDuration.WithLabelValues("mysql", c.operation)
		span := trace.SpanContextFromContext(c.ctx)
		if exemplar, ok := observer.(prometheus.ExemplarObserver); ok && span.IsSampled() {
			exemplar.ObserveWithExemplar(duration.Seconds(), prometheus.Labels{"trace_id": span.TraceID().String()})
		} else {
			observer.Observe(duration.Seconds())
		}
		fields := []any{"event", "downstream_request", "dependency", "mysql", "operation", c.operation, "queryId", c.id,
			"success", outcome == "success", "outcome", outcome, "durationMs", float64(duration.Microseconds()) / 1000, "rows", c.rows}
		if c.raw {
			fields = append(fields, "sql", c.query, "args", c.args)
			fields = append(fields, attrs...)
		}
		if errors.Is(err, sql.ErrNoRows) {
			fields = append(fields, "empty", true)
		} else if err != nil {
			fields = append(fields, "error", err)
		}
		// The owning HTTP/background boundary logs failures at WARN/ERROR.
		// This per-operation access record is INFO to avoid counting a single
		// propagated failure again in every layer.
		slog.InfoContext(c.ctx, "downstream request", fields...)
	})
}

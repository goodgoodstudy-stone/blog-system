package bloghttp

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"blog-system/backend/internal/mysqlrepo"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestRequestObservabilityUsesRouteAndTraceID(t *testing.T) {
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	t.Cleanup(func() { _ = provider.Shutdown(t.Context()) })
	handler := otelhttp.NewHandler(New(mysqlrepo.Store{}, "", "http://localhost:8080", "", false), "http.server",
		otelhttp.WithTracerProvider(provider), otelhttp.WithPropagators(propagation.TraceContext{}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/articles/not-a-number", nil)
	const traceID = "0123456789abcdef0123456789abcdef"
	request.Header.Set("traceparent", "00-"+traceID+"-0123456789abcdef-01")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.Code)
	}
	requestID := response.Header().Get("X-Request-ID")
	if requestID == "" {
		t.Fatal("missing X-Request-ID")
	}
	var entry map[string]any
	if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
		t.Fatalf("decode request log: %v", err)
	}
	if entry["requestId"] != requestID || entry["traceId"] != traceID || entry["route"] != "/api/v1/articles/{id}" {
		t.Fatalf("unexpected correlation fields: %#v", entry)
	}
	publicMetrics := httptest.NewRecorder()
	handler.ServeHTTP(publicMetrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if publicMetrics.Code != http.StatusNotFound {
		t.Fatalf("public /metrics status = %d, want 404", publicMetrics.Code)
	}

	metrics := httptest.NewRecorder()
	metricsRequest := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRequest.Header.Set("Accept", "application/openmetrics-text; version=1.0.0")
	promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{EnableOpenMetrics: true}).ServeHTTP(metrics, metricsRequest)
	if !strings.Contains(metrics.Body.String(), `http_requests_total{method="GET",route="/api/v1/articles/{id}",status="404"}`) {
		t.Fatal("request counter missing route template")
	}
	if strings.Contains(metrics.Body.String(), "not-a-number") {
		t.Fatal("raw path leaked into metric labels")
	}
	if !strings.Contains(metrics.Body.String(), `# {trace_id="`+traceID+`"}`) {
		t.Fatal("latency histogram missing trace exemplar")
	}
	if !strings.Contains(metrics.Body.String(), `http_requests_total{method="GET",route="unmatched",status="404"}`) {
		t.Fatal("unmatched request missing bounded route label")
	}
}

func TestServerErrorLogIncludesRequestID(t *testing.T) {
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	request := httptest.NewRequest(http.MethodGet, "/api/v1/tags", nil)
	response := httptest.NewRecorder()
	response.Header().Set("X-Request-ID", "test-request")
	failErr(response, request, errors.New("database unavailable"))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", response.Code)
	}
	var entry map[string]any
	if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
		t.Fatalf("decode error log: %v", err)
	}
	if entry["requestId"] != "test-request" {
		t.Fatalf("requestId = %v", entry["requestId"])
	}
}

func TestLocalRawDebugLogsRequestAndResponseBodies(t *testing.T) {
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	body := `{"email":"demo@example.com","password":`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	request.Header.Set("Origin", "http://localhost:8080")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	New(mysqlrepo.Store{}, "", "http://localhost:8080", "", true).ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}

	var requestLogged, responseLogged bool
	for _, line := range bytes.Split(bytes.TrimSpace(logs.Bytes()), []byte("\n")) {
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("decode log: %v", err)
		}
		switch entry["msg"] {
		case "http request body":
			requestLogged = entry["body"] == body
		case "http response body":
			responseLogged = entry["body"] == response.Body.String()
		}
	}
	if !requestLogged || !responseLogged {
		t.Fatalf("body logs missing: request=%t response=%t", requestLogged, responseLogged)
	}
}

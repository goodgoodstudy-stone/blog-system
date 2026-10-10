package observ

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	coltrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

func TestInitTracerExportsOTLP(t *testing.T) {
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	})
	received := make(chan []byte, 1)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/traces" {
			t.Errorf("export path = %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read export: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		received <- body
		w.Header().Set("Content-Type", "application/x-protobuf")
	}))
	defer collector.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	shutdown, err := InitTracer(ctx, strings.TrimPrefix(collector.URL, "http://"), 1)
	if err != nil {
		t.Fatalf("init tracer: %v", err)
	}
	_, span := otel.Tracer("test").Start(ctx, "test.operation")
	span.End()
	if err := shutdown(ctx); err != nil {
		t.Fatalf("shutdown tracer: %v", err)
	}
	select {
	case body := <-received:
		var request coltrace.ExportTraceServiceRequest
		if err := proto.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode OTLP export: %v", err)
		}
		if len(request.ResourceSpans) != 1 || len(request.ResourceSpans[0].ScopeSpans) != 1 || len(request.ResourceSpans[0].ScopeSpans[0].Spans) != 1 {
			t.Fatalf("unexpected span structure: %#v", request.ResourceSpans)
		}
		if name := request.ResourceSpans[0].ScopeSpans[0].Spans[0].Name; name != "test.operation" {
			t.Fatalf("span name = %q", name)
		}
	default:
		t.Fatal("collector did not receive a trace")
	}
}

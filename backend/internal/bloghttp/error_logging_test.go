package bloghttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"blog-system/backend/internal/mysqlrepo"
	"blog-system/backend/internal/observ"
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
)

func captureErrorLogs(t *testing.T) (*bytes.Buffer, *prometheus.Registry) {
	t.Helper()
	output := new(bytes.Buffer)
	registry := prometheus.NewRegistry()
	previous := slog.Default()
	slog.SetDefault(slog.New(observ.NewLogHandler(slog.NewJSONHandler(output, nil), registry)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return output, registry
}

func assertLogCounts(t *testing.T, registry *prometheus.Registry, warn, failure float64) {
	t.Helper()
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]float64{}
	for _, f := range families {
		for _, m := range f.Metric {
			for _, label := range m.Label {
				if label.GetName() == "level" {
					counts[label.GetValue()] += m.Counter.GetValue()
				}
			}
		}
	}
	if counts["warn"] != warn || counts["error"] != failure {
		t.Fatalf("log counts = %v, want warn=%v error=%v", counts, warn, failure)
	}
}

func TestErrorClassificationAndCounters(t *testing.T) {
	for _, tt := range []struct {
		name          string
		err           error
		status        int
		warn, failure float64
	}{
		{"internal", errors.New("unexpected database failure"), 500, 0, 1},
		{"wrapped deadline", fmt.Errorf("mysql: %w", context.DeadlineExceeded), 504, 1, 0},
		{"network timeout", &net.OpError{Op: "read", Net: "tcp", Err: context.DeadlineExceeded}, 504, 1, 0},
		{"client canceled", context.Canceled, 408, 1, 0},
		{"not found", mysqlrepo.ErrNotFound, 404, 0, 0},
		{"conflict", mysqlrepo.ErrConflict, 409, 0, 0},
		{"forbidden", mysqlrepo.ErrForbidden, 403, 0, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, registry := captureErrorLogs(t)
			r := httptest.NewRequest(http.MethodGet, "/api/v1/tags", nil)
			w := httptest.NewRecorder()
			failErr(w, r, tt.err)
			if w.Code != tt.status {
				t.Fatalf("status = %d, want %d", w.Code, tt.status)
			}
			assertLogCounts(t, registry, tt.warn, tt.failure)
		})
	}
}

func TestJSONFailuresAreClassifiedBySource(t *testing.T) {
	for _, body := range []string{`{"name":`, `{"name":1}`, `{"unknown":true}`, `{} {}`, ``} {
		t.Run(body, func(t *testing.T) {
			_, registry := captureErrorLogs(t)
			r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
			var input struct{ Name string }
			if err := decode(r, &input); err == nil {
				t.Fatal("invalid request accepted")
			}
			assertLogCounts(t, registry, 1, 0)
		})
	}
	t.Run("invalid internal target", func(t *testing.T) {
		_, registry := captureErrorLogs(t)
		r := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{}`))
		w := httptest.NewRecorder()
		if decodeRequest(w, r, nil) {
			t.Fatal("invalid destination accepted")
		}
		if w.Code != 500 {
			t.Fatalf("internal decode failure status = %d, want 500", w.Code)
		}
		assertLogCounts(t, registry, 0, 1)
	})
	t.Run("response encode", func(t *testing.T) {
		output, registry := captureErrorLogs(t)
		r := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		respond(w, r, 200, map[string]any{"invalid": make(chan int)})
		if w.Code != 500 || !json.Valid(w.Body.Bytes()) || !strings.Contains(output.String(), "json_response_encode") {
			t.Fatalf("response = %d %s, logs = %s", w.Code, w.Body.String(), output.String())
		}
		assertLogCounts(t, registry, 0, 1)
	})
}

func TestPanicRecoveryEmitsErrorAndKeepsRequestMetrics(t *testing.T) {
	output, registry := captureErrorLogs(t)
	s := &Server{}
	h := s.logRequests(recoverRequests(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("unexpected invariant") })))
	// logRequests expects the same chi context installed by the router.
	r := httptest.NewRequest(http.MethodGet, "/panic", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, chi.NewRouteContext()))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 500 || !strings.Contains(output.String(), "request panic") || !strings.Contains(output.String(), "http request") {
		t.Fatalf("panic not observed: status=%d logs=%s", w.Code, output.String())
	}
	assertLogCounts(t, registry, 0, 1)
}

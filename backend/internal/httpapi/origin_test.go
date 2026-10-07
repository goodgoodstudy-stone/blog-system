package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOriginAllowlist(t *testing.T) {
	s := &Server{Origins: trustedOrigins("http://localhost:8080", " https://blog.example.test, http://127.0.0.1:8080 ")}
	handler := s.checkOrigin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, tc := range []struct {
		name   string
		method string
		origin string
		status int
	}{
		{"primary", http.MethodPost, "http://localhost:8080", http.StatusNoContent},
		{"additional", http.MethodPost, "https://blog.example.test", http.StatusNoContent},
		{"second additional", http.MethodPost, "http://127.0.0.1:8080", http.StatusNoContent},
		{"unknown", http.MethodPost, "https://example.com", http.StatusForbidden},
		{"missing", http.MethodPost, "", http.StatusForbidden},
		{"read", http.MethodGet, "https://example.com", http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/api/v1/auth/register", nil)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("status = %d, want %d", w.Code, tc.status)
			}
		})
	}
}

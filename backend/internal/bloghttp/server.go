package bloghttp

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"blog-system/backend/internal/application"
	"blog-system/backend/internal/auth"
	"blog-system/backend/internal/mysqlrepo"
	"blog-system/backend/internal/observ"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-sql-driver/mysql"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"
)

type Server struct {
	Store      mysqlrepo.Store
	App        application.Service
	LoginLimit *loginLimiter
	UploadDir  string
	Origins    map[string]struct{}
	DebugRaw   bool
}
type userKey struct{}

func New(s mysqlrepo.Store, uploadDir, origin, additionalOrigins string, debugRaw bool) http.Handler {
	server := &Server{Store: s, App: application.Service{Articles: s, Social: s}, LoginLimit: newLoginLimiter(), UploadDir: uploadDir, Origins: trustedOrigins(origin, additionalOrigins), DebugRaw: debugRaw}
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, server.logRequests, recoverRequests, server.authContext, server.checkOrigin)
	r.Get("/api/v1/health/live", func(w http.ResponseWriter, r *http.Request) { respond(w, r, 200, map[string]bool{"ok": true}) })
	r.Get("/api/v1/health/ready", server.ready)
	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/register", server.register)
		r.Post("/auth/login", server.login)
		r.Post("/auth/logout", server.logout)
		r.Get("/auth/me", server.me)
		r.Get("/articles", server.publicArticles)
		r.Get("/articles/{id}", server.publicArticle)
		r.Get("/articles/{id}/comments", server.publicComments)
		r.Get("/tags", server.tags)
		r.Get("/images/{id}", server.image)
		r.Get("/favorites", server.favorites)
		r.Put("/favorites/{id}", server.addFavorite)
		r.Delete("/favorites/{id}", server.deleteFavorite)
		r.Post("/articles/{id}/comments", server.createComment)
		r.Delete("/comments/{id}", server.deleteComment)
		r.Get("/manage/articles", server.manageArticles)
		r.Post("/manage/articles", server.saveArticle)
		r.Get("/manage/articles/{id}", server.manageArticle)
		r.Patch("/manage/articles/{id}", server.saveArticle)
		r.Get("/manage/articles/{id}/preview", server.manageArticle)
		r.Post("/manage/articles/{id}/publish", server.publish)
		r.Post("/manage/articles/{id}/unpublish", server.unpublish)
		r.Delete("/manage/articles/{id}", server.deleteArticle)
		r.Get("/manage/trash", server.trash)
		r.Post("/manage/articles/{id}/restore", server.restore)
		r.Post("/manage/images", server.uploadImage)
		r.Get("/manage/comments", server.manageComments)
		r.Get("/admin/users", server.users)
		r.Patch("/admin/users/{id}/role", server.role)
		r.Post("/admin/tags", server.createTag)
		r.Patch("/admin/tags/{id}", server.updateTag)
		r.Delete("/admin/tags/{id}", server.deleteTag)
	})
	return r
}

func respond(w http.ResponseWriter, r *http.Request, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		logRequestFailure(w, r, "response JSON encoding failed", err, "json_response_encode")
		status = http.StatusInternalServerError
		// This fixed payload cannot fail JSON encoding, and is written before
		// headers so an encoding failure never leaves a successful status.
		body, _ = json.Marshal(map[string]string{"code": "internal", "message": "响应生成失败", "requestId": w.Header().Get("X-Request-ID")})
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write(append(body, '\n')); err != nil {
		logRequestFailure(w, r, "response write failed", err, "response_write")
	}
}
func fail(w http.ResponseWriter, r *http.Request, status int, code, msg string) {
	respond(w, r, status, map[string]any{"code": code, "message": msg, "requestId": w.Header().Get("X-Request-ID")})
}
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		id := middleware.GetReqID(r.Context())
		r = r.WithContext(observ.WithRequestID(r.Context(), id))
		w.Header().Set("X-Request-ID", id)
		wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		var requestBody captureRequestBody
		var responseBody captureResponseBody
		if s.DebugRaw {
			if isTextBody(r.Header.Get("Content-Type")) && r.Body != nil {
				requestBody.ReadCloser = r.Body
				r.Body = &requestBody
			}
			responseBody.header = wrapped.Header()
			wrapped.Tee(&responseBody)
		}
		next.ServeHTTP(wrapped, r)
		route := chi.RouteContext(r.Context()).RoutePattern()
		if route == "" {
			route = "unmatched"
		}
		status := wrapped.Status()
		if status == 0 {
			status = http.StatusOK
		}
		duration := time.Since(started)
		httpRequests.WithLabelValues(r.Method, route, strconv.Itoa(status)).Inc()
		spanContext := trace.SpanContextFromContext(r.Context())
		traceID := ""
		if spanContext.IsValid() {
			traceID = spanContext.TraceID().String()
		}
		observer := httpDuration.WithLabelValues(r.Method, route)
		if exemplarObserver, ok := observer.(prometheus.ExemplarObserver); ok && spanContext.IsSampled() {
			exemplarObserver.ObserveWithExemplar(duration.Seconds(), prometheus.Labels{"trace_id": traceID})
		} else {
			observer.Observe(duration.Seconds())
		}
		trace.SpanFromContext(r.Context()).SetName(r.Method + " " + route)
		if route == "/api/v1/health/live" || route == "/api/v1/health/ready" {
			return
		}
		slog.InfoContext(r.Context(), "http request", "requestId", id, "traceId", traceID, "spanId", spanContext.SpanID().String(), "method", r.Method, "route", route, "query", r.URL.RawQuery, "status", status, "success", status < 400, "durationMs", duration.Milliseconds())
		if s.DebugRaw {
			logBodyChunks(r.Context(), id, "http request body", requestBody.Bytes())
			logBodyChunks(r.Context(), id, "http response body", responseBody.Bytes())
		}
	})
}
func failErr(w http.ResponseWriter, r *http.Request, err error) {
	var my *mysql.MySQLError
	switch {
	case errors.Is(err, mysqlrepo.ErrNotFound):
		fail(w, r, 404, "not_found", "内容不存在或暂不可访问")
	case errors.Is(err, mysqlrepo.ErrForbidden):
		fail(w, r, 403, "forbidden", "没有操作权限")
	case errors.Is(err, mysqlrepo.ErrConflict):
		fail(w, r, 409, "conflict", "内容已变化，请刷新后重试")
	case errors.As(err, &my) && my.Number == 1062:
		fail(w, r, 409, "duplicate", "内容已存在")
	case errors.As(err, &my) && my.Number == 1452:
		fail(w, r, 400, "invalid_reference", "所选内容不存在")
	default:
		var timeout net.Error
		event := "request_failure"
		if errors.Is(err, context.Canceled) {
			event = "request_canceled"
		} else if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()) {
			event = "downstream_timeout"
		}
		logRequestFailure(w, r, "request failed", err, event)
		switch {
		case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()):
			fail(w, r, http.StatusGatewayTimeout, "downstream_timeout", "下游服务超时，请稍后重试")
		case errors.Is(err, context.Canceled):
			fail(w, r, http.StatusRequestTimeout, "request_canceled", "请求已取消")
		default:
			fail(w, r, 500, "internal", "操作失败，请稍后重试")
		}
	}
}

func logRequestFailure(w http.ResponseWriter, r *http.Request, message string, err error, event string) {
	span := trace.SpanContextFromContext(r.Context())
	attrs := []any{"event", event, "requestId", w.Header().Get("X-Request-ID"), "method", r.Method}
	if span.IsValid() {
		attrs = append(attrs, "traceId", span.TraceID().String(), "spanId", span.SpanID().String())
	}
	if routeContext := chi.RouteContext(r.Context()); routeContext != nil {
		attrs = append(attrs, "route", routeContext.RoutePattern())
	}
	trace.SpanFromContext(r.Context()).RecordError(err)
	if event == "json_response_encode" || event == "panic" {
		slog.ErrorContext(r.Context(), message, append(attrs, "error", err)...)
	} else {
		observ.LogFailure(r.Context(), message, err, attrs...)
	}
}

func decode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 2<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	err := d.Decode(v)
	if err == nil {
		var extra any
		if err = d.Decode(&extra); errors.Is(err, io.EOF) {
			return nil
		} else if err == nil {
			err = errors.New("request body contains multiple JSON values")
		}
	}
	level := observ.FailureLevel(err)
	event := "json_request_decode"
	if invalidRequestJSON(err) {
		level = slog.LevelWarn
		event = "invalid_request_json"
	}
	slog.Log(r.Context(), level, "request JSON decoding failed", "event", event, "error", err, "requestId", middleware.GetReqID(r.Context()), "method", r.Method)
	return err
}

// Invalid client JSON and failures while reading a request are different from
// programming mistakes such as an invalid decode destination.
func invalidRequestJSON(err error) bool {
	if err == nil {
		return false
	}
	var invalidTarget *json.InvalidUnmarshalError
	var syntax *json.SyntaxError
	var fieldType *json.UnmarshalTypeError
	var tooLarge *http.MaxBytesError
	return !errors.As(err, &invalidTarget) && (errors.As(err, &syntax) || errors.As(err, &fieldType) ||
		errors.As(err, &tooLarge) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		strings.HasPrefix(err.Error(), "json: unknown field ") || err.Error() == "request body contains multiple JSON values")
}

func decodeRequest(w http.ResponseWriter, r *http.Request, v any) bool {
	err := decode(r, v)
	if err == nil {
		return true
	}
	if invalidRequestJSON(err) {
		fail(w, r, 400, "bad_request", "输入格式不正确")
	} else if observ.FailureLevel(err) == slog.LevelWarn {
		fail(w, r, http.StatusRequestTimeout, "request_timeout", "请求读取超时或已取消")
	} else {
		fail(w, r, 500, "internal", "请求读取失败")
	}
	return false
}

func paramID(r *http.Request) (int64, error) { return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64) }
func pagination(r *http.Request) (int, int) {
	p, _ := strconv.Atoi(r.URL.Query().Get("page"))
	s, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if p < 1 {
		p = 1
	}
	if s < 1 {
		s = 20
	}
	if s > 50 {
		s = 50
	}
	return p, s
}
func current(r *http.Request) *mysqlrepo.User {
	v := r.Context().Value(userKey{})
	if v == nil {
		return nil
	}
	return v.(*mysqlrepo.User)
}
func require(w http.ResponseWriter, r *http.Request) *mysqlrepo.User {
	u := current(r)
	if u == nil {
		fail(w, r, 401, "unauthorized", "请先登录")
	}
	return u
}
func requireAuthor(w http.ResponseWriter, r *http.Request) *mysqlrepo.User {
	u := require(w, r)
	if u != nil && u.Role != "author" && u.Role != "admin" {
		fail(w, r, 403, "forbidden", "没有博主权限")
		return nil
	}
	return u
}
func requireAdmin(w http.ResponseWriter, r *http.Request) *mysqlrepo.User {
	u := require(w, r)
	if u != nil && u.Role != "admin" {
		fail(w, r, 403, "forbidden", "没有管理员权限")
		return nil
	}
	return u
}
func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.Store.DB.PingContext(ctx); err != nil {
		logRequestFailure(w, r, "database readiness check failed", err, "database_readiness")
		fail(w, r, 503, "not_ready", "数据库未就绪")
		return
	}
	respond(w, r, 200, map[string]bool{"ok": true})
}
func (s *Server) authContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("blog_session")
		if err == nil && c.Value != "" {
			u, e := s.Store.UserBySession(r.Context(), c.Value)
			if e == nil {
				r = r.WithContext(context.WithValue(r.Context(), userKey{}, &u))
			} else if !errors.Is(e, mysqlrepo.ErrNotFound) {
				failErr(w, r, e)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
func (s *Server) checkOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
			if _, ok := s.Origins[r.Header.Get("Origin")]; !ok {
				fail(w, r, 403, "bad_origin", "请求来源不受信任")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func trustedOrigins(primary, additional string) map[string]struct{} {
	origins := map[string]struct{}{primary: {}}
	for _, origin := range strings.Split(additional, ",") {
		if origin = strings.TrimSpace(origin); origin != "" {
			origins[origin] = struct{}{}
		}
	}
	return origins
}

func sessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func (s *Server) setSession(w http.ResponseWriter, r *http.Request, u mysqlrepo.User) error {
	token, err := sessionToken()
	if err != nil {
		return err
	}
	expires := time.Now().UTC().Add(7 * 24 * time.Hour)
	if err = s.Store.CreateSession(r.Context(), u.ID, token, expires); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: "blog_session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: strings.HasPrefix(r.Header.Get("Origin"), "https://"), Expires: expires})
	return nil
}
func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var v struct{ Email, Nickname, Password, Confirm string }
	if !decodeRequest(w, r, &v) {
		return
	}
	v.Email = mysqlrepo.NormalizeEmail(v.Email)
	v.Nickname = strings.TrimSpace(v.Nickname)
	address, addressErr := mail.ParseAddress(v.Email)
	if addressErr != nil || address.Address != v.Email || len(v.Email) > 255 {
		fail(w, r, 400, "validation", "邮箱格式不正确")
		return
	}
	if v.Nickname == "" || len([]rune(v.Nickname)) > 100 {
		fail(w, r, 400, "validation", "昵称须为 1–100 个字符")
		return
	}
	if len([]rune(v.Password)) < 8 {
		fail(w, r, 400, "validation", "密码至少 8 位")
		return
	}
	if v.Password != v.Confirm {
		fail(w, r, 400, "validation", "两次输入的密码不一致")
		return
	}
	hash, err := auth.HashPassword(v.Password)
	if err != nil {
		failErr(w, r, err)
		return
	}
	u, err := s.Store.CreateUser(r.Context(), v.Email, v.Nickname, hash)
	if err != nil {
		failErr(w, r, err)
		return
	}
	if err = s.setSession(w, r, u); err != nil {
		failErr(w, r, err)
		return
	}
	respond(w, r, 201, u)
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	key := r.RemoteAddr
	if host, _, err := net.SplitHostPort(key); err == nil {
		key = host
	}
	if s.LoginLimit.blocked(key) {
		fail(w, r, 429, "rate_limited", "登录尝试过多，请稍后重试")
		return
	}
	var v struct{ Email, Password string }
	if !decodeRequest(w, r, &v) {
		return
	}
	u, err := s.Store.UserByEmail(r.Context(), v.Email)
	if err != nil && !errors.Is(err, mysqlrepo.ErrNotFound) {
		failErr(w, r, err)
		return
	}
	if err != nil || !auth.VerifyPassword(v.Password, u.PasswordHash) {
		s.LoginLimit.fail(key)
		fail(w, r, 401, "invalid_credentials", "邮箱或密码错误")
		return
	}
	s.LoginLimit.reset(key)
	if err = s.setSession(w, r, u); err != nil {
		failErr(w, r, err)
		return
	}
	respond(w, r, 200, u)
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("blog_session"); err == nil {
		if err := s.Store.DeleteSession(r.Context(), c.Value); err != nil {
			failErr(w, r, err)
			return
		}
	}
	http.SetCookie(w, &http.Cookie{Name: "blog_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: strings.HasPrefix(r.Header.Get("Origin"), "https://")})
	respond(w, r, 200, map[string]bool{"ok": true})
}
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u := require(w, r)
	if u != nil {
		respond(w, r, 200, u)
	}
}

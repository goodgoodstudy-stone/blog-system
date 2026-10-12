package bloghttp

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/go-chi/chi/v5/middleware"
)

func recoverRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				if value == http.ErrAbortHandler {
					panic(value)
				}
				logRequestFailure(w, r, "request panic", fmt.Errorf("%v\n%s", value, debug.Stack()), "panic")
				if wrapped, ok := w.(middleware.WrapResponseWriter); !ok || wrapped.Status() == 0 {
					fail(w, r, http.StatusInternalServerError, "internal", "操作失败，请稍后重试")
				}
			}
		}()
		next.ServeHTTP(w, r)
	})
}

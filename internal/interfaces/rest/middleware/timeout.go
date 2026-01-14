package middleware

import (
	"context"
	"net/http"
	"time"
)

func Timeout(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			r = r.WithContext(ctx)

			timeoutHandler := http.TimeoutHandler(
				next,
				timeout,
				`{"success":false,"error":{"code":"TIMEOUT","message":"Request timeout"}}`,
			)

			timeoutHandler.ServeHTTP(w, r)
		})
	}
}

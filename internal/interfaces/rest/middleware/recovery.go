package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/interfaces/rest"
)

// Recovery creates middleware that recovers from panics and returns 500
func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error(
						"panic recovered",
						"panic", rec,
						"method", r.Method,
						"path", r.URL.Path,
						"stack", string(debug.Stack()),
					)

					err := application.NewInternalError(fmt.Errorf("panic: %v", rec))
					rest.WriteError(w, err, logger)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/interfaces/rest"
)

type timeoutWriter struct {
	http.ResponseWriter
	h    http.Header
	code int
}

func (tw *timeoutWriter) Header() http.Header {
	return tw.h
}

func (tw *timeoutWriter) Write(p []byte) (int, error) {
	return tw.ResponseWriter.Write(p)
}

func (tw *timeoutWriter) WriteHeader(code int) {
	tw.code = code
	tw.ResponseWriter.WriteHeader(code)
}

// Timeout creates middleware that enforces a request timeout.
// If the timeout is exceeded, it returns a 408 Request Timeout with a JSON error.
func Timeout(timeout time.Duration, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			r = r.WithContext(ctx)

			tw := &timeoutWriter{ResponseWriter: w, h: make(http.Header)}

			done := make(chan struct{})
			panicChan := make(chan any, 1)

			go func() {
				defer func() {
					if p := recover(); p != nil {
						panicChan <- p
					}
				}()
				next.ServeHTTP(tw, r)
				close(done)
			}()

			select {
			case <-ctx.Done():
				if ctx.Err() == context.DeadlineExceeded {
					logger.Warn("request timed out", "path", r.URL.Path, "method", r.Method)
					if tw.code == 0 {
						err := application.NewTimeoutError()
						rest.WriteError(w, err, logger)
					}
				}
			case p := <-panicChan:
				panic(p)
			case <-done:
			}
		})
	}
}

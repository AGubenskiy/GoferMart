package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

func RequestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			startedAt := time.Now()

			next.ServeHTTP(w, r)

			log.Info(
				"request handled",
				"method", r.Method,
				"path", r.URL.Path,
				"duration", time.Since(startedAt).String(),
				"remote_addr", r.RemoteAddr,
			)
		})
	}
}

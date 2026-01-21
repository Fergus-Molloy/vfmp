package http

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type CorrelationID string

func addCorrelationID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		correlationID, err := uuid.NewV7()
		if err != nil {
			slog.Error("could not generate correlation id", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		r.Header.Add("X-Correlation-ID", correlationID.String())

		next.ServeHTTP(w, r)
	})
}

func logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		next.ServeHTTP(w, r)

		logger := slog.With(slog.GroupAttrs("request", getRequestAttrs(r)...))
		logger.Info("handled request", "latency", time.Since(start))
	})
}

func getRequestAttrs(r *http.Request) []slog.Attr {
	attrs := []slog.Attr{
		slog.Any("url", r.URL),
		slog.String("method", r.Method),
	}
	if correlationID := r.Header.Get("X-Correlation-ID"); correlationID != "" {
		attrs = append(attrs, slog.String("correlationID", correlationID))
	}

	return attrs
}

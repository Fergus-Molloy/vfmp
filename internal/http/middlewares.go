package http

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"fergus.molloy.xyz/vfmp/internal/metrics"
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
		metrics.HTTPCount.WithLabelValues("in").Inc()
		start := time.Now()

		next.ServeHTTP(w, r)

		d := time.Since(start)

		logger := slog.With(slog.GroupAttrs("request", getRequestAttrs(r)...))

		us := float64(d.Microseconds()) / float64(1_000_000)
		metrics.HTTPLatencySec.WithLabelValues(r.Method, r.URL.Path).Observe(us)
		logger.Info("handled request", "latency_secs", fmt.Sprintf("%.3g", us))
		metrics.HTTPCount.WithLabelValues("out").Inc()
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

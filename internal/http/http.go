package http

import (
	"context"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"sync"
	"time"
)

// StartHttpServer creates and starts a [http.Server]. Returns the server so that shutdown can be called.
func StartHttpServer(wg *sync.WaitGroup, addr string) *http.Server {
	mux := http.NewServeMux()
	srv := &http.Server{
		Addr:         addr,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		Handler:      mux,
	}

	registerHandlers(mux)

	go serveHttpServer(wg, srv)

	return srv
}

func serveHttpServer(wg *sync.WaitGroup, srv *http.Server) {
	defer wg.Done()

	slog.Info("starting http server", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		slog.Error("error while serving http server", "err", err, "addr", srv.Addr)
	}
	slog.Debug("http server stopped", "addr", srv.Addr)
}

func StartPprofServer(wg *sync.WaitGroup, addr string) *http.Server {
	srv := &http.Server{
		Addr:         addr,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		Handler:      http.DefaultServeMux,
	}

	go serveHttpServer(wg, srv)

	return srv
}

func ShutdownServer(srv *http.Server, ctx context.Context) {
	slog.Warn("shutting down server", "addr", srv.Addr)
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("error shutting down server", "err", err, "addr", srv.Addr)
	}
	slog.Debug("shutting down complete", "addr", srv.Addr)
}

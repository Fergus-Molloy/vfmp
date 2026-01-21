package http

import (
	"context"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"sync"

	"fergus.molloy.xyz/vfmp/internal/config"
)

// StartHttpServer creates and starts a [http.Server]. Returns the server so that shutdown can be called.
func StartHttpServer(wg *sync.WaitGroup, config *config.Config) *http.Server {
	mux := http.NewServeMux()
	srv := &http.Server{
		Addr:         config.HTTPAddr,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
		Handler:      mux,
	}

	registerHandlers(mux)

	go serveHttpServer(wg, srv)

	return srv
}

func serveHttpServer(wg *sync.WaitGroup, srv *http.Server) {
	defer wg.Done()

	if srv.Addr == "" {
		slog.Warn("ignoring http server with empty address")
		return
	}

	slog.Info("starting http server", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		slog.Error("error while serving http server", "err", err, "addr", srv.Addr)
	}
	slog.Debug("http server stopped", "addr", srv.Addr)
}

func StartPprofServer(wg *sync.WaitGroup, config *config.Config) *http.Server {
	srv := &http.Server{
		Addr:         config.PprofAddr,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
		Handler:      http.DefaultServeMux,
	}

	go serveHttpServer(wg, srv)

	return srv
}

func ShutdownServer(srv *http.Server, ctx context.Context) {
	if srv.Addr == "" {
		return // nothing todo
	}

	slog.Warn("shutting down server", "addr", srv.Addr)
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("error shutting down server", "err", err, "addr", srv.Addr)
	}
	slog.Debug("shutting down complete", "addr", srv.Addr)
}

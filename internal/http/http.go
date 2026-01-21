package http

import (
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"sync"
	"time"
)

// StartHttpServer creates and starts a [http.Server]. Returns the server so that shutdown can be called.
func StartHttpServer(wg *sync.WaitGroup, addr string) *http.Server {
	srv := &http.Server{
		Addr:         addr,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go serveHttpServer(wg, srv)

	return srv
}

func serveHttpServer(wg *sync.WaitGroup, srv *http.Server) {
	defer wg.Done()

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		slog.Error("error while serving http server", "err", err, "addr", srv.Addr)
	}
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

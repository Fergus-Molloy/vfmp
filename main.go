package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"fergus.molloy.xyz/vfmp/internal/http"
)

func main() {
	slog.Info("starting vfmp")

	signal, sigCancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer sigCancel()

	wg := new(sync.WaitGroup)

	wg.Add(1)
	srv := http.StartHttpServer(wg, ":8080")

	wg.Add(1)
	pprof := http.StartPprofServer(wg, ":5050")

	<-signal.Done()
	slog.Warn("shutting down vfmp")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go http.ShutdownServer(srv, shutdownCtx)
	go http.ShutdownServer(pprof, shutdownCtx)

	wg.Wait()
	slog.Warn("shut down complete")
}

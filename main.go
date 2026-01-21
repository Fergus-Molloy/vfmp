package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"fergus.molloy.xyz/vfmp/internal/config"
	"fergus.molloy.xyz/vfmp/internal/http"
)

var (
	configPath string
)

func main() {
	flag.StringVar(&configPath, "config", "", "path to config file to use")
	flag.Parse()

	config, err := config.Load(configPath)
	if err != nil {
		slog.Error("error loading config", "err", err)
		os.Exit(1)
	}

	var level slog.Level
	switch config.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	default:
		level = slog.LevelInfo
	}
	slog.SetLogLoggerLevel(level)

	slog.Info("starting vfmp")

	signal, sigCancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer sigCancel()
	wg := new(sync.WaitGroup)

	wg.Add(1)
	srv := http.StartHttpServer(wg, config)

	wg.Add(1)
	pprof := http.StartPprofServer(wg, config)

	<-signal.Done()
	slog.Warn("shutting down vfmp")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), config.ShutdownTimeout)
	defer cancel()

	go http.ShutdownServer(srv, shutdownCtx)
	go http.ShutdownServer(pprof, shutdownCtx)

	wg.Wait()
	slog.Warn("shut down complete")
}

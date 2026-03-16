package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"fergus.molloy.xyz/vfmp/internal/broker"
	"fergus.molloy.xyz/vfmp/internal/config"
	"fergus.molloy.xyz/vfmp/internal/grpc"
	"fergus.molloy.xyz/vfmp/internal/http"
	"fergus.molloy.xyz/vfmp/internal/logger"
	"fergus.molloy.xyz/vfmp/internal/metrics"
)

func main() {
	config, err := config.Load()
	if err != nil {
		slog.Error("error loading config", "err", err)
		os.Exit(1)
	}

	err = configureLogger(config)
	if err != nil {
		slog.Error("could not configure logger", "err", err)
		os.Exit(1)
	}

	slog.Info("loaded configuration", "cfg", config)

	slog.Info("starting vfmp")

	signal, sigCancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer sigCancel()

	metrics.RegisterMetrics()

	wg := new(sync.WaitGroup)
	wg.Add(1)
	broker := broker.StartBroker(signal, wg)

	wg.Add(1)
	srv := http.StartHttpServer(broker, wg, config)

	wg.Add(1)
	metric := http.StartMetricServer(wg, config)

	wg.Add(1)
	grpcSrv := grpc.StartGRPCServer(broker, signal, wg, config)

	<-signal.Done()
	slog.Warn("shutting down vfmp")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), config.ShutdownTimeout)
	defer cancel()

	go http.ShutdownServer(srv, shutdownCtx)
	go http.ShutdownServer(metric, shutdownCtx)

	timeout, cancel := context.WithTimeout(context.Background(), config.ShutdownTimeout)
	done := false
	defer cancel()
	go func() {
		grpcSrv.GracefulStop()
		done = true
		cancel()
	}()
	<-timeout.Done()
	if !done {
		grpcSrv.Stop()
	}

	wg.Wait()
	slog.Warn("shut down complete")
}

func configureLogger(cfg *config.Config) error {
	var level slog.Level
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	default:
		level = slog.LevelInfo
	}

	if cfg.LogPath == "" {
		slog.SetLogLoggerLevel(level)
		return nil
	}

	file, err := os.Create(cfg.LogPath)
	if err != nil {
		return err
	}

	logger := logger.NewTeeLogger(
		slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}),
		slog.NewTextHandler(file, &slog.HandlerOptions{Level: level}),
	)
	slog.SetDefault(logger)

	return nil
}

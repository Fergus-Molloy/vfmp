package http

import (
	"errors"
	"io"
	"log/slog"
	"net/http"

	"fergus.molloy.xyz/vfmp/internal/broker"
	"fergus.molloy.xyz/vfmp/internal/model"
	"fergus.molloy.xyz/vfmp/internal/version"
)

func registerHandlers(mux *http.ServeMux, broker *broker.Broker) {
	mux.HandleFunc("/control/healthcheck", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/control/version", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(version.Version))
	})

	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		msg, err := io.ReadAll(r.Body)
		defer r.Body.Close()

		if err != nil && !errors.Is(err, io.EOF) {
			slog.Error("could not read body", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		slog.Info("echoing message", "msg", string(msg))

		_, _ = w.Write(msg)
	})

	mux.HandleFunc("POST /messages/{topic...}", func(w http.ResponseWriter, r *http.Request) {
		msg, err := io.ReadAll(r.Body)
		defer r.Body.Close()

		if err != nil && !errors.Is(err, io.EOF) {
			slog.Error("could not read body", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		topic := r.PathValue("topic")
		correlationID := r.Header.Get("X-Correlation-ID")

		slog.Info("new message", "len", len(msg), "topic", topic, "correlationID", correlationID)
		broker.MsgChan <- model.NewMessage(msg, topic, correlationID)

	})
}

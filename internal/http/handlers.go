package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"fergus.molloy.xyz/vfmp/internal/broker"
	"fergus.molloy.xyz/vfmp/internal/model"
	"fergus.molloy.xyz/vfmp/internal/version"
	"github.com/google/uuid"
)

func registerHandlers(mux *http.ServeMux, broker *broker.Broker) {
	mux.HandleFunc("/control/healthcheck", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/control/version", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(version.Version))
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
		correlationID, err := uuid.Parse(r.Header.Get("X-Correlation-ID"))
		if err != nil {
			slog.Error("could not parse correlation id header", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		slog.Info("new message", "bytes", len(msg), "topic", topic, "correlationID", correlationID)
		broker.MsgChan <- model.NewMessage(msg, topic, correlationID)
	})

	mux.HandleFunc("GET /messages/{topic...}", func(w http.ResponseWriter, r *http.Request) {
		topic := r.PathValue("topic")

		data := r.URL.Query().Get("data")

		switch data {
		case "count":
			count := broker.GetCount(topic)
			_, _ = w.Write(fmt.Appendf(nil, `{"count": %d}`, count))
			return
		case "peek":
			msg, err := broker.Peek(topic)
			if err != nil {
				slog.Error("could not find message", "err", err, "topic", topic, "correlationID", r.Header.Get("X-Correlation-ID"))
				w.WriteHeader(http.StatusNotFound)
				return
			}

			data, err := json.Marshal(msg)
			if err != nil {
				slog.Error("could not marshal message", "err", err, "topic", topic, "correlationID", r.Header.Get("X-Correlation-ID"))
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			_, _ = w.Write(data)
			return
		default:
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	})
}

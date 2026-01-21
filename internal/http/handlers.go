package http

import (
	"errors"
	"io"
	"log/slog"
	"net/http"

	"fergus.molloy.xyz/vfmp/internal/version"
)

func registerHandlers(mux *http.ServeMux) {
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
}

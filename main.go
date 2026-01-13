package main

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
)

func main() {

	http.HandleFunc("/control/healthcheck", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	http.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		message := make([]byte, 50)
		_, err := r.Body.Read(message)
		defer r.Body.Close()

		if err != nil && !errors.Is(err, io.EOF) {
			slog.Error("could not read body", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		msg := bytes.Trim(message, "\x00")
		slog.Info("echoing message", "msg", string(msg))

		w.Write(msg)
	})

	http.ListenAndServe(":8080", nil)
}

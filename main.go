package main

// a simple resumable upload
// use tus.io protocol

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
)

func main() {
	mux := buildServeMux()

	// starting the app
	slog.Info("running app at :1080")
	if err := http.ListenAndServe(":1080", mux); err != nil {
		panic(err)
	}
}

type FileInitResponse struct {
	ID string `json:"id"`
}

func buildServeMux() *http.ServeMux {
	mux := http.NewServeMux()
	// POST /file => create session
	mux.HandleFunc("POST /file", func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.NewUUID()
		if err != nil {
			slog.Error("Failed to generate new id", slog.Any("Error", err))
			http.Error(w, "Error allocating id", http.StatusInternalServerError)
			return
		}
		resp := FileInitResponse{ID: id.String()}
		jsonResponse, err := json.Marshal(resp)
		if err != nil {
			slog.Error("Failed to marshal json response", slog.Any("Error", err))
			http.Error(w, "Error allocating id", http.StatusInternalServerError)
			return
		}
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write(jsonResponse)
	})

	// Head => show status

	// Patch => upload file (maybe in chunk)
	return mux
}

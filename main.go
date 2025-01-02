package main

// a simple resumable upload
// use tus.io protocol

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/google/uuid"
)

const (
	MAX_SIZE             int = 1024 * 1024 * 1024
	TUS_PROTOCOL_VERSION     = "1.0.0"

	//	headers
	HEADER_TUS_RESUMABLE = "Tus-Resumable"
	HEADER_TUS_VERSION   = "Tus-Version"
	HEADER_TUS_EXTENSION = "Tus-Extension"
	HEADER_TUS_MAX_SIZE  = "Tus-Max-Size"
	HEADER_LOCATION      = "Location"
	HEADER_UPLOAD_LENGTH = "Upload-Length"
)

func main() {
	mux := buildServeMux(&ServerConfig{
		UploadDir: "upload",
		Host:      "localhost",
		Protocol:  "http",
	})

	// starting the app
	slog.Info("running app at :1080")
	if err := http.ListenAndServe(":1080", mux); err != nil {
		panic(err)
	}
}

type FileInitResponse struct {
	ID string `json:"id"`
}

type File struct {
	ID   uuid.UUID
	Size int
}

type Storage map[uuid.UUID]*File

type ServerConfig struct {
	UploadDir string // the directory wher all file is being uploaded to
	Host      string
	Port      int
	Protocol  string
}

func buildServeMux(config *ServerConfig) *http.ServeMux {
	var host, protocol string
	port := config.Port
	storage := make(Storage)
	if len(config.Host) <= 0 {
		host = "localhost"
	} else {
		host = config.Host
	}
	if len(config.Protocol) <= 0 {
		protocol = "http"
	} else {
		protocol = config.Protocol
	}

	mux := http.NewServeMux()
	// POST /file => create session
	mux.HandleFunc("POST /file", func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.NewUUID()
		if err != nil {
			slog.Error("Failed to generate new id", slog.Any("Error", err))
			http.Error(w, "Error allocating id", http.StatusInternalServerError)
			return
		}

		dir := filepath.Join(config.UploadDir, id.String())
		if err = os.MkdirAll(dir, 0666); err != nil {
			slog.Error("Failed to create file directory", slog.String("dir", dir), slog.Any("Error", err))
			http.Error(w, "Error allocating storage", http.StatusInternalServerError)
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

	// Options
	mux.HandleFunc("OPTIONS /files", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(HEADER_TUS_RESUMABLE, TUS_PROTOCOL_VERSION)
		w.Header().Set(HEADER_TUS_VERSION, TUS_PROTOCOL_VERSION)
		w.Header().Set(HEADER_TUS_EXTENSION, "creation")
		w.Header().Set(HEADER_TUS_MAX_SIZE, strconv.Itoa(int(MAX_SIZE)))
		w.WriteHeader(http.StatusNoContent)
	})

	// Creation
	mux.HandleFunc("POST /files", func(w http.ResponseWriter, r *http.Request) {
		uploadLength := r.Header.Get(HEADER_UPLOAD_LENGTH)
		if len(uploadLength) <= 0 {
			uploadLength = "0"
		}
		l, err := strconv.Atoi(uploadLength)
		if err != nil {
			slog.Error("Failed to convert upload length", slog.Any("Error", err))
			w.WriteHeader(http.StatusLengthRequired)
		}
		if l > MAX_SIZE {
			w.Header().Set(HEADER_TUS_MAX_SIZE, strconv.Itoa(MAX_SIZE))
			w.Header().Set(HEADER_TUS_RESUMABLE, TUS_PROTOCOL_VERSION)
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		id, err := uuid.NewUUID()
		if err != nil {
			slog.Error("Failed to generate new file id", slog.Any("Error", err))
			http.Error(w, "Failed allocating new file id", http.StatusInternalServerError)
		}
		f := &File{
			ID:   id,
			Size: l,
		}
		storage[id] = f
		w.Header().Set(HEADER_LOCATION, fmt.Sprintf("%s://%s:%d/files/%s", protocol, host, port, id.String()))
		w.Header().Set(HEADER_TUS_RESUMABLE, TUS_PROTOCOL_VERSION)
		w.WriteHeader(http.StatusCreated)
	})

	// Head => show status

	// Patch => upload file (maybe in chunk)
	return mux
}

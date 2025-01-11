package main

// a simple resumable upload
// use tus.io protocol

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/google/uuid"
)

const (
	MAX_SIZE                         int = 1024 * 1024 * 1024
	CHUNK_SIZE                       int = 1024 * 1024
	TUS_PROTOCOL_VERSION                 = "1.0.0"
	CONTENT_TYPE_OFFSET_OCTET_STREAM     = "application/offset+octet-stream"

	//	headers
	HEADER_TUS_RESUMABLE  = "Tus-Resumable"
	HEADER_TUS_VERSION    = "Tus-Version"
	HEADER_TUS_EXTENSION  = "Tus-Extension"
	HEADER_TUS_MAX_SIZE   = "Tus-Max-Size"
	HEADER_LOCATION       = "Location"
	HEADER_UPLOAD_LENGTH  = "Upload-Length"
	HEADER_UPLOAD_OFFSET  = "Upload-Offset"
	HEADER_CONTENT_LENGTH = "Content-Length"
	HEADER_CONTENT_TYPE   = "Content-Type"
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
	ID     uuid.UUID
	Size   int
	Offset int
	mu     sync.Mutex
}

func (f *File) calculateOffset(contentLength int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Offset = f.Offset + contentLength
}

func (f *File) create() error {
	path := filepath.Join(uploadDir, f.ID.String())
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return nil
}

func (f *File) write(body io.Reader) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// write to temp file, assumption is the file
	// has been created when POST /files
	path := filepath.Join(uploadDir, f.ID.String())
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// write per 1024 * 1024 byte
	reader := bufio.NewReader(body)
	buff := make([]byte, CHUNK_SIZE)

	for {
		n, err := reader.Read(buff)
		if err != nil {
			if err != io.EOF {
				return fmt.Errorf("Error reading data %v", err)
			}

			// write the last chunk
			if err = f.writeToFile(file, buff[:n]); err != nil {
				return err
			}
			break
		}
		if err = f.writeToFile(file, buff[:n]); err != nil {
			return err
		}
	}

	return nil
}

func (f *File) writeToFile(file *os.File, buff []byte) error {
	if _, err := file.Write(buff); err != nil {
		return fmt.Errorf("Error writing data to file %v", err)
	}
	f.Offset = f.Offset + len(buff)
	return nil
}

type Storage map[string]*File

type ServerConfig struct {
	UploadDir string // the directory wher all file is being uploaded to
	Host      string
	Port      int
	Protocol  string
}

var uploadDir = "./temp"

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
	if len(config.UploadDir) > 0 {
		uploadDir = config.UploadDir
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
		if err = f.create(); err != nil {
			slog.Error("Failed to create new file", slog.Any("Error", err))
		}
		storage[id.String()] = f
		w.Header().Set(HEADER_LOCATION, fmt.Sprintf("%s://%s:%d/files/%s", protocol, host, port, id.String()))
		w.Header().Set(HEADER_TUS_RESUMABLE, TUS_PROTOCOL_VERSION)
		w.WriteHeader(http.StatusCreated)
	})

	// Head => show status
	mux.HandleFunc("HEAD /files/{id}", func(w http.ResponseWriter, r *http.Request) {
		fileId := r.PathValue("id")
		file := storage[fileId]
		if file == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set(HEADER_TUS_RESUMABLE, TUS_PROTOCOL_VERSION)
		w.Header().Set(HEADER_UPLOAD_OFFSET, strconv.Itoa(file.Offset))
		w.WriteHeader(http.StatusOK)
	})

	// Patch => upload file (maybe in chunk)
	mux.HandleFunc("PATCH /files/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(HEADER_TUS_RESUMABLE, TUS_PROTOCOL_VERSION)
		contentType := r.Header.Get(HEADER_CONTENT_TYPE)
		if contentType != CONTENT_TYPE_OFFSET_OCTET_STREAM {
			w.WriteHeader(http.StatusUnsupportedMediaType)
			return
		}

		fileId := r.PathValue("id")
		file := storage[fileId]
		if file == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		offsetValue := r.Header.Get(HEADER_UPLOAD_OFFSET)
		if len(offsetValue) <= 0 {
			offsetValue = "0"
		}
		offset, err := strconv.Atoi(offsetValue)

		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if offset != file.Offset {
			w.WriteHeader(http.StatusConflict)
			return
		}

		// write to temp file
		if err = file.write(r.Body); err != nil {
			slog.Error("Fail to write r.Body", slog.Any("Error", err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set(HEADER_UPLOAD_OFFSET, strconv.Itoa(file.Offset))

		w.WriteHeader(http.StatusNoContent)
	})

	return mux
}

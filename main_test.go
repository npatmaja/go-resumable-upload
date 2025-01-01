package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/google/uuid"
)

var serverAddr, uploadDir string

func TestMain(m *testing.M) {
	serverAddr = "localhost:1071"
	uploadDir = os.TempDir()

	// run server
	mux := buildServeMux(&ServerConfig{
		UploadDir: uploadDir,
	})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		http.ListenAndServe(serverAddr, mux)
	}()

	exit := m.Run()

	// clean up
	os.RemoveAll(uploadDir)

	os.Exit(exit)
}

func TestFileShouldReturn201WithNewFileId(t *testing.T) {
	res, err := http.DefaultClient.Post(
		fmt.Sprintf("http://%s/file", serverAddr),
		"",
		nil,
	)
	if err != nil {
		t.Errorf("Failed to call /file. Error %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != 201 {
		t.Errorf("POST /file does not return 201. got=%v", res.StatusCode)
	}

	if res.Status != "201 Created" {
		t.Errorf("POST /file does not return 201 Created. got=%v", res.Status)
	}

	if res.Header.Get("Content-Type") != "application/json" {
		t.Errorf("POST /file does not return Content-Type [application/json]. got=%v", res.Header.Get("Content-Type"))
	}

	var r FileInitResponse
	err = json.NewDecoder(res.Body).Decode(&r)
	if err != nil {
		t.Errorf("POST /file does not return correct json. got=%v", err)
	}
	_, err = uuid.Parse(r.ID)
	if err != nil {
		t.Errorf("POST /file does not return correct uuid. got=%v", err)
	}

	// create a temp directory
	dir := filepath.Join(uploadDir, r.ID)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Errorf("POST /file does not create the %s dir. got=%v", r.ID, err)
	}
}

func TestOptionsShouldReturn204(t *testing.T) {
	tests := []struct {
		host                    string
		headers                 map[string]string
		expectedResponseStatus  int
		expectedResponseHeaders map[string]string
	}{
		{
			host: fmt.Sprintf("http://%s", serverAddr),
			headers: map[string]string{
				"Host": serverAddr,
			},
			expectedResponseStatus: http.StatusNoContent,
			expectedResponseHeaders: map[string]string{
				"Tus-Resumable": "1.0.0",
				"Tus-Version":   "1.0.0",
				"Tus-Max-Size":  "1073741824", // 1GB
				"Tus-Extension": "creation",
			},
		},
	}

	for _, tt := range tests {
		req, err := http.NewRequest(http.MethodOptions, fmt.Sprintf("%s/file", tt.host), nil)
		for k, v := range tt.headers {
			req.Header.Add(k, v)
		}
		if err != nil {
			t.Fatalf("Fail to create new request: %v", err)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Fail to execute the request. err=%v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != tt.expectedResponseStatus {
			t.Errorf("OPTIONS /file does not return %v. got=%v", tt.expectedResponseStatus, res.StatusCode)
		}

		for k, v := range tt.expectedResponseHeaders {
			if res.Header.Get(k) != v {
				t.Errorf("OPTIONS /file does not return correct value for header %v, expected=%v. got=%v", k, v, res.Header.Get(k))
			}
		}
	}
}

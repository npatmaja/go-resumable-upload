package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
)

var serverAddr, uploadDir string
var port = 1071

func TestMain(m *testing.M) {
	serverAddr = "localhost:1071"
	uploadDir = os.TempDir()

	// run server
	mux := buildServeMux(&ServerConfig{
		UploadDir: uploadDir,
		Host:      "localhost",
		Port:      port,
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
		req, err := http.NewRequest(http.MethodOptions, fmt.Sprintf("%s/files", tt.host), nil)
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
			t.Errorf("OPTIONS /files does not return %v. got=%v", tt.expectedResponseStatus, res.StatusCode)
		}

		for k, v := range tt.expectedResponseHeaders {
			if res.Header.Get(k) != v {
				t.Errorf("OPTIONS /files does not return correct value for header %v, expected=%v. got=%v", k, v, res.Header.Get(k))
			}
		}
	}
}

func TestCreation(t *testing.T) {
	tests := []struct {
		host                   string
		uploadLength           string
		uploadMetadata         map[string]string
		expectedResponseStatus int
		expectedResponseHeader map[string]string
	}{
		{
			host:         fmt.Sprintf("http://%s/files", serverAddr),
			uploadLength: "1000",
			expectedResponseHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
				"Location":      fmt.Sprintf("http://%s/files", serverAddr),
			},
			expectedResponseStatus: http.StatusCreated,
		},
		{
			host:         fmt.Sprintf("http://%s/files", serverAddr),
			uploadLength: strconv.Itoa(2 * 1024 * 1024 * 1024),
			expectedResponseHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
			},
			expectedResponseStatus: http.StatusRequestEntityTooLarge,
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("test #%d - upload length: %s", i, tt.uploadLength), func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, tt.host, nil)
			req.Header.Set(HEADER_UPLOAD_LENGTH, tt.uploadLength)
			if err != nil {
				t.Fatalf("Fail to create new request. error=%v", err)
			}

			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Fail to execute the request. error=%v", err)
			}
			defer res.Body.Close()

			if res.StatusCode != tt.expectedResponseStatus {
				t.Errorf("POST /files does not return %v. got=%v", tt.expectedResponseStatus, res.StatusCode)
			}

			if res.Header.Get(HEADER_TUS_RESUMABLE) != tt.expectedResponseHeader[HEADER_TUS_RESUMABLE] {
				t.Errorf("POST /files does not return correct value for header %s, expected=%v. got=%v", HEADER_TUS_RESUMABLE, tt.expectedResponseHeader[HEADER_TUS_RESUMABLE], res.Header.Get(HEADER_TUS_RESUMABLE))
			}

			if res.StatusCode == http.StatusCreated {
				location := res.Header.Get(HEADER_LOCATION)
				lastSlashIndex := strings.LastIndex(location, "/")
				baseUrl := location[:lastSlashIndex]
				id := location[lastSlashIndex+1:]
				if baseUrl != tt.host {
					t.Errorf("POST /files does not return correct header %s base url, expected=%s. got=%s", HEADER_LOCATION, tt.host, baseUrl)
				}
				if _, err := uuid.Parse(id); err != nil {
					t.Errorf("POST /files does not return correct file id. got error=%v", err)
				}

			}
		})
	}
}

func TestHead(t *testing.T) {
	// initiate test data
	host := fmt.Sprintf("http://%s/files", serverAddr)
	postReq, err := http.NewRequest(http.MethodPost, host, nil)
	if err != nil {
		t.Fatalf("Fail to create test data. Error=%v", err)
	}
	postReq.Header.Set(HEADER_UPLOAD_LENGTH, "1024")
	postRes, err := http.DefaultClient.Do(postReq)
	if err != nil {
		t.Fatalf("Fail to create test data. Error=%v", err)
	}
	if postRes.StatusCode != http.StatusCreated {
		t.Fatalf("Fail to create test data. Got status=%d", postRes.StatusCode)
	}

	location := postRes.Header.Get(HEADER_LOCATION)
	lastSlashIdx := strings.LastIndex(location, "/")
	fileId := location[lastSlashIdx+1:]

	tests := []struct {
		testName               string
		host                   string
		fileId                 string
		expectedResponseStatus int
		expectedHeader         map[string]string
	}{
		{
			testName:               "test success after file creation",
			host:                   fmt.Sprintf("http://%s/files", serverAddr),
			fileId:                 fileId,
			expectedResponseStatus: http.StatusOK,
			expectedHeader: map[string]string{
				"Upload-Offset": "0",
			},
		},
		{
			testName:               "test file not found",
			host:                   fmt.Sprintf("http://%s/files", serverAddr),
			fileId:                 "dummy-not-found",
			expectedResponseStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodHead, fmt.Sprintf("%s/%s", tt.host, tt.fileId), nil)
			if err != nil {
				t.Fatalf("Fail to create HEAD request. error=%v", err)
			}
			req.Header.Set(HEADER_TUS_RESUMABLE, "1.0.0")
			req.Header.Set("Host", serverAddr)

			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Fail to execute HEAD request. error=%v", err)
			}

			if res.StatusCode != tt.expectedResponseStatus {
				t.Errorf("HEAD /files/%s does not return %v. got=%v", tt.fileId, tt.expectedResponseStatus, res.StatusCode)
			}

			for k, v := range tt.expectedHeader {
				if res.Header.Get(k) != v {
					t.Errorf("HEAD /files does not return correct value for header %v, expected=%v. got=%v", k, v, res.Header.Get(k))
				}
			}
		})
	}
}

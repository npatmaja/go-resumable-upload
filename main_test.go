package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
)

var serverAddr string

func TestMain(m *testing.M) {
	// addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	// if err != nil {
	// 	slog.Error("Failed to get available port for testing", slog.Any("Error", err))
	// 	os.Exit(1)
	// }
	//
	// listener, err := net.ListenTCP("tcp", addr)
	// if err != nil {
	// 	slog.Error("Failed to get available port for testing", slog.Any("Error", err))
	// 	os.Exit(1)
	// }
	//
	// defer listener.Close()
	// serverAddr = listener.Addr().(*net.TCPAddr).String()
	serverAddr = "localhost:1071"

	// run server
	mux := buildServeMux()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		http.ListenAndServe(serverAddr, mux)
	}()

	exit := m.Run()
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
}

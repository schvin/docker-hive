package server

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
)

var (
	tmpDir, _   = ioutil.TempDir("/tmp", "hive_")
	listenPort  = 45000
	testAddress = fmt.Sprintf("http://localhost:%d", listenPort)
)

func newTestServer() *Server {
	testServer := New(tmpDir, "", listenPort, "/var/run/docker.sock", "")
	testServer.Start()
	return testServer
}

func getTestUrl(path string) string {
	return fmt.Sprintf("%s%s", testAddress, path)
}

func TestGetAllNodeConnectionStrings(t *testing.T) {
	testServer := newTestServer()
	c := testServer.AllNodeConnectionStrings()
	if len(c) == 0 {
		t.Fatalf("Unable to get node connection strings")
	}
}

func TestHandleIndexReturnsWithStatusOK(t *testing.T) {
	request, _ := http.NewRequest("GET", getTestUrl("/"), nil)
	response := httptest.NewRecorder()

	testServer := newTestServer()
	testServer.indexHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("Non-expected status code%v:\n\tbody: %v", "200", response.Code)
	}
}

func TestHandleDBGetReturnsWithStatusOK(t *testing.T) {
	request, _ := http.NewRequest("GET", getTestUrl("/db/foo"), nil)
	response := httptest.NewRecorder()

	testServer := newTestServer()
	testServer.indexHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("Non-expected status code%v:\n\tbody: %v", "200", response.Code)
	}
}

func TestHandleDBPostReturnsWithStatusOK(t *testing.T) {
	data := "foo"
	b := bytes.NewBufferString(data)
	request, _ := http.NewRequest("POST", getTestUrl("/db/foo"), b)
	response := httptest.NewRecorder()

	testServer := newTestServer()
	testServer.indexHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("Non-expected status code%v:\n\tbody: %v", "200", response.Code)
	}
}

func TestHandleImagesReturnsWithStatusOK(t *testing.T) {
	request, _ := http.NewRequest("GET", getTestUrl("/images/json"), nil)
	response := httptest.NewRecorder()

	testServer := newTestServer()
	testServer.containersHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("Non-expected status code%v:\n\tbody: %v", "200", response.Code)
	}
}

func TestHandleContainersReturnsWithStatusOK(t *testing.T) {
	request, _ := http.NewRequest("GET", getTestUrl("/containers/json"), nil)
	response := httptest.NewRecorder()

	testServer := newTestServer()
	testServer.containersHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("Non-expected status code%v:\n\tbody: %v", "200", response.Code)
	}
}

func TestHandleCreateReturnsWithStatusCreated(t *testing.T) {
	body := "{ \"Image\": \"base\", \"Cmd\": [\"echo\", \"hello\"] }"
	b := bytes.NewBufferString(body)
	request, _ := http.NewRequest("POST", "/v1.9/containers/create", b)
	request.Header.Add("Content-Type", "application/json")
	response := httptest.NewRecorder()

	testServer := newTestServer()
	testServer.containerCreateHandler(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("Non-expected status code%d:\n\tbody: %v", http.StatusCreated, response.Code)
	}
}

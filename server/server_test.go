/*
   Copyright Evan Hazlett

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/
package server

import (
	"bytes"
	"encoding/json"
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

func TestNewContainer(t *testing.T) {
	cId := newTestContainer()
	testServer := newTestServer()
	info := testServer.getContainer(cId)
	if info.Container.Id != cId {
		t.Fatalf("Invalid ID: expected %s ; received %s", cId, info.Container.Id)
	}

}

func newTestContainer() string {
	body := "{ \"Image\": \"base\", \"Cmd\": [\"echo\", \"hello\"] }"
	b := bytes.NewBufferString(body)
	request, _ := http.NewRequest("POST", "/v1.9/containers/create", b)
	request.Header.Add("Content-Type", "application/json")
	response := httptest.NewRecorder()

	testServer := newTestServer()
	testServer.containerCreateHandler(response, request)
	var c APIContainer
	decoder := json.NewDecoder(response.Body)
	err := decoder.Decode(&c)
	if err != nil {
		panic("Unable to parse JSON from newTestContainer")
	}
	return c.Id
}

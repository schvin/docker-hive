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
	"os"
	"testing"
)

var (
	tmpDir, _   = ioutil.TempDir("/tmp", "hive_")
	listenPort  = 45000
	testAddress = fmt.Sprintf("http://localhost:%d", listenPort)
)

func newTestServer() *Server {
	dockerPath := os.Getenv("DOCKER_PATH")
	if dockerPath == "" {
		dockerPath = "/var/run/docker.sock"
	}
	testServer := New(tmpDir, "", listenPort, dockerPath, "", 1, "dev", "default")
	testServer.Start()
	return testServer
}

func getTestUrl(path string) string {
	return fmt.Sprintf("%s%s", testAddress, path)
}

func newTestContainer(cmd string) string {
	if cmd == "" {
		cmd = "echo newTestContainer"
	}
	body := fmt.Sprintf("{ \"Image\": \"base\", \"Cmd\": [\"%s\"] }", cmd)
	b := bytes.NewBufferString(body)
	request, _ := http.NewRequest("POST", "/v1.10/containers/create", b)
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

func newTestRunningContainer(cmd string) string {
	if cmd == "" {
		cmd = "echo testRunningContainer"
	}
	body := fmt.Sprintf("{ \"Image\": \"base\", \"Cmd\": [\"%s\"], \"Tty\": true, \"OpenStdin\": true, \"StdinOnce\": false }", cmd)
	b := bytes.NewBufferString(body)
	request, _ := http.NewRequest("POST", "/v1.10/containers/create", b)
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
		t.Fatalf("Non-expected status code %v: expected %v\nbody: %v", response.Code, "200", response.Body)
	}
}

func TestHandleInfoReturnsWithStatusOK(t *testing.T) {
	request, _ := http.NewRequest("GET", getTestUrl("/info"), nil)
	response := httptest.NewRecorder()

	testServer := newTestServer()
	testServer.indexHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("Non-expected status code %v: expected %v\nbody: %v", response.Code, "200", response.Body)
	}
}

func TestHandlePingReturnsWithStatusOK(t *testing.T) {
	request, _ := http.NewRequest("GET", getTestUrl("/ping"), nil)
	response := httptest.NewRecorder()

	testServer := newTestServer()
	testServer.indexHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("Non-expected status code %v: expected %v\nbody: %v", response.Code, "200", response.Body)
	}
}

func TestHandleDBGetReturnsWithStatusOK(t *testing.T) {
	request, _ := http.NewRequest("GET", getTestUrl("/db/foo"), nil)
	response := httptest.NewRecorder()

	testServer := newTestServer()
	testServer.indexHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("Non-expected status code %v: expected %v\nbody: %v", response.Code, "200", response.Body)
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
		t.Fatalf("Non-expected status code %v: expected %v\nbody: %v", response.Code, "200", response.Body)
	}
}

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
	"crypto/sha1"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleImagesReturnsWithStatusOK(t *testing.T) {
	request, _ := http.NewRequest("GET", getTestUrl("/images/json"), nil)
	response := httptest.NewRecorder()

	testServer := newTestServer()
	testServer.containersHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("Non-expected status code %v: expected %v\nbody: %v", response.Code, "200", response.Body)
	}
}

func TestHandleImageCreateReturnsWithStatusOK(t *testing.T) {
	request, _ := http.NewRequest("POST", getTestUrl("/images/create?fromImage=busybox&tag="), nil)
	response := httptest.NewRecorder()

	testServer := newTestServer()
	testServer.imageCreateHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("Non-expected status code %v: expected %v\nbody: %v", response.Code, "200", response.Body)
	}
}

func TestHandleImageDeleteReturnsWithStatusOK(t *testing.T) {
	request, _ := http.NewRequest("DELETE", getTestUrl("/images/busybox"), nil)
	response := httptest.NewRecorder()

	testServer := newTestServer()
	testServer.imageCreateHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("Non-expected status code %v: expected %v\nbody: %v", response.Code, "200", response.Body)
	}
}

func TestHandleContainersReturnsWithStatusOK(t *testing.T) {
	request, _ := http.NewRequest("GET", getTestUrl("/containers/json"), nil)
	response := httptest.NewRecorder()

	testServer := newTestServer()
	testServer.containersHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("Non-expected status code %v: expected %v\nbody: %v", response.Code, "200", response.Body)
	}
}

func TestHandleContainerRestartReturnsWithStatusNoContent(t *testing.T) {
	cId := newTestContainer("")
	testServer := newTestServer()
	request, _ := http.NewRequest("POST", getTestUrl(fmt.Sprintf("/containers/%s/restart", cId)), nil)
	response := httptest.NewRecorder()

	testServer.containerRestartHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("Non-expected status code %v: expected %v\nbody: %v", response.Code, "204", response.Body)
	}
}

func TestHandleContainerStartReturnsWithStatusNoContent(t *testing.T) {
	cId := newTestContainer("")
	testServer := newTestServer()
	request, _ := http.NewRequest("POST", getTestUrl(fmt.Sprintf("/containers/%s/start", cId)), nil)
	response := httptest.NewRecorder()

	testServer.containerStartHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("Non-expected status code %v: expected %v\nbody: %v", response.Code, "204", response.Body)
	}
}

func TestHandleContainerStopReturnsWithStatusNoContent(t *testing.T) {
	cId := newTestContainer("")
	testServer := newTestServer()
	request, _ := http.NewRequest("POST", getTestUrl(fmt.Sprintf("/containers/%s/stop", cId)), nil)
	response := httptest.NewRecorder()

	testServer.containerStopHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("Non-expected status code %v: expected %v\nbody: %v", response.Code, "204", response.Body)
	}
}

func TestHandleContainerRemoveReturnsWithStatusNoContent(t *testing.T) {
	cId := newTestContainer("")
	testServer := newTestServer()
	request, _ := http.NewRequest("DELETE", getTestUrl(fmt.Sprintf("/containers/%s", cId)), nil)
	response := httptest.NewRecorder()

	testServer.containerRemoveHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("Non-expected status code %v: expected %v\nbody: %v", response.Code, "204", response.Body)
	}
}

func TestHandleContainerTopReturnsWithStatusOK(t *testing.T) {
	cId := newTestRunningContainer("/bin/bash")
	testServer := newTestServer()

	// start container
	request, _ := http.NewRequest("POST", getTestUrl(fmt.Sprintf("/containers/%s/start", cId)), nil)
	response := httptest.NewRecorder()

	testServer.containerStartHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("Non-expected status code %v: expected %v\nbody: %v", response.Code, "204", response.Body)
	}

	// top
	request, _ = http.NewRequest("GET", getTestUrl(fmt.Sprintf("/containers/%s/top", cId)), nil)
	response = httptest.NewRecorder()

	testServer.containerTopHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("Non-expected status code %v: expected %v\nbody: %v", response.Code, "200", response.Body)
	}

	// kill container
	request, _ = http.NewRequest("POST", getTestUrl(fmt.Sprintf("/containers/%s/kill", cId)), nil)
	testServer.containerKillHandler(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("Non-expected status code %v: expected %v\nbody: %v", response.Code, "200", response.Body)
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
		t.Fatalf("Non-expected status code %v: expected %v\nbody: %v", response.Code, "204", response.Body)
	}
}

func TestNewContainer(t *testing.T) {
	cId := newTestContainer("")
	testServer := newTestServer()
	info := testServer.getContainerInfo(cId)
	for _, c := range info {
		if c.Container.Id != cId {
			t.Fatalf("Invalid ID: expected %s ; received %s", cId, c.Container.Id)
		}
	}
}

func TestGetContainer(t *testing.T) {
	cId := newTestContainer("")
	testServer := newTestServer()
	info := testServer.getContainerInfo(cId)
	for _, c := range info {
		if c.Container.Id != cId {
			t.Fatalf("Invalid ID: expected %s ; received %s", cId, c.Container.Id)
		}
		if c.Container.Config.CpuShares != 0 {
			t.Fatalf("Expected 0 Container.Config.CpuShares; received %d", c.Container.Config.CpuShares)
		}
	}
}

func TestHandleContainerDiffReturnsWithStatusOK(t *testing.T) {
	cId := newTestContainer("")
	testServer := newTestServer()
	response := httptest.NewRecorder()

	request, _ := http.NewRequest("GET", getTestUrl(fmt.Sprintf("/containers/%s/changes", cId)), nil)

	testServer.containerChangesHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("Non-expected status code %v: expected %v\nbody: %v", response.Code, "200", response.Body)
	}
}

func TestHandleContainerKillReturnsWithStatusNoContent(t *testing.T) {
	cId := newTestContainer("")
	testServer := newTestServer()
	response := httptest.NewRecorder()

	request, _ := http.NewRequest("POST", getTestUrl(fmt.Sprintf("/containers/%s/kill", cId)), nil)
	testServer.containerKillHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("Non-expected status code %v: expected %v\nbody: %v", response.Code, "204", response.Body)
	}
}

func TestHandleContainerInspectReturnsWithStatusOK(t *testing.T) {
	cId := newTestContainer("")
	testServer := newTestServer()
	response := httptest.NewRecorder()

	request, _ := http.NewRequest("GET", getTestUrl(fmt.Sprintf("/containers/%s/json", cId)), nil)

	testServer.containerInspectHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("Non-expected status code %v: expected %v\nbody: %v", response.Code, "200", response.Body)
	}
}

func TestHandleImageHistoryReturnsWithStatusOK(t *testing.T) {
	testServer := newTestServer()
	response := httptest.NewRecorder()

	request, _ := http.NewRequest("POST", getTestUrl("/images/create?fromImage=busybox&tag="), nil)
	testServer.imageCreateHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("Non-expected status code %v: expected %v\nbody: %v", response.Code, "200", response.Body)
	}

	request, _ = http.NewRequest("GET", getTestUrl("/images/busybox/json"), nil)
	testServer.imageHistoryHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("Non-expected status code %v: expected %v\nbody: %v", response.Code, "200", response.Body)
	}
}

func TestHandleImageSearchReturnsWithStatusOK(t *testing.T) {
	testServer := newTestServer()
	response := httptest.NewRecorder()

	request, _ := http.NewRequest("GET", getTestUrl("/images/search?term=busybox"), nil)

	testServer.imageSearchHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("Non-expected status code %v: expected %v\nbody: %v", response.Code, "200", response.Body)
	}
}

func TestHandleImageTagReturnsWithStatusOK(t *testing.T) {
	testServer := newTestServer()
	response := httptest.NewRecorder()
	h := sha1.New()
	io.WriteString(h, "docker-hive test image name")
	imgName := h.Sum(nil)

	// pull image
	request, _ := http.NewRequest("POST", getTestUrl("/images/create?fromImage=busybox&tag="), nil)
	testServer.imageCreateHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("Non-expected status code %v: expected %v\nbody: %v", response.Code, "200", response.Body)
	}

	// tag
	request, _ = http.NewRequest("POST", getTestUrl(fmt.Sprintf("/images/busybox/tag?repo=%s&tag=", imgName)), nil)
	testServer.imageHistoryHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("Non-expected status code %v: expected %v\nbody: %v", response.Code, "200", response.Body)
	}

	// remove tag
	request, _ = http.NewRequest("DELETE", getTestUrl(fmt.Sprintf("/images/%s", imgName)), nil)

	testServer.imageCreateHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("Non-expected status code %v: expected %v\nbody: %v", response.Code, "200", response.Body)
	}
}

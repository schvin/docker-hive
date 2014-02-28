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
	"log"
	"net/http"
	"strings"
)

func containersResponse(s *Server, w http.ResponseWriter, all string) {
	value := "{}"
	var allContainers []APIContainer
	for _, host := range s.AllNodeConnectionStrings() {
		path := fmt.Sprintf("%s/docker/containers/json?all=1", host)
		resp, err := http.Get(path)
		if err != nil {
			log.Printf("Error getting host containers for %s: %s", host, err)
			continue
		}
		contents, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		value = string(contents)
		// filter out not running if requested
		if all == "" && value != "" {
			var containers []APIContainer
			s := bytes.NewBufferString(value)
			d := json.NewDecoder(s)
			if err := d.Decode(&containers); err != nil {
				log.Printf("Error decoding container JSON: %s", err)
			}
			for _, v := range containers {
				if strings.Index(v.Status, "Up") != -1 {
					allContainers = append(allContainers, v)
				}
			}
			b, err := json.Marshal(allContainers)
			if err != nil {
				log.Printf("Error marshaling containers to JSON: %s", err)
			}
			value = string(b)
		}
	}
	w.Write([]byte(value))
}

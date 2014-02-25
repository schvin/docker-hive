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
		// filter out not running
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
		}
		b, err := json.Marshal(allContainers)
		if err != nil {
			log.Printf("Error marshaling containers to JSON: %s", err)
		}
		value = string(b)
	}
	w.Write([]byte(value))
}

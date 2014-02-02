package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/dotcloud/docker"
	"log"
	"net/http"
        "strings"
)

func containerActionResponse(s *Server, w http.ResponseWriter, all string) {
	var allHosts []string
	allHosts = append(allHosts, s.RaftServer.Name())
	for _, p := range s.RaftServer.Peers() {
		allHosts = append(allHosts, p.Name)
	}
	value := "{}"
	var allContainers []*docker.APIContainers
	for _, host := range allHosts {
		key := fmt.Sprintf("containers:%s", host)
		value = s.db.Get(key)
		// filter out not running
		if all == "" && value != "" {
			var containers []*docker.APIContainers
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

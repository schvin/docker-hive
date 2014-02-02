package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

func imageActionResponse(s *Server, w http.ResponseWriter) {
	var allHosts []string
	allHosts = append(allHosts, s.RaftServer.Name())
	for _, p := range s.RaftServer.Peers() {
		allHosts = append(allHosts, p.Name)
	}
	value := "{}"
	var allImages []*Image
	for _, host := range allHosts {
		key := fmt.Sprintf("images:%s", host)
		value = s.db.Get(key)
		var images []*Image
		s := bytes.NewBufferString(value)
		d := json.NewDecoder(s)
		if err := d.Decode(&images); err != nil {
			log.Printf("Error decoding image JSON: %s", err)
		}
		for _, i := range images {
			allImages = append(allImages, i)
		}
	}
	b, err := json.Marshal(allImages)
	if err != nil {
		log.Printf("Error marshaling images to JSON: %s", err)
	}
	//value = string(b)
	//w.Write([]byte(value))
	w.Write(b)
}


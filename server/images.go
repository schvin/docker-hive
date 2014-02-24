package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
)

func imagesResponse(s *Server, w http.ResponseWriter) {
	value := "{}"
	var allImages []*Image
	for _, host := range s.AllNodeConnectionStrings() {
		path := fmt.Sprintf("%s/docker/images/json?all=1", host)
		resp, err := http.Get(path)
		if err != nil {
			log.Printf("Error getting host images for %s: %s", host, err)
			continue
		}
		contents, err := ioutil.ReadAll(resp.Body)
		value = string(contents)
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
	w.Write(b)
}

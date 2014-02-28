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
		resp.Body.Close()
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

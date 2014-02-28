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
package db

import (
	"strings"
	"sync"
)

// The key-value database.
type DB struct {
	data  map[string]string
	mutex sync.RWMutex
}

// Creates a new database.
func New() *DB {
	return &DB{
		data: make(map[string]string),
	}
}

// Retrieves the value for a given key.
func (db *DB) Get(key string) string {
	db.mutex.RLock()
	defer db.mutex.RUnlock()
	return db.data[key]
}

func (db *DB) Find(key string) string {
	db.mutex.RLock()
	defer db.mutex.RUnlock()
	// TODO: probably need a better algorithm here
	for k, v := range db.data {
		if strings.Index(k, key) != -1 {
			return v
		}
	}
	return ""
}

// Sets the value for a given key.
func (db *DB) Put(key string, value string) {
	db.mutex.Lock()
	defer db.mutex.Unlock()
	db.data[key] = value
}

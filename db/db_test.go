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
	"fmt"
	"strconv"
	"testing"
)

func TestDBReadWrite(t *testing.T) {
	key := "foo"
	val := "bar"
	db := New()
	db.Put(key, val)
	if actual := db.Get(key); actual != val {
		t.Fatalf("Expected %s got %s", val, actual)
	}
}

func TestDBFind(t *testing.T) {
	key := "abcdefg"
	val := "123"
	db := New()
	db.Put(key, val)
	if actual := db.Find("abcd"); actual != val {
		t.Fatalf("Expected %s got %s", val, actual)
	}
}

func BenchmarkDBWrite(b *testing.B) {
	db := New()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("abc%d", i)
		db.Put(key, fmt.Sprintf("foo%d", i))
	}
}

func BenchmarkDBRead(b *testing.B) {
	db := New()
	key := "foo"
	val := "bar"
	db.Put(key, val)
	for i := 0; i < b.N; i++ {
		db.Get(key)
	}
}

func BenchmarkDBFind(b *testing.B) {
	db := New()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("abc%d", i)
		db.Put(key, fmt.Sprintf("foo%d", i))
		db.Find(strconv.Itoa(i))
	}
}

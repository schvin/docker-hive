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

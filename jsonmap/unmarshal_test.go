package jsonmap

import (
	"encoding/json"
	"testing"

	goccy "github.com/goccy/go-json"

	"github.com/oarkflow/json/jsonparser"
)

type Custom struct {
	Arr []string `json:"arr"`
	Obj struct {
		Inner string `json:"inner"`
	} `json:"obj"`
}
type T struct {
	Key1    string  `json:"key1"`
	Key2    float64 `json:"key2"`
	Key3    bool    `json:"key3"`
	Key4    any     `json:"key4"`
	Nested  Custom  `json:"nested"`
	Escaped string  `json:"escaped"`
}

var complexJSON = []byte(`{
		"key1": "value1",
		"key2": 123.45,
		"key3": true,
		"key4": null,
		"nested": {
			"arr": ["a", "b", "c"],
			"obj": {"inner": "value"}
		},
		"escaped": "Line1\nLine2\tTabbed\u0021"
	}`)

func BenchmarkStandardUnmarshal(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var result any
		if err := json.Unmarshal(complexJSON, &result); err != nil {
			b.Fatalf("standard json.Unmarshal error: %v", err)
		}
	}
}
func BenchmarkGoccyUnmarshal(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var result any
		if err := goccy.Unmarshal(complexJSON, &result); err != nil {
			b.Fatalf("standard json.Unmarshal error: %v", err)
		}
	}
}

func BenchmarkCustomUnmarshal(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var result T
		if err := Unmarshal(complexJSON, &result); err != nil {
			b.Fatalf("custom Unmarshal error: %v", err)
		}
	}
}

func BenchmarkCustomGet(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := Get(complexJSON, "key3"); err != nil {
			b.Fatalf("custom Get error: %v", err)
		}
	}
}

func BenchmarkCustomSet(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := Set(complexJSON, "key3", "test"); err != nil {
			b.Fatalf("custom Set error: %v", err)
		}
	}
}

func BenchmarkJSONParserGet(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, _, _, err := jsonparser.Get(complexJSON, "key3"); err != nil {
			b.Fatalf("jsonparser Get error: %v", err)
		}
	}
}

func BenchmarkJSONParserSet(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := jsonparser.Set(complexJSON, []byte("test"), "key3"); err != nil {
			b.Fatalf("jsonparser Set error: %v", err)
		}
	}
}

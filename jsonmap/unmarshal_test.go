package jsonmap

import (
	"encoding/json"
	"testing"

	goccy "github.com/goccy/go-json"
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

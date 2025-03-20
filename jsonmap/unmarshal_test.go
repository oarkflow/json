package jsonmap

import (
	"encoding/json"
	"testing"
)

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

func BenchmarkCustomUnmarshal(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var result map[string]any
		if err := Unmarshal(complexJSON, &result); err != nil {
			b.Fatalf("custom Unmarshal error: %v", err)
		}
	}
}

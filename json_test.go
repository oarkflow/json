package json_test

import (
	"testing"

	"github.com/oarkflow/json"
)

func BenchmarkIs(b *testing.B) {

	tests := []string{
		`{"name": "John", "age": 30, "city": "New York"}`,
		`[{"name": "John"}, {"name": "Jane"}]`,
		`{name: "John", age: 30, city: "New York"}`,
		`{"name": "John", "age": 30, "city": "New York"}`,
		``,
		`"name": "John", "age": 30, "city": "New York"}`,
	}

	for _, test := range tests {
		b.Run(test, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				json.IsValid(test)
			}
		})
	}
}

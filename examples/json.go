package main

import (
	"github.com/oarkflow/json"
)

func main() {
	tests := []string{
		`{"name": "John", "age": 30, "city": "New York"}`,
		`[{"name": "John"}, {"name": "Jane"}]`,
		`{name: "John", age: 30, city: "New York"}`,
		`{"name": "John", "age": 30, "city": "New York"`,
		`{"name": "John", "age": 30, "city": "New York"}`,
		``,
		`{}`,
		`"name": "John", "age": 30, "city": "New York"}`,
	}

	for _, test := range tests {
		println(json.Is(test))
	}
}

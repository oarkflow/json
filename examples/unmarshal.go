package main

import (
	"fmt"

	"github.com/oarkflow/json"
	"github.com/oarkflow/json/jsonmap"
)

// ----------------------
// Example Usage
// ----------------------

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

func main() {
	var result json.RawMessage
	if err := jsonmap.Unmarshal(complexJSON, &result); err != nil {
		panic(err)
	}
	fmt.Printf("Decoded: %+v\n", result)

	// Marshal back to JSON.
	marshaled, err := jsonmap.Marshal(result)
	if err != nil {
		panic(err)
	}
	fmt.Println("Marshaled:", string(marshaled))
}

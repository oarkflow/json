package main

import (
	"fmt"

	"github.com/oarkflow/json"
)

type Options struct {
	Id      string         `json:"id,omitempty"`
	Verbose bool           `json:"verbose,omitempty"`
	Level   int            `json:"level"`
	Power   int            `json:"power"`
	Options map[string]any `json:"options"`
}

func main() {
	// JSON where power is an int and level is a string
	jsonData1 := `{
		"id": "123",
		"verbose": true,
		"level": "5",
		"power": 100,
		"options": {"idle": 123}
	}`

	var opts1 Options

	// Unmarshal both JSON examples using the generic unmarshal function
	if err := json.FixAndUnmarshal([]byte(jsonData1), &opts1); err != nil {
		fmt.Printf("Error unmarshaling opts1: %v\n", err)
		return
	}
	fmt.Printf("Unmarshaled opts1: %+v\n", opts1)

}

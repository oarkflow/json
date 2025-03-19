package main

import (
	"fmt"
	"os"

	v2 "github.com/oarkflow/json/jsonschema/v2"
)

// --------------------- Main Function & Example ---------------------

func main() {
	// Sample schema for a User.
	schemaJSON, err := os.ReadFile("schema.json")
	if err != nil {
		panic(err)
	}
	// Sample JSON data that should pass validation.
	dataJSON, err := os.ReadFile("instance_valid.json")
	var result map[string]any

	if err := v2.Unmarshal(dataJSON, &result, schemaJSON); err != nil {
		fmt.Printf("Unmarshal failed: %v\n", err)
	} else {
		fmt.Println(result)
	}
}

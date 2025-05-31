package main

import (
	"fmt"
	"os"
	
	v2 "github.com/oarkflow/json/jsonschema/v2"
)

var data1 = map[string]any{"last_name": "Alice"}

var data2 = map[string]any{
	"last_name": "Alice",
	"age":       12,
}

func main() {
	// Sample schema for a User.
	schemaJSON, err := os.ReadFile("simple_schema.json")
	if err != nil {
		panic(err)
	}
	compiler := v2.NewCompiler()
	schema, err := compiler.Compile(schemaJSON)
	if err != nil {
		panic(err)
	}
	d1, err1 := schema.SmartUnmarshal(data1)
	if err1 != nil {
		fmt.Println(err1, err)
	}
	d2, err2 := schema.SmartUnmarshal(data2)
	if err2 != nil {
		fmt.Println(err2, err)
	}
	fmt.Println(d1, d2)
}

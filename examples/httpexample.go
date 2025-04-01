package main

import (
	"fmt"
	"log"
	"net/http"

	v2 "github.com/oarkflow/json/jsonschema/v2"
)

var sampleSchema = []byte(`{
	"type": "object",
	"properties": {
		"name": { "type": "string" },
		"age": { "type": "number", "in": "query" }
	},
	"required": ["name", "age"]
}`)

type Person struct {
	Name string  `json:"name"`
	Age  float64 `json:"age,omitempty"`
}

func handler(w http.ResponseWriter, r *http.Request) {
	var p Person
	err := v2.UnmarshalAndValidateRequest(r, &p, sampleSchema)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	fmt.Fprintf(w, "Valid Person: %+v", p)
}

func main() {
	http.HandleFunc("/person", handler)
	log.Println("Listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

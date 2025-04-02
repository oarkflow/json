package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/oarkflow/json/examples/models"
	v2 "github.com/oarkflow/json/jsonschema/v2"
)

func handler(w http.ResponseWriter, r *http.Request) {
	var p models.Person
	sampleSchema, _ := os.ReadFile("schema_request.json")
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

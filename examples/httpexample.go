package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	v2 "github.com/oarkflow/json/jsonschema/v2"
)

type Name struct {
	FirstName  string `json:"firstName"`
	MiddleName string `json:"middleName"`
	LastName   string `json:"lastName"`
}

type Auth struct {
	Token string `json:"token"`
}

type Person struct {
	Name Name `json:"name"`
	Auth Auth `json:"auth"`
	Age  int  `json:"age,omitempty"`
}

func handler(w http.ResponseWriter, r *http.Request) {
	var p Person
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

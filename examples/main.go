package main

import (
	"fmt"

	"github.com/oarkflow/json"
	"github.com/oarkflow/json/jsonschema"
)

var data = []byte(`
{
	"email": "2021-01-01"
}
`)
var schemeBytes = []byte(`{"type":"object","description":"users","properties":{"avatar":{"type":"string","maxLength":255},"created_at":{"type":"string","default":"now()"},"created_by":{"type":"integer","maxLength":64},"deleted_at":{"type":"string"},"email":{"type":"string","maxLength":255},"email_verified_at":{"type":"string"},"first_name":{"type":"string","maxLength":255},"is_active":{"type":"boolean","default":"false"},"last_name":{"type":"string","maxLength":255},"middle_name":{"type":"string","maxLength":255},"status":{"type":"string","maxLength":30},"title":{"type":"string","maxLength":10},"updated_at":{"type":"string","default":"now()"},"updated_by":{"type":"integer","maxLength":64},"user_id":{"type":"integer","maxLength":64},"verification_token":{"type":"string","maxLength":255}},"required":["email"],"primaryKeys":["user_id"]}`)

var schema = []byte(`{
				"type": "object",
				"description": "Join a room",
				"properties": {
					"rid": {
						"type": "string|null",
						"properties": null,
						"items": null,
						"in": "param"
					}
				},
				"additionalProperties": false
			}`)

func ma1in() {
	var sch jsonschema.Schema
	err := json.Unmarshal(schema, &sch)
	if err != nil {
		panic(err)
	}
	fmt.Println(sch)
}

func main() {
	var sch jsonschema.Schema
	err := json.Unmarshal(schemeBytes, &sch)
	if err != nil {
		panic(err)
	}
	var d map[string]any
	err = sch.ValidateAndUnmarshalJSON(data, &d)
	if err != nil {
		panic(err)
	}
	fmt.Println(d)
}

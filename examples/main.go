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
var schemeBytes = []byte(`{"properties":{"client_disposition_name":{"type":"string"},"encounter_dos":{"type":"string"},"encounter_dos_end":{"type":["string","null"]},"encounter_ins_type":{"type":["string","null"]},"encounter_type":{"type":["string","null"]},"facility_id":{"type":"integer"},"patient_dob":{"type":"string"},"patient_fin":{"type":"string"},"patient_mrn":{"type":"string"},"patient_name":{"type":"string"},"patient_sex":{"type":"string"},"wid":{"type":"string","in":"param"}},"type":"object","description":"Add new encounter and start coding"}`)

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

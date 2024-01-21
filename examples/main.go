package main

import (
	"fmt"

	"github.com/oarkflow/json"
	"github.com/oarkflow/json/jsonschema"
)

var data = []byte(`
{
    "em": {
        "encounter_uid": 1,
        "work_item_uid": 2,
        "billing_provider": "Test provider",
        "resident_provider": "Test Resident Provider"
    },
    "cpt": [
        {
            "code": "001",
            "billing_provider": "Test provider",
            "resident_provider": "Test Resident Provider"
        },
        {
            "code": "OBS01",
            "billing_provider": "Test provider",
            "resident_provider": "Test Resident Provider"
        },
        {
            "code": "SU002",
            "billing_provider": "Test provider",
            "resident_provider": "Test Resident Provider"
        }
    ]
}
`)
var schemeBytes = []byte(`{
	"type": "object",
	"properties": {
		"em": {
			"type": "object",
			"properties": {
				"code": {
					"type": "string",
					"default": "N/A"
				},
				"encounter_uid": {
					"type": "integer"
				},
				"work_item_uid": {
					"type": "integer"
				},
				"billing_provider": {
					"type": "string"
				},
				"resident_provider": {
					"type": "string"
				}
			},
			"required": ["code"]
		},
		"cpt": {
			"type" : "array",
			"items" : {
				"type": "object",
				"properties": {
					"code": {
						"type": "string"
					},
					"encounter_uid": {
						"type": "integer"
					},
					"work_item_uid": {
						"type": "integer"
					},
					"billing_provider": {
						"type": "string"
					},
					"resident_provider": {
						"type": "string"
					}
				}
			}
		}
	}
}`)

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

func main() {
	var sch jsonschema.Schema
	err := json.Unmarshal(schema, &sch)
	if err != nil {
		panic(err)
	}
	fmt.Println(sch)
}

func unmarshal() {
	var mp map[string]any
	err := json.Unmarshal(data, &mp)
	if err != nil {
		panic(err)
	}
	fmt.Println(mp)
	err = json.Unmarshal(data, &mp, schemeBytes)
	if err != nil {
		panic(err)
	}
	fmt.Println(mp)
}

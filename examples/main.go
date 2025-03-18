package main

import (
	"fmt"
	"time"

	v2 "github.com/oarkflow/json/jsonschema/v2"
)

type User struct {
	UserID     int               `json:"user_id"`
	Activities map[string]string `json:"activities"`
	CreatedAt  time.Time         `json:"created_at"`
}

var data = []byte(`{"user_id": 1, "created_at": "2025-01-01"}`)
var schemeBytes = []byte(`{
    "type": "object",
    "description": "users",
	"required": ["user_id"],
    "properties": {
        "created_at": {
            "type": ["object", "string"]
        },
        "activities": {
            "type": ["object"],
            "default": {"inactive": "0"}
        },
        "user_id": {
            "type": ["number"],
            "maxLength": 64
        }
    }
}`)

func main() {
	start := time.Now()
	compiler := v2.NewCompiler()
	schema, err := compiler.CompileSchema(schemeBytes)
	if err != nil {
		panic(err)
	}
	d, err := schema.SmartUnmarshal(data)
	if err != nil {
		panic(err)
	}
	fmt.Println(d)
	fmt.Println(time.Since(start))

	start = time.Now()
	d, err = schema.SmartUnmarshal(data)
	if err != nil {
		panic(err)
	}
	fmt.Println(d)
	fmt.Println(time.Since(start))
}

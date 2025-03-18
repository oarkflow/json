package main

import (
	"fmt"
	"time"

	v2 "github.com/oarkflow/json/jsonschema/v2"
)

type User struct {
	UserID     string            `json:"user_id"`
	Activities map[string]string `json:"activities"`
	CreatedAt  time.Time         `json:"created_at"`
}

var data = []byte(`{"user_id": 1}`)
var schemeBytes = []byte(`{
    "type": "object",
    "description": "users",
	"required": ["user_id"],
    "properties": {
        "created_at": {
            "type": ["object", "string"],
            "default": "now()"
        },
        "activities": {
            "type": ["object"],
            "default": "{'inactive':0}"
        },
        "user_id": {
            "type": [
                "integer",
                "string"
            ],
            "maxLength": 64
        }
    }
}`)

func main() {
	start := time.Now()
	var d User
	err := v2.Unmarshal(data, &d, schemeBytes)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(d)
	fmt.Println(time.Since(start))
}

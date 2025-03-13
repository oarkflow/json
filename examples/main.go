package main

import (
	"fmt"
	"time"

	"github.com/oarkflow/json"
)

type User struct {
	UserID     string            `json:"user_id"`
	Activities map[string]string `json:"activities"`
	CreatedAt  time.Time         `json:"created_at"`
}

var data = []byte(`{"user_id": 1,"created_at":"2025-01-01"}`)
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
	err := json.FixAndUnmarshal(data, &d, schemeBytes)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(d)
	fmt.Println(time.Since(start))
}

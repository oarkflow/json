package main

import (
	"fmt"
	"time"

	"github.com/oarkflow/json"
)

type User struct {
	UserID    int       `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}

var data = []byte(`{"user_id": 1, "created_at":"2025-03-12"}`)
var schemeBytes = []byte(`{
    "type": "object",
    "description": "users",
	"required": ["user_id"],
    "properties": {
        "created_at": {
            "type": ["object", "string"],
            "default": "now()"
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
	var d User
	err := json.Unmarshal(data, &d, schemeBytes)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(d)
}

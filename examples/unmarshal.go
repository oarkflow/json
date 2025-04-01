package main

import (
	"fmt"
	"log"
	
	"github.com/oarkflow/json/jsonmap"
)

// ----------------------
// Example Usage
// ----------------------

type Custom struct {
	Arr []string `json:"arr"`
	Obj struct {
		Inner string `json:"inner"`
	} `json:"obj"`
}

type T struct {
	Key1    string  `json:"key1"`
	Key2    float64 `json:"key2"`
	Key3    bool    `json:"key3"`
	Key4    any     `json:"key4"`
	Nested  Custom  `json:"nested"`
	Escaped string  `json:"escaped"`
}

var complexJSON = []byte(`{
		"key1": "value1",
		"key2": 123.45,
		"key3": true,
		"key4": null,
		"nested": {
			"arr": ["a", "b", "c"],
			"obj": {"inner": "value"}
		},
		"escaped": "Line1\nLine2\tTabbed\u0021"
	}`)

func main() {
	var result T
	if err := jsonmap.Unmarshal(complexJSON, &result); err != nil {
		panic(err)
	}
	fmt.Printf("Decoded: %+v\n", result)
	
	// Marshal back to JSON.
	marshaled, err := jsonmap.Marshal(result)
	if err != nil {
		panic(err)
	}
	fmt.Println("Marshaled:", string(marshaled))
	
	jsonStr := []byte(`{
		"key1": {
			"key2": "value2",
			"arr": [
				{"key2": "arrval1"},
				{"key2": "arrval2"}
			]
		}
	}`)
	
	// --- Get a single value ---
	// Get "value2" from key1.key2
	val, err := jsonmap.Get(jsonStr, "key1.key2")
	if err != nil {
		log.Fatalf("Get error: %v", err)
	}
	fmt.Printf("Get(key1.key2) => %v\n", val)
	
	// --- Get with array index ---
	// Get first element's key2: key1.arr.0.key2 or key1.[0].key2
	val2, err := jsonmap.Get(jsonStr, "key1.arr.0.key2")
	if err != nil {
		log.Fatalf("Get array index error: %v", err)
	}
	fmt.Printf("Get(key1.arr.0.key2) => %v\n", val2)
	
	// --- GetAll using wildcard ---
	// Get all "key2" values from all elements in key1.arr
	allVals, err := jsonmap.GetAll(jsonStr, "key1.arr.#.key2")
	if err != nil {
		log.Fatalf("GetAll error: %v", err)
	}
	fmt.Printf("GetAll(key1.arr.#.key2) => %v\n", allVals)
	
	// --- Set a single value ---
	// Change key1.key2 to "newValue"
	updatedJSON, err := jsonmap.Set(jsonStr, "key1.key2", "newValue")
	if err != nil {
		log.Fatalf("Set error: %v", err)
	}
	fmt.Printf("After Set(key1.key2): %s\n", updatedJSON)
	
	// --- SetAll using wildcard ---
	// Change all key1.arr.#.key2 values to "newArrValue"
	updatedJSON2, err := jsonmap.SetAll(jsonStr, "key1.arr.#.key2", "newArrValue")
	if err != nil {
		log.Fatalf("SetAll error: %v", err)
	}
	fmt.Printf("After SetAll(key1.arr.#.key2): %s\n", updatedJSON2)
	
	// --- Delete a key ---
	// Remove key1.key2 from the object
	updatedJSON3, err := jsonmap.Delete(jsonStr, "key1.key2")
	if err != nil {
		log.Fatalf("Delete error: %v", err)
	}
	fmt.Printf("After Delete(key1.key2): %s\n", updatedJSON3)
}

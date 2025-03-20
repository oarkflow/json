package main

import (
	"errors"
	"fmt"
	"time"
	"unsafe"

	"github.com/oarkflow/json/jsonmap"
)

func b2s(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

// ----------------------------------------------------------------------
// Example: custom type implementing JSONUnmarshaler.
type MyTime time.Time

func (mt *MyTime) UnmarshalJSONCustom(data []byte) error {
	// Expect a string in RFC3339 format.
	// Strip quotes.
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return errors.New("invalid time format")
	}
	s := b2s(data[1 : len(data)-1])
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return err
	}
	*mt = MyTime(t)
	return nil
}

// ----------------------------------------------------------------------
// Example Usage and Testing
type Example struct {
	Key1    string         // decoded from JSON string
	Key2    float64        // JSON number
	Key3    bool           // JSON boolean
	Nested  map[string]any // JSON object
	Escaped string         // JSON string with escapes
	TimeVal MyTime         // custom time type via JSONUnmarshaler
	Array   []int          // JSON array of numbers converted to []int
}

func main() {
	// JSON sample containing various types.
	jsonData := []byte(`{
		"key1": "value1",
		"key2": 123.45,
		"key3": true,
		"nested": {"a": "b", "c": 3},
		"escaped": "Line1\nLine2\tTabbed\u0021",
		"timeval": "2025-03-20T15:04:05Z",
		"array": [1, 2, 3, 4]
	}`)

	// Using our custom unmarshal (without reflect) for a struct.
	// Since Example is not one of the natively supported types,
	// we manually decode it via an intermediate map and then assign fields.
	var intermediate map[string]any
	if err := jsonmap.Unmarshal(jsonData, &intermediate); err != nil {
		panic(err)
	}
	// Manually map fields (this step can be generated or written by hand).
	var ex Example
	if s, ok := intermediate["key1"].(string); ok {
		ex.Key1 = s
	}
	if n, ok := intermediate["key2"].(float64); ok {
		ex.Key2 = n
	}
	if b, ok := intermediate["key3"].(bool); ok {
		ex.Key3 = b
	}
	if m, ok := intermediate["nested"].(map[string]any); ok {
		ex.Nested = m
	}
	if s, ok := intermediate["escaped"].(string); ok {
		ex.Escaped = s
	}
	if s, ok := intermediate["timeval"].(string); ok {
		// Use the custom unmarshaler.
		var mt MyTime
		if err := mt.UnmarshalJSONCustom([]byte(`"` + s + `"`)); err != nil {
			panic(err)
		}
		ex.TimeVal = mt
	}
	if arr, ok := intermediate["array"].([]any); ok {
		ex.Array = make([]int, len(arr))
		for i, v := range arr {
			if f, ok := v.(float64); ok {
				ex.Array[i] = int(f)
			}
		}
	}
	fmt.Printf("Decoded Example:\n%+v\n", ex)

	// Direct unmarshal into a map (supported natively)
	var m map[string]any
	if err := jsonmap.Unmarshal(jsonData, &m); err != nil {
		panic(err)
	}
	fmt.Println("\nDirectly decoded map[string]any:")
	fmt.Printf("%#v\n", m)

	// Direct unmarshal into an array of objects.
	arrayJSON := []byte(`[
		{"id": 1, "name": "One"},
		{"id": 2, "name": "Two"}
	]`)
	var arrObjs []map[string]any
	if err := jsonmap.Unmarshal(arrayJSON, &arrObjs); err != nil {
		panic(err)
	}
	fmt.Println("\nDirectly decoded []map[string]any:")
	fmt.Printf("%#v\n", arrObjs)
}

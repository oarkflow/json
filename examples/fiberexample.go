package main

import (
	"encoding/json"
	"fmt"
	
	v2 "github.com/oarkflow/json/jsonschema/v2"
)

// FakeCtx is a simple implementation of the Ctx interface.
type FakeCtx struct {
	body    []byte
	params  map[string]string
	queries map[string]string
	headers map[string]string
}

func (c *FakeCtx) Params(key string) string {
	return c.params[key]
}

func (c *FakeCtx) Query(key string) string {
	return c.queries[key]
}

func (c *FakeCtx) Body() []byte {
	return c.body
}

func (c *FakeCtx) Get(key string) string {
	return c.headers[key]
}

func (c *FakeCtx) BodyParser(dest interface{}) error {
	return json.Unmarshal(c.body, dest)
}

var fiberSchema = []byte(`{
	"type": "object",
	"properties": {
		"email": {"type": "string", "format": "email"}
	},
	"required": ["email"],
	"in": "body"
}`)

type Contact struct {
	Email string `json:"email"`
}

func main() {
	// Simulate a fiber request with valid JSON body.
	body := `{"email": "test@example.com"}`
	ctx := &FakeCtx{
		body:    []byte(body),
		params:  map[string]string{},
		queries: map[string]string{},
	}
	var contact Contact
	err := (&v2.Schema{}).UnmarshalFiberCtx(ctx, &contact)
	if err != nil {
		fmt.Println("Fiber validation error:", err)
	} else {
		fmt.Printf("Fiber validated contact: %+v\n", contact)
	}
	
	// Example with field extraction using query.
	// Here, the schema "in" is set to "query" and requires a field name.
	fiberQuerySchema := []byte(`{
		"type": "string",
		"in": "query",
		"field": "token"
	}`)
	ctx2 := &FakeCtx{
		body:    []byte(""), // not used
		params:  map[string]string{},
		queries: map[string]string{"token": "abc123"},
	}
	var token string
	// Compile the schema and use UnmarshalFiberCtx.
	compiler := v2.NewCompiler()
	schema, err := compiler.Compile(fiberQuerySchema)
	if err != nil {
		fmt.Println("Schema compile error:", err)
		return
	}
	err = schema.UnmarshalFiberCtx(ctx2, &token)
	if err != nil {
		fmt.Println("Fiber query validation error:", err)
	} else {
		fmt.Printf("Fiber extracted token: %s\n", token)
	}
}

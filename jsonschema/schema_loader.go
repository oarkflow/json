package jsonschema

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sync"
)

var lr *LoaderRegistry
var lrLock sync.Mutex

// LoaderRegistry maintains a lookup table between uri schemes and associated loader
type LoaderRegistry struct {
	loaderLookup map[string]SchemaLoaderFunc
}

// SchemaLoaderFunc is a function that loads a schema for a specific URI Scheme
type SchemaLoaderFunc func(ctx context.Context, uri *url.URL, schema *Schema) error

// NewLoaderRegistry allocates a new schema loader registry
func NewLoaderRegistry() *LoaderRegistry {
	r := &LoaderRegistry{
		loaderLookup: map[string]SchemaLoaderFunc{},
	}

	r.Register("http", HTTPSchemaLoader)
	r.Register("https", HTTPSchemaLoader)
	r.Register("file", FileSchemaLoader)

	return r
}

// Register a new schema loader for a specific URI Scheme
func (r *LoaderRegistry) Register(scheme string, loader SchemaLoaderFunc) {
	r.loaderLookup[scheme] = loader
}

// Get the schema loader func for a specific URI Scheme
func (r *LoaderRegistry) Get(scheme string) (SchemaLoaderFunc, bool) {
	l, exists := r.loaderLookup[scheme]
	return l, exists
}

// Copy returns a copy of the loader registry
func (r *LoaderRegistry) Copy() *LoaderRegistry {
	nr := NewLoaderRegistry()

	for key, loader := range r.loaderLookup {
		nr.Register(key, loader)
	}

	return nr
}

// GetSchemaLoaderRegistry provides an accessor to a globally available (schema) loader registry
func GetSchemaLoaderRegistry() *LoaderRegistry {
	lrLock.Lock()
	if lr == nil {
		lr = NewLoaderRegistry()
	}
	lrLock.Unlock()
	return lr
}

// FetchSchema downloads and loads a schema from a remote location
func FetchSchema(ctx context.Context, uri string, schema *Schema) error {
	globalRegistry := GetSchemaLoaderRegistry()
	return FetchSchemaWithRegistry(ctx, uri, schema, globalRegistry)
}

// FetchSchemaWithRegistry downloads and loads a schema from a remote location using a specific LoaderRegistry
// The global loader registry will be used if no registry is passed
func FetchSchemaWithRegistry(ctx context.Context, uri string, schema *Schema, registry *LoaderRegistry) error {
	schemaDebug(fmt.Sprintf("[FetchSchema] Fetching: %s", uri))
	u, err := url.Parse(uri)
	if err != nil {
		return err
	}

	if registry == nil {
		return fmt.Errorf("missing registry parameter")
	}

	loader, exists := registry.Get(u.Scheme)

	if !exists {
		return fmt.Errorf("URI scheme %s is not supported for uri: %s", u.Scheme, uri)
	}

	return loader(ctx, u, schema)
}

// HTTPSchemaLoader loads a schema from a http or https URI
func HTTPSchemaLoader(ctx context.Context, uri *url.URL, schema *Schema) error {
	var req *http.Request
	if ctx != nil {
		req, _ = http.NewRequestWithContext(ctx, "GET", uri.String(), nil)
	} else {
		req, _ = http.NewRequest("GET", uri.String(), nil)
	}
	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if schema == nil {
		schema = &Schema{}
	}
	return json.Unmarshal(body, schema)
}

// FileSchemaLoader loads a schema from a file URI
func FileSchemaLoader(ctx context.Context, uri *url.URL, schema *Schema) error {
	body, err := os.ReadFile(uri.Path)
	if err != nil {
		return err
	}
	if schema == nil {
		schema = &Schema{}
	}
	return json.Unmarshal(body, schema)
}

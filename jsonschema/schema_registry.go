package jsonschema

import (
	"context"
	"fmt"
	"strings"
)

var sr *SchemaRegistry

// UseScopedRegistries controls if Schema types should use global or
// local registries.
// Unscoped Mode (false): This is the default behaviour. This means that any
//
//	 	external schema reference will be cached/shared between Schema{}
//		instances. This might can be troublesome if you expect such external
//		reference to be updated.
//
// Scoped Mode (true): Each Schema{} instance starts with copy of the global
//
//	 	registries(Schema, Loader). External schema references will be cached
//		but are scoped to that schema instance.
var UseScopedRegistries = false

// SchemaRegistry maintains a lookup table between schema string references
// and actual schemas
type SchemaRegistry struct {
	schemaLookup  map[string]*Schema
	contextLookup map[string]*Schema

	localLoaderRegistry *LoaderRegistry
}

// NewSchemaRegistry allocates a new schema registry
func NewSchemaRegistry() *SchemaRegistry {
	return &SchemaRegistry{
		schemaLookup:  map[string]*Schema{},
		contextLookup: map[string]*Schema{},
	}
}

// GetSchemaRegistry provides an accessor to a globally available schema registry
func GetSchemaRegistry() *SchemaRegistry {
	if sr == nil {
		sr = NewSchemaRegistry()
	}
	return sr
}

// ResetSchemaRegistry resets the main SchemaRegistry
func ResetSchemaRegistry() {
	sr = nil
}

func (sr *SchemaRegistry) GetLoaderRegistry() *LoaderRegistry {
	if sr.localLoaderRegistry == nil {
		sr.localLoaderRegistry = GetSchemaLoaderRegistry().Copy()
	}
	return sr.localLoaderRegistry
}

// Get fetches a schema from the top level context registry or fetches it from a remote
func (sr *SchemaRegistry) Get(ctx context.Context, uri string) *Schema {
	uri = strings.TrimRight(uri, "#")
	schema := sr.schemaLookup[uri]

	loaderRegistry := sr.GetLoaderRegistry()

	if schema == nil {
		fetchedSchema := &Schema{}
		err := FetchSchemaWithRegistry(ctx, uri, fetchedSchema, loaderRegistry)
		if err != nil {
			schemaDebug(fmt.Sprintf("[SchemaRegistry] Fetch error: %s", err.Error()))
			return nil
		}
		if fetchedSchema == nil {
			return nil
		}
		fetchedSchema.docPath = uri
		// TODO(arqu): meta validate schema
		schema = fetchedSchema
		sr.schemaLookup[uri] = schema
	}
	return schema
}

// GetKnown fetches a schema from the top level context registry
func (sr *SchemaRegistry) GetKnown(uri string) *Schema {
	uri = strings.TrimRight(uri, "#")
	return sr.schemaLookup[uri]
}

// GetLocal fetches a schema from the local context registry
func (sr *SchemaRegistry) GetLocal(uri string) *Schema {
	uri = strings.TrimRight(uri, "#")
	return sr.contextLookup[uri]
}

// Register registers a schema to the top level context
func (sr *SchemaRegistry) Register(sch *Schema) {
	if sch.docPath == "" {
		return
	}
	sr.schemaLookup[sch.docPath] = sch
}

// RegisterLocal registers a schema to a local context
func (sr *SchemaRegistry) RegisterLocal(sch *Schema) {
	if sch.id != "" && IsLocalSchemaID(sch.id) {
		sr.contextLookup[sch.id] = sch
	}

	if sch.HasKeyword("$anchor") {
		anchorKeyword := sch.keywords["$anchor"].(*Anchor)
		anchorURI := sch.docPath + "#" + string(*anchorKeyword)
		if sr.contextLookup == nil {
			sr.contextLookup = map[string]*Schema{}
		}
		sr.contextLookup[anchorURI] = sch
	}
}

func (sr *SchemaRegistry) Copy() *SchemaRegistry {
	nsr := NewSchemaRegistry()

	for key, schema := range sr.schemaLookup {
		nsr.schemaLookup[key] = schema
	}

	for key, schema := range sr.contextLookup {
		nsr.contextLookup[key] = schema
	}

	return nsr
}

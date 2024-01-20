package jsonparser

import "net/url"

type schemaLocation struct {
	rel []ReferenceToken // derived solely from the JSON document structure
	id  *ID              // derived from the "$id" set on the schema or one of its ancestors
}

// parseSchema parses the root JSON Schema, walking it recursively to record each (sub)schema's
// relative location (from the root schema).
//
// It returns a map of each (sub)schema to its relative location.
func parseSchema(root *Schema) (map[*Schema]schemaLocation, error) {
	var err error
	v := locationVisitor{
		locations: map[*Schema]schemaLocation{},
		err:       &err,
	}
	Walk(&v, root)
	return v.locations, err
}

// locationVisitor implements Visitor.
type locationVisitor struct {
	locations map[*Schema]schemaLocation
	err       *error

	location schemaLocation
}

// Visit implements Visitor.
func (v *locationVisitor) Visit(schema *Schema, rel []ReferenceToken) Visitor {
	if schema == nil || *v.err != nil {
		return nil
	}

	// TODO(sqs): Don't walk if/then/else because we're not validating, and those are usually only
	// used for validation (not for defining types).
	if len(rel) > 0 {
		if t := rel[len(rel)-1]; t.Keyword && (t.Name == "if" || t.Name == "then" || t.Name == "else") {
			return nil
		}
	}

	// Skip trivial schemas.
	//
	// TODO(sqs): The ref-to-primitive test case demonstrates a downside to this simple filter: some
	// schemas must have a description for them to be $ref'd. Make this (and/or the resolution
	// logic) smarter.
	if schema.IsEmpty || schema.IsNegated || (len(schema.Type) == 1 && schema.Description == nil && goBuiltinType(schema.Type[0]) != "") {
		return nil
	}

	w := *v // copy

	// The (sub)schema has 2 possible IDs here.

	//
	// 1. Construct the JSON key path from the root
	//
	w.location.rel = make([]ReferenceToken, len(v.location.rel)+len(rel))
	copy(w.location.rel, v.location.rel)
	copy(w.location.rel[len(v.location.rel):], rel)

	//
	// 2. Construct the id based on the "$id" set here or on one of its ancestors.
	//
	if schema.ID != nil {
		u, err := url.Parse(*schema.ID)
		if err != nil {
			*v.err = err
			return nil
		}
		// Resolve our id against our parent's base URI
		// (https://tools.ietf.org/html/draft-handrews-json-schema-01#section-8.2).
		if v.location.id != nil {
			u = v.location.id.URI().ResolveReference(u)
		}
		w.location.id = &ID{Base: u}
	} else if v.location.id != nil {
		id := v.location.id.ResolveReference(rel)
		w.location.id = &id
	}

	w.locations[schema] = w.location
	return &w
}

func goBuiltinType(typ PrimitiveType) string {
	switch typ {
	case NullType:
		return "nil"
	case BooleanType:
		return "bool"
	case NumberType:
		return "float64"
	case StringType:
		return "string"
	case IntegerType:
		return "int"
	default:
		return ""
	}
}

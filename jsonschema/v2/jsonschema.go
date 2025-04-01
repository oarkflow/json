package v2

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/brianvoe/gofakeit/v6"
	"github.com/oarkflow/date"
	"github.com/oarkflow/expr"
)

func convertValue(val any, expectedType string) (any, error) {
	switch expectedType {
	case "number":
		switch v := val.(type) {
		case string:
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil, fmt.Errorf("cannot convert %q to number: %v", v, err)
			}
			return f, nil
		default:
			return val, nil
		}
	case "integer":
		switch v := val.(type) {
		case string:
			i, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("cannot convert %q to integer: %v", v, err)
			}
			return i, nil
		default:
			return val, nil
		}
	case "boolean":
		switch v := val.(type) {
		case string:
			if v == "true" {
				return true, nil
			} else if v == "false" {
				return false, nil
			}
			return nil, fmt.Errorf("cannot convert %q to boolean", v)
		default:
			return val, nil
		}
	default:
		return val, nil
	}
}

type JSONParser struct {
	data []byte
	pos  int
}

func (p *JSONParser) skipWhitespace() {
	for p.pos < len(p.data) {
		switch p.data[p.pos] {
		case ' ', '\n', '\t', '\r':
			p.pos++
		default:
			return
		}
	}
}

func (p *JSONParser) parseValue() (any, error) {
	p.skipWhitespace()
	if p.pos >= len(p.data) {
		return nil, errors.New("unexpected end of input")
	}
	ch := p.data[p.pos]
	switch ch {
	case '{':
		return p.parseObject()
	case '[':
		return p.parseArray()
	case '"':
		return p.parseString()
	case 't':
		return p.parseLiteral("true", true)
	case 'f':
		return p.parseLiteral("false", false)
	case 'n':
		return p.parseLiteral("null", nil)
	default:
		if ch == '-' || (ch >= '0' && ch <= '9') {
			return p.parseNumber()
		}
	}
	return nil, fmt.Errorf("unexpected character '%c' at position %d", ch, p.pos)
}

func (p *JSONParser) parseLiteral(lit string, value any) (any, error) {
	end := p.pos + len(lit)
	if end > len(p.data) || string(p.data[p.pos:end]) != lit {
		return nil, fmt.Errorf("invalid literal at position %d", p.pos)
	}
	p.pos += len(lit)
	return value, nil
}

func (p *JSONParser) parseObject() (any, error) {
	obj := make(map[string]any)
	p.pos++
	p.skipWhitespace()
	if p.pos < len(p.data) && p.data[p.pos] == '}' {
		p.pos++
		return obj, nil
	}
	for {
		p.skipWhitespace()
		if p.pos >= len(p.data) || p.data[p.pos] != '"' {
			return nil, errors.New("expected string key in object")
		}
		key, err := p.parseString()
		if err != nil {
			return nil, err
		}
		p.skipWhitespace()
		if p.pos >= len(p.data) || p.data[p.pos] != ':' {
			return nil, errors.New("expected ':' after key in object")
		}
		p.pos++
		p.skipWhitespace()
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		obj[key] = value
		p.skipWhitespace()
		if p.pos < len(p.data) && p.data[p.pos] == '}' {
			p.pos++
			break
		}
		if p.pos < len(p.data) && p.data[p.pos] == ',' {
			p.pos++
			continue
		}
		return nil, errors.New("expected ',' or '}' in object")
	}
	return obj, nil
}

func (p *JSONParser) parseArray() (any, error) {
	arr := []any{}
	p.pos++
	p.skipWhitespace()
	if p.pos < len(p.data) && p.data[p.pos] == ']' {
		p.pos++
		return arr, nil
	}
	for {
		p.skipWhitespace()
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		arr = append(arr, value)
		p.skipWhitespace()
		if p.pos < len(p.data) && p.data[p.pos] == ']' {
			p.pos++
			break
		}
		if p.pos < len(p.data) && p.data[p.pos] == ',' {
			p.pos++
			continue
		}
		return nil, errors.New("expected ',' or ']' in array")
	}
	return arr, nil
}

func (p *JSONParser) parseString() (string, error) {
	if p.data[p.pos] != '"' {
		return "", errors.New("expected '\"' at beginning of string")
	}
	p.pos++
	var result []rune
	for p.pos < len(p.data) {
		ch := p.data[p.pos]
		if ch == '"' {
			p.pos++
			return string(result), nil
		}
		if ch == '\\' {
			p.pos++
			if p.pos >= len(p.data) {
				return "", errors.New("unexpected end of input in string escape")
			}
			esc := p.data[p.pos]
			if esc == 'u' {
				if p.pos+4 >= len(p.data) {
					return "", errors.New("incomplete unicode escape")
				}
				hexStr := string(p.data[p.pos+1 : p.pos+5])
				code, err := strconv.ParseInt(hexStr, 16, 32)
				if err != nil {
					return "", fmt.Errorf("invalid unicode escape: %v", err)
				}
				result = append(result, rune(code))
				p.pos += 5
				continue
			}
			switch esc {
			case '"', '\\', '/':
				result = append(result, rune(esc))
			case 'b':
				result = append(result, '\b')
			case 'f':
				result = append(result, '\f')
			case 'n':
				result = append(result, '\n')
			case 'r':
				result = append(result, '\r')
			case 't':
				result = append(result, '\t')
			default:
				return "", fmt.Errorf("invalid escape character '%c'", esc)
			}
			p.pos++
		} else {
			result = append(result, rune(ch))
			p.pos++
		}
	}
	return "", errors.New("unexpected end of string")
}

func (p *JSONParser) parseNumber() (any, error) {
	start := p.pos
	if p.data[p.pos] == '-' {
		p.pos++
	}
	for p.pos < len(p.data) && (p.data[p.pos] >= '0' && p.data[p.pos] <= '9') {
		p.pos++
	}
	if p.pos < len(p.data) && p.data[p.pos] == '.' {
		p.pos++
		for p.pos < len(p.data) && (p.data[p.pos] >= '0' && p.data[p.pos] <= '9') {
			p.pos++
		}
	}
	if p.pos < len(p.data) && (p.data[p.pos] == 'e' || p.data[p.pos] == 'E') {
		p.pos++
		if p.pos < len(p.data) && (p.data[p.pos] == '+' || p.data[p.pos] == '-') {
			p.pos++
		}
		for p.pos < len(p.data) && (p.data[p.pos] >= '0' && p.data[p.pos] <= '9') {
			p.pos++
		}
	}
	numStr := string(p.data[start:p.pos])
	if i, err := strconv.Atoi(numStr); err == nil {
		return i, nil
	}
	f, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return nil, err
	}
	return f, nil
}

var jsonParserPool = sync.Pool{
	New: func() interface{} {
		return &JSONParser{}
	},
}

func ParseJSON(data []byte) (any, error) {
	parser := jsonParserPool.Get().(*JSONParser)
	parser.data = data
	parser.pos = 0
	ret, err := parser.parseValue()

	parser.data = nil
	jsonParserPool.Put(parser)
	return ret, err
}

type SchemaType []string

func (st *SchemaType) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*st = []string{single}
		return nil
	}
	var arr []string
	if err := json.Unmarshal(data, &arr); err != nil {
		return err
	}
	*st = arr
	return nil
}

type Rat float64
type SchemaMap map[string]*Schema

// NEW: add discriminator struct
type Discriminator struct {
	PropertyName string            `json:"propertyName"`
	Mapping      map[string]string `json:"mapping,omitempty"`
}

type Schema struct {
	compiledPatterns          map[string]*regexp.Regexp
	compiler                  *Compiler
	parent                    *Schema
	anchors                   map[string]*Schema
	dynamicAnchors            map[string]*Schema
	ID                        string              `json:"$id,omitempty"`
	Schema                    string              `json:"$schema,omitempty"`
	Format                    *string             `json:"format,omitempty"`
	Ref                       string              `json:"$ref,omitempty"`
	DynamicRef                string              `json:"$dynamicRef,omitempty"`
	RecursiveRef              string              `json:"$recursiveRef,omitempty"`
	Anchor                    string              `json:"$anchor,omitempty"`
	RecursiveAnchor           bool                `json:"$recursiveAnchor,omitempty"`
	DynamicAnchor             string              `json:"$dynamicAnchor,omitempty"`
	Defs                      map[string]*Schema  `json:"$defs,omitempty"`
	Comment                   *string             `json:"$comment,omitempty"`
	Vocabulary                map[string]bool     `json:"$vocabulary,omitempty"`
	Boolean                   *bool               `json:"-"`
	AllOf                     []*Schema           `json:"allOf,omitempty"`
	AnyOf                     []*Schema           `json:"anyOf,omitempty"`
	OneOf                     []*Schema           `json:"oneOf,omitempty"`
	Not                       *Schema             `json:"not,omitempty"`
	If                        *Schema             `json:"if,omitempty"`
	Then                      *Schema             `json:"then,omitempty"`
	Else                      *Schema             `json:"else,omitempty"`
	DependentSchemas          map[string]*Schema  `json:"dependentSchemas,omitempty"`
	DependentRequired         map[string][]string `json:"dependentRequired,omitempty"`
	PrefixItems               []*Schema           `json:"prefixItems,omitempty"`
	Items                     *Schema             `json:"items,omitempty"`
	UnevaluatedItems          *Schema             `json:"unevaluatedItems,omitempty"`
	Contains                  *Schema             `json:"contains,omitempty"`
	Properties                *SchemaMap          `json:"properties,omitempty"`
	PatternProperties         *SchemaMap          `json:"patternProperties,omitempty"`
	AdditionalProperties      *Schema             `json:"additionalProperties,omitempty"`
	PropertyNames             *Schema             `json:"propertyNames,omitempty"`
	UnevaluatedProperties     *Schema             `json:"unevaluatedProperties,omitempty"`
	UnevaluatedPropertiesBool *bool               `json:"-"`
	Type                      SchemaType          `json:"type,omitempty"`
	Enum                      []any               `json:"enum,omitempty"`
	Const                     any                 `json:"const,omitempty"`
	MultipleOf                *Rat                `json:"multipleOf,omitempty"`
	Maximum                   *Rat                `json:"maximum,omitempty"`
	ExclusiveMaximum          *Rat                `json:"exclusiveMaximum,omitempty"`
	Minimum                   *Rat                `json:"minimum,omitempty"`
	ExclusiveMinimum          *Rat                `json:"exclusiveMinimum,omitempty"`
	MaxLength                 *float64            `json:"maxLength,omitempty"`
	MinLength                 *float64            `json:"minLength,omitempty"`
	Pattern                   *string             `json:"pattern,omitempty"`
	MaxItems                  *float64            `json:"maxItems,omitempty"`
	MinItems                  *float64            `json:"minItems,omitempty"`
	UniqueItems               *bool               `json:"uniqueItems,omitempty"`
	MaxContains               *float64            `json:"maxContains,omitempty"`
	MinContains               *float64            `json:"minContains,omitempty"`
	MaxProperties             *float64            `json:"maxProperties,omitempty"`
	MinProperties             *float64            `json:"minProperties,omitempty"`
	Required                  []string            `json:"required,omitempty"`
	ContentEncoding           *string             `json:"contentEncoding,omitempty"`
	ContentMediaType          *string             `json:"contentMediaType,omitempty"`
	ContentSchema             *Schema             `json:"contentSchema,omitempty"`
	Title                     *string             `json:"title,omitempty"`
	Description               *string             `json:"description,omitempty"`
	Default                   any                 `json:"default,omitempty"`
	Deprecated                *bool               `json:"deprecated,omitempty"`
	ReadOnly                  *bool               `json:"readOnly,omitempty"`
	WriteOnly                 *bool               `json:"writeOnly,omitempty"`
	Examples                  []any               `json:"examples,omitempty"`
	In                        *string             `json:"in,omitempty"`
	Field                     *string             `json:"field,omitempty"`
	// NEW: add discriminator field per 2020-12 specification
	Discriminator *Discriminator `json:"discriminator,omitempty"`
}

type Compiler struct {
	schemas map[string]*Schema
	cache   map[string]*Schema
	cacheMu sync.RWMutex
}

func NewCompiler() *Compiler {
	return &Compiler{
		schemas: make(map[string]*Schema),
		cache:   make(map[string]*Schema),
	}
}

func inferType(m map[string]any) {
	if _, exists := m["pattern"]; exists {
		if _, hasType := m["type"]; !hasType {
			m["type"] = "string"
		}
	}
	if _, exists := m["minItems"]; exists || m["maxItems"] != nil {
		if _, hasType := m["type"]; !hasType {
			m["type"] = "array"
		}
	}
	if m["minimum"] != nil || m["maximum"] != nil || m["exclusiveMinimum"] != nil || m["exclusiveMaximum"] != nil {
		if _, hasType := m["type"]; !hasType {
			m["type"] = "number"
		}
	}
}

func compileSchema(value any, compiler *Compiler, parent *Schema) (*Schema, error) {
	if b, ok := value.(bool); ok {
		return &Schema{
			Boolean:  &b,
			compiler: compiler,
			parent:   parent,
		}, nil
	}
	m, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("schema must be an object or boolean")
	}
	inferType(m)
	if defs, exists := m["definitions"]; exists && m["$defs"] == nil {
		m["$defs"] = defs
	}
	if dep, exists := m["dependencies"]; exists {
		if depMap, ok := dep.(map[string]any); ok {
			for key, val := range depMap {
				switch v := val.(type) {
				case []any:
					if m["dependentRequired"] == nil {
						m["dependentRequired"] = map[string]any{}
					}
					m["dependentRequired"].(map[string]any)[key] = v
				case map[string]any:
					if m["dependentSchemas"] == nil {
						m["dependentSchemas"] = map[string]any{}
					}
					m["dependentSchemas"].(map[string]any)[key] = v
				}
			}
		}
	}
	schema := &Schema{
		compiler:         compiler,
		parent:           parent,
		compiledPatterns: make(map[string]*regexp.Regexp),
		anchors:          make(map[string]*Schema),
		dynamicAnchors:   make(map[string]*Schema),
	}
	if id, exists := m["$id"]; exists {
		if idStr, ok := id.(string); ok {
			schema.ID = idStr
		}
	}
	if sVal, exists := m["$schema"]; exists {
		if sStr, ok := sVal.(string); ok {
			schema.Schema = sStr
		}
	}
	if format, exists := m["format"]; exists {
		if fStr, ok := format.(string); ok {
			schema.Format = &fStr
		}
	}
	if ref, exists := m["$ref"]; exists {
		if refStr, ok := ref.(string); ok {
			schema.Ref = refStr
		}
	}
	if dynRef, exists := m["$dynamicRef"]; exists {
		if dynRefStr, ok := dynRef.(string); ok {
			schema.DynamicRef = dynRefStr
		}
	}
	if recRef, exists := m["$recursiveRef"]; exists {
		if recRefStr, ok := recRef.(string); ok {
			schema.RecursiveRef = recRefStr
		}
	}
	if anchor, exists := m["$anchor"]; exists {
		if anchorStr, ok := anchor.(string); ok {
			schema.Anchor = anchorStr
			if parent != nil {
				if parent.anchors == nil {
					parent.anchors = make(map[string]*Schema)
				}
				parent.anchors[anchorStr] = schema
			}
		}
	}
	if recAnchor, exists := m["$recursiveAnchor"]; exists {
		if recAnchorBool, ok := recAnchor.(bool); ok {
			schema.RecursiveAnchor = recAnchorBool
		}
	}
	if dynAnchor, exists := m["$dynamicAnchor"]; exists {
		if dynAnchorStr, ok := dynAnchor.(string); ok {
			schema.DynamicAnchor = dynAnchorStr
			if parent != nil {
				if parent.dynamicAnchors == nil {
					parent.dynamicAnchors = make(map[string]*Schema)
				}
				parent.dynamicAnchors[dynAnchorStr] = schema
			}
		}
	}
	if comment, exists := m["$comment"]; exists {
		if commentStr, ok := comment.(string); ok {
			schema.Comment = &commentStr
		}
	}
	if vocab, exists := m["$vocabulary"]; exists {
		if vocabMap, ok := vocab.(map[string]any); ok {
			schema.Vocabulary = make(map[string]bool)
			for k, v := range vocabMap {
				if b, ok := v.(bool); ok {
					schema.Vocabulary[k] = b
				}
			}
			if err := checkVocabularyCompliance(schema); err != nil {
				return nil, err
			}
		}
	}
	if defs, exists := m["$defs"]; exists {
		if defsMap, ok := defs.(map[string]any); ok {
			schema.Defs = make(map[string]*Schema)
			for key, defVal := range defsMap {
				compiledDef, err := compileSchema(defVal, compiler, schema)
				if err != nil {
					return nil, fmt.Errorf("error compiling $defs[%s]: %v", key, err)
				}
				schema.Defs[key] = compiledDef
			}
		}
	}
	if allOf, exists := m["allOf"]; exists {
		if arr, ok := allOf.([]any); ok {
			for _, item := range arr {
				subSchema, err := compileSchema(item, compiler, schema)
				if err != nil {
					return nil, fmt.Errorf("error compiling allOf: %v", err)
				}
				schema.AllOf = append(schema.AllOf, subSchema)
			}
		}
	}
	if anyOf, exists := m["anyOf"]; exists {
		if arr, ok := anyOf.([]any); ok {
			for _, item := range arr {
				subSchema, err := compileSchema(item, compiler, schema)
				if err != nil {
					return nil, fmt.Errorf("error compiling anyOf: %v", err)
				}
				schema.AnyOf = append(schema.AnyOf, subSchema)
			}
		}
	}
	if oneOf, exists := m["oneOf"]; exists {
		if arr, ok := oneOf.([]any); ok {
			for _, item := range arr {
				subSchema, err := compileSchema(item, compiler, schema)
				if err != nil {
					return nil, fmt.Errorf("error compiling oneOf: %v", err)
				}
				schema.OneOf = append(schema.OneOf, subSchema)
			}
		}
	}
	if not, exists := m["not"]; exists {
		subSchema, err := compileSchema(not, compiler, schema)
		if err != nil {
			return nil, fmt.Errorf("error compiling not: %v", err)
		}
		schema.Not = subSchema
	}
	if ifVal, exists := m["if"]; exists {
		subSchema, err := compileSchema(ifVal, compiler, schema)
		if err != nil {
			return nil, fmt.Errorf("error compiling if: %v", err)
		}
		schema.If = subSchema
	}
	if thenVal, exists := m["then"]; exists {
		subSchema, err := compileSchema(thenVal, compiler, schema)
		if err != nil {
			return nil, fmt.Errorf("error compiling then: %v", err)
		}
		schema.Then = subSchema
	}
	if elseVal, exists := m["else"]; exists {
		subSchema, err := compileSchema(elseVal, compiler, schema)
		if err != nil {
			return nil, fmt.Errorf("error compiling else: %v", err)
		}
		schema.Else = subSchema
	}
	if depSchemas, exists := m["dependentSchemas"]; exists {
		if depMap, ok := depSchemas.(map[string]any); ok {
			schema.DependentSchemas = make(map[string]*Schema)
			for key, depVal := range depMap {
				subSchema, err := compileSchema(depVal, compiler, schema)
				if err != nil {
					return nil, fmt.Errorf("error compiling dependentSchemas[%s]: %v", key, err)
				}
				schema.DependentSchemas[key] = subSchema
			}
		}
	}
	if depReq, exists := m["dependentRequired"]; exists {
		if depMap, ok := depReq.(map[string]any); ok {
			schema.DependentRequired = make(map[string][]string)
			for key, val := range depMap {
				if arr, ok := val.([]any); ok {
					for _, item := range arr {
						if str, ok := item.(string); ok {
							schema.DependentRequired[key] = append(schema.DependentRequired[key], str)
						}
					}
				}
			}
		}
	}
	if prefixItems, exists := m["prefixItems"]; exists {
		if arr, ok := prefixItems.([]any); ok {
			for _, item := range arr {
				subSchema, err := compileSchema(item, compiler, schema)
				if err != nil {
					return nil, fmt.Errorf("error compiling prefixItems: %v", err)
				}
				schema.PrefixItems = append(schema.PrefixItems, subSchema)
			}
		}
	}
	if items, exists := m["items"]; exists {
		subSchema, err := compileSchema(items, compiler, schema)
		if err != nil {
			return nil, fmt.Errorf("error compiling items: %v", err)
		}
		schema.Items = subSchema
	}
	if unevaluatedItems, exists := m["unevaluatedItems"]; exists {
		subSchema, err := compileSchema(unevaluatedItems, compiler, schema)
		if err != nil {
			return nil, fmt.Errorf("error compiling unevaluatedItems: %v", err)
		}
		schema.UnevaluatedItems = subSchema
	}
	if contains, exists := m["contains"]; exists {
		subSchema, err := compileSchema(contains, compiler, schema)
		if err != nil {
			return nil, fmt.Errorf("error compiling contains: %v", err)
		}
		schema.Contains = subSchema
	}
	if props, exists := m["properties"]; exists {
		if propsMap, ok := props.(map[string]any); ok {
			sMap := SchemaMap{}
			for key, propVal := range propsMap {
				subSchema, err := compileSchema(propVal, compiler, schema)
				if err != nil {
					return nil, fmt.Errorf("error compiling properties[%s]: %v", key, err)
				}
				sMap[key] = subSchema
			}
			schema.Properties = &sMap
		}
	}

	if schema.Properties != nil {
		for key, prop := range *schema.Properties {
			if prop.In != nil && *prop.In != "" {
				found := false
				for _, req := range schema.Required {
					if req == key {
						found = true
						break
					}
				}
				if !found {
					schema.Required = append(schema.Required, key)
				}
			}
		}
	}

	if ifVal, ok := m["if"]; ok {
		if thenVal, ok2 := m["then"]; ok2 {

			if ifMap, ok := ifVal.(map[string]any); ok {
				if reqArr, ok := ifMap["required"].([]any); ok {
					for _, reqKey := range reqArr {
						if key, ok := reqKey.(string); ok {

							if thenMap, ok := thenVal.(map[string]any); ok {
								if thenProps, ok := thenMap["properties"].(map[string]any); ok {
									if mod, exists := thenProps[key]; exists {
										if modMap, ok := mod.(map[string]any); ok {
											if reqFields, ok := modMap["required"].([]any); ok {

												if schema.Properties != nil {
													if propSchema, exists := (*schema.Properties)[key]; exists {
														for _, field := range reqFields {
															if strField, ok := field.(string); ok {
																found := false
																for _, r := range propSchema.Required {
																	if r == strField {
																		found = true
																		break
																	}
																}
																if !found {
																	propSchema.Required = append(propSchema.Required, strField)
																}
															}
														}
													}
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	if patProps, exists := m["patternProperties"]; exists {
		if patMap, ok := patProps.(map[string]any); ok {
			sMap := SchemaMap{}
			for pattern, patVal := range patMap {
				subSchema, err := compileSchema(patVal, compiler, schema)
				if err != nil {
					return nil, fmt.Errorf("error compiling patternProperties[%s]: %v", pattern, err)
				}
				sMap[pattern] = subSchema
				re, err := regexp.Compile(pattern)
				if err != nil {
					return nil, fmt.Errorf("invalid pattern regex '%s': %v", pattern, err)
				}
				schema.compiledPatterns[pattern] = re
			}
			schema.PatternProperties = &sMap
		}
	}
	if addProps, exists := m["additionalProperties"]; exists {
		subSchema, err := compileSchema(addProps, compiler, schema)
		if err != nil {
			return nil, fmt.Errorf("error compiling additionalProperties: %v", err)
		}
		schema.AdditionalProperties = subSchema
	}
	if propNames, exists := m["propertyNames"]; exists {
		subSchema, err := compileSchema(propNames, compiler, schema)
		if err != nil {
			return nil, fmt.Errorf("error compiling propertyNames: %v", err)
		}
		schema.PropertyNames = subSchema
	}
	if up, exists := m["unevaluatedProperties"]; exists {
		switch v := up.(type) {
		case bool:
			schema.UnevaluatedPropertiesBool = &v
		default:
			subSchema, err := compileSchema(v, compiler, schema)
			if err != nil {
				return nil, fmt.Errorf("error compiling unevaluatedProperties: %v", err)
			}
			schema.UnevaluatedProperties = subSchema
		}
	}
	if t, exists := m["type"]; exists {
		switch v := t.(type) {
		case string:
			schema.Type = SchemaType{v}
		case []any:
			var types []string
			for _, elem := range v {
				if str, ok := elem.(string); ok {
					types = append(types, str)
				}
			}
			for _, typ := range types {
				if typ == "array" {
					schema.Type = SchemaType{typ}
					goto TypeDone
				}
			}
			schema.Type = SchemaType(types)
		}
	TypeDone:
	}
	if schema.Pattern != nil && len(schema.Type) == 0 {
		schema.Type = SchemaType{"string"}
	}
	if enumVal, exists := m["enum"]; exists {
		if enumArr, ok := enumVal.([]any); ok {
			schema.Enum = enumArr
		}
	}
	if constVal, exists := m["const"]; exists {
		schema.Const = constVal
	}
	if multOf, exists := m["multipleOf"]; exists {
		if num, ok := toFloat(multOf); ok {
			r := Rat(num)
			schema.MultipleOf = &r
		}
	}
	if max, exists := m["maximum"]; exists {
		if num, ok := toFloat(max); ok {
			r := Rat(num)
			schema.Maximum = &r
		}
	}
	if exMax, exists := m["exclusiveMaximum"]; exists {
		if num, ok := toFloat(exMax); ok {
			r := Rat(num)
			schema.ExclusiveMaximum = &r
		}
	}
	if min, exists := m["minimum"]; exists {
		if num, ok := toFloat(min); ok {
			r := Rat(num)
			schema.Minimum = &r
		}
	}
	if exMin, exists := m["exclusiveMinimum"]; exists {
		if num, ok := toFloat(exMin); ok {
			r := Rat(num)
			schema.ExclusiveMinimum = &r
		}
	}
	if maxLen, exists := m["maxLength"]; exists {
		if num, ok := toFloat(maxLen); ok {
			schema.MaxLength = &num
		}
	}
	if minLen, exists := m["minLength"]; exists {
		if num, ok := toFloat(minLen); ok {
			schema.MinLength = &num
		}
	}
	if pattern, exists := m["pattern"]; exists {
		if patStr, ok := pattern.(string); ok {
			schema.Pattern = &patStr
			re, err := regexp.Compile(patStr)
			if err != nil {
				return nil, fmt.Errorf("invalid pattern regex '%s': %v", patStr, err)
			}
			schema.compiledPatterns[patStr] = re
		}
	}
	if maxItems, exists := m["maxItems"]; exists {
		if num, ok := toFloat(maxItems); ok {
			schema.MaxItems = &num
		}
	}
	if minItems, exists := m["minItems"]; exists {
		if num, ok := toFloat(minItems); ok {
			schema.MinItems = &num
		}
	}
	if unique, exists := m["uniqueItems"]; exists {
		if b, ok := unique.(bool); ok {
			schema.UniqueItems = &b
		}
	}
	if maxContains, exists := m["maxContains"]; exists {
		if num, ok := toFloat(maxContains); ok {
			schema.MaxContains = &num
		}
	}
	if minContains, exists := m["minContains"]; exists {
		if num, ok := toFloat(minContains); ok {
			schema.MinContains = &num
		}
	}
	if maxProps, exists := m["maxProperties"]; exists {
		if num, ok := toFloat(maxProps); ok {
			schema.MaxProperties = &num
		}
	}
	if minProps, exists := m["minProperties"]; exists {
		if num, ok := toFloat(minProps); ok {
			schema.MinProperties = &num
		}
	}
	if req, exists := m["required"]; exists {
		if arr, ok := req.([]any); ok {
			for _, item := range arr {
				if str, ok := item.(string); ok {
					schema.Required = append(schema.Required, str)
				}
			}
		}
	}
	if depReq, exists := m["dependentRequired"]; exists {
		if depMap, ok := depReq.(map[string]any); ok {
			schema.DependentRequired = make(map[string][]string)
			for key, val := range depMap {
				if arr, ok := val.([]any); ok {
					for _, item := range arr {
						if str, ok := item.(string); ok {
							schema.DependentRequired[key] = append(schema.DependentRequired[key], str)
						}
					}
				}
			}
		}
	}
	if contentEnc, exists := m["contentEncoding"]; exists {
		if s, ok := contentEnc.(string); ok {
			schema.ContentEncoding = &s
		}
	}
	if contentMedia, exists := m["contentMediaType"]; exists {
		if s, ok := contentMedia.(string); ok {
			schema.ContentMediaType = &s
		}
	}
	if contentSchema, exists := m["contentSchema"]; exists {
		subSchema, err := compileSchema(contentSchema, compiler, schema)
		if err != nil {
			return nil, fmt.Errorf("error compiling contentSchema: %v", err)
		}
		schema.ContentSchema = subSchema
	}
	if title, exists := m["title"]; exists {
		if s, ok := title.(string); ok {
			schema.Title = &s
		}
	}
	if desc, exists := m["description"]; exists {
		if s, ok := desc.(string); ok {
			schema.Description = &s
		}
	}
	if def, exists := m["default"]; exists {
		d, err := prepareDefault(def)
		if err != nil {
			return nil, err
		}
		schema.Default = d
	}
	if dep, exists := m["deprecated"]; exists {
		if b, ok := dep.(bool); ok {
			schema.Deprecated = &b
		}
	}
	if readOnly, exists := m["readOnly"]; exists {
		if b, ok := readOnly.(bool); ok {
			schema.ReadOnly = &b
		}
	}
	if writeOnly, exists := m["writeOnly"]; exists {
		if b, ok := writeOnly.(bool); ok {
			schema.WriteOnly = &b
		}
	}
	if examples, exists := m["examples"]; exists {
		if arr, ok := examples.([]any); ok {
			schema.Examples = arr
		}
	}

	if inVal, exists := m["in"]; exists {
		if inStr, ok := inVal.(string); ok {
			schema.In = &inStr
		}
	}
	if fieldVal, exists := m["field"]; exists {
		if fieldStr, ok := fieldVal.(string); ok {
			schema.Field = &fieldStr
		}
	}
	if m["minimum"] != nil || m["maximum"] != nil || m["exclusiveMinimum"] != nil || m["exclusiveMaximum"] != nil {
		schema.Type = SchemaType{"number"}
	}
	if schema.Properties != nil {
		schema.Type = SchemaType{"object"}
	}
	if schema.ID != "" {
		compiler.schemas[schema.ID] = schema
	}
	if len(schema.Type) == 0 {
		var unionTypes []string
		if len(schema.OneOf) > 0 {
			for _, candidate := range schema.OneOf {
				if len(candidate.Type) > 0 {
					for _, t := range candidate.Type {
						if !slices.Contains(unionTypes, t) {
							unionTypes = append(unionTypes, t)
						}
					}
				}
			}
		}
		if len(unionTypes) == 0 && len(schema.AnyOf) > 0 {
			for _, candidate := range schema.AnyOf {
				if len(candidate.Type) > 0 {
					for _, t := range candidate.Type {
						if !slices.Contains(unionTypes, t) {
							unionTypes = append(unionTypes, t)
						}
					}
				}
			}
		}
		if len(unionTypes) > 0 {
			schema.Type = SchemaType(unionTypes)
		} else if schema.Properties != nil || (schema.If != nil || schema.Then != nil || schema.Else != nil) {
			schema.Type = SchemaType{"object"}
		}
	}
	if err := compileDraft2020Keywords(m, schema); err != nil {
		return nil, err
	}
	if err := selfValidateSchema(schema); err != nil {
		return nil, fmt.Errorf("schema self‑validation failed: %w", err)
	}
	// At the end, before "if err := compileDraft2020Keywords..."
	if disc, exists := m["discriminator"]; exists {
		if d, ok := disc.(map[string]any); ok {
			prop, ok := d["propertyName"].(string)
			if !ok || prop == "" {
				return nil, errors.New("discriminator: propertyName must be a non-empty string")
			}
			mapping := make(map[string]string)
			if mapp, ok := d["mapping"].(map[string]any); ok {
				for k, v := range mapp {
					if str, ok := v.(string); ok {
						mapping[k] = str
					}
				}
			}
			schema.Discriminator = &Discriminator{
				PropertyName: prop,
				Mapping:      mapping,
			}
		} else {
			return nil, errors.New("discriminator must be an object")
		}
	}
	return schema, nil
}

// NEW: add helper function for discriminator-based validation
func validateDiscriminator(instance any, s *Schema) error {
	obj, ok := instance.(map[string]any)
	if !ok {
		return errors.New("discriminator: instance is not an object")
	}
	discVal, ok := obj[s.Discriminator.PropertyName]
	if !ok {
		return fmt.Errorf("discriminator: property '%s' not found", s.Discriminator.PropertyName)
	}
	discStr, ok := discVal.(string)
	if !ok {
		return fmt.Errorf("discriminator: property '%s' is not a string", s.Discriminator.PropertyName)
	}
	var candidate *Schema
	// If mapping provided, use it to select candidate from oneOf
	if len(s.Discriminator.Mapping) > 0 {
		if ref, exists := s.Discriminator.Mapping[discStr]; exists {
			for _, cand := range s.OneOf {
				if cand.ID == ref || cand.Ref == ref {
					candidate = cand
					break
				}
			}
			if candidate == nil {
				return fmt.Errorf("discriminator: no candidate schema found with reference %q", ref)
			}
		} else {
			return fmt.Errorf("discriminator: no mapping defined for value %q", discStr)
		}
	} else {
		// Otherwise, try to pick exactly one candidate that validates the instance.
		validCount := 0
		for _, cand := range s.OneOf {
			if err := cand.Validate(instance); err == nil {
				candidate = cand
				validCount++
			}
		}
		if validCount != 1 {
			return fmt.Errorf("discriminator: expected exactly one valid candidate, got %d", validCount)
		}
	}
	if err := candidate.Validate(instance); err != nil {
		return fmt.Errorf("discriminator: candidate schema validation failed: %v", err)
	}
	return nil
}

var remoteCache = struct {
	sync.RWMutex
	schemas map[string]*Schema
}{schemas: make(map[string]*Schema)}

func compileDraft2020Keywords(m map[string]any, schema *Schema) error {
	if recAnchor, exists := m["$recursiveAnchor"]; exists {
		if recBool, ok := recAnchor.(bool); ok {
			schema.RecursiveAnchor = recBool
		} else {
			return fmt.Errorf("$recursiveAnchor must be a boolean")
		}
	}
	if vocab, exists := m["$vocabulary"]; exists {
		if vocabMap, ok := vocab.(map[string]any); ok {
			schema.Vocabulary = make(map[string]bool)
			for k, v := range vocabMap {
				if b, ok := v.(bool); ok {
					schema.Vocabulary[k] = b
				} else {
					return fmt.Errorf("vocabulary value for '%s' must be a boolean", k)
				}
			}
		} else {
			return fmt.Errorf("$vocabulary must be an object")
		}
	}
	return nil
}

var bufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

func canonicalize(v any) (string, error) {
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	if err := canonicalizeToBuffer(buf, v); err != nil {
		bufferPool.Put(buf)
		return "", err
	}
	result := buf.String()
	bufferPool.Put(buf)
	return result, nil
}

func canonicalizeToBuffer(buf *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case map[string]any:
		buf.WriteByte('{')

		var keys []string
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}

			b, err := json.Marshal(k)
			if err != nil {
				return err
			}
			buf.Write(b)
			buf.WriteByte(':')
			if err := canonicalizeToBuffer(buf, t[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	case []any:
		buf.WriteByte('[')
		for i, elem := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := canonicalizeToBuffer(buf, elem); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	default:

		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		buf.Write(b)
	}
	return nil
}

func computeCacheKey(v any) (string, error) {
	canonical, err := canonicalize(v)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(h[:]), nil
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	}
	return 0, false
}

var formatValidators = map[string]func(string) error{
	"email": func(value string) error {
		_, err := mail.ParseAddress(value)
		if err != nil {
			return fmt.Errorf("invalid email: %v", err)
		}
		return nil
	},
	"uri": func(value string) error {
		u, err := url.Parse(value)
		if err != nil || u.Scheme == "" {
			return fmt.Errorf("invalid URI")
		}
		return nil
	},
	"uri-reference": func(value string) error {
		_, err := url.Parse(value)
		if err != nil {
			return fmt.Errorf("invalid URI reference: %v", err)
		}
		return nil
	},
	"date": func(value string) error {
		_, err := date.Parse(value)
		if err != nil {
			return fmt.Errorf("invalid date: %v", err)
		}
		return nil
	},
	"date-time": func(value string) error {
		if _, err := time.Parse(time.RFC3339, value); err != nil {
			return fmt.Errorf("invalid date-time: %v", err)
		}
		return nil
	},
	"ipv4": func(value string) error {
		if net.ParseIP(value) == nil || strings.Contains(value, ":") {
			return fmt.Errorf("invalid IPv4 address")
		}
		return nil
	},
	"ipv6": func(value string) error {
		if net.ParseIP(value) == nil || !strings.Contains(value, ":") {
			return fmt.Errorf("invalid IPv6 address")
		}
		return nil
	},
	"hostname": func(value string) error {
		if len(value) == 0 || len(value) > 253 {
			return fmt.Errorf("invalid hostname length")
		}
		matched, err := regexp.MatchString(`^(?:(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}|localhost)$`, value)
		if err != nil || !matched {
			return fmt.Errorf("invalid hostname")
		}
		return nil
	},
	"uuid": func(value string) error {
		matched, err := regexp.MatchString(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`, value)
		if err != nil || !matched {
			return fmt.Errorf("invalid UUID")
		}
		return nil
	},
	"json-pointer": func(value string) error {
		if value != "" && !strings.HasPrefix(value, "/") {
			return fmt.Errorf("invalid JSON pointer")
		}
		return nil
	},
	"relative-json-pointer": func(value string) error {
		matched, err := regexp.MatchString(`^\d+(?:#(?:\/.*)?)?$`, value)
		if err != nil || !matched {
			return fmt.Errorf("invalid relative JSON pointer")
		}
		return nil
	},
}

func RegisterFormatValidator(name string, validator func(string) error) {
	formatValidators[name] = validator
}

func validateFormat(format, value string) error {
	if fn, ok := formatValidators[format]; ok {
		return fn(value)
	}
	return nil
}

var httpClient = &http.Client{
	Timeout: 5 * time.Second,
}

func (s *Schema) resolveRemoteRef(ref string) (*Schema, error) {
	remoteCache.RLock()
	if cached, ok := remoteCache.schemas[ref]; ok {
		remoteCache.RUnlock()
		return cached, nil
	}
	remoteCache.RUnlock()
	if s.compiler != nil {
		if schema, exists := s.compiler.schemas[ref]; exists {
			return schema, nil
		}
	}
	u, err := url.Parse(ref)
	if err != nil || u.Scheme == "" {
		return nil, fmt.Errorf("invalid remote reference '%s'", ref)
	}
	resp, err := httpClient.Get(ref)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch remote schema from '%s': %v", ref, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read remote schema from '%s': %v", ref, err)
	}
	remoteSchema, err := s.compiler.Compile(body)
	if err != nil {
		return nil, fmt.Errorf("error compiling remote schema from '%s': %v", ref, err)
	}
	remoteCache.Lock()
	remoteCache.schemas[ref] = remoteSchema
	remoteCache.Unlock()
	return remoteSchema, nil
}

func (c *Compiler) Compile(data []byte) (*Schema, error) {
	var tmp any
	if err := json.Unmarshal(data, &tmp); err != nil {
		return nil, err
	}
	key, err := computeCacheKey(tmp)
	if err != nil {
		return nil, err
	}
	c.cacheMu.RLock()
	if schema, ok := c.cache[key]; ok {
		c.cacheMu.RUnlock()
		return schema, nil
	}
	c.cacheMu.RUnlock()
	parsed, err := ParseJSON(data)
	if err != nil {
		return nil, err
	}
	s, err := compileSchema(parsed, c, nil)
	if err != nil {
		return nil, err
	}
	c.cacheMu.Lock()
	c.cache[key] = s
	c.cacheMu.Unlock()
	return s, nil
}

func (s *Schema) resolveRef(ref string) (*Schema, error) {
	if strings.HasPrefix(ref, "#") {
		anchor := strings.TrimPrefix(ref, "#")
		for cur := s; cur != nil; cur = cur.parent {
			if cur.Anchor == anchor {
				return cur, nil
			}
		}
		return nil, fmt.Errorf("unable to resolve reference '%s'", ref)
	}
	return s.resolveRemoteRef(ref)
}

func (s *Schema) resolveRecursiveRef(ref string) (*Schema, error) {
	if ref == "#" {
		for cur := s.parent; cur != nil; cur = cur.parent {
			if cur.RecursiveAnchor {
				return cur, nil
			}
		}
		return nil, fmt.Errorf("unable to resolve recursive reference '#'")
	}
	return s.resolveRef(ref)
}

func (s *Schema) resolveDynamicRef(ref string) (*Schema, error) {
	if !strings.HasPrefix(ref, "#") {
		return nil, fmt.Errorf("invalid dynamic reference '%s'", ref)
	}
	anchor := strings.TrimPrefix(ref, "#")
	if dyn := s.findDynamicAnchor(anchor); dyn != nil {
		return dyn, nil
	}
	return nil, fmt.Errorf("unable to resolve dynamic reference '%s'", ref)
}

func (s *Schema) findDynamicAnchor(anchor string) *Schema {
	if s.dynamicAnchors != nil {
		if schema, ok := s.dynamicAnchors[anchor]; ok {
			return schema
		}
	}
	for cur := s.parent; cur != nil; cur = cur.parent {
		if cur.dynamicAnchors != nil {
			if schema, ok := cur.dynamicAnchors[anchor]; ok {
				return schema
			}
		}
	}
	return nil
}

func (s *Schema) prepareData(data any) (any, error) {
	switch data := data.(type) {
	case map[string]any, []map[string]any, []any, float64, bool, nil:
		return data, nil
	case string:
		return data, nil
	default:
		b, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}
		var v any
		err = json.Unmarshal(b, &v)
		if err != nil {
			return nil, err
		}
		return v, nil
	}
}

func (s *Schema) validateAsType(candidate string, data any) error {
	switch candidate {
	case "object":
		var obj map[string]any
		switch d := data.(type) {
		case map[string]any:
			obj = d
		case string:
			if s.ContentEncoding != nil {
				decodedBytes, err := base64.StdEncoding.DecodeString(d)
				if err != nil {
					return fmt.Errorf("object branch: base64 decoding failed: %v", err)
				}
				err = json.Unmarshal(decodedBytes, &obj)
				if err != nil {
					return fmt.Errorf("object branch: unmarshal failed: %v", err)
				}
			} else {
				return fmt.Errorf("expected object, got string")
			}
		default:
			return fmt.Errorf("object branch: expected object, got %T", data)
		}
		return nil
	case "array":
		if _, ok := data.([]any); !ok {
			return fmt.Errorf("expected array, got %T", data)
		}
		return nil
	case "string":
		if s.ContentEncoding != nil {
			str, ok := data.(string)
			if !ok {
				return fmt.Errorf("expected string, got %T", data)
			}
			if _, err := base64.StdEncoding.DecodeString(str); err != nil {
				return fmt.Errorf("invalid base64 encoding: %v", err)
			}
			if s.ContentMediaType != nil && *s.ContentMediaType == "application/json" {
				decoded, err := base64.StdEncoding.DecodeString(str)
				if err != nil {
					return fmt.Errorf("base64 decode error: %v", err)
				}
				var tmp any
				if err := json.Unmarshal(decoded, &tmp); err != nil {
					return fmt.Errorf("decoded value is not valid JSON: %v", err)
				}
			}
			return nil
		}
		switch data.(type) {
		case string:
			return nil
		case time.Time:
			if s.Format != nil && *s.Format == "date-time" {
				return nil
			}
			return fmt.Errorf("expected string, got time.Time")
		default:
			return fmt.Errorf("expected string, got %T", data)
		}
	case "integer":
		switch v := data.(type) {
		case int:
		case float64:
			if v != float64(int(v)) {
				return errors.New("expected integer, got non-integer number")
			}
		case string:
			if _, err := strconv.Atoi(v); err != nil {
				return errors.New("expected integer, got non-integer string")
			}
		default:
			return fmt.Errorf("expected integer, got %T", data)
		}
		return nil
	case "number":
		switch d := data.(type) {
		case float64, int:
		case string:
			if _, err := strconv.ParseFloat(d, 64); err != nil {
				return errors.New("expected number, got non-number string")
			}
		default:
			return fmt.Errorf("expected number, got %T", data)
		}
		return nil
	case "boolean":
		if _, ok := data.(bool); !ok {
			return errors.New("expected boolean")
		}
		return nil
	case "null":
		if data != nil {
			return errors.New("expected null")
		}
		return nil
	default:
		return fmt.Errorf("unsupported type candidate: %s", candidate)
	}
}

func validateSimpleConstraints(data any, s *Schema) error {

	if str, ok := data.(string); ok {
		if s.MaxLength != nil && float64(len(str)) > *s.MaxLength {
			return fmt.Errorf("string length %d exceeds maxLength %v", len(str), *s.MaxLength)
		}
		if s.MinLength != nil && float64(len(str)) < *s.MinLength {
			return fmt.Errorf("string length %d is less than minLength %v", len(str), *s.MinLength)
		}
	}
	if arr, ok := data.([]any); ok {
		if s.MaxItems != nil && float64(len(arr)) > *s.MaxItems {
			return fmt.Errorf("array has %d items exceeding maxItems %v", len(arr), *s.MaxItems)
		}
		if s.MinItems != nil && float64(len(arr)) < *s.MinItems {
			return fmt.Errorf("array has %d items fewer than minItems %v", len(arr), *s.MinItems)
		}
	}
	if obj, ok := data.(map[string]any); ok {
		if s.MaxProperties != nil && float64(len(obj)) > *s.MaxProperties {
			return fmt.Errorf("object has %d properties exceeding maxProperties %v", len(obj), *s.MaxProperties)
		}
		if s.MinProperties != nil && float64(len(obj)) < *s.MinProperties {
			return fmt.Errorf("object has %d properties fewer than minProperties %v", len(obj), *s.MinProperties)
		}
	}

	if s.Pattern != nil {
		if str, ok := data.(string); ok {
			re, ok := s.compiledPatterns[*s.Pattern]
			if !ok {
				var err error
				re, err = regexp.Compile(*s.Pattern)
				if err != nil {
					return fmt.Errorf("invalid pattern: %v", err)
				}
				s.compiledPatterns[*s.Pattern] = re
			}
			if !re.MatchString(str) {
				return fmt.Errorf("value %q does not match pattern %q", str, *s.Pattern)
			}
		}
	}
	if len(s.Enum) > 0 {
		found := slices.Contains(s.Enum, data)
		if !found {
			return fmt.Errorf("value %v not in enum %v", data, s.Enum)
		}
	}
	if s.Const != nil {
		if s.Const != data {
			return fmt.Errorf("value %v is not equal to const %v", data, s.Const)
		}
	}
	if s.Format != nil {
		if str, ok := data.(string); ok {
			if err := validateFormat(*s.Format, str); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Schema) ValidateWithPath(unprepared any, instancePath string) error {
	data, err := s.prepareData(unprepared)
	if err != nil {
		return fmt.Errorf("at %s: failed to prepare data: %w", instancePath, err)
	}
	if obj, ok := data.(map[string]any); ok {
		for _, field := range s.Required {
			if _, exists := obj[field]; !exists {
				if s.Properties != nil {
					if propSchema, ok := (*s.Properties)[field]; ok && propSchema.Default != nil {
						continue
					}
				}
				return fmt.Errorf("at %s: missing required field '%s'", instancePath, field)
			}
		}
	}
	if err := validateApplicatorKeywords(data, s); err != nil {
		return fmt.Errorf("at %s: %w", instancePath, err)
	}
	if s.Contains != nil {
		if err := validateContains(data, s.Contains); err != nil {
			return fmt.Errorf("at %s: contains validation error: %w", instancePath, err)
		}
	}
	if len(s.Type) == 0 && s.Properties != nil {
		s.Type = SchemaType{"object"}
	}
	var candidateErrors []error
	validCandidateCount := 0
	for _, candidate := range s.Type {
		if err := s.validateAsType(candidate, data); err != nil {
			candidateErrors = append(candidateErrors, fmt.Errorf("[%s]: %v", candidate, err))
			continue
		}
		if err := validateSimpleConstraints(data, s); err != nil {
			candidateErrors = append(candidateErrors, fmt.Errorf("[%s] simple constraints: %v", candidate, err))
			continue
		}
		if candidate == "object" {
			if obj, ok := data.(map[string]any); ok && s.Properties != nil {
				for key, propSchema := range *s.Properties {
					if val, exists := obj[key]; exists {
						if err := propSchema.Validate(val); err != nil {
							candidateErrors = append(candidateErrors, fmt.Errorf("property '%s' validation failed: %v", key, err))
							goto NextCandidate
						}
					}
				}
			}
		}
		validCandidateCount++
	NextCandidate:
	}
	if validCandidateCount < 1 {
		return fmt.Errorf("at %s: data does not match any candidate types %v. Details: %v", instancePath, s.Type, candidateErrors)
	}
	return nil
}

func (s *Schema) Validate(unprepared any) error {
	return s.ValidateWithPath(unprepared, "/")
}

func (s *Schema) validateType(tp string, data any) (bool, any, error) {
	switch tp {
	case "string":
		if s.ContentEncoding != nil {
			str, ok := data.(string)
			if !ok {
				return false, nil, fmt.Errorf("expected string, got %T", data)
			}
			decoded, err := base64.StdEncoding.DecodeString(str)
			if err != nil {
				return false, nil, fmt.Errorf("invalid base64 encoding: %v", err)
			}
			if s.ContentMediaType != nil && *s.ContentMediaType == "application/json" {
				var tmp any
				if err := json.Unmarshal(decoded, &tmp); err != nil {
					return false, nil, fmt.Errorf("decoded value is not valid JSON: %v", err)
				}
				return true, tmp, nil
			}
			return true, string(decoded), nil
		}
		switch data.(type) {
		case string:
			return true, data, nil
		case time.Time:
			if s.Format != nil && *s.Format == "date-time" {
				return true, data, nil
			}
			return false, nil, fmt.Errorf("expected string, got time.Time")
		default:
			return false, nil, fmt.Errorf("expected string, got %T", data)
		}
	case "object":
		var obj map[string]any
		switch d := data.(type) {
		case map[string]any:
			obj = d
		case string:
			if s.ContentEncoding != nil {
				decodedBytes, err := base64.StdEncoding.DecodeString(d)
				if err != nil {
					return false, nil, fmt.Errorf("object branch: base64 decoding failed: %v", err)
				}
				err = json.Unmarshal(decodedBytes, &obj)
				if err != nil {
					return false, nil, fmt.Errorf("object branch: unmarshal failed: %v", err)
				}
			} else {
				return false, nil, fmt.Errorf("expected object, got string")
			}
		default:
			return false, nil, fmt.Errorf("object branch: expected object, got %T", data)
		}
		newObj := make(map[string]any)
		if s.Properties != nil {
			for key, propSchema := range *s.Properties {
				if v, exists := obj[key]; exists {
					merged, err := propSchema.Unmarshal(v)
					if err != nil {
						return false, nil, fmt.Errorf("error unmarshalling property '%s': %v", key, err)
					}
					newObj[key] = merged
				} else if propSchema.Default != nil {
					merged, err := propSchema.Unmarshal(propSchema.Default)
					if err != nil {
						return false, nil, fmt.Errorf("error unmarshalling property '%s' with default: %v", key, err)
					}
					newObj[key] = merged
				}
			}
		}
		if s.AdditionalProperties != nil {
			for key, val := range obj {
				if s.Properties != nil {
					if _, exists := (*s.Properties)[key]; exists {
						continue
					}
				}
				merged, err := s.AdditionalProperties.Unmarshal(val)
				if err != nil {
					return false, nil, fmt.Errorf("error unmarshalling additional property '%s': %v", key, err)
				}
				newObj[key] = merged
			}
		} else {
			for key, val := range obj {
				if s.Properties != nil {
					if _, exists := (*s.Properties)[key]; exists {
						continue
					}
				}
				newObj[key] = val
			}
		}
		return true, newObj, nil
	case "array":
		arr, ok := data.([]any)
		if !ok {
			return false, nil, errors.New("expected array for unmarshalling")
		}
		newArr := make([]any, len(arr))
		evaluatedIndexes := make(map[int]bool)
		if len(s.PrefixItems) > 0 {
			for i, prefixSchema := range s.PrefixItems {
				if i < len(arr) {
					merged, err := prefixSchema.Unmarshal(arr[i])
					if err != nil {
						return false, nil, fmt.Errorf("error unmarshalling prefixItems at index %d: %v", i, err)
					}
					newArr[i] = merged
					evaluatedIndexes[i] = true
				}
			}
		}
		if s.Items != nil {
			for i := len(s.PrefixItems); i < len(arr); i++ {
				merged, err := s.Items.Unmarshal(arr[i])
				if err != nil {
					return false, nil, fmt.Errorf("error unmarshalling items at index %d: %v", i, err)
				}
				newArr[i] = merged
				evaluatedIndexes[i] = true
			}
		}
		if s.UnevaluatedItems != nil {
			for i := 0; i < len(arr); i++ {
				if !evaluatedIndexes[i] {
					merged, err := s.UnevaluatedItems.Unmarshal(arr[i])
					if err != nil {
						return false, nil, fmt.Errorf("error unmarshalling unevaluatedItems at index %d: %v", i, err)
					}
					newArr[i] = merged
				}
			}
		}
		for i := 0; i < len(arr); i++ {
			if newArr[i] == nil {
				newArr[i] = arr[i]
			}
		}
		return true, newArr, nil
	default:
		if data == nil && s.Default != nil {
			return true, s.Default, nil
		}
		return true, data, nil
	}
}

func (s *Schema) Unmarshal(unprepared any) (any, error) {
	data, err := s.prepareData(unprepared)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare data: %w", err)
	}
	if data == nil && s.Default != nil {
		data = s.Default
	}
	if s.Boolean != nil {
		if *s.Boolean {
			return data, nil
		}
		return nil, errors.New("schema is false; no instance is valid")
	}
	if s.Ref != "" {
		refSchema, err := s.resolveRef(s.Ref)
		if err != nil {
			return nil, err
		}
		return refSchema.Unmarshal(data)
	}
	if s.DynamicRef != "" {
		dynSchema, err := enhancedResolveDynamicRef(s, s.DynamicRef)
		if err != nil {
			return nil, err
		}
		return dynSchema.Unmarshal(data)
	}
	if s.RecursiveRef != "" {
		recSchema, err := enhancedResolveRecursiveRef(s, s.RecursiveRef)
		if err != nil {
			return nil, err
		}
		return recSchema.Unmarshal(data)
	}
	if data == nil && s.Default != nil {
		data = s.Default
	}
	if s.ContentEncoding != nil {
		if err := validateContentEncoding(data, *s.ContentEncoding); err != nil {
			return nil, err
		}
	}
	if s.ContentMediaType != nil {
		if err := validateContentMediaType(data, *s.ContentMediaType); err != nil {
			return nil, err
		}
	}
	for _, candidate := range s.Type {
		isValid, d, _ := s.validateType(candidate, data)
		if isValid {
			return d, nil
		}
	}
	return nil, fmt.Errorf("data does not match any candidate types: %v", s.Type)
}

func (s *Schema) SmartUnmarshal(data any) (any, error) {
	if err := s.Validate(data); err != nil {
		return nil, fmt.Errorf("validation failed: %v", err)
	}
	return s.Unmarshal(data)
}

func (s *Schema) GenerateExample() (any, error) {
	if len(s.Examples) > 0 {
		return s.Examples[0], nil
	}
	if s.Default != nil {
		return s.Default, nil
	}
	var tp string
	if len(s.Type) > 0 {
		tp = s.Type[0]
	}
	switch tp {
	case "object":
		obj := map[string]any{}
		if s.Properties != nil {
			for key, propSchema := range *s.Properties {
				sample, err := propSchema.GenerateExample()
				if err != nil {
					continue
				}
				obj[key] = sample
			}
		}
		return obj, nil
	case "array":
		if s.Items != nil {
			sample, err := s.Items.GenerateExample()
			if err != nil {
				return nil, err
			}
			return []any{sample}, nil
		}
		return []any{}, nil
	case "string":
		if s.Format != nil && *s.Format == "email" {
			return gofakeit.Email(), nil
		}
		return gofakeit.Word(), nil
	case "number":
		return gofakeit.Float64Range(1, 100), nil
	case "boolean":
		return gofakeit.Bool(), nil
	case "null":
		return nil, nil
	default:
		return nil, fmt.Errorf("cannot generate example for unknown type: %v", s.Type)
	}
}

func evaluateExpression(exprStr string) (any, error) {
	if strings.HasPrefix(exprStr, "{{") && strings.HasSuffix(exprStr, "}}") {
		jsonStr := strings.ReplaceAll(exprStr, "'", "\"")
		var m any
		if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
			return nil, err
		}
		return m, nil
	}
	vm, err := expr.Parse(exprStr)
	if err != nil {
		return nil, err
	}
	return vm.Eval(nil)
}

func prepareDefault(def any) (any, error) {
	if def == nil {
		return nil, nil
	}
	defStr, ok := def.(string)
	if !ok {
		return def, nil
	}
	if strings.HasPrefix(defStr, "{{") && strings.HasSuffix(defStr, "}}") {
		trimmed := strings.TrimPrefix(defStr, "{{")
		trimmed = strings.TrimSuffix(trimmed, "}}")
		return evaluateExpression(trimmed)
	}
	return def, nil
}

func Unmarshal(data []byte, dest any, schemaBytes ...[]byte) error {
	if len(schemaBytes) == 0 {
		return json.Unmarshal(data, dest)
	}
	compiler := NewCompiler()
	schema, err := compiler.Compile(schemaBytes[0])
	if err != nil {
		return fmt.Errorf("failed to compile schema: %v", err)
	}
	var intermediate any
	if err := json.Unmarshal(data, &intermediate); err != nil {
		return fmt.Errorf("failed to unmarshal into intermediate: %v", err)
	}
	merged, err := schema.SmartUnmarshal(intermediate)
	if err != nil {
		return fmt.Errorf("failed SmartUnmarshal: %v", err)
	}

	if mDest, ok := dest.(*map[string]any); ok {
		if m, ok := merged.(map[string]any); ok {
			*mDest = m
			return nil
		}
	}
	switch v := merged.(type) {
	case string:
		if ptr, ok := dest.(*string); ok {
			*ptr = v
			return nil
		}
	case float64:
		if ptr, ok := dest.(*float64); ok {
			*ptr = v
			return nil
		}
		if ptr, ok := dest.(*int); ok {
			*ptr = int(v)
			return nil
		}
	case bool:
		if ptr, ok := dest.(*bool); ok {
			*ptr = v
			return nil
		}
	case []any:
		if ptr, ok := dest.(*[]any); ok {
			*ptr = v
			return nil
		}
	}
	mergedBytes, err := json.Marshal(merged)
	if err != nil {
		return fmt.Errorf("failed to marshal merged result: %v", err)
	}
	if err := json.Unmarshal(mergedBytes, dest); err != nil {
		return fmt.Errorf("failed to unmarshal merged bytes into dest: %v", err)
	}
	return nil
}

var vocabularyValidators = map[string]func(schema *Schema) error{}

func RegisterVocabularyValidator(name string, validator func(schema *Schema) error) {
	vocabularyValidators[name] = validator
}

func checkVocabularyCompliance(schema *Schema) error {
	for vocab, enabled := range schema.Vocabulary {
		if enabled {
			if validator, ok := vocabularyValidators[vocab]; ok {
				if err := validator(schema); err != nil {
					return fmt.Errorf("vocabulary '%s' validation failed: %v", vocab, err)
				}
			}
		}
	}
	return nil
}

func validateSubschemas(keyword string, subschemas []*Schema, instance any) error {
	var errs []string
	validCount := 0
	for i, sub := range subschemas {
		if err := sub.Validate(instance); err != nil {
			errs = append(errs, fmt.Sprintf("%s[%d]: %v", keyword, i, err))
		} else {
			validCount++
		}
	}
	switch keyword {
	case "allOf":
		if len(errs) > 0 {
			return fmt.Errorf("allOf validation errors: %s", strings.Join(errs, "; "))
		}
	case "anyOf":
		if validCount < 1 {
			return fmt.Errorf("anyOf failed: %s", strings.Join(errs, "; "))
		}
	case "oneOf":
		if validCount != 1 {
			return fmt.Errorf("oneOf failed: expected exactly 1 valid schema, got %d; details: %s", validCount, strings.Join(errs, "; "))
		}
	}
	return nil
}

func validateApplicatorKeywords(instance any, s *Schema) error {
	var errs []string
	if err := validateSubschemas("allOf", s.AllOf, instance); err != nil {
		errs = append(errs, err.Error())
	}
	if len(s.AnyOf) > 0 {
		if err := validateSubschemas("anyOf", s.AnyOf, instance); err != nil {
			errs = append(errs, err.Error())
		}
	}
	// NEW: if discriminator is set in a oneOf then use it
	if len(s.OneOf) > 0 {
		if s.Discriminator != nil {
			if err := validateDiscriminator(instance, s); err != nil {
				errs = append(errs, err.Error())
			}
		} else {
			if err := validateSubschemas("oneOf", s.OneOf, instance); err != nil {
				errs = append(errs, err.Error())
			}
		}
	}
	if s.Not != nil {
		if err := s.Not.Validate(instance); err == nil {
			errs = append(errs, "not failed: instance must not match the 'not' schema")
		}
	}
	if s.If != nil {
		if err := s.If.Validate(instance); err == nil {
			if s.Then != nil {
				if err := s.Then.Validate(instance); err != nil {
					errs = append(errs, fmt.Sprintf("then failed: %v", err))
				}
			}
		} else if s.Else != nil {
			if err := s.Else.Validate(instance); err != nil {
				errs = append(errs, fmt.Sprintf("else failed: %v", err))
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("applicator validation errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

func validateContentEncoding(instance any, encoding string) error {
	if encoding == "base64" {
		if str, ok := instance.(string); ok {
			if _, err := base64.StdEncoding.DecodeString(str); err != nil {
				return fmt.Errorf("contentEncoding 'base64' failed: %v", err)
			}
		}
	}
	return nil
}

func validateContentMediaType(instance any, mediaType string) error {
	if mediaType == "application/json" {
		if str, ok := instance.(string); ok {
			if decoded, err := base64.StdEncoding.DecodeString(str); err == nil {
				var dummy any
				if err := json.Unmarshal(decoded, &dummy); err != nil {
					return fmt.Errorf("contentMediaType 'application/json' failed after base64 decode: %v", err)
				}
				return nil
			}
			var dummy any
			if err := json.Unmarshal([]byte(str), &dummy); err != nil {
				return fmt.Errorf("contentMediaType 'application/json' failed: %v", err)
			}
		}
	}
	return nil
}

func validateContains(instance any, containsSchema *Schema) error {
	arr, ok := instance.([]any)
	if !ok {
		return nil
	}
	var found bool
	var errs []string
	for i, item := range arr {
		if err := containsSchema.Validate(item); err == nil {
			found = true
			break
		} else {
			errs = append(errs, fmt.Sprintf("contains[%d]: %v", i, err))
		}
	}
	if !found {
		return fmt.Errorf("contains validation failed; no item matches: %s", strings.Join(errs, "; "))
	}
	return nil
}

func annotateError(path string, err error) error {
	return fmt.Errorf("at %s: %w", path, err)
}

func enhancedResolveDynamicRef(s *Schema, ref string) (*Schema, error) {
	dyn, err := s.resolveDynamicRef(ref)
	if err != nil {
		return nil, annotateError(ref, fmt.Errorf("dynamic ref resolution failed: %w", err))
	}
	return dyn, nil
}

func enhancedResolveRecursiveRef(s *Schema, ref string) (*Schema, error) {
	rec, err := s.resolveRecursiveRef(ref)
	if err != nil {
		return nil, annotateError(ref, fmt.Errorf("recursive ref resolution failed: %w", err))
	}
	return rec, nil
}

func selfValidateSchema(schema *Schema) error {
	if schema.Vocabulary != nil {
		if enabled, ok := schema.Vocabulary["https://json-schema.org/draft/2020-12/vocab/meta-data"]; ok && enabled {
			if schema.Title != nil && *schema.Title == "" {
				return errors.New("meta-data: title must not be empty")
			}
		}
	}
	return nil
}

func getNestedValue(m map[string]any, field string) (any, bool) {
	parts := strings.Split(field, ".")
	var value any = m
	for _, part := range parts {
		obj, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok = obj[part]
		if !ok {
			return nil, false
		}
	}
	return value, true
}

func extractDataFromRequest(r *http.Request, in string, field *string) (any, error) {
	switch strings.ToLower(in) {
	case "query":
		m := map[string]any{}
		for k, v := range r.URL.Query() {
			if len(v) == 1 {
				m[k] = v[0]
			} else {
				m[k] = v
			}
		}
		if field != nil && *field != "" {
			if strings.Contains(*field, ".") {
				if val, exists := getNestedValue(m, *field); exists {
					return val, nil
				}
			} else {
				if val, exists := m[*field]; exists {
					return val, nil
				}
			}
			return nil, fmt.Errorf("field %q not found in query", *field)
		}
		return m, nil
	case "params":
		if params, ok := r.Context().Value("params").(map[string]string); ok {
			m := map[string]any{}
			for k, v := range params {
				m[k] = v
			}
			if field != nil && *field != "" {
				if strings.Contains(*field, ".") {
					if val, exists := getNestedValue(m, *field); exists {
						return val, nil
					}
				} else {
					if val, exists := m[*field]; exists {
						return val, nil
					}
				}
				return nil, fmt.Errorf("field %q not found in params", *field)
			}
			return m, nil
		}
		return nil, fmt.Errorf("no params found in context")
	case "header":
		m := map[string]any{}
		for k, v := range r.Header {
			if len(v) == 1 {
				m[k] = v[0]
			} else {
				m[k] = v
			}
		}
		if field != nil && *field != "" {
			if strings.Contains(*field, ".") {
				if val, exists := getNestedValue(m, *field); exists {
					return val, nil
				}
			} else {
				if val, exists := m[*field]; exists {
					return val, nil
				}
			}
			return nil, fmt.Errorf("field %q not found in header", *field)
		}
		return m, nil
	case "body":
		fallthrough
	default:
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %v", err)
		}
		r.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))
		var data any
		if err := json.Unmarshal(bodyBytes, &data); err != nil {
			return nil, fmt.Errorf("failed to unmarshal request body: %v", err)
		}
		if field != nil && *field != "" {
			if m, ok := data.(map[string]any); ok {
				if strings.Contains(*field, ".") {
					if val, exists := getNestedValue(m, *field); exists {
						return val, nil
					}
				} else {
					if val, exists := m[*field]; exists {
						return val, nil
					}
				}
				return nil, fmt.Errorf("field %q not found in body", *field)
			}
			return nil, fmt.Errorf("request body is not a JSON object")
		}
		return data, nil
	}
}

func (s *Schema) UnmarshalRequest(r *http.Request, dest any) error {
	var data any
	if len(s.Type) == 1 && s.Type[0] == "object" && s.Properties != nil {
		bodyData, err := extractDataFromRequest(r, "body", nil)
		if err != nil {
			bodyData = map[string]any{}
		}
		m, ok := bodyData.(map[string]any)
		if !ok {
			m = map[string]any{}
		}
		overrideFromRequest(r, m, s)
		data = m
	} else {
		in := "body"
		if s.In != nil && *s.In != "" {
			in = *s.In
		}
		var err error
		data, err = extractDataFromRequest(r, in, s.Field)
		if err != nil {
			return err
		}
	}
	merged, err := s.SmartUnmarshal(data)
	if err != nil {
		return fmt.Errorf("validation failed: %v", err)
	}
	mergedBytes, err := json.Marshal(merged)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(mergedBytes, dest); err != nil {
		return err
	}
	return nil
}

func overrideFromRequest(r *http.Request, data map[string]any, schema *Schema) {
	for key, propSchema := range *schema.Properties {
		if propSchema.In != nil && *propSchema.In != "body" {
			fieldName := key
			if propSchema.Field != nil && *propSchema.Field != "" {
				fieldName = *propSchema.Field
			}
			if val, err := extractDataFromRequest(r, *propSchema.In, &fieldName); err == nil {
				if len(propSchema.Type) > 0 {
					if conv, err := convertValue(val, propSchema.Type[0]); err == nil {
						data[key] = conv
					}
				} else {
					data[key] = val
				}
			}
		}
		if len(propSchema.Type) > 0 && propSchema.Type[0] == "object" && propSchema.Properties != nil {
			nested, ok := data[key].(map[string]any)
			if !ok {
				nested = map[string]any{}
				data[key] = nested
			}
			overrideFromRequest(r, nested, propSchema)
		}
	}
}

func UnmarshalAndValidateRequest(r *http.Request, dest any, schemaBytes []byte) error {
	compiler := NewCompiler()
	schema, err := compiler.Compile(schemaBytes)
	if err != nil {
		return fmt.Errorf("failed to compile schema: %v", err)
	}
	return schema.UnmarshalRequest(r, dest)
}

type Ctx interface {
	Params(key string) string
	Query(key string) string
	Body() []byte
	Get(key string) string
	BodyParser(dest interface{}) error
}

func extractDataFromFiberCtx(ctx Ctx, in string, field *string) (any, error) {
	switch strings.ToLower(in) {
	case "header":
		if field != nil && *field != "" {
			return ctx.Get(*field), nil
		}
		return nil, errors.New("for header extraction, a field name must be provided")
	case "query":
		if field != nil && *field != "" {
			val := ctx.Query(*field)
			if val != "" {
				return val, nil
			}
			var m map[string]any
			if err := json.Unmarshal([]byte(ctx.Query("")), &m); err == nil && strings.Contains(*field, ".") {
				if v, exists := getNestedValue(m, *field); exists {
					return v, nil
				}
			}
			return nil, errors.New("for query extraction, a valid field name must be provided")
		}
		return nil, errors.New("for query extraction, a field name must be provided")
	case "params":
		if field != nil && *field != "" {
			val := ctx.Params(*field)
			if val != "" {
				return val, nil
			}
			var m map[string]any
			if strings.Contains(*field, ".") {
				if v, exists := getNestedValue(m, *field); exists {
					return v, nil
				}
			}
			return nil, errors.New("for params extraction, a valid field name must be provided")
		}
		return nil, errors.New("for params extraction, a field name must be provided")
	case "body":
		fallthrough
	default:
		if field != nil && *field != "" {
			var m map[string]any
			if err := ctx.BodyParser(&m); err != nil {
				return nil, fmt.Errorf("failed to parse body: %v", err)
			}
			if strings.Contains(*field, ".") {
				if val, exists := getNestedValue(m, *field); exists {
					return val, nil
				}
			} else {
				if val, exists := m[*field]; exists {
					return val, nil
				}
			}
			return nil, fmt.Errorf("field %q not found in body", *field)
		}
		var data any
		bodyBytes := ctx.Body()
		if err := json.Unmarshal(bodyBytes, &data); err != nil {
			return nil, fmt.Errorf("failed to unmarshal body: %v", err)
		}
		return data, nil
	}
}

func (s *Schema) UnmarshalFiberCtx(ctx Ctx, dest any) error {
	var data any
	if len(s.Type) == 1 && s.Type[0] == "object" && s.Properties != nil {
		bodyData, err := extractDataFromFiberCtx(ctx, "body", nil)
		if err != nil {
			bodyData = map[string]any{}
		}
		m, ok := bodyData.(map[string]any)
		if !ok {
			m = map[string]any{}
		}
		overrideFromFiberCtx(ctx, m, s)
		data = m
	} else {
		in := "body"
		if s.In != nil && *s.In != "" {
			in = *s.In
		}
		var err error
		data, err = extractDataFromFiberCtx(ctx, in, s.Field)
		if err != nil {
			return err
		}
	}
	merged, err := s.SmartUnmarshal(data)
	if err != nil {
		return fmt.Errorf("validation failed: %v", err)
	}
	mergedBytes, err := json.Marshal(merged)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(mergedBytes, dest); err != nil {
		return err
	}
	return nil
}

func overrideFromFiberCtx(ctx Ctx, data map[string]any, schema *Schema) {
	for key, propSchema := range *schema.Properties {
		if propSchema.In != nil && *propSchema.In != "body" {
			fieldName := key
			if propSchema.Field != nil && *propSchema.Field != "" {
				fieldName = *propSchema.Field
			}
			if val, err := extractDataFromFiberCtx(ctx, *propSchema.In, &fieldName); err == nil {
				if len(propSchema.Type) > 0 {
					if conv, err := convertValue(val, propSchema.Type[0]); err == nil {
						data[key] = conv
					}
				} else {
					data[key] = val
				}
			}
		}
		if len(propSchema.Type) > 0 && propSchema.Type[0] == "object" && propSchema.Properties != nil {
			nested, ok := data[key].(map[string]any)
			if !ok {
				nested = map[string]any{}
				data[key] = nested
			}
			overrideFromFiberCtx(ctx, nested, propSchema)
		}
	}
}

package v2

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/brianvoe/gofakeit/v6"
	"github.com/oarkflow/expr"
)

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

func ParseJSON(data []byte) (any, error) {
	parser := JSONParser{data: data, pos: 0}
	return parser.parseValue()
}

type SchemaType string
type Rat float64
type SchemaMap map[string]*Schema

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
}

type Compiler struct {
	schemas map[string]*Schema
}

func NewCompiler() *Compiler {
	return &Compiler{schemas: make(map[string]*Schema)}
}

func (c *Compiler) CompileSchema(data []byte) (*Schema, error) {
	parsed, err := ParseJSON(data)
	if err != nil {
		return nil, err
	}
	return compileSchema(parsed, c, nil)
}

var remoteCache = struct {
	sync.RWMutex
	schemas map[string]*Schema
}{schemas: make(map[string]*Schema)}

func compileDraft2020Keywords(m map[string]any, schema *Schema, compiler *Compiler) error {
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
		if typeStr, ok := t.(string); ok {
			schema.Type = SchemaType(typeStr)
		}
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
	if schema.Type == "" && (m["minimum"] != nil || m["maximum"] != nil || m["exclusiveMinimum"] != nil || m["exclusiveMaximum"] != nil) {
		schema.Type = "number"
	}
	if schema.Type == "" && schema.Properties != nil {
		schema.Type = "object"
	}
	if schema.ID != "" {
		compiler.schemas[schema.ID] = schema
	}
	if err := compileDraft2020Keywords(m, schema, compiler); err != nil {
		return nil, err
	}
	return schema, nil
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
		_, err := time.Parse("2006-01-02", value)
		if err != nil {
			return fmt.Errorf("invalid date: %v", err)
		}
		return nil
	},
	"date-time": func(value string) error {
		_, err := time.Parse(time.RFC3339, value)
		if err != nil {
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
	resp, err := http.Get(ref)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch remote schema from '%s': %v", ref, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read remote schema from '%s': %v", ref, err)
	}
	remoteSchema, err := s.compiler.CompileSchema(body)
	if err != nil {
		return nil, fmt.Errorf("error compiling remote schema from '%s': %v", ref, err)
	}
	remoteCache.Lock()
	remoteCache.schemas[ref] = remoteSchema
	remoteCache.Unlock()
	return remoteSchema, nil
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

func prepareData(data any) (any, error) {
	switch data.(type) {
	case map[string]any, []map[string]any, []any, string, float64, bool, nil:
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

func (s *Schema) Validate(data any) error {
	prepared, err := prepareData(data)
	if err != nil {
		return fmt.Errorf("failed to prepare data: %v", err)
	}
	data = prepared
	if s.Boolean != nil {
		if *s.Boolean {
			return nil
		}
		return errors.New("schema is false; no instance is valid")
	}
	if s.Ref != "" {
		refSchema, err := s.resolveRef(s.Ref)
		if err != nil {
			return err
		}
		return refSchema.Validate(data)
	}
	if s.DynamicRef != "" {
		dynSchema, err := s.resolveDynamicRef(s.DynamicRef)
		if err != nil {
			return err
		}
		return dynSchema.Validate(data)
	}
	if s.RecursiveRef != "" {
		recSchema, err := s.resolveRecursiveRef(s.RecursiveRef)
		if err != nil {
			return err
		}
		return recSchema.Validate(data)
	}
	for _, subschema := range s.AllOf {
		if err := subschema.Validate(data); err != nil {
			return fmt.Errorf("allOf validation failed: %v", err)
		}
	}
	if len(s.AnyOf) > 0 {
		valid := false
		for _, subschema := range s.AnyOf {
			if err := subschema.Validate(data); err == nil {
				valid = true
				break
			}
		}
		if !valid {
			return errors.New("anyOf validation failed")
		}
	}
	if len(s.OneOf) > 0 {
		count := 0
		for _, subschema := range s.OneOf {
			if err := subschema.Validate(data); err == nil {
				count++
			}
		}
		if count != 1 {
			return errors.New("oneOf validation failed")
		}
	}
	if s.Not != nil {
		if err := s.Not.Validate(data); err == nil {
			return errors.New("not validation failed: instance should not be valid")
		}
	}
	if s.If != nil {
		if err := s.If.Validate(data); err == nil {
			if s.Then != nil {
				if err := s.Then.Validate(data); err != nil {
					return fmt.Errorf("then validation failed: %v", err)
				}
			}
		} else {
			if s.Else != nil {
				if err := s.Else.Validate(data); err != nil {
					return fmt.Errorf("else validation failed: %v", err)
				}
			}
		}
	}
	if obj, ok := data.(map[string]any); ok {
		for _, req := range s.Required {
			if _, exists := obj[req]; !exists {
				return fmt.Errorf("required property '%s' missing", req)
			}
		}
	}
	switch s.Type {
	case "object":
		obj, ok := data.(map[string]any)
		if !ok {
			return errors.New("expected object")
		}
		if s.PropertyNames != nil {
			for key := range obj {
				if err := s.PropertyNames.Validate(key); err != nil {
					return fmt.Errorf("propertyNames validation failed for key '%s': %v", key, err)
				}
			}
		}
		if s.Properties != nil {
			for key, propSchema := range *s.Properties {
				if val, exists := obj[key]; exists {
					if err := propSchema.Validate(val); err != nil {
						return fmt.Errorf("property '%s' validation failed: %v", key, err)
					}
				}
			}
		}
		if s.PatternProperties != nil {
			for pattern, patSchema := range *s.PatternProperties {
				re, exists := s.compiledPatterns[pattern]
				if !exists {
					continue
				}
				for key, val := range obj {
					if re.MatchString(key) {
						if err := patSchema.Validate(val); err != nil {
							return fmt.Errorf("pattern property '%s' validation failed: %v", key, err)
						}
					}
				}
			}
		}
		if s.DependentRequired != nil {
			for key, reqList := range s.DependentRequired {
				if _, exists := obj[key]; exists {
					for _, reqProp := range reqList {
						if _, exists := obj[reqProp]; !exists {
							return fmt.Errorf("dependentRequired: property '%s' is required when '%s' is present", reqProp, key)
						}
					}
				}
			}
		}
		if s.DependentSchemas != nil {
			for key, depSchema := range s.DependentSchemas {
				if _, exists := obj[key]; exists {
					if err := depSchema.Validate(data); err != nil {
						return fmt.Errorf("dependentSchemas: instance does not satisfy schema for '%s': %v", key, err)
					}
				}
			}
		}
		if s.UnevaluatedPropertiesBool != nil && !*s.UnevaluatedPropertiesBool {
			evaluated := make(map[string]bool)
			if s.Properties != nil {
				for key := range *s.Properties {
					evaluated[key] = true
				}
			}
			if s.PatternProperties != nil {
				for pattern := range *s.PatternProperties {
					re, exists := s.compiledPatterns[pattern]
					if exists {
						for key := range obj {
							if re.MatchString(key) {
								evaluated[key] = true
							}
						}
					}
				}
			}
			if s.AdditionalProperties != nil {
				for key, val := range obj {
					if !evaluated[key] {
						if err := s.AdditionalProperties.Validate(val); err == nil {
							evaluated[key] = true
						}
					}
				}
			}
			for key := range obj {
				if !evaluated[key] {
					return fmt.Errorf("unevaluated property '%s' is not allowed", key)
				}
			}
		}
	case "array":
		arr, ok := data.([]any)
		if !ok {
			return errors.New("expected array")
		}
		evaluatedIndexes := make(map[int]bool)
		if len(s.PrefixItems) > 0 {
			for i, prefixSchema := range s.PrefixItems {
				if i < len(arr) {
					if err := prefixSchema.Validate(arr[i]); err != nil {
						return fmt.Errorf("prefixItems index %d validation failed: %v", i, err)
					}
					evaluatedIndexes[i] = true
				}
			}
		}
		if s.Items != nil {
			for i := len(s.PrefixItems); i < len(arr); i++ {
				if err := s.Items.Validate(arr[i]); err != nil {
					return fmt.Errorf("items validation failed at index %d: %v", i, err)
				}
				evaluatedIndexes[i] = true
			}
		}
		if s.UnevaluatedItems != nil {
			for i := 0; i < len(arr); i++ {
				if !evaluatedIndexes[i] {
					if err := s.UnevaluatedItems.Validate(arr[i]); err != nil {
						return fmt.Errorf("unevaluatedItems validation failed at index %d: %v", i, err)
					}
				}
			}
		}
		if s.UniqueItems != nil && *s.UniqueItems {
			seen := make(map[string]bool)
			for i, item := range arr {
				key := fmt.Sprintf("%v", item)
				if seen[key] {
					return fmt.Errorf("array has duplicate items at index %d", i)
				}
				seen[key] = true
			}
		}
	case "string":
		str, ok := data.(string)
		if !ok {
			return errors.New("expected string")
		}
		if s.MinLength != nil && float64(len(str)) < *s.MinLength {
			return errors.New("string is shorter than minLength")
		}
		if s.MaxLength != nil && float64(len(str)) > *s.MaxLength {
			return errors.New("string is longer than maxLength")
		}
		if s.Pattern != nil {
			re, exists := s.compiledPatterns[*s.Pattern]
			if !exists {
				var err error
				re, err = regexp.Compile(*s.Pattern)
				if err != nil {
					return fmt.Errorf("invalid pattern: %v", err)
				}
				s.compiledPatterns[*s.Pattern] = re
			}
			if !re.MatchString(str) {
				return fmt.Errorf("string '%s' does not match pattern %s", str, *s.Pattern)
			}
		}
		if s.Format != nil {
			if err := validateFormat(*s.Format, str); err != nil {
				return err
			}
		}
		if s.ContentEncoding != nil {
			if *s.ContentEncoding == "base64" {
				decoded, err := base64.StdEncoding.DecodeString(str)
				if err != nil {
					return fmt.Errorf("contentEncoding validation failed: %v", err)
				}
				if s.ContentSchema != nil && s.ContentMediaType != nil && *s.ContentMediaType == "application/json" {
					var decodedJSON any
					if err := json.Unmarshal(decoded, &decodedJSON); err != nil {
						return fmt.Errorf("contentSchema validation failed: decoded content is not valid JSON: %v", err)
					}
					if err := s.ContentSchema.Validate(decodedJSON); err != nil {
						return fmt.Errorf("contentSchema validation failed: %v", err)
					}
				}
			}
		}
	case "integer":
		var num int
		switch v := data.(type) {
		case int:
			num = v
		case float64:
			if v != float64(int(v)) {
				return errors.New("expected integer, but got non-integer number")
			}
			num = int(v)
		default:
			return errors.New("expected integer")
		}
		fnum := float64(num)
		if s.Minimum != nil && fnum < float64(*s.Minimum) {
			return errors.New("number is less than minimum")
		}
		if s.Maximum != nil && fnum > float64(*s.Maximum) {
			return errors.New("number is greater than maximum")
		}
		if s.ExclusiveMinimum != nil && fnum <= float64(*s.ExclusiveMinimum) {
			return errors.New("number is not greater than exclusiveMinimum")
		}
		if s.ExclusiveMaximum != nil && fnum >= float64(*s.ExclusiveMaximum) {
			return errors.New("number is not less than exclusiveMaximum")
		}
	case "number":
		var num float64
		switch v := data.(type) {
		case float64:
			num = v
		case int:
			num = float64(v)
		default:
			return errors.New("expected number")
		}
		if s.Minimum != nil && num < float64(*s.Minimum) {
			return errors.New("number is less than minimum")
		}
		if s.Maximum != nil && num > float64(*s.Maximum) {
			return errors.New("number is greater than maximum")
		}
		if s.ExclusiveMinimum != nil && num <= float64(*s.ExclusiveMinimum) {
			return errors.New("number is not greater than exclusiveMinimum")
		}
		if s.ExclusiveMaximum != nil && num >= float64(*s.ExclusiveMaximum) {
			return errors.New("number is not less than exclusiveMaximum")
		}
	case "boolean":
		if _, ok := data.(bool); !ok {
			return errors.New("expected boolean")
		}
	case "null":
		if data != nil {
			return errors.New("expected null")
		}
	}
	if s.Enum != nil {
		found := false
		for _, enumVal := range s.Enum {
			if fmt.Sprintf("%v", enumVal) == fmt.Sprintf("%v", data) {
				found = true
				break
			}
		}
		if !found {
			return errors.New("value not in enum")
		}
	}
	if s.Const != nil {
		if fmt.Sprintf("%v", s.Const) != fmt.Sprintf("%v", data) {
			return errors.New("value does not match const")
		}
	}
	return nil
}

func (s *Schema) Unmarshal(data any) (any, error) {
	prepared, err := prepareData(data)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare data: %v", err)
	}
	data = prepared
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
		dynSchema, err := s.resolveDynamicRef(s.DynamicRef)
		if err != nil {
			return nil, err
		}
		return dynSchema.Unmarshal(data)
	}
	if s.RecursiveRef != "" {
		recSchema, err := s.resolveRecursiveRef(s.RecursiveRef)
		if err != nil {
			return nil, err
		}
		return recSchema.Unmarshal(data)
	}
	if data == nil && s.Default != nil {
		data = s.Default
	}
	switch s.Type {
	case "object":
		obj, ok := data.(map[string]any)
		if !ok {
			return nil, errors.New("expected object for unmarshalling")
		}
		newObj := make(map[string]any)
		if s.Properties != nil {
			for key, propSchema := range *s.Properties {
				var val any
				if v, exists := obj[key]; exists {
					val = v
				} else if propSchema.Default != nil {
					val = propSchema.Default
				}
				merged, err := propSchema.Unmarshal(val)
				if err != nil {
					return nil, fmt.Errorf("error unmarshalling property '%s': %v", key, err)
				}
				newObj[key] = merged
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
					return nil, fmt.Errorf("error unmarshalling additional property '%s': %v", key, err)
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
		return newObj, nil
	case "array":
		arr, ok := data.([]any)
		if !ok {
			return nil, errors.New("expected array for unmarshalling")
		}
		newArr := make([]any, len(arr))
		evaluatedIndexes := make(map[int]bool)
		if len(s.PrefixItems) > 0 {
			for i, prefixSchema := range s.PrefixItems {
				if i < len(arr) {
					merged, err := prefixSchema.Unmarshal(arr[i])
					if err != nil {
						return nil, fmt.Errorf("error unmarshalling prefixItems at index %d: %v", i, err)
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
					return nil, fmt.Errorf("error unmarshalling items at index %d: %v", i, err)
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
						return nil, fmt.Errorf("error unmarshalling unevaluatedItems at index %d: %v", i, err)
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
		return newArr, nil
	default:
		if data == nil && s.Default != nil {
			return s.Default, nil
		}
		return data, nil
	}
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
	switch s.Type {
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
		return nil, fmt.Errorf("cannot generate example for unknown type: %s", s.Type)
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

// NEW: isExpression returns true if the string looks like an expression.
func isExpression(s string) bool {
	// For example, if it contains both "(" and ")" we treat it as an expression.
	return strings.Contains(s, "(") && strings.Contains(s, ")")
}

// NEW: applyDefault checks the schema.Default. If it is a string expression, it evaluates it.
func prepareDefault(def any) (any, error) {
	if def == nil {
		return nil, nil
	}
	defStr, ok := def.(string)
	if !ok {
		return def, nil
	}
	defStr = strings.TrimPrefix(defStr, "{{")
	defStr = strings.TrimSuffix(defStr, "}}")
	if isExpression(defStr) {
		return evaluateExpression(defStr)
	}
	return def, nil
}

func Unmarshal(data []byte, dest any, schemaBytes ...[]byte) error {
	if len(schemaBytes) == 0 {

		return json.Unmarshal(data, dest)
	}
	compiler := NewCompiler()
	schema, err := compiler.CompileSchema(schemaBytes[0])
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
	mergedBytes, err := json.Marshal(merged)
	if err != nil {
		return fmt.Errorf("failed to marshal merged result: %v", err)
	}
	if err := json.Unmarshal(mergedBytes, dest); err != nil {
		return fmt.Errorf("failed to unmarshal merged bytes into dest: %v", err)
	}
	return nil
}

package v2

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/brianvoe/gofakeit/v6"
)

var regexCache sync.Map

func getCompiledRegex(pattern string) (*regexp.Regexp, error) {
	if v, ok := regexCache.Load(pattern); ok {
		return v.(*regexp.Regexp), nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	regexCache.Store(pattern, re)
	return re, nil
}

func parseJSONPointer(ptr string) ([]string, error) {
	if ptr == "" {
		return nil, errors.New("empty JSON pointer")
	}
	if ptr[0] != '/' {
		return nil, errors.New("JSON pointer must start with '/'")
	}
	parts := strings.Split(ptr[1:], "/")

	for i, token := range parts {
		token = strings.ReplaceAll(token, "~1", "/")
		token = strings.ReplaceAll(token, "~0", "~")
		parts[i] = token
	}
	return parts, nil
}

var nestedKeysCache sync.Map

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
	In                        []string            `json:"in,omitempty"`
	Field                     *string             `json:"field,omitempty"`
	Discriminator             *Discriminator      `json:"discriminator,omitempty"`
}

var remoteCache = struct {
	sync.RWMutex
	schemas map[string]*Schema
}{schemas: make(map[string]*Schema)}

func compileDraft2020Keywords(m map[string]any, schema *Schema) error {
	var draft string = "2020-12"
	if schema.compiler != nil && schema.compiler.Options != nil {
		draft = schema.compiler.Options.DraftVersion
	}
	switch draft {
	case "2020-12":
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
	case "2019-09":

		return nil
	default:
		return fmt.Errorf("unsupported JSON Schema draft version: %s", draft)
	}
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

func (s *Schema) resolveLocalRef(ptr string) (*Schema, error) {
	pointer := strings.TrimPrefix(ptr, "#")
	if pointer == "" {
		return s, nil
	}
	if pointer[0] != '/' {
		return nil, fmt.Errorf("invalid JSON pointer %q", pointer)
	}
	tokens, err := parseJSONPointer(pointer)
	if err != nil {
		return nil, fmt.Errorf("error parsing JSON pointer: %v", err)
	}
	root := s
	for root.parent != nil {
		root = root.parent
	}
	cur := root
	for _, token := range tokens {
		found := false
		if cur.anchors != nil {
			if next, ok := cur.anchors[token]; ok {
				cur = next
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("unable to resolve token %q", token)
		}
	}
	return cur, nil
}

func (s *Schema) resolveRef(ref string) (*Schema, error) {
	if strings.HasPrefix(ref, "#") {
		return s.resolveLocalRef(ref)
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
	case map[string]any, []any, float64, bool, nil:
		return data, nil
	case string:
		return data, nil
	default:
		b, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}
		var v any
		dec := json.NewDecoder(bytes.NewReader(b))
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		return v, nil
	}
}

func annotateError(path string, err error) error {
	return fmt.Errorf("at %s: %w", path, err)
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
		if s.Pattern != nil {
			re, err := getCompiledRegex(*s.Pattern)
			if err != nil {
				return fmt.Errorf("invalid pattern: %v", err)
			}
			if !re.MatchString(str) {
				return fmt.Errorf("value %q does not match pattern %q", str, *s.Pattern)
			}
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
		if s.PropertyNames != nil {
			for name := range obj {
				if err := s.PropertyNames.Validate(name); err != nil {
					return fmt.Errorf("propertyNames: key %q invalid: %v", name, err)
				}
			}
		}
	}
	if len(s.Enum) > 0 {
		found := false
		for _, enumVal := range s.Enum {
			if enumVal == data {
				found = true
				break
			}
		}
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

func validateDependentSchemas(obj map[string]any, s *Schema) error {
	for prop, depSchema := range s.DependentSchemas {
		if _, exists := obj[prop]; exists {
			if err := depSchema.Validate(obj); err != nil {
				return fmt.Errorf("dependentSchemas: property %q: %v", prop, err)
			}
		}
	}
	return nil
}

func validateDependentRequired(obj map[string]any, s *Schema) error {
	for prop, reqFields := range s.DependentRequired {
		if _, exists := obj[prop]; exists {
			for _, r := range reqFields {
				if _, ok := obj[r]; !ok {
					return fmt.Errorf("dependentRequired: property %q requires field %q", prop, r)
				}
			}
		}
	}
	return nil
}

func validateAdditionalProperties(obj map[string]any, s *Schema) error {
	if s.Properties == nil {
		return nil
	}
	extras := make([]string, 0)
	for key := range obj {
		if _, ok := (*s.Properties)[key]; !ok {
			matched := false
			if s.PatternProperties != nil {
				for pattern := range *s.PatternProperties {
					re, err := getCompiledRegex(pattern)
					if err != nil {
						continue
					}
					if re.MatchString(key) {
						matched = true
						break
					}
				}
			}
			if !matched {
				if s.AdditionalProperties != nil && s.AdditionalProperties.Boolean != nil && !*s.AdditionalProperties.Boolean {
					extras = append(extras, key)
				}
			}
		}
	}
	if len(extras) > 0 {
		return fmt.Errorf("additional properties not allowed: %v", extras)
	}
	return nil
}

func validatePatternProperties(obj map[string]any, s *Schema) error {
	if s.PatternProperties == nil {
		return nil
	}
	for pattern, schema := range *s.PatternProperties {
		re, err := getCompiledRegex(pattern)
		if err != nil {
			return fmt.Errorf("patternProperties: invalid pattern %q: %v", pattern, err)
		}
		for key, val := range obj {
			if re.MatchString(key) {
				if err := schema.Validate(val); err != nil {
					return fmt.Errorf("patternProperties: key %q: %v", key, err)
				}
			}
		}
	}
	return nil
}

func validateUnevaluatedProperties(obj map[string]any, s *Schema) error {
	if s.UnevaluatedProperties == nil {
		return nil
	}
	for key, val := range obj {
		if err := s.UnevaluatedProperties.Validate(val); err != nil {
			return fmt.Errorf("unevaluatedProperties: key %q: %v", key, err)
		}
	}
	return nil
}

func validateUnevaluatedItems(arr []any, s *Schema) error {
	if s.UnevaluatedItems == nil {
		return nil
	}
	for i, item := range arr {
		if err := s.UnevaluatedItems.Validate(item); err != nil {
			return fmt.Errorf("unevaluatedItems: index %d: %v", i, err)
		}
	}
	return nil
}

func (s *Schema) ValidateWithPath(unprepared any, instancePath string) error {
	data, err := s.prepareData(unprepared)
	if err != nil {
		return annotateError(instancePath, fmt.Errorf("failed to prepare data: %w", err))
	}
	if obj, ok := data.(map[string]any); ok {
		for _, field := range s.Required {
			if _, exists := obj[field]; !exists {
				if s.Properties != nil {
					if propSchema, ok := (*s.Properties)[field]; ok && propSchema.Default != nil {
						continue
					}
				}
				return annotateError(instancePath, fmt.Errorf("missing required field %q", field))
			}
		}
		if err := validateDependentSchemas(obj, s); err != nil {
			return annotateError(instancePath, err)
		}
		if err := validateDependentRequired(obj, s); err != nil {
			return annotateError(instancePath, err)
		}
		if s.Properties != nil && s.AdditionalProperties != nil && s.AdditionalProperties.Boolean != nil && !*s.AdditionalProperties.Boolean {
			if err := validateAdditionalProperties(obj, s); err != nil {
				return annotateError(instancePath, err)
			}
		}
		if err := validatePatternProperties(obj, s); err != nil {
			return annotateError(instancePath, err)
		}
		if err := validateUnevaluatedProperties(obj, s); err != nil {
			return annotateError(instancePath, err)
		}
	}
	if err := validateApplicatorKeywords(data, s); err != nil {
		return annotateError(instancePath, err)
	}
	if s.Contains != nil {
		if err := validateContains(data, s.Contains); err != nil {
			return annotateError(instancePath, fmt.Errorf("contains validation error: %w", err))
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
							candidateErrors = append(candidateErrors, fmt.Errorf("property %q validation failed: %v", key, err))
							goto NextCandidate
						}
					}
				}
			}
		}
		if candidate == "array" {
			if arr, ok := data.([]any); ok {
				if err := validateUnevaluatedItems(arr, s); err != nil {
					candidateErrors = append(candidateErrors, fmt.Errorf("array unevaluatedItems: %v", err))
					goto NextCandidate
				}
			}
		}
		validCandidateCount++
	NextCandidate:
	}
	if validCandidateCount < 1 {
		return annotateError(instancePath, fmt.Errorf("data does not match any candidate types %v; details: %v", s.Type, candidateErrors))
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
						return false, nil, fmt.Errorf("error unmarshalling property %q: %v", key, err)
					}
					newObj[key] = merged
				} else if propSchema.Default != nil {

					if defObj, ok := propSchema.Default.(map[string]any); ok {
						newObj[key] = defObj
					} else {
						newObj[key] = propSchema.Default
					}
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
					return false, nil, fmt.Errorf("error unmarshalling additional property %q: %v", key, err)
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

var vocabularyValidators = map[string]func(schema *Schema) error{}

func RegisterVocabularyValidator(name string, validator func(schema *Schema) error) {
	vocabularyValidators[name] = validator
}

func checkVocabularyCompliance(schema *Schema) error {
	for vocab, enabled := range schema.Vocabulary {
		if enabled {
			if validator, ok := vocabularyValidators[vocab]; ok {
				if err := validator(schema); err != nil {
					return fmt.Errorf("vocabulary %q validation failed: %v", vocab, err)
				}
			}
		}
	}
	return nil
}

var customKeywordValidators = map[string]func(s *Schema, keywordValue any) error{}

func RegisterCustomKeyword(keyword string, validator func(s *Schema, keywordValue any) error) {
	customKeywordValidators[keyword] = validator
}

func processCustomKeywords(m map[string]any, schema *Schema) error {
	for key, val := range m {
		if validator, exists := customKeywordValidators[key]; exists {
			if err := validator(schema, val); err != nil {
				return fmt.Errorf("custom keyword %q validation failed: %w", key, err)
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

func validateDiscriminator(instance any, s *Schema) error {
	obj, ok := instance.(map[string]any)
	if !ok {
		return errors.New("discriminator: instance is not an object")
	}
	discVal, ok := obj[s.Discriminator.PropertyName]
	if !ok {
		return fmt.Errorf("discriminator: property %q not found", s.Discriminator.PropertyName)
	}
	discStr, ok := discVal.(string)
	if !ok {
		return fmt.Errorf("discriminator: property %q is not a string", s.Discriminator.PropertyName)
	}
	var candidate *Schema
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
	var parts []string
	if cached, ok := nestedKeysCache.Load(field); ok {
		parts = cached.([]string)
	} else {
		parts = strings.Split(field, ".")
		nestedKeysCache.Store(field, parts)
	}
	value := any(m)
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

var subschemaSem = make(chan struct{}, 16)

func compileSubschemaAsync(item any, compiler *Compiler, parent *Schema, result chan<- *Schema, errChan chan<- error) {
	go func() {
		subschemaSem <- struct{}{}
		defer func() { <-subschemaSem }()
		subSchema, err := compileSchema(item, compiler, parent)
		if err != nil {
			errChan <- err
			return
		}
		result <- subSchema
	}()
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

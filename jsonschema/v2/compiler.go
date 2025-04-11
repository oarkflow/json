package v2

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sync"

	"github.com/oarkflow/json/jsonmap"
)

// NEW: Added Options struct for extended configuration and performance tuning.
type Options struct {
	DraftVersion                    string         // e.g., "2020-12", "2019-09", etc.
	EnableAsyncSubschemaCompilation bool           // true to use asynchronous compilation
	ErrorReportingMode              string         // "first" or "all"
	CustomKeywordConfig             map[string]any // optional configuration for custom keyword plugins
	VocabularyConfig                map[string]any // optional configuration for vocabulary validators
}

// Option is a function that modifies the Options struct.
type Option func(*Options)

// WithDraftVersion sets the DraftVersion field.
func WithDraftVersion(version string) Option {
	return func(opts *Options) {
		opts.DraftVersion = version
	}
}

// WithAsyncSubschemaCompilation sets EnableAsyncSubschemaCompilation.
func WithAsyncSubschemaCompilation(enabled bool) Option {
	return func(opts *Options) {
		opts.EnableAsyncSubschemaCompilation = enabled
	}
}

// WithErrorReportingMode sets the ErrorReportingMode.
func WithErrorReportingMode(mode string) Option {
	return func(opts *Options) {
		opts.ErrorReportingMode = mode
	}
}

// WithCustomKeywordConfig sets the CustomKeywordConfig.
func WithCustomKeywordConfig(config map[string]any) Option {
	return func(opts *Options) {
		opts.CustomKeywordConfig = config
	}
}

// WithVocabularyConfig sets the VocabularyConfig.
func WithVocabularyConfig(config map[string]any) Option {
	return func(opts *Options) {
		opts.VocabularyConfig = config
	}
}

// Modified Compiler to include Options.
type Compiler struct {
	schemas map[string]*Schema
	cache   map[string]*Schema
	cacheMu sync.RWMutex
	Options *Options
}

// NewCompiler creates a new Compiler instance with provided functional options.
func NewCompiler(opts ...Option) *Compiler {
	// Set default options.
	defaultOpts := &Options{
		DraftVersion:                    "2020-12",
		EnableAsyncSubschemaCompilation: true,
		ErrorReportingMode:              "all",
		CustomKeywordConfig:             make(map[string]any), // Provide default empty maps if needed
		VocabularyConfig:                make(map[string]any),
	}

	// Apply each option to the default options.
	for _, opt := range opts {
		opt(defaultOpts)
	}

	return &Compiler{
		schemas: make(map[string]*Schema),
		cache:   make(map[string]*Schema),
		Options: defaultOpts,
	}
}

func (c *Compiler) Compile(data []byte) (*Schema, error) {
	var tmp any
	if err := jsonmap.Unmarshal(data, &tmp); err != nil {
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

// NEW: Pool compiled regexes to avoid repeated allocations.
var compiledRegexPool sync.Map

func compileSchema(value any, compiler *Compiler, parent *Schema) (*Schema, error) {
	// Handle boolean schema shortcut.
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

	// Migrate "definitions" to "$defs" if needed.
	if defs, exists := m["definitions"]; exists && m["$defs"] == nil {
		m["$defs"] = defs
	}

	// Process legacy "dependencies" field.
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

	// Initialize a new schema instance.
	schema := &Schema{
		compiler:         compiler,
		parent:           parent,
		compiledPatterns: make(map[string]*regexp.Regexp),
		anchors:          make(map[string]*Schema),
		dynamicAnchors:   make(map[string]*Schema),
	}

	// Process core keywords.
	if id, ok := getString(m, "$id"); ok {
		schema.ID = id
	}
	if s, ok := getString(m, "$schema"); ok {
		schema.Schema = s
	}
	if format, ok := getString(m, "format"); ok {
		schema.Format = &format
	}
	if ref, ok := getString(m, "$ref"); ok {
		schema.Ref = ref
	}
	if dynRef, ok := getString(m, "$dynamicRef"); ok {
		schema.DynamicRef = dynRef
	}
	if recRef, ok := getString(m, "$recursiveRef"); ok {
		schema.RecursiveRef = recRef
	}
	if anchor, ok := getString(m, "$anchor"); ok {
		schema.Anchor = anchor
		if parent != nil {
			if parent.anchors == nil {
				parent.anchors = make(map[string]*Schema)
			}
			parent.anchors[anchor] = schema
		}
	}
	if recAnchorVal, exists := m["$recursiveAnchor"]; exists {
		if recAnchorBool, ok := recAnchorVal.(bool); ok {
			schema.RecursiveAnchor = recAnchorBool
		}
	}
	if dynAnchor, ok := getString(m, "$dynamicAnchor"); ok {
		schema.DynamicAnchor = dynAnchor
		if parent != nil {
			if parent.dynamicAnchors == nil {
				parent.dynamicAnchors = make(map[string]*Schema)
			}
			parent.dynamicAnchors[dynAnchor] = schema
		}
	}
	if comment, ok := getString(m, "$comment"); ok {
		schema.Comment = &comment
	}

	// Process $vocabulary.
	if vocabRaw, exists := m["$vocabulary"]; exists {
		if vocabMap, ok := vocabRaw.(map[string]any); ok {
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

	// Process subschema definitions.
	if defs, ok := getMap(m, "$defs"); ok {
		schema.Defs = make(map[string]*Schema)
		for key, defVal := range defs {
			compiledDef, err := compileSchema(defVal, compiler, schema)
			if err != nil {
				return nil, fmt.Errorf("error compiling $defs[%s]: %v", key, err)
			}
			schema.Defs[key] = compiledDef
		}
	}

	// Process combinator keywords.
	if err := compileSubschemaArray(m, "allOf", compiler, schema, &schema.AllOf); err != nil {
		return nil, fmt.Errorf("error compiling allOf: %v", err)
	}
	if err := compileSubschemaArray(m, "anyOf", compiler, schema, &schema.AnyOf); err != nil {
		return nil, fmt.Errorf("error compiling anyOf: %v", err)
	}
	if err := compileSubschemaArray(m, "oneOf", compiler, schema, &schema.OneOf); err != nil {
		return nil, fmt.Errorf("error compiling oneOf: %v", err)
	}
	if notVal, exists := m["not"]; exists {
		subSchema, err := compileSchema(notVal, compiler, schema)
		if err != nil {
			return nil, fmt.Errorf("error compiling not: %v", err)
		}
		schema.Not = subSchema
	}
	if err := compileConditional(m, "if", "then", schema, compiler, &schema.If, &schema.Then); err != nil {
		return nil, err
	}
	if elseVal, exists := m["else"]; exists {
		subSchema, err := compileSchema(elseVal, compiler, schema)
		if err != nil {
			return nil, fmt.Errorf("error compiling else: %v", err)
		}
		schema.Else = subSchema
	}

	// Process dependent schemas and required.
	if depSchemas, ok := getMap(m, "dependentSchemas"); ok {
		schema.DependentSchemas = make(map[string]*Schema)
		for key, depVal := range depSchemas {
			subSchema, err := compileSchema(depVal, compiler, schema)
			if err != nil {
				return nil, fmt.Errorf("error compiling dependentSchemas[%s]: %v", key, err)
			}
			schema.DependentSchemas[key] = subSchema
		}
	}
	// Process dependentRequired (only once; it may have been set by "dependencies").
	if depReqRaw, exists := m["dependentRequired"]; exists {
		if depMap, ok := depReqRaw.(map[string]any); ok {
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

	// Process array-related subschemas.
	if err := compileSubschemaArray(m, "prefixItems", compiler, schema, &schema.PrefixItems); err != nil {
		return nil, fmt.Errorf("error compiling prefixItems: %v", err)
	}
	if itemsVal, exists := m["items"]; exists {
		subSchema, err := compileSchema(itemsVal, compiler, schema)
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
	if containsVal, exists := m["contains"]; exists {
		subSchema, err := compileSchema(containsVal, compiler, schema)
		if err != nil {
			return nil, fmt.Errorf("error compiling contains: %v", err)
		}
		schema.Contains = subSchema
	}

	// Process properties.
	if propsRaw, exists := m["properties"]; exists {
		if propsMap, ok := propsRaw.(map[string]any); ok {
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
	// Mark properties with an "in" field as required.
	if schema.Properties != nil {
		for key, prop := range *schema.Properties {
			if prop.In != nil && len(prop.In) > 0 {
				if !slices.Contains(schema.Required, key) {
					schema.Required = append(schema.Required, key)
				}
			}
		}
	}
	// Process conditional required fields.
	processConditionalRequired(m, schema)

	// Process patternProperties.
	if patProps, exists := m["patternProperties"]; exists {
		if patMap, ok := patProps.(map[string]any); ok {
			sMap := SchemaMap{}
			for pattern, patVal := range patMap {
				subSchema, err := compileSchema(patVal, compiler, schema)
				if err != nil {
					return nil, fmt.Errorf("error compiling patternProperties[%s]: %v", pattern, err)
				}
				sMap[pattern] = subSchema
				// Use pooled regex to avoid recompilation.
				var re *regexp.Regexp
				if cached, ok := compiledRegexPool.Load(pattern); ok {
					re = cached.(*regexp.Regexp)
				} else {
					re, err = regexp.Compile(pattern)
					if err != nil {
						return nil, fmt.Errorf("invalid pattern regex '%s': %v", pattern, err)
					}
					compiledRegexPool.Store(pattern, re)
				}
				schema.compiledPatterns[pattern] = re
			}
			schema.PatternProperties = &sMap
		}
	}

	// Process additionalProperties and propertyNames.
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

	// Process type.
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
			// Prefer "array" if present.
			for _, typ := range types {
				if typ == "array" {
					schema.Type = SchemaType{typ}
					goto TypeDone
				}
			}
			schema.Type = SchemaType(types)
		}
	}
TypeDone:
	if schema.Pattern != nil && len(schema.Type) == 0 {
		schema.Type = SchemaType{"string"}
	}

	// Process enum and const.
	if enumVal, exists := m["enum"]; exists {
		if enumArr, ok := enumVal.([]any); ok {
			schema.Enum = enumArr
		}
	}
	if constVal, exists := m["const"]; exists {
		schema.Const = constVal
	}

	// Process numeric validations.
	if num, ok := getFloat(m, "multipleOf"); ok {
		r := Rat(num)
		schema.MultipleOf = &r
	}
	if num, ok := getFloat(m, "maximum"); ok {
		r := Rat(num)
		schema.Maximum = &r
	}
	if num, ok := getFloat(m, "exclusiveMaximum"); ok {
		r := Rat(num)
		schema.ExclusiveMaximum = &r
	}
	if num, ok := getFloat(m, "minimum"); ok {
		r := Rat(num)
		schema.Minimum = &r
	}
	if num, ok := getFloat(m, "exclusiveMinimum"); ok {
		r := Rat(num)
		schema.ExclusiveMinimum = &r
	}
	if num, ok := getFloat(m, "maxLength"); ok {
		schema.MaxLength = &num
	}
	if num, ok := getFloat(m, "minLength"); ok {
		schema.MinLength = &num
	}
	if patStr, ok := getString(m, "pattern"); ok {
		schema.Pattern = &patStr
		re, err := regexp.Compile(patStr)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern regex '%s': %v", patStr, err)
		}
		schema.compiledPatterns[patStr] = re
	}

	// Process array length and uniqueness.
	if num, ok := getFloat(m, "maxItems"); ok {
		schema.MaxItems = &num
	}
	if num, ok := getFloat(m, "minItems"); ok {
		schema.MinItems = &num
	}
	if unique, exists := m["uniqueItems"]; exists {
		if b, ok := unique.(bool); ok {
			schema.UniqueItems = &b
		}
	}
	if num, ok := getFloat(m, "maxContains"); ok {
		schema.MaxContains = &num
	}
	if num, ok := getFloat(m, "minContains"); ok {
		schema.MinContains = &num
	}

	// Process object property counts.
	if num, ok := getFloat(m, "maxProperties"); ok {
		schema.MaxProperties = &num
	}
	if num, ok := getFloat(m, "minProperties"); ok {
		schema.MinProperties = &num
	}
	if reqArr, exists := m["required"]; exists {
		if arr, ok := reqArr.([]any); ok {
			for _, item := range arr {
				if str, ok := item.(string); ok {
					schema.Required = append(schema.Required, str)
				}
			}
		}
	}

	// Process content keywords.
	if s, ok := getString(m, "contentEncoding"); ok {
		schema.ContentEncoding = &s
	}
	if s, ok := getString(m, "contentMediaType"); ok {
		schema.ContentMediaType = &s
	}
	if cs, exists := m["contentSchema"]; exists {
		subSchema, err := compileSchema(cs, compiler, schema)
		if err != nil {
			return nil, fmt.Errorf("error compiling contentSchema: %v", err)
		}
		schema.ContentSchema = subSchema
	}

	// Process documentation keywords.
	if s, ok := getString(m, "title"); ok {
		schema.Title = &s
	}
	if s, ok := getString(m, "description"); ok {
		schema.Description = &s
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
		switch val := inVal.(type) {
		case string:
			schema.In = []string{val}
		case []any:
			var inArr []string
			for _, item := range val {
				if s, ok := item.(string); ok {
					inArr = append(inArr, s)
				}
			}
			schema.In = inArr
		}
	}
	if fieldVal, ok := getString(m, "field"); ok {
		schema.Field = &fieldVal
	}

	// Override type to "number" if numeric validations exist.
	if m["minimum"] != nil || m["maximum"] != nil || m["exclusiveMinimum"] != nil || m["exclusiveMaximum"] != nil {
		schema.Type = SchemaType{"number"}
	}
	// Override type to "object" if properties exist.
	if schema.Properties != nil {
		schema.Type = SchemaType{"object"}
	}

	// Register schema by its ID.
	if schema.ID != "" {
		compiler.schemas[schema.ID] = schema
	}

	// Infer union types from oneOf/anyOf if type is still undefined.
	if len(schema.Type) == 0 {
		var unionTypes []string
		for _, candidates := range [][]*Schema{schema.OneOf, schema.AnyOf} {
			for _, candidate := range candidates {
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
		} else if schema.Properties != nil || schema.If != nil || schema.Then != nil || schema.Else != nil {
			schema.Type = SchemaType{"object"}
		}
	}

	// Process Draft2020 keywords and perform self‑validation.
	if err := compileDraft2020Keywords(m, schema); err != nil {
		return nil, err
	}
	if err := selfValidateSchema(schema); err != nil {
		return nil, fmt.Errorf("schema self‑validation failed: %w", err)
	}

	// Process discriminator.
	if disc, exists := m["discriminator"]; exists {
		if d, ok := disc.(map[string]any); ok {
			prop, ok := d["propertyName"].(string)
			if !ok || prop == "" {
				return nil, errors.New("discriminator: propertyName must be a non‑empty string")
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

	// NEW: Process custom keywords (if any)
	if err := processCustomKeywords(m, schema); err != nil {
		return nil, err
	}

	return schema, nil
}

// compileSubschemaArray processes keywords whose value is an array of subschemas.
func compileSubschemaArray(m map[string]any, key string, compiler *Compiler, parent *Schema, target *[]*Schema) error {
	raw, exists := m[key]
	if !exists {
		return nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("%s must be an array", key)
	}
	// Decide whether to compile asynchronously.
	if (key == "allOf" || key == "oneOf" || key == "anyOf") &&
		(compiler.Options != nil && compiler.Options.EnableAsyncSubschemaCompilation) {
		resultChan := make(chan *Schema, len(arr))
		errChan := make(chan error, len(arr))
		for _, item := range arr {
			compileSubschemaAsync(item, compiler, parent, resultChan, errChan)
		}
		var errorsList []error
		for i := 0; i < len(arr); i++ {
			select {
			case subSchema := <-resultChan:
				*target = append(*target, subSchema)
			case err := <-errChan:
				if compiler.Options != nil && compiler.Options.ErrorReportingMode == "first" {
					return annotateError(key, err)
				}
				errorsList = append(errorsList, err)
			}
		}
		if len(errorsList) > 0 && compiler.Options != nil && compiler.Options.ErrorReportingMode == "all" {
			return annotateError(key, fmt.Errorf("multiple errors: %v", errorsList))
		}
	} else { // Synchronous compilation.
		for _, item := range arr {
			subSchema, err := compileSchema(item, compiler, parent)
			if err != nil {
				return annotateError(key, err)
			}
			*target = append(*target, subSchema)
		}
	}
	return nil
}

// compileConditional compiles the "if" and "then" keywords together.
func compileConditional(m map[string]any, ifKey, thenKey string, parent *Schema, compiler *Compiler, ifTarget, thenTarget **Schema) error {
	if ifVal, exists := m[ifKey]; exists {
		subSchema, err := compileSchema(ifVal, compiler, parent)
		if err != nil {
			return fmt.Errorf("error compiling %s: %v", ifKey, err)
		}
		*ifTarget = subSchema
		if thenVal, exists2 := m[thenKey]; exists2 {
			subSchema, err := compileSchema(thenVal, compiler, parent)
			if err != nil {
				return fmt.Errorf("error compiling %s: %v", thenKey, err)
			}
			*thenTarget = subSchema
		}
	}
	return nil
}

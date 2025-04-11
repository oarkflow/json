package v2

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
)

func extractToken(authHeader string) (authType, token string, err error) {
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 {
		return "", "", errors.New("invalid authorization header format")
	}
	authType = strings.TrimSpace(parts[0])
	token = strings.TrimSpace(parts[1])
	if authType == "" || token == "" {
		return "", "", errors.New("authorization type or token is empty")
	}
	return authType, token, nil
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
			if len(v) > 0 {
				m[k] = v[0]
			}
		}
		if field != nil && *field != "" {
			if strings.EqualFold(*field, "Authorization") {
				if val, exists := m["Authorization"]; exists {
					strVal, _ := val.(string)
					_, token, err := extractToken(strVal)
					return token, err
				}
			} else if strings.EqualFold(*field, "authorization") {
				if val, exists := m["authorization"]; exists {
					strVal, _ := val.(string)
					_, token, err := extractToken(strVal)
					return token, err
				}
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
		if s.In != nil && len(s.In) > 0 {
			in = s.In[0]
		}
		var err error
		data, err = extractDataFromRequest(r, in, s.Field)
		if err != nil {
			return err
		}
	}
	merged, err := s.SmartUnmarshal(data)
	if err != nil {
		return err
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
		// Only override if a valid "in" is defined and does not include "body".
		if len(propSchema.In) > 0 && !slices.Contains(propSchema.In, "body") {
			fieldName := key
			if propSchema.Field != nil && *propSchema.Field != "" {
				fieldName = *propSchema.Field
			}
			var val any
			// Try each location in order.
			for _, location := range propSchema.In {
				if v, err := extractDataFromRequest(r, location, &fieldName); err == nil {
					val = v
					break
				}
			}
			if val != nil {
				if len(propSchema.Type) > 0 {
					if conv, err := convertValue(val, propSchema.Type[0]); err == nil {
						data[key] = conv
					}
				} else {
					data[key] = val
				}
			} else {
				delete(data, key)
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
	Params(key string, defaultVal ...string) string
	Query(key string, defaultVal ...string) string
	Body() []byte
	Get(key string, defaultVal ...string) string
	BodyParser(dest interface{}) error
}

func extractDataFromFiberCtx(ctx Ctx, in string, field *string) (any, error) {
	switch strings.ToLower(in) {
	case "header":
		if field != nil && *field != "" {
			hVal := ctx.Get(*field)
			if strings.EqualFold(*field, "Authorization") {
				_, token, err := extractToken(hVal)
				return token, err
			} else if strings.EqualFold(*field, "authorization") {
				_, token, err := extractToken(hVal)
				return token, err
			}
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
		if s.In != nil && len(s.In) > 0 {
			in = s.In[0]
		}
		var err error
		data, err = extractDataFromFiberCtx(ctx, in, s.Field)
		if err != nil {
			return err
		}
	}
	merged, err := s.SmartUnmarshal(data)
	if err != nil {
		return err
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
		if len(propSchema.In) > 0 && !slices.Contains(propSchema.In, "body") {
			fieldName := key
			if propSchema.Field != nil && *propSchema.Field != "" {
				fieldName = *propSchema.Field
			}
			var val any
			for _, location := range propSchema.In {
				if v, err := extractDataFromFiberCtx(ctx, location, &fieldName); err == nil {
					val = v
					break
				}
			}
			if val != nil {
				if len(propSchema.Type) > 0 {
					if conv, err := convertValue(val, propSchema.Type[0]); err == nil {
						data[key] = conv
					}
				} else {
					data[key] = val
				}
			} else {
				// Remove value if not found in the designated source.
				delete(data, key)
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

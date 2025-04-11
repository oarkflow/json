package v2

import (
	"fmt"
	"strconv"
)

// Global helper functions.
func getString(m map[string]any, key string) (string, bool) {
	if val, exists := m[key]; exists {
		if str, ok := val.(string); ok {
			return str, true
		}
	}
	return "", false
}

func getMap(m map[string]any, key string) (map[string]any, bool) {
	if val, exists := m[key]; exists {
		if mp, ok := val.(map[string]any); ok {
			return mp, true
		}
	}
	return nil, false
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

func getFloat(m map[string]any, key string) (float64, bool) {
	if val, exists := m[key]; exists {
		return toFloat(val)
	}
	return 0, false
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

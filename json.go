package json

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/oarkflow/json/jsonschema"
	"github.com/oarkflow/json/sjson"
)

func init() {
	DefaultMarshaler()
	DefaultUnmarshaler()
	DefaultDecoder()
	DefaultEncoder()
}

// unmarshalHelper helps in unmarshalling JSON fields which may be numbers or strings
func unmarshalHelper(data json.RawMessage, field reflect.Value) error {
	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var intValue int64
		if err := json.Unmarshal(data, &intValue); err == nil {
			field.SetInt(intValue)
			return nil
		}
		var strValue string
		if err := json.Unmarshal(data, &strValue); err == nil {
			intValue, err := strconv.ParseInt(strValue, 10, 64)
			if err != nil {
				return err
			}
			field.SetInt(intValue)
			return nil
		}
	case reflect.Float32, reflect.Float64:
		var floatValue float64
		if err := json.Unmarshal(data, &floatValue); err == nil {
			field.SetFloat(floatValue)
			return nil
		}
		var strValue string
		if err := json.Unmarshal(data, &strValue); err == nil {
			floatValue, err := strconv.ParseFloat(strValue, 64)
			if err != nil {
				return err
			}
			field.SetFloat(floatValue)
			return nil
		}
	case reflect.Bool:
		var boolValue bool
		if err := json.Unmarshal(data, &boolValue); err == nil {
			field.SetBool(boolValue)
			return nil
		}
		var strValue string
		if err := json.Unmarshal(data, &strValue); err == nil {
			boolValue, err := strconv.ParseBool(strValue)
			if err != nil {
				return err
			}
			field.SetBool(boolValue)
			return nil
		}
	case reflect.String:
		var strValue string
		if err := json.Unmarshal(data, &strValue); err != nil {
			return err
		}
		field.SetString(strValue)
		return nil
	}

	return fmt.Errorf("unsupported type: %v", field.Kind())
}

// GenericUnmarshal is a generic unmarshal function to handle struct fields with different types
func GenericUnmarshal(data []byte, v any) error {
	var raw map[string]json.RawMessage
	if err := unmarshaler(data, &raw); err != nil {
		return err
	}
	val := reflect.ValueOf(v).Elem()
	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldName := typ.Field(i).Tag.Get("json")
		if fieldName == "" || fieldName == "-" {
			continue
		}
		rawValue, ok := raw[fieldName]
		if !ok {
			continue
		}
		if err := unmarshalHelper(rawValue, field); err != nil {
			return fmt.Errorf("error unmarshaling field %s: %v", fieldName, err)
		}
	}
	return nil
}

func Marshal(data any) ([]byte, error) {
	return marshaler(data)
}

func Unmarshal(data []byte, dst any, scheme ...[]byte) error {
	if reflect.ValueOf(dst).Kind() != reflect.Ptr {
		return errors.New("dst is not pointer type")
	}
	if len(scheme) == 0 {
		return GenericUnmarshal(data, dst)
	}
	schemeBytes := scheme[0]
	var rs jsonschema.Schema
	if err := unmarshaler(schemeBytes, &rs); err != nil {
		return err
	}
	return rs.ValidateAndUnmarshalJSON(data, dst, GenericUnmarshal)
}

func Validate(data []byte, scheme []byte) error {
	var rs jsonschema.Schema
	if err := unmarshaler(scheme, &rs); err != nil {
		return err
	}
	return rs.Validate(data)
}

func Get(jsonBytes []byte, path string) sjson.Result {
	return sjson.GetBytes(jsonBytes, path)
}

func Set(jsonBytes []byte, path string, val any) ([]byte, error) {
	return sjson.SetBytes(jsonBytes, path, val)
}

func IsValid(s string) bool {
	return sjson.Valid(s)
}

var re = regexp.MustCompile(`([{,])\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*:`)

func Fix(input string) (string, error) {
	if input == "" {
		return "", fmt.Errorf("input is empty")
	}
	input = re.ReplaceAllString(input, `$1"$2":`)
	input = strings.ReplaceAll(input, `'`, `"`)
	if !strings.HasPrefix(input, "{") && strings.Contains(input, ":") && !strings.ContainsAny(input, "[]") {
		input = "{" + input
	}
	if strings.Count(input, `"`)%2 != 0 {
		input += `"`
	}
	switch {
	case strings.HasPrefix(input, "{") && !strings.HasSuffix(input, "}"):
		input += `}`
	case strings.HasPrefix(input, "[") && !strings.HasSuffix(input, "]"):
		input += `]`
	}
	var js json.RawMessage
	if err := Unmarshal([]byte(input), &js); err != nil {
		return "", fmt.Errorf("failed to fix JSON: %w", err)
	}
	return input, nil
}

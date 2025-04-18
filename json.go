package json

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/goccy/go-reflect"

	"github.com/oarkflow/json/jsonschema"
	"github.com/oarkflow/json/sjson"
)

func init() {
	if marshaler == nil {
		DefaultMarshaler()
	}
	if unmarshaler == nil {
		DefaultUnmarshaler()
	}
	if indenter == nil {
		DefaultIndenter()
	}
	if decoderFactory == nil {
		DefaultDecoder()
	}
	if encoderFactory == nil {
		DefaultEncoder()
	}
}

type RawMessage []byte

// MarshalJSON returns m as the JSON encoding of m.
func (m RawMessage) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	return json.RawMessage(m).MarshalJSON()
}

// UnmarshalJSON sets *m to a copy of data.
func (m *RawMessage) UnmarshalJSON(data []byte) error {
	if m == nil {
		return errors.New("json.RawMessage: UnmarshalJSON on nil pointer")
	}
	*m = append((*m)[0:0], data...)
	return nil
}

func unmarshalHelper(data json.RawMessage, field reflect.Value) error {
	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		// Handle integer types (including string representation)
		var intValue int64
		if err := unmarshaler(data, &intValue); err == nil {
			field.SetInt(intValue)
			return nil
		}
		var strValue string
		if err := unmarshaler(data, &strValue); err == nil {
			intValue, err := strconv.ParseInt(strValue, 10, 64)
			if err != nil {
				return err
			}
			field.SetInt(intValue)
			return nil
		}
	case reflect.Float32, reflect.Float64:
		// Handle float types (including string representation)
		var floatValue float64
		if err := unmarshaler(data, &floatValue); err == nil {
			field.SetFloat(floatValue)
			return nil
		}
		var strValue string
		if err := unmarshaler(data, &strValue); err == nil {
			floatValue, err := strconv.ParseFloat(strValue, 64)
			if err != nil {
				return err
			}
			field.SetFloat(floatValue)
			return nil
		}
	case reflect.Bool:
		// Handle boolean types (including string representation)
		var boolValue bool
		if err := unmarshaler(data, &boolValue); err == nil {
			field.SetBool(boolValue)
			return nil
		}
		var strValue string
		if err := unmarshaler(data, &strValue); err == nil {
			boolValue, err := strconv.ParseBool(strValue)
			if err != nil {
				return err
			}
			field.SetBool(boolValue)
			return nil
		}
	case reflect.String:
		// Handle string types
		var strValue string
		if err := unmarshaler(data, &strValue); err != nil {
			return err
		}
		field.SetString(strValue)
		return nil
	case reflect.Slice:
		// Handle slices (including slices of structs or maps)
		sliceType := field.Type().Elem()
		slice := reflect.MakeSlice(field.Type(), 0, 0)
		var rawSlice []json.RawMessage
		if err := unmarshaler(data, &rawSlice); err != nil {
			return err
		}
		for _, rawElem := range rawSlice {
			elem := reflect.New(sliceType).Elem()
			if err := unmarshalHelper(rawElem, elem); err != nil {
				return err
			}
			slice = reflect.Append(slice, elem)
		}
		field.Set(slice)
		return nil
	case reflect.Map:
		// Handle maps (with string keys and any value type)
		mapType := field.Type()
		mapValue := reflect.MakeMap(mapType)
		var rawMap map[string]json.RawMessage
		if err := unmarshaler(data, &rawMap); err != nil {
			return err
		}
		for key, rawElem := range rawMap {
			elem := reflect.New(mapType.Elem()).Elem()
			if err := unmarshalHelper(rawElem, elem); err != nil {
				return err
			}
			mapValue.SetMapIndex(reflect.ValueOf(key), elem)
		}
		field.Set(mapValue)
		return nil
	case reflect.Struct:
		// Handle structs by calling GenericUnmarshal recursively
		structValue := reflect.New(field.Type()).Elem()
		if err := GenericUnmarshal(data, structValue.Addr().Interface()); err != nil {
			return err
		}
		field.Set(structValue)
		return nil
	case reflect.Interface:
		// Handle interface{} by determining the underlying type dynamically
		var anyValue any
		if err := unmarshaler(data, &anyValue); err != nil {
			return err
		}

		field.Set(reflect.ValueOf(anyValue))
		return nil
	}

	return fmt.Errorf("unsupported type: %v", field.Kind())
}

// GenericUnmarshal is a generic unmarshal function to handle struct fields with different types
func GenericUnmarshal(data []byte, v any) error {
	// Unmarshal data into an interface{} to handle different JSON structures
	var raw interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	val := reflect.ValueOf(v).Elem()
	typ := val.Type()

	// Check if the data is a map (object)
	if rawMap, ok := raw.(map[string]interface{}); ok {
		for i := 0; i < val.NumField(); i++ {
			field := val.Field(i)
			fieldName := typ.Field(i).Tag.Get("json")

			// Skip unexported fields or fields without a json tag
			if fieldName == "" || fieldName == "-" {
				continue
			}

			// Get the raw value for this field from the JSON
			if rawValue, exists := rawMap[fieldName]; exists {
				rawBytes, err := json.Marshal(rawValue)
				if err != nil {
					return err
				}
				if err := unmarshalHelper(rawBytes, field); err != nil {
					return fmt.Errorf("error unmarshaling field %s: %v", fieldName, err)
				}
			}
		}
	} else if rawArray, ok := raw.([]interface{}); ok {
		// Handle slices separately
		if val.Kind() == reflect.Slice {
			sliceType := val.Type().Elem()
			slice := reflect.MakeSlice(val.Type(), len(rawArray), len(rawArray))
			for i, item := range rawArray {
				itemBytes, err := json.Marshal(item)
				if err != nil {
					return err
				}
				elem := reflect.New(sliceType).Elem()
				if err := unmarshalHelper(itemBytes, elem); err != nil {
					return err
				}
				slice.Index(i).Set(elem)
			}
			val.Set(slice)
		}
	} else {
		// Handle other types directly
		if err := unmarshalHelper(data, val); err != nil {
			return err
		}
	}

	return nil
}

func Marshal(data any) ([]byte, error) {
	return marshaler(data)
}

func MarshalIndent(v any, prefix, indent string) ([]byte, error) {
	return indenter(v, prefix, indent)
}

func Unmarshal(data []byte, dst any, scheme ...[]byte) error {
	if reflect.ValueOf(dst).Kind() != reflect.Ptr {
		return errors.New("dst is not pointer type")
	}
	if len(scheme) == 0 {
		return unmarshaler(data, dst)
	}
	schemeBytes := scheme[0]
	var rs jsonschema.Schema
	if err := unmarshaler(schemeBytes, &rs); err != nil {
		return err
	}
	return rs.ValidateAndUnmarshalJSON(data, dst)
}

func FixAndUnmarshal(data []byte, dst any, scheme ...[]byte) error {
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
	return rs.ValidateAndUnmarshalJSON(data, dst)
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

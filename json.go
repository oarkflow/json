package json

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
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

func Marshal(data any) ([]byte, error) {
	return marshaler(data)
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

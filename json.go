package json

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/oarkflow/json/decoder"
	"github.com/oarkflow/json/encoder"
	"io"
	"reflect"
	"regexp"
	"strings"

	"github.com/oarkflow/json/jsonschema"
	"github.com/oarkflow/json/marshaler"
	"github.com/oarkflow/json/sjson"
	"github.com/oarkflow/json/unmarshaler"
)

func Marshal(data any) ([]byte, error) {
	return marshaler.Instance()(data)
}

func NewEncoder(r io.Writer) encoder.IEncoder {
	return encoder.NewEncoder(r)
}

func NewDecoder(r io.Reader) decoder.IDecoder {
	return decoder.NewDecoder(r)
}

func Unmarshal(data []byte, dst any, scheme ...[]byte) error {
	if reflect.ValueOf(dst).Kind() != reflect.Ptr {
		return errors.New("dst is not pointer type")
	}
	if len(scheme) == 0 {
		return unmarshaler.Instance()(data, dst)
	}
	schemeBytes := scheme[0]
	var rs jsonschema.Schema
	if err := unmarshaler.Instance()(schemeBytes, &rs); err != nil {
		return err
	}
	return rs.ValidateAndUnmarshalJSON(data, dst)
}

func Validate(data []byte, scheme []byte) error {
	var rs jsonschema.Schema
	if err := unmarshaler.Instance()(scheme, &rs); err != nil {
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

func Is(s string) bool {
	if len(s) == 0 {
		return false
	}
	s = strings.TrimSpace(s)
	if s[0] != '{' && s[0] != '[' {
		return false
	}
	if s[len(s)-1] != '}' && s[len(s)-1] != ']' {
		return false
	}
	const maxDepth = 1024
	var stack [maxDepth]rune
	sp := 0

	for i := 0; i < len(s); i++ {
		char := s[i]
		switch char {
		case '{', '[':
			if sp >= maxDepth {
				return false
			}
			stack[sp] = rune(char)
			sp++
		case '}', ']':
			if sp == 0 {
				return false
			}
			sp--
			opening := stack[sp]
			if (char == '}' && opening != '{') || (char == ']' && opening != '[') {
				return false
			}
		case '"':
			i++
			for i < len(s) {
				if s[i] == '\\' {
					i++
				} else if s[i] == '"' {
					break
				}
				i++
			}
		}
	}

	return sp == 0
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

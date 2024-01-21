package json

import (
	"errors"
	"reflect"

	"github.com/oarkflow/json/jsonschema"
	"github.com/oarkflow/json/marshaler"
	"github.com/oarkflow/json/sjson"
	"github.com/oarkflow/json/unmarshaler"
)

func Marshal(data any) ([]byte, error) {
	return marshaler.Instance()(data)
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

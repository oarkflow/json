package json

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	
	"github.com/oarkflow/json/jsonschema"
)

type Marshaler func(any) ([]byte, error)
type Unmarshaler func([]byte, any) error

var (
	marshaler   Marshaler
	unmarshaler Unmarshaler
)

func init() {
	marshaler = json.Marshal
	unmarshaler = json.Unmarshal
}

func SetMarshaler(m Marshaler) {
	marshaler = m
}

func SetUnmarshaler(m Unmarshaler) {
	unmarshaler = m
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
	ctx := context.Background()
	var rs jsonschema.Schema
	if err := json.Unmarshal(schemeBytes, &rs); err != nil {
		return err
	}
	errs, err := rs.ValidateBytes(ctx, data)
	if err != nil {
		return err
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return unmarshaler(data, dst)
}

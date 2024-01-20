package json

import (
	"context"
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
	ctx := context.Background()
	var rs jsonschema.Schema
	if err := unmarshaler.Instance()(schemeBytes, &rs); err != nil {
		return err
	}
	errs, err := rs.ValidateBytesToDst(ctx, data, dst)
	if len(errs) > 0 {
		var et []error
		for _, e := range errs {
			et = append(et, e)
		}
		return errors.Join(et...)
	}
	return err
}

func Validate(data []byte, scheme []byte) error {
	ctx := context.Background()
	var rs jsonschema.Schema
	if err := unmarshaler.Instance()(scheme, &rs); err != nil {
		return err
	}
	errs, err := rs.ValidateBytes(ctx, data)
	if len(errs) > 0 {
		var et []error
		for _, e := range errs {
			et = append(et, e)
		}
		return errors.Join(et...)
	}
	return err
}

func Get(jsonBytes []byte, path string) sjson.Result {
	return sjson.GetBytes(jsonBytes, path)
}

func Set(jsonBytes []byte, path string, val any) ([]byte, error) {
	return sjson.SetBytes(jsonBytes, path, val)
}

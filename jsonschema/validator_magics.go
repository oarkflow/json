package jsonschema

import (
	"fmt"

	"github.com/oarkflow/expr"
)

type ConstVal struct {
	Val any
}

func (cc ConstVal) Validate(c *ValidateCtx, value any) {

}

type DefaultVal struct {
	Val any
}

func (d DefaultVal) Validate(c *ValidateCtx, value any) {

}

type ReplaceKey string

func (r ReplaceKey) Validate(c *ValidateCtx, value any) {

}

func NewConstVal(i any, path string, parent Validator) (Validator, error) {
	return &ConstVal{
		Val: i,
	}, nil
}

func NewDefaultVal(i any, path string, parent Validator) (Validator, error) {
	data := make(map[string]any)
	switch i := i.(type) {
	case string:
		val, err := expr.Eval(i, data)
		if err == nil {
			return &DefaultVal{val}, nil
		}
	case []byte:
		val, err := expr.Eval(string(i), data)
		if err == nil {
			return &DefaultVal{val}, nil
		}
	default:
		val, err := expr.Eval(fmt.Sprintf("%v", i), data)
		if err == nil {
			return &DefaultVal{val}, nil
		}
	}
	return &DefaultVal{i}, nil
}

func NewReplaceKey(i any, path string, parent Validator) (Validator, error) {
	s, ok := i.(string)
	if !ok {
		return nil, fmt.Errorf("value of 'replaceKey' must be string :%v", i)
	}
	return ReplaceKey(s), nil

}

type FormatVal _type

func (f FormatVal) Validate(c *ValidateCtx, value any) {

}

func (f FormatVal) Convert(value any) any {
	switch _type(f) {
	case typeString:
		return StringOf(value)
	case typeBool:
		return BoolOf(value)
	case typeInteger, typeNumber:
		return NumberOf(value)
	}
	return value
}

func NewFormatVal(i any, path string, parent Validator) (Validator, error) {
	str, ok := i.(string)
	if !ok {
		return nil, fmt.Errorf("value of format must be string:%s", str)
	}
	return FormatVal(types[str]), nil
}

type SetVal map[*JsonPathCompiled]Value

func (s SetVal) Validate(c *ValidateCtx, value any) {
	m, ok := value.(map[string]any)
	if !ok {
		return
	}
	ctx := Context(m)
	for key, val := range s {
		v := val.Get(ctx)
		key.Set(m, v)
	}
}

func NewSetVal(i any, path string, parent Validator) (Validator, error) {
	m, ok := i.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("value of setVal must be map[string]any :%v", i)
	}
	setVal := SetVal{}
	for key, val := range m {
		v, err := parseValue(val)
		if err != nil {
			return nil, err
		}
		jp, err := parseJpathCompiled(key)
		if err != nil {
			return nil, err
		}
		setVal[jp] = v
	}
	return setVal, nil
}

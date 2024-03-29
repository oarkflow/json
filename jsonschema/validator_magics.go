package jsonschema

import (
	"fmt"

	"github.com/oarkflow/pkg/evaluate"
)

type ConstVal struct {
	Val interface{}
}

func (cc ConstVal) Validate(c *ValidateCtx, value interface{}) {

}

type DefaultVal struct {
	Val interface{}
}

func (d DefaultVal) Validate(c *ValidateCtx, value interface{}) {

}

type ReplaceKey string

func (r ReplaceKey) Validate(c *ValidateCtx, value interface{}) {

}

func NewConstVal(i interface{}, path string, parent Validator) (Validator, error) {
	return &ConstVal{
		Val: i,
	}, nil
}

func NewDefaultVal(i interface{}, path string, parent Validator) (Validator, error) {
	p, _ := evaluate.Parse(fmt.Sprintf("%v", i), true)
	pr := evaluate.NewEvalParams(make(map[string]interface{}))
	val, err := p.Eval(pr)
	if err == nil {
		return &DefaultVal{val}, nil
	}
	return &DefaultVal{i}, nil
}

func NewReplaceKey(i interface{}, path string, parent Validator) (Validator, error) {
	s, ok := i.(string)
	if !ok {
		return nil, fmt.Errorf("value of 'replaceKey' must be string :%v", i)
	}
	return ReplaceKey(s), nil

}

type FormatVal _type

func (f FormatVal) Validate(c *ValidateCtx, value interface{}) {

}

func (f FormatVal) Convert(value interface{}) interface{} {
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

func NewFormatVal(i interface{}, path string, parent Validator) (Validator, error) {
	str, ok := i.(string)
	if !ok {
		return nil, fmt.Errorf("value of format must be string:%s", str)
	}
	return FormatVal(types[str]), nil
}

type SetVal map[*JsonPathCompiled]Value

func (s SetVal) Validate(c *ValidateCtx, value interface{}) {
	m, ok := value.(map[string]interface{})
	if !ok {
		return
	}
	ctx := Context(m)
	for key, val := range s {
		v := val.Get(ctx)
		key.Set(m, v)
	}
}

func NewSetVal(i interface{}, path string, parent Validator) (Validator, error) {
	m, ok := i.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("value of setVal must be map[string]interface{} :%v", i)
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

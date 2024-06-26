package jsonschema

import "fmt"

func init() {
	RegisterValidator("minProperties", NewMinProperties)
	RegisterValidator("maxProperties", NewMaxProperties)
	RegisterValidator("oneOf", NewOneOf)
	AddIgnoreKeys("description")
	AddIgnoreKeys("$schema")
	AddIgnoreKeys("$comment")
	AddIgnoreKeys("examples")
}

type MinProperties struct {
	Path  string
	Value int
}

func (m *MinProperties) Validate(c *ValidateCtx, value any) {
	switch value.(type) {
	case map[string]any:
		if len(value.(map[string]any)) < m.Value {
			c.AddError(Error{
				Path: m.Path,
				Info: fmt.Sprintf("min properties is : %d", m.Value),
			})
		}
	case []any:
		if len(value.([]any)) < m.Value {
			c.AddError(Error{
				Path: m.Path,
				Info: fmt.Sprintf("min properties is : %d", m.Value),
			})
		}
	}
}

func NewMinProperties(i any, path string, parent Validator) (Validator, error) {
	fi, ok := i.(float64)
	if !ok {
		return nil, fmt.Errorf("value of minProperties must be number:%v,path:%s", desc(i), path)
	}
	if fi < 0 {
		return nil, fmt.Errorf("value of minProperties must be >0 :%v,path:%s", fi, path)
	}
	return &MinProperties{
		Path:  path,
		Value: int(fi),
	}, nil
}

type MaxProperties struct {
	Path  string
	Value int
}

func (m *MaxProperties) Validate(c *ValidateCtx, value any) {
	switch value.(type) {
	case map[string]any:
		if len(value.(map[string]any)) > m.Value {
			c.AddError(Error{
				Path: m.Path,
				Info: fmt.Sprintf("max properties is :%v ", m.Value),
			})
		}
	case []any:
		if len(value.([]any)) > m.Value {
			c.AddError(Error{
				Path: m.Path,
				Info: fmt.Sprintf("max properties is :%v", m.Value),
			})
		}
	}
}

func NewMaxProperties(i any, path string, parent Validator) (Validator, error) {
	fi, ok := i.(float64)
	if !ok {
		return nil, fmt.Errorf("value of maxProperties must be number:%v,path:%s", desc(i), path)
	}
	if fi < 0 {
		return nil, fmt.Errorf("value of maxProperties must be >=0 :%v,path:%s", fi, path)
	}
	return &MinProperties{
		Path:  path,
		Value: int(fi),
	}, nil
}

type OneOf []Validator

func (a OneOf) Validate(c *ValidateCtx, value any) {
	allErrs := []Error{}
	for _, validator := range a {
		cb := c.Clone()
		validator.Validate(cb, value)
		if len(cb.errors) == 0 {
			return
		}
		allErrs = append(allErrs, cb.errors...)
	}

	c.AddErrors(allErrs...)
}

func NewOneOf(i any, path string, parent Validator) (Validator, error) {
	m, ok := i.([]any)
	if !ok {
		return nil, fmt.Errorf("value of oneOf must be array:%v,path:%s", desc(i), path)
	}
	any := OneOf{}
	for idx, v := range m {
		ip, err := NewProp(v, path)
		if err != nil {
			return nil, fmt.Errorf("oneOf index:%d is invalid:%w %v,path:%s", idx, err, v, path)
		}
		any = append(any, ip)
	}
	if len(any) == 0 {
		return nil, fmt.Errorf("oneof length must be > 0,path:%s", path)
	}
	return any, nil
}

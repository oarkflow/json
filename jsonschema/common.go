package jsonschema

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/goccy/go-reflect"
)

var (
	sprintf = fmt.Sprintf
)

type Error struct {
	Path string
	Info string
}

type ValidateCtx struct {
	errors []Error
	root   Validator
}

func (v *ValidateCtx) AddError(e Error) {
	v.errors = append(v.errors, e)
}

func (v *ValidateCtx) AddErrorInfo(path string, info string) {
	v.errors = append(v.errors, Error{Path: path, Info: info})
}

func (v *ValidateCtx) AddErrors(e ...Error) {
	for i, _ := range e {
		v.AddError(e[i])
	}
}

func (v *ValidateCtx) Clone() *ValidateCtx {
	return &ValidateCtx{root: v.root}
}

type Validator interface {
	Validate(c *ValidateCtx, value any)
}

type NewValidatorFunc func(i any, path string, parent Validator) (Validator, error)

func appendString(s ...string) string {
	sb := strings.Builder{}
	for _, str := range s {
		sb.WriteString(str)
	}
	return sb.String()
}

func panicf(f string, args ...any) {
	panic(fmt.Sprintf(f, args...))
}

func StringOf(v any) string {
	switch vv := v.(type) {
	case string:
		return vv
	case bool:
		if vv {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(vv, 'f', -1, 64)
	case int:
		return strconv.Itoa(vv)
	case nil:
		return ""

	}
	return fmt.Sprintf("%v", v)
}

func NumberOf(v any) float64 {
	switch vv := v.(type) {
	case float64:
		return vv
	case bool:
		if vv {
			return 1
		}
		return 0
	case string:
		i, err := strconv.ParseFloat(vv, 64)
		if err != nil {
			return i
		}
		if vv == "true" {
			return 1
		}
		return 0
	}
	return 0
}

func BoolOf(v any) bool {
	switch vv := v.(type) {
	case float64:
		return vv > 0
	case string:
		return vv == "true"
	case bool:
		return vv
	default:
		if NumberOf(v) > 0 {
			return true
		}
	}
	return false
}

func notNil(v any) bool {
	switch v := v.(type) {
	case string:
		return v != ""
	case nil:
		return false

	}
	return true
}

func Equal(a, b any) bool {
	return StringOf(a) == StringOf(b)
}

func desc(i any) string {
	ty := reflect.TypeOf(i)
	return fmt.Sprintf("value:%v,type:%s", i, ty.String())
}

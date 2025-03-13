package jsonschema

import (
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"github.com/oarkflow/date"
)

type unmarshalFunc func(path string, in any, v reflect.Value) error

var unmarshalFuncCache sync.Map

var jsonUnmarshalType = reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()

func UnmarshalFromMap(in any, template any) error {
	v := reflect.ValueOf(template)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		panic("template value is nil or not a pointer")
	}
	uf := getUnmarshalFunc(v.Type())
	return uf("", in, v)
}

func getUnmarshalFunc(t reflect.Type) unmarshalFunc {

	if t == reflect.TypeOf(time.Duration(0)) {
		return buildDurationUnmarshalFunc(t)
	}

	if t == reflect.TypeOf(big.Int{}) {
		return buildBigIntUnmarshalFunc(t)
	}
	if t == reflect.TypeOf(big.Float{}) {
		return buildBigFloatUnmarshalFunc(t)
	}

	if f, ok := unmarshalFuncCache.Load(t); ok {
		return f.(unmarshalFunc)
	}

	var f unmarshalFunc
	switch t.Kind() {
	case reflect.Ptr:
		f = buildPtrUnmarshalFunc(t)
	case reflect.Interface:
		f = buildInterfaceUnmarshalFunc(t)
	case reflect.Struct:
		f = buildStructUnmarshalFunc(t)
	case reflect.Slice:
		f = buildSliceUnmarshalFunc(t)
	case reflect.Map:
		f = buildMapUnmarshalFunc(t)
	case reflect.String:
		f = buildStringUnmarshalFunc(t)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		f = buildIntUnmarshalFunc(t)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		f = buildUintUnmarshalFunc(t)
	case reflect.Float32, reflect.Float64:
		f = buildFloatUnmarshalFunc(t)
	case reflect.Bool:
		f = buildBoolUnmarshalFunc(t)
	case reflect.Array:
		f = buildArrayUnmarshalFunc(t)
	case reflect.Complex64, reflect.Complex128:
		f = buildComplexUnmarshalFunc(t)
	case reflect.Chan:

		f = func(path string, in any, v reflect.Value) error {
			return fmt.Errorf("unmarshalling into channel types is not supported")
		}
	default:

		if t.Implements(jsonUnmarshalType) || reflect.PtrTo(t).Implements(jsonUnmarshalType) {
			f = buildCustomUnmarshalFunc(t)
		} else {

			f = func(path string, in any, v reflect.Value) error {
				bytes, err := json.Marshal(in)
				if err != nil {
					return err
				}
				return json.Unmarshal(bytes, v.Addr().Interface())
			}
		}
	}
	unmarshalFuncCache.Store(t, f)
	return f
}

func buildPtrUnmarshalFunc(t reflect.Type) unmarshalFunc {
	elemType := t.Elem()
	elemFunc := getUnmarshalFunc(elemType)
	return func(path string, in any, v reflect.Value) error {
		if v.IsNil() {
			newVal := reflect.New(elemType)
			if err := elemFunc(path, in, newVal.Elem()); err != nil {
				return err
			}
			v.Set(newVal)
			return nil
		}
		return elemFunc(path, in, v.Elem())
	}
}

func buildInterfaceUnmarshalFunc(t reflect.Type) unmarshalFunc {
	return func(path string, in any, v reflect.Value) error {
		if in == nil {
			v.Set(reflect.Zero(t))
			return nil
		}
		val := reflect.ValueOf(in)
		if val.Type().AssignableTo(t) {
			v.Set(val)
			return nil
		}
		bytes, err := json.Marshal(in)
		if err != nil {
			return err
		}
		return json.Unmarshal(bytes, v.Addr().Interface())
	}
}

func buildStructUnmarshalFunc(t reflect.Type) unmarshalFunc {
	if t == reflect.TypeOf(time.Time{}) {
		return func(path string, in any, v reflect.Value) error {
			switch val := in.(type) {
			case time.Time:
				v.Set(reflect.ValueOf(val))
				return nil
			case string:
				parsed, err := date.Parse(val)
				if err != nil {
					return fmt.Errorf("failed to parse time: %v", err)
				}
				v.Set(reflect.ValueOf(parsed))
				return nil
			default:
				return fmt.Errorf("unsupported type for time.Time: %T", in)
			}
		}
	}
	numField := t.NumField()
	type fieldInfo struct {
		name   string
		index  int
		fn     unmarshalFunc
		inline bool
	}
	fields := make([]fieldInfo, 0, numField)
	for i := 0; i < numField; i++ {
		field := t.Field(i)
		tag := field.Tag.Get("json")
		name, inline := parseJSONTag(tag, field.Name)
		fields = append(fields, fieldInfo{
			name:   name,
			index:  i,
			fn:     getUnmarshalFunc(field.Type),
			inline: inline,
		})
	}
	return func(path string, in any, v reflect.Value) error {
		m, ok := in.(map[string]any)
		if !ok {
			return fmt.Errorf("expected input to be map[string]any for struct %s, got %T", t, in)
		}
		for _, field := range fields {

			if field.inline {
				if err := field.fn(field.name, in, v.Field(field.index)); err != nil {
					return err
				}
				continue
			}
			if fieldValue, exists := m[field.name]; exists {
				if err := field.fn(field.name, fieldValue, v.Field(field.index)); err != nil {
					return err
				}
			}
		}
		return nil
	}
}

func buildSliceUnmarshalFunc(t reflect.Type) unmarshalFunc {
	elemType := t.Elem()
	elemFunc := getUnmarshalFunc(elemType)
	return func(path string, in any, v reflect.Value) error {
		arr, ok := in.([]any)
		if !ok {
			return fmt.Errorf("expected input to be slice for %s, got %T", path, in)
		}
		slice := reflect.MakeSlice(t, len(arr), len(arr))
		for i, item := range arr {
			if err := elemFunc(fmt.Sprintf("%s[%d]", path, i), item, slice.Index(i)); err != nil {
				return err
			}
		}
		v.Set(slice)
		return nil
	}
}

func buildMapUnmarshalFunc(t reflect.Type) unmarshalFunc {
	keyType := t.Key()
	elemType := t.Elem()
	elemFunc := getUnmarshalFunc(elemType)
	return func(path string, in any, v reflect.Value) error {
		mVal := reflect.ValueOf(in)
		if mVal.Kind() != reflect.Map {
			return fmt.Errorf("expected input to be a map for %s, got %T", path, in)
		}
		newMap := reflect.MakeMap(t)
		for _, k := range mVal.MapKeys() {
			inputElem := mVal.MapIndex(k)
			var newKey reflect.Value

			if k.Type() != keyType {

				if k.Type() == reflect.TypeOf("") && keyType.Kind() == reflect.Int {
					parsed, err := strconv.Atoi(k.String())
					if err != nil {
						return fmt.Errorf("failed to convert key %v to int: %v", k.Interface(), err)
					}
					newKey = reflect.ValueOf(parsed).Convert(keyType)
				} else if k.Type() == reflect.TypeOf("") && keyType.Kind() == reflect.Bool {
					b, err := strconv.ParseBool(k.String())
					if err != nil {
						return fmt.Errorf("failed to convert key %v to bool: %v", k.Interface(), err)
					}
					newKey = reflect.ValueOf(b).Convert(keyType)
				} else if k.Type() == reflect.TypeOf("") && (keyType.Kind() == reflect.Float32 || keyType.Kind() == reflect.Float64) {
					f, err := strconv.ParseFloat(k.String(), 64)
					if err != nil {
						return fmt.Errorf("failed to convert key %v to float: %v", k.Interface(), err)
					}
					newKey = reflect.ValueOf(f).Convert(keyType)
				} else if k.Type().ConvertibleTo(keyType) {
					newKey = k.Convert(keyType)
				} else {
					return fmt.Errorf("cannot convert key type %s to %s", k.Type(), keyType)
				}
			} else {
				newKey = k
			}
			elemValue := reflect.New(elemType).Elem()
			if err := elemFunc(fmt.Sprintf("%v", newKey.Interface()), inputElem.Interface(), elemValue); err != nil {
				return err
			}
			newMap.SetMapIndex(newKey, elemValue)
		}
		v.Set(newMap)
		return nil
	}
}

func buildStringUnmarshalFunc(_ reflect.Type) unmarshalFunc {
	return func(path string, in any, v reflect.Value) error {
		v.SetString(fmt.Sprintf("%v", in))
		return nil
	}
}

func buildIntUnmarshalFunc(_ reflect.Type) unmarshalFunc {
	return func(path string, in any, v reflect.Value) error {
		n, err := intValueOf(in)
		if err != nil {
			return err
		}
		v.SetInt(n)
		return nil
	}
}

func buildUintUnmarshalFunc(_ reflect.Type) unmarshalFunc {
	return func(path string, in any, v reflect.Value) error {
		n, err := intValueOf(in)
		if err != nil {
			return err
		}
		v.SetUint(uint64(n))
		return nil
	}
}

func buildFloatUnmarshalFunc(_ reflect.Type) unmarshalFunc {
	return func(path string, in any, v reflect.Value) error {
		f, err := floatValueOf(in)
		if err != nil {
			return err
		}
		v.SetFloat(f)
		return nil
	}
}

func buildBoolUnmarshalFunc(_ reflect.Type) unmarshalFunc {
	return func(path string, in any, v reflect.Value) error {
		b, err := boolValueOf(in)
		if err != nil {
			return err
		}
		v.SetBool(b)
		return nil
	}
}

func buildArrayUnmarshalFunc(t reflect.Type) unmarshalFunc {
	elemType := t.Elem()
	elemFunc := getUnmarshalFunc(elemType)
	length := t.Len()
	return func(path string, in any, v reflect.Value) error {
		arr, ok := in.([]any)
		if !ok {
			return fmt.Errorf("expected input to be slice for array %s, got %T", path, in)
		}
		if len(arr) != length {
			return fmt.Errorf("expected array length %d for %s, got %d", length, path, len(arr))
		}
		for i := 0; i < length; i++ {
			if err := elemFunc(fmt.Sprintf("%s[%d]", path, i), arr[i], v.Index(i)); err != nil {
				return err
			}
		}
		return nil
	}
}

func buildComplexUnmarshalFunc(t reflect.Type) unmarshalFunc {
	bitSize := 128
	if t.Kind() == reflect.Complex64 {
		bitSize = 64
	}
	return func(path string, in any, v reflect.Value) error {
		var c complex128
		switch val := in.(type) {
		case string:
			parsed, err := strconv.ParseComplex(val, bitSize)
			if err != nil {
				return fmt.Errorf("failed to parse complex number: %v", err)
			}
			c = parsed
		case float64:
			c = complex(val, 0)
		case int:
			c = complex(float64(val), 0)
		case map[string]any:
			realVal, ok1 := val["real"]
			imagVal, ok2 := val["imag"]
			if !ok1 || !ok2 {
				return fmt.Errorf("expected map with 'real' and 'imag' for complex number at %s", path)
			}
			r, err := floatValueOf(realVal)
			if err != nil {
				return err
			}
			i, err := floatValueOf(imagVal)
			if err != nil {
				return err
			}
			c = complex(r, i)
		default:
			return fmt.Errorf("cannot unmarshal %T into complex number", in)
		}
		if t.Kind() == reflect.Complex64 {
			v.SetComplex(c)
		} else {
			v.SetComplex(c)
		}
		return nil
	}
}

func buildCustomUnmarshalFunc(_ reflect.Type) unmarshalFunc {
	return func(path string, in any, v reflect.Value) error {
		var unmarshaler json.Unmarshaler
		if v.CanAddr() {
			unmarshaler = v.Addr().Interface().(json.Unmarshaler)
		} else {
			return fmt.Errorf("cannot address value for custom unmarshal at %s", path)
		}
		bytes, err := json.Marshal(in)
		if err != nil {
			return err
		}
		return unmarshaler.UnmarshalJSON(bytes)
	}
}

func buildDurationUnmarshalFunc(_ reflect.Type) unmarshalFunc {
	return func(path string, in any, v reflect.Value) error {
		switch val := in.(type) {
		case string:
			d, err := time.ParseDuration(val)
			if err != nil {
				return fmt.Errorf("failed to parse duration: %v", err)
			}
			v.SetInt(int64(d))
			return nil
		default:
			n, err := intValueOf(in)
			if err != nil {
				return err
			}
			v.SetInt(n)
			return nil
		}
	}
}

func buildBigIntUnmarshalFunc(_ reflect.Type) unmarshalFunc {
	return func(path string, in any, v reflect.Value) error {
		var s string
		switch val := in.(type) {
		case string:
			s = val
		case json.Number:
			s = val.String()
		case float64:
			s = fmt.Sprintf("%.0f", val)
		default:
			return fmt.Errorf("cannot unmarshal %T into big.Int", in)
		}
		bi := new(big.Int)
		if _, ok := bi.SetString(s, 10); !ok {
			return fmt.Errorf("failed to parse big.Int from %s", s)
		}
		v.Set(reflect.ValueOf(*bi))
		return nil
	}
}

func buildBigFloatUnmarshalFunc(_ reflect.Type) unmarshalFunc {
	return func(path string, in any, v reflect.Value) error {
		var s string
		switch val := in.(type) {
		case string:
			s = val
		case json.Number:
			s = val.String()
		case float64:
			s = fmt.Sprintf("%f", val)
		default:
			return fmt.Errorf("cannot unmarshal %T into big.Float", in)
		}
		bf := new(big.Float)
		if _, ok := bf.SetString(s); !ok {
			return fmt.Errorf("failed to parse big.Float from %s", s)
		}
		v.Set(reflect.ValueOf(*bf))
		return nil
	}
}

func intValueOf(v any) (int64, error) {
	switch t := v.(type) {
	case float64:
		return int64(t), nil
	case float32:
		return int64(t), nil
	case int:
		return int64(t), nil
	case int32:
		return int64(t), nil
	case int64:
		return t, nil
	case string:
		return strconv.ParseInt(t, 10, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to int", v)
	}
}

func boolValueOf(v any) (bool, error) {
	switch b := v.(type) {
	case bool:
		return b, nil
	case int:
		return b != 0, nil
	case float64:
		return b != 0, nil
	case string:
		return strconv.ParseBool(b)
	default:
		return false, fmt.Errorf("cannot convert %T to bool", v)
	}
}

func floatValueOf(v any) (float64, error) {
	switch t := v.(type) {
	case int:
		return float64(t), nil
	case float64:
		return t, nil
	case float32:
		return float64(t), nil
	case string:
		return strconv.ParseFloat(t, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to float", v)
	}
}

func parseJSONTag(tag, defaultName string) (string, bool) {
	if tag == "" {
		return defaultName, false
	}
	parts := splitTag(tag)
	name := parts[0]
	if name == "" {
		name = defaultName
	}
	inline := false
	for _, p := range parts[1:] {
		if p == "inline" {
			inline = true
		}
	}
	return name, inline
}

func splitTag(tag string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(tag); i++ {
		if tag[i] == ',' {
			parts = append(parts, tag[start:i])
			start = i + 1
		}
	}
	parts = append(parts, tag[start:])
	return parts
}

func bytesOf(p uintptr, len uintptr) []byte {
	h := &reflect.SliceHeader{
		Data: p,
		Len:  int(len),
		Cap:  int(len),
	}
	return *(*[]byte)(unsafe.Pointer(h))
}

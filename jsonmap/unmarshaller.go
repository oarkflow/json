// Package jsonmap implements JSON encoding and decoding similar to encoding/json.
// Note: For the Marshal([]byte) APIs the encoder is freshly allocated so that its
// internal buffer can be returned directly (avoiding an extra copy). For streaming
// scenarios, use NewEncoder to write directly to an io.Writer.
package jsonmap

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
	"unsafe"
)

type Unmarshaler interface {
	UnmarshalJSON([]byte) error
}

type DecoderOptions struct {
	AllowTrailingComma bool
	WithErrorContext   bool
	Lenient            bool
}

type decoder struct {
	data    []byte
	pos     int
	len     int
	options DecoderOptions
}

// errorf creates a formatted error message including context.
func (d *decoder) errorf(msg string) error {
	if d.options.WithErrorContext {
		snippetEnd := d.pos + 20
		if snippetEnd > d.len {
			snippetEnd = d.len
		}
		snippet := b2s(d.data[d.pos:snippetEnd])
		return fmt.Errorf("%s at pos %d: near '%s'", msg, d.pos, snippet)
	}
	return errors.New(msg)
}

// skipWhitespace advances the position past any whitespace.
func (d *decoder) skipWhitespace() {
	for d.pos < d.len {
		switch d.data[d.pos] {
		case ' ', '\n', '\r', '\t':
			d.pos++
		default:
			return
		}
	}
}

// decodeValue decodes a JSON value.
func (d *decoder) decodeValue() (any, error) {
	d.skipWhitespace()
	if d.pos >= d.len {
		return nil, d.errorf("unexpected end of input")
	}
	switch d.data[d.pos] {
	case '"':
		return d.decodeString()
	case '{':
		return d.decodeObject()
	case '[':
		return d.decodeArray()
	case 't', 'f':
		return d.decodeBool()
	case 'n':
		return d.decodeNull()
	default:
		return d.decodeNumber()
	}
}

// decodeObject decodes a JSON object.
func (d *decoder) decodeObject() (map[string]any, error) {
	obj := make(map[string]any)
	d.pos++
	d.skipWhitespace()
	if d.pos < d.len && d.data[d.pos] == '}' {
		d.pos++
		return obj, nil
	}
	for {
		d.skipWhitespace()
		if d.pos >= d.len || d.data[d.pos] != '"' {
			return nil, d.errorf("expected string key")
		}
		key, err := d.decodeString()
		if err != nil {
			return nil, err
		}
		d.skipWhitespace()
		if d.pos >= d.len || d.data[d.pos] != ':' {
			return nil, d.errorf("expected ':' after key")
		}
		d.pos++
		d.skipWhitespace()
		val, err := d.decodeValue()
		if err != nil {
			return nil, err
		}
		obj[key] = val
		d.skipWhitespace()
		if d.pos >= d.len {
			return nil, d.errorf("unexpected end of object")
		}
		if d.data[d.pos] == ',' {
			d.pos++
			d.skipWhitespace()
			if d.options.AllowTrailingComma && d.pos < d.len && d.data[d.pos] == '}' {
				d.pos++
				break
			}
			continue
		} else if d.data[d.pos] == '}' {
			d.pos++
			break
		} else {
			return nil, d.errorf("expected ',' or '}' in object")
		}
	}
	return obj, nil
}

// decodeArray decodes a JSON array.
func (d *decoder) decodeArray() ([]any, error) {
	arr := make([]any, 0, 8)
	d.pos++
	d.skipWhitespace()
	if d.pos < d.len && d.data[d.pos] == ']' {
		d.pos++
		return arr, nil
	}
	for {
		d.skipWhitespace()
		val, err := d.decodeValue()
		if err != nil {
			return nil, err
		}
		arr = append(arr, val)
		d.skipWhitespace()
		if d.pos >= d.len {
			return nil, d.errorf("unexpected end of array")
		}
		if d.data[d.pos] == ',' {
			d.pos++
			d.skipWhitespace()
			if d.options.AllowTrailingComma && d.pos < d.len && d.data[d.pos] == ']' {
				d.pos++
				break
			}
			continue
		} else if d.data[d.pos] == ']' {
			d.pos++
			break
		} else {
			return nil, d.errorf("expected ',' or ']' in array")
		}
	}
	return arr, nil
}

// decodeString decodes a JSON string.
func (d *decoder) decodeString() (string, error) {
	d.pos++ // skip initial quote
	start := d.pos
	noEscape := true
	for d.pos < d.len {
		c := d.data[d.pos]
		if c == '"' {
			if noEscape {
				s := b2s(d.data[start:d.pos])
				d.pos++
				return s, nil
			}
		}
		if c == '\\' {
			noEscape = false
			break
		}
		d.pos++
	}
	// Reset position to start for a full escape decode.
	d.pos = start
	return d.decodeStringEscaped()
}

// decodeStringEscaped handles strings with escapes.
func (d *decoder) decodeStringEscaped() (string, error) {
	var runeBuffer [64]rune
	var runes []rune
	if d.len-d.pos <= 64 {
		runes = runeBuffer[:0]
	} else {
		runes = make([]rune, 0, d.len-d.pos)
	}
	for d.pos < d.len {
		c := d.data[d.pos]
		if c == '"' {
			d.pos++
			return string(runes), nil
		}
		if c == '\\' {
			d.pos++
			if d.pos >= d.len {
				return "", d.errorf("unexpected end after escape")
			}
			esc := d.data[d.pos]
			var r rune
			switch esc {
			case '"', '\\', '/':
				r = rune(esc)
			case 'b':
				r = '\b'
			case 'f':
				r = '\f'
			case 'n':
				r = '\n'
			case 'r':
				r = '\r'
			case 't':
				r = '\t'
			case 'u':
				if d.pos+4 >= d.len {
					return "", d.errorf("incomplete unicode escape")
				}
				hex := b2s(d.data[d.pos+1 : d.pos+5])
				v, err := strconv.ParseUint(hex, 16, 16)
				if err != nil {
					return "", d.errorf("invalid unicode escape")
				}
				r = rune(v)
				d.pos += 4
			default:
				return "", d.errorf("invalid escape character")
			}
			runes = append(runes, r)
			d.pos++
			continue
		}
		r, size := utf8.DecodeRune(d.data[d.pos:])
		runes = append(runes, r)
		d.pos += size
	}
	return "", d.errorf("unterminated string")
}

// decodeNumber decodes a JSON number.
func (d *decoder) decodeNumber() (float64, error) {
	start := d.pos
	for d.pos < d.len {
		c := d.data[d.pos]
		if (c >= '0' && c <= '9') || c == '-' || c == '+' || c == '.' || c == 'e' || c == 'E' {
			d.pos++
		} else {
			break
		}
	}
	numStr := b2s(d.data[start:d.pos])
	n, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, d.errorf("invalid number")
	}
	return n, nil
}

// decodeBool decodes a JSON boolean.
func (d *decoder) decodeBool() (bool, error) {
	if d.pos+4 <= d.len && b2s(d.data[d.pos:d.pos+4]) == "true" {
		d.pos += 4
		return true, nil
	}
	if d.pos+5 <= d.len && b2s(d.data[d.pos:d.pos+5]) == "false" {
		d.pos += 5
		return false, nil
	}
	return false, d.errorf("invalid boolean literal")
}

// decodeNull decodes a JSON null.
func (d *decoder) decodeNull() (any, error) {
	if d.pos+4 <= d.len && b2s(d.data[d.pos:d.pos+4]) == "null" {
		d.pos += 4
		return nil, nil
	}
	return nil, d.errorf("invalid null literal")
}

// captureValue returns the bytes for a JSON value.
func (d *decoder) captureValue() ([]byte, error) {
	start := d.pos
	_, err := d.decodeValue()
	if err != nil {
		return nil, err
	}
	return d.data[start:d.pos], nil
}

// ---------------------------
// Unmarshaling API (similar to encoding/json)
// ---------------------------

// Unmarshal parses the JSON-encoded data and stores the result in the value pointed to by v.
func Unmarshal(data []byte, v any) error {
	return UnmarshalWithOptions(data, v, DecoderOptions{})
}

// UnmarshalWithOptions is like Unmarshal but accepts custom decoding options.
func UnmarshalWithOptions(data []byte, v any, opts DecoderOptions) error {
	if v == nil {
		return errors.New("nil target provided")
	}
	switch target := v.(type) {
	case *map[string]any:
		d := newDecoder(data, opts)
		d.skipWhitespace()
		obj, err := d.decodeObject()
		if err != nil {
			return err
		}
		*target = obj
		return nil
	case *[]map[string]any:
		d := newDecoder(data, opts)
		d.skipWhitespace()
		arrRaw, err := d.decodeArray()
		if err != nil {
			return err
		}
		out := make([]map[string]any, len(arrRaw))
		for i, elem := range arrRaw {
			m, ok := elem.(map[string]any)
			if !ok {
				return fmt.Errorf("element %d is not an object", i)
			}
			out[i] = m
		}
		*target = out
		return nil
	case *string:
		d := newDecoder(data, opts)
		s, err := d.decodeString()
		if err != nil {
			return err
		}
		*target = s
		return nil
	case *float64:
		d := newDecoder(data, opts)
		n, err := d.decodeNumber()
		if err != nil {
			return err
		}
		*target = n
		return nil
	case *int:
		d := newDecoder(data, opts)
		n, err := d.decodeNumber()
		if err != nil {
			return err
		}
		*target = int(n)
		return nil
	case *bool:
		d := newDecoder(data, opts)
		b, err := d.decodeBool()
		if err != nil {
			return err
		}
		*target = b
		return nil
	case *interface{}:
		d := newDecoder(data, opts)
		d.skipWhitespace()
		val, err := d.decodeValue()
		if err != nil {
			return err
		}
		*target = val
		return nil
	}

	// For other types, use reflection.
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return errors.New("target must be a non-nil pointer")
	}
	if rv.Elem().Kind() == reflect.Struct {
		d := newDecoder(data, opts)
		d.skipWhitespace()
		return directDecodeStruct(d, rv.Elem())
	}
	return fmt.Errorf("unsupported type: %T", v)
}

// newDecoder creates a new decoder instance.
func newDecoder(data []byte, opts DecoderOptions) *decoder {
	return &decoder{
		data:    data,
		pos:     0,
		len:     len(data),
		options: opts,
	}
}

// ----------------------------
// Supporting reflection decode functions
// ----------------------------

type fieldInfo struct {
	index []int
	name  string
}

// structCache caches field information for struct types.
var structCache sync.Map

// getStructFields returns information about exported fields for a given struct type.
func getStructFields(t reflect.Type) []fieldInfo {
	if cached, ok := structCache.Load(t); ok {
		return cached.([]fieldInfo)
	}
	var fields []fieldInfo
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" { // unexported field
			continue
		}
		key := field.Name
		if tag := field.Tag.Get("json"); tag != "" {
			parts := strings.Split(tag, ",")
			if parts[0] != "" {
				key = parts[0]
			}
		}
		fields = append(fields, fieldInfo{index: field.Index, name: key})
	}
	structCache.Store(t, fields)
	return fields
}

// directDecodeStruct decodes a JSON object into a struct value using reflection.
func directDecodeStruct(d *decoder, v reflect.Value) error {
	if d.pos >= d.len || d.data[d.pos] != '{' {
		return d.errorf("expected '{' at beginning of object")
	}
	d.pos++
	d.skipWhitespace()

	fields := getStructFields(v.Type())
	fieldMap := make(map[string]fieldInfo, len(fields))
	for _, fi := range fields {
		fieldMap[fi.name] = fi
	}
	first := true
	for {
		d.skipWhitespace()
		if d.pos < d.len && d.data[d.pos] == '}' {
			d.pos++
			break
		}
		if !first {
			if d.data[d.pos] != ',' {
				return d.errorf("expected ',' between object fields")
			}
			d.pos++
			d.skipWhitespace()
		}
		first = false

		key, err := d.decodeString()
		if err != nil {
			return err
		}
		d.skipWhitespace()
		if d.pos >= d.len || d.data[d.pos] != ':' {
			return d.errorf("expected ':' after object key")
		}
		d.pos++
		d.skipWhitespace()
		if fi, ok := fieldMap[key]; ok {
			fv := v.FieldByIndex(fi.index)
			if fv.CanAddr() && fv.Addr().Type().Implements(reflect.TypeOf((*Unmarshaler)(nil)).Elem()) {
				raw, err := d.captureValue()
				if err != nil {
					return err
				}
				um := fv.Addr().Interface().(Unmarshaler)
				if err := um.UnmarshalJSON(raw); err != nil {
					return fmt.Errorf("field %q: %w", key, err)
				}
			} else {
				if err := decodeValueDirect(d, fv); err != nil {
					return fmt.Errorf("field %q: %w", key, err)
				}
			}
		} else {
			// Skip unknown field.
			if _, err := d.decodeValue(); err != nil {
				return err
			}
		}
		d.skipWhitespace()
	}
	return nil
}

// decodeValueDirect decodes a JSON value directly into a reflect.Value.
func decodeValueDirect(d *decoder, v reflect.Value) error {
	if v.CanAddr() && v.Addr().Type().Implements(reflect.TypeOf((*Unmarshaler)(nil)).Elem()) {
		raw, err := d.captureValue()
		if err != nil {
			return err
		}
		um := v.Addr().Interface().(Unmarshaler)
		return um.UnmarshalJSON(raw)
	}
	switch v.Kind() {
	case reflect.String:
		s, err := d.decodeString()
		if err != nil {
			return err
		}
		v.SetString(s)
	case reflect.Float64:
		n, err := d.decodeNumber()
		if err != nil {
			return err
		}
		v.SetFloat(n)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := d.decodeNumber()
		if err != nil {
			return err
		}
		v.SetInt(int64(n))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := d.decodeNumber()
		if err != nil {
			return err
		}
		if n < 0 {
			return errors.New("negative number for unsigned type")
		}
		v.SetUint(uint64(n))
	case reflect.Bool:
		b, err := d.decodeBool()
		if err != nil {
			return err
		}
		v.SetBool(b)
	case reflect.Struct:
		if v.Type() == reflect.TypeOf(time.Time{}) {
			s, err := d.decodeString()
			if err != nil {
				return err
			}
			t, err := time.Parse(time.RFC3339, s)
			if err != nil {
				return fmt.Errorf("invalid time format: %w", err)
			}
			v.Set(reflect.ValueOf(t))
		} else {
			return directDecodeStruct(d, v)
		}
	case reflect.Slice:
		arr, err := d.decodeArray()
		if err != nil {
			return err
		}
		slice := reflect.MakeSlice(v.Type(), len(arr), len(arr))
		for i := 0; i < len(arr); i++ {
			if err := assignValue(slice.Index(i), arr[i]); err != nil {
				return fmt.Errorf("index %d: %w", i, err)
			}
		}
		v.Set(slice)
	case reflect.Ptr:
		ptrVal := reflect.New(v.Type().Elem())
		if err := decodeValueDirect(d, ptrVal.Elem()); err != nil {
			return err
		}
		v.Set(ptrVal)
	default:
		val, err := d.decodeValue()
		if err != nil {
			return err
		}
		if err := assignValue(v, val); err != nil {
			return err
		}
	}
	return nil
}

// assignValue assigns a raw decoded value to a reflect.Value.
func assignValue(fv reflect.Value, raw any) error {
	if fv.CanAddr() && fv.Addr().Type().Implements(reflect.TypeOf((*Unmarshaler)(nil)).Elem()) {
		bytes, err := Marshal(raw)
		if err != nil {
			return err
		}
		um := fv.Addr().Interface().(Unmarshaler)
		return um.UnmarshalJSON(bytes)
	}
	if raw == nil {
		fv.Set(reflect.Zero(fv.Type()))
		return nil
	}
	switch fv.Kind() {
	case reflect.String:
		s, ok := raw.(string)
		if !ok {
			return fmt.Errorf("expected string, got %T", raw)
		}
		fv.SetString(s)
	case reflect.Float64:
		n, ok := raw.(float64)
		if !ok {
			return fmt.Errorf("expected float64, got %T", raw)
		}
		fv.SetFloat(n)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, ok := raw.(float64)
		if !ok {
			return fmt.Errorf("expected number for int, got %T", raw)
		}
		fv.SetInt(int64(n))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, ok := raw.(float64)
		if !ok {
			return fmt.Errorf("expected number for uint, got %T", raw)
		}
		if n < 0 {
			return errors.New("negative number for unsigned field")
		}
		fv.SetUint(uint64(n))
	case reflect.Bool:
		b, ok := raw.(bool)
		if !ok {
			return fmt.Errorf("expected bool, got %T", raw)
		}
		fv.SetBool(b)
	case reflect.Struct:
		m, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("expected object for struct, got %T", raw)
		}
		return decodeStruct(fv, m)
	case reflect.Slice:
		arr, ok := raw.([]any)
		if !ok {
			return fmt.Errorf("expected array, got %T", raw)
		}
		slice := reflect.MakeSlice(fv.Type(), len(arr), len(arr))
		for i := 0; i < len(arr); i++ {
			if err := assignValue(slice.Index(i), arr[i]); err != nil {
				return fmt.Errorf("index %d: %w", i, err)
			}
		}
		fv.Set(slice)
	case reflect.Ptr:
		ptrVal := reflect.New(fv.Type().Elem())
		if err := assignValue(ptrVal.Elem(), raw); err != nil {
			return err
		}
		fv.Set(ptrVal)
	default:
		val := reflect.ValueOf(raw)
		if !val.IsValid() {
			fv.Set(reflect.Zero(fv.Type()))
		} else {
			fv.Set(val)
		}
	}
	return nil
}

// decodeStruct decodes an object into a struct.
func decodeStruct(v reflect.Value, data map[string]any) error {
	fields := getStructFields(v.Type())
	for _, info := range fields {
		raw, exists := data[info.name]
		if !exists {
			continue
		}
		fv := v.FieldByIndex(info.index)
		if !fv.CanSet() {
			continue
		}
		if err := assignValue(fv, raw); err != nil {
			return fmt.Errorf("field %q: %w", info.name, err)
		}
	}
	return nil
}

// ----------------------
// Utility functions
// ----------------------

// b2s converts a byte slice to a string without allocation.
func b2s(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

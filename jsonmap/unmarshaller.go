package jsonmap

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
	"unsafe"
)

// ----------------------
// Zero-Allocation Helpers
// ----------------------

// b2s converts []byte to string without extra copy.
// (Be sure that the underlying slice is not modified.)
func b2s(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

// ----------------------
// JSON Decoder (Zero-Alloc)
// ----------------------

type decoder struct {
	data []byte
	pos  int
	len  int
}

func newDecoder(data []byte) *decoder {
	return &decoder{data: data, pos: 0, len: len(data)}
}

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

func (d *decoder) decodeValue() (any, error) {
	d.skipWhitespace()
	if d.pos >= d.len {
		return nil, errors.New("unexpected end of input")
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

func (d *decoder) decodeObject() (map[string]any, error) {
	obj := make(map[string]any)
	d.pos++ // skip '{'
	d.skipWhitespace()
	if d.pos < d.len && d.data[d.pos] == '}' {
		d.pos++
		return obj, nil
	}
	for {
		d.skipWhitespace()
		if d.pos >= d.len || d.data[d.pos] != '"' {
			return nil, errors.New("expected string key")
		}
		key, err := d.decodeString()
		if err != nil {
			return nil, err
		}
		d.skipWhitespace()
		if d.pos >= d.len || d.data[d.pos] != ':' {
			return nil, errors.New("expected ':' after key")
		}
		d.pos++ // skip ':'
		d.skipWhitespace()
		val, err := d.decodeValue()
		if err != nil {
			return nil, err
		}
		obj[key] = val
		d.skipWhitespace()
		if d.pos >= d.len {
			return nil, errors.New("unexpected end of object")
		}
		if d.data[d.pos] == ',' {
			d.pos++
			continue
		} else if d.data[d.pos] == '}' {
			d.pos++
			break
		} else {
			return nil, errors.New("expected ',' or '}' in object")
		}
	}
	return obj, nil
}

func (d *decoder) decodeArray() ([]any, error) {
	var arr []any
	d.pos++ // skip '['
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
			return nil, errors.New("unexpected end of array")
		}
		if d.data[d.pos] == ',' {
			d.pos++
			continue
		} else if d.data[d.pos] == ']' {
			d.pos++
			break
		} else {
			return nil, errors.New("expected ',' or ']' in array")
		}
	}
	return arr, nil
}

func (d *decoder) decodeString() (string, error) {
	// Consume opening quote.
	d.pos++
	start := d.pos
	for d.pos < d.len {
		c := d.data[d.pos]
		if c == '"' {
			// Fast path: no escapes.
			s := b2s(d.data[start:d.pos])
			d.pos++
			return s, nil
		}
		if c == '\\' {
			return d.decodeStringEscaped(start)
		}
		d.pos++
	}
	return "", errors.New("unterminated string")
}

func (d *decoder) decodeStringEscaped(start int) (string, error) {
	var runeStack [64]rune
	var runes []rune
	capEstimate := d.len - d.pos
	if capEstimate <= 64 {
		runes = runeStack[:0]
	} else {
		runes = make([]rune, 0, capEstimate)
	}
	// Append already-read bytes.
	runes = append(runes, []rune(b2s(d.data[start:d.pos]))...)
	for d.pos < d.len {
		c := d.data[d.pos]
		if c == '"' {
			d.pos++
			return string(runes), nil
		}
		if c == '\\' {
			d.pos++
			if d.pos >= d.len {
				return "", errors.New("unexpected end after escape")
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
					return "", errors.New("incomplete unicode escape")
				}
				hex := b2s(d.data[d.pos+1 : d.pos+5])
				v, err := strconv.ParseUint(hex, 16, 16)
				if err != nil {
					return "", errors.New("invalid unicode escape")
				}
				r = rune(v)
				d.pos += 4
			default:
				return "", errors.New("invalid escape character")
			}
			runes = append(runes, r)
			d.pos++
			continue
		}
		r, size := utf8.DecodeRune(d.data[d.pos:])
		runes = append(runes, r)
		d.pos += size
	}
	return "", errors.New("unterminated string")
}

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
	return strconv.ParseFloat(numStr, 64)
}

func (d *decoder) decodeBool() (bool, error) {
	if d.pos+4 <= d.len && b2s(d.data[d.pos:d.pos+4]) == "true" {
		d.pos += 4
		return true, nil
	}
	if d.pos+5 <= d.len && b2s(d.data[d.pos:d.pos+5]) == "false" {
		d.pos += 5
		return false, nil
	}
	return false, errors.New("invalid boolean literal")
}

func (d *decoder) decodeNull() (any, error) {
	if d.pos+4 <= d.len && b2s(d.data[d.pos:d.pos+4]) == "null" {
		d.pos += 4
		return nil, nil
	}
	return nil, errors.New("invalid null literal")
}

// ----------------------
// Caching of Struct Field Metadata
// ----------------------

type fieldInfo struct {
	index []int  // Field index chain (for nested fields)
	name  string // JSON key name to match
}

var structCache sync.Map // map[reflect.Type][]fieldInfo

func getStructFields(t reflect.Type) []fieldInfo {
	if cached, ok := structCache.Load(t); ok {
		return cached.([]fieldInfo)
	}
	var fields []fieldInfo
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		// Only process exported fields.
		if field.PkgPath != "" {
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

// ----------------------
// Unmarshal Implementation
// ----------------------

// Unmarshal decodes JSON data into the provided variable. It supports basic types,
// maps, slices, and structs (using reflection and caching).
func Unmarshal(data []byte, v any) error {
	if v == nil {
		return errors.New("nil target provided")
	}
	// Handle basic types directly.
	switch target := v.(type) {
	case *map[string]any:
		d := newDecoder(data)
		d.skipWhitespace()
		obj, err := d.decodeObject()
		if err != nil {
			return err
		}
		*target = obj
		return nil
	case *[]map[string]any:
		d := newDecoder(data)
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
		d := newDecoder(data)
		s, err := d.decodeString()
		if err != nil {
			return err
		}
		*target = s
		return nil
	case *float64:
		d := newDecoder(data)
		n, err := d.decodeNumber()
		if err != nil {
			return err
		}
		*target = n
		return nil
	case *int:
		d := newDecoder(data)
		n, err := d.decodeNumber()
		if err != nil {
			return err
		}
		*target = int(n)
		return nil
	case *bool:
		d := newDecoder(data)
		b, err := d.decodeBool()
		if err != nil {
			return err
		}
		*target = b
		return nil
	}

	// For structs, decode into a map first and then use reflection.
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return errors.New("target must be a non-nil pointer")
	}
	elem := rv.Elem()
	if elem.Kind() != reflect.Struct {
		return fmt.Errorf("unsupported type: %T", v)
	}

	d := newDecoder(data)
	d.skipWhitespace()
	m, err := d.decodeObject()
	if err != nil {
		return err
	}
	return decodeStruct(elem, m)
}

// decodeStruct uses reflection (with cached metadata) to assign values from the decoded map.
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

// assignValue converts and assigns the raw value to the field.
func assignValue(fv reflect.Value, raw any) error {
	// Handle nil: assign zero value.
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

// ----------------------
// Minimal JSON Marshal (Zero-Alloc for supported types)
// ----------------------

// Marshal encodes the provided value into JSON.
// It supports basic types, maps, slices, arrays, and structs.
func Marshal(v any) ([]byte, error) {
	if v == nil {
		return []byte("null"), nil
	}
	switch vv := v.(type) {
	case string:
		return []byte(fmt.Sprintf(`"%s"`, escapeString(vv))), nil
	case float64:
		return []byte(strconv.FormatFloat(vv, 'f', -1, 64)), nil
	case int:
		return []byte(strconv.Itoa(vv)), nil
	case bool:
		if vv {
			return []byte("true"), nil
		}
		return []byte("false"), nil
	case map[string]any:
		var buf bytes.Buffer
		estCapacity := 2 + len(vv)*20
		buf.Grow(estCapacity)
		buf.WriteByte('{')
		first := true
		for k, val := range vv {
			if !first {
				buf.WriteByte(',')
			}
			first = false
			buf.WriteString(fmt.Sprintf(`"%s":`, escapeString(k)))
			b, err := Marshal(val)
			if err != nil {
				return nil, err
			}
			buf.Write(b)
		}
		buf.WriteByte('}')
		return buf.Bytes(), nil
	case []any:
		var buf bytes.Buffer
		buf.WriteByte('[')
		for i, val := range vv {
			if i > 0 {
				buf.WriteByte(',')
			}
			b, err := Marshal(val)
			if err != nil {
				return nil, err
			}
			buf.Write(b)
		}
		buf.WriteByte(']')
		return buf.Bytes(), nil
	}

	// Fallback: use reflection to support slices, arrays, and structs.
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		var buf bytes.Buffer
		buf.WriteByte('[')
		for i := 0; i < rv.Len(); i++ {
			if i > 0 {
				buf.WriteByte(',')
			}
			elem := rv.Index(i).Interface()
			b, err := Marshal(elem)
			if err != nil {
				return nil, err
			}
			buf.Write(b)
		}
		buf.WriteByte(']')
		return buf.Bytes(), nil
	case reflect.Struct:
		m := structToMap(rv)
		return Marshal(m)
	default:
		return nil, fmt.Errorf("unsupported type for marshal: %T", v)
	}
}

// structToMap converts a struct into a map[string]any using json tags.
func structToMap(v reflect.Value) map[string]any {
	fields := getStructFields(v.Type())
	m := make(map[string]any, len(fields))
	for _, info := range fields {
		fv := v.FieldByIndex(info.index)
		m[info.name] = fv.Interface()
	}
	return m
}

// escapeString uses a single-pass strings.Builder to reduce allocations.
func escapeString(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

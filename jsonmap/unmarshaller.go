package jsonmap

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
	"unsafe"
)

func b2s(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

type DecoderOptions struct {
	AllowTrailingComma bool
	WithErrorContext   bool
}

type decoder struct {
	data    []byte
	pos     int
	len     int
	options DecoderOptions
}

func newDecoder(data []byte, opts DecoderOptions) *decoder {
	return &decoder{data: data, pos: 0, len: len(data), options: opts}
}

func (d *decoder) errorf(msg string) error {
	if d.options.WithErrorContext {
		return fmt.Errorf("%s at pos %d", msg, d.pos)
	}
	return errors.New(msg)
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

func (d *decoder) decodeString() (string, error) {
	d.pos++
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

	d.pos = start
	return d.decodeStringEscaped()
}

func (d *decoder) decodeStringEscaped() (string, error) {
	var runeStack [64]rune
	var runes []rune
	if d.len-d.pos <= 64 {
		runes = runeStack[:0]
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

func (d *decoder) decodeNull() (any, error) {
	if d.pos+4 <= d.len && b2s(d.data[d.pos:d.pos+4]) == "null" {
		d.pos += 4
		return nil, nil
	}
	return nil, d.errorf("invalid null literal")
}

type fieldInfo struct {
	index []int
	name  string
}

var structCache sync.Map

func getStructFields(t reflect.Type) []fieldInfo {
	if cached, ok := structCache.Load(t); ok {
		return cached.([]fieldInfo)
	}
	var fields []fieldInfo
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

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

func Unmarshal(data []byte, v any) error {
	return UnmarshalWithOptions(data, v, DecoderOptions{})
}

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

	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return errors.New("target must be a non-nil pointer")
	}
	elem := rv.Elem()
	if elem.Kind() != reflect.Struct {
		return fmt.Errorf("unsupported type: %T", v)
	}

	d := newDecoder(data, opts)
	d.skipWhitespace()
	m, err := d.decodeObject()
	if err != nil {
		return err
	}
	return decodeStruct(elem, m)
}

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

func assignValue(fv reflect.Value, raw any) error {
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

type encoder struct {
	buf []byte
	cap int
}

func newEncoder() *encoder {
	const initialCapacity = 4096
	return &encoder{
		buf: make([]byte, 0, initialCapacity),
		cap: initialCapacity,
	}
}

func (e *encoder) reset() {
	e.buf = e.buf[:0]
}

func (e *encoder) ensureCapacity(n int) error {
	if len(e.buf)+n > e.cap {
		return errors.New("buffer too small")
	}
	return nil
}

func (e *encoder) writeByte(b byte) error {
	if err := e.ensureCapacity(1); err != nil {
		return err
	}
	e.buf = append(e.buf, b)
	return nil
}

func (e *encoder) writeString(s string) error {
	if err := e.ensureCapacity(len(s)); err != nil {
		return err
	}
	e.buf = append(e.buf, s...)
	return nil
}

func (e *encoder) encode(v any) error {
	switch vv := v.(type) {
	case nil:
		return e.writeString("null")
	case string:
		if err := e.writeByte('"'); err != nil {
			return err
		}
		if err := e.encodeString(vv); err != nil {
			return err
		}
		return e.writeByte('"')
	case float64:
		s := strconv.FormatFloat(vv, 'f', -1, 64)
		return e.writeString(s)
	case int:
		s := strconv.FormatInt(int64(vv), 10)
		return e.writeString(s)
	case bool:
		if vv {
			return e.writeString("true")
		}
		return e.writeString("false")
	case map[string]any:
		return e.encodeMap(vv)
	case []any:
		return e.encodeSlice(vv)
	default:
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.Slice, reflect.Array:
			return e.encodeReflectSlice(rv)
		case reflect.Struct:
			return e.encodeStruct(rv)
		default:
			return fmt.Errorf("unsupported type for marshal: %T", v)
		}
	}
}

func (e *encoder) encodeMap(m map[string]any) error {
	if err := e.writeByte('{'); err != nil {
		return err
	}
	first := true
	for k, val := range m {
		if !first {
			if err := e.writeByte(','); err != nil {
				return err
			}
		}
		first = false
		if err := e.writeByte('"'); err != nil {
			return err
		}
		if err := e.encodeString(k); err != nil {
			return err
		}
		if err := e.writeByte('"'); err != nil {
			return err
		}
		if err := e.writeByte(':'); err != nil {
			return err
		}
		if err := e.encode(val); err != nil {
			return err
		}
	}
	return e.writeByte('}')
}

func (e *encoder) encodeSlice(s []any) error {
	if err := e.writeByte('['); err != nil {
		return err
	}
	for i, val := range s {
		if i > 0 {
			if err := e.writeByte(','); err != nil {
				return err
			}
		}
		if err := e.encode(val); err != nil {
			return err
		}
	}
	return e.writeByte(']')
}

func (e *encoder) encodeReflectSlice(rv reflect.Value) error {
	if err := e.writeByte('['); err != nil {
		return err
	}
	n := rv.Len()
	for i := 0; i < n; i++ {
		if i > 0 {
			if err := e.writeByte(','); err != nil {
				return err
			}
		}
		if err := e.encode(rv.Index(i).Interface()); err != nil {
			return err
		}
	}
	return e.writeByte(']')
}

func (e *encoder) encodeStruct(rv reflect.Value) error {
	if err := e.writeByte('{'); err != nil {
		return err
	}
	rt := rv.Type()
	first := true
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		if field.PkgPath != "" {
			continue
		}
		if !first {
			if err := e.writeByte(','); err != nil {
				return err
			}
		}
		first = false
		key := field.Name
		if tag := field.Tag.Get("json"); tag != "" {
			parts := strings.Split(tag, ",")
			if parts[0] != "" {
				key = parts[0]
			}
		}
		if err := e.writeByte('"'); err != nil {
			return err
		}
		if err := e.encodeString(key); err != nil {
			return err
		}
		if err := e.writeByte('"'); err != nil {
			return err
		}
		if err := e.writeByte(':'); err != nil {
			return err
		}
		if err := e.encode(rv.Field(i).Interface()); err != nil {
			return fmt.Errorf("field %q: %w", key, err)
		}
	}
	return e.writeByte('}')
}

func (e *encoder) encodeString(s string) error {
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' || c == '"' || c < 0x20 {
			if start < i {
				if err := e.writeString(s[start:i]); err != nil {
					return err
				}
			}
			switch c {
			case '\\', '"':
				if err := e.writeString(`\`); err != nil {
					return err
				}
				if err := e.writeByte(c); err != nil {
					return err
				}
			case '\n':
				if err := e.writeString(`\n`); err != nil {
					return err
				}
			case '\r':
				if err := e.writeString(`\r`); err != nil {
					return err
				}
			case '\t':
				if err := e.writeString(`\t`); err != nil {
					return err
				}
			default:
				if err := e.writeString(`\u00`); err != nil {
					return err
				}
				hex := "0123456789abcdef"
				if err := e.writeByte(hex[c>>4]); err != nil {
					return err
				}
				if err := e.writeByte(hex[c&0xF]); err != nil {
					return err
				}
			}
			start = i + 1
		}
	}
	if start < len(s) {
		return e.writeString(s[start:])
	}
	return nil
}

var encoderPool = sync.Pool{
	New: func() any { return newEncoder() },
}

func Marshal(v any) ([]byte, error) {
	enc := encoderPool.Get().(*encoder)
	enc.reset()
	if err := enc.encode(v); err != nil {
		encoderPool.Put(enc)
		return nil, err
	}
	ret := make([]byte, len(enc.buf))
	copy(ret, enc.buf)
	encoderPool.Put(enc)
	return ret, nil
}

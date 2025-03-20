package jsonmap

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
	"unsafe"
)

// b2s converts []byte to string without extra copy.
// (Be sure that the underlying slice is not modified.)
func b2s(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

// JSONUnmarshaler may be implemented by types that want to decode themselves.
type JSONUnmarshaler interface {
	UnmarshalJSONCustom([]byte) error
}

// ----------------------------------------------------------------------------
// Custom Decoder (zero-allocation techniques used where possible)
// ----------------------------------------------------------------------------

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

// Updated decodeStringEscaped using a fixed-size stack buffer for small allocations.
func (d *decoder) decodeStringEscaped(start int) (string, error) {
	// Use a fixed-size array if the estimated capacity is small.
	var runeStack [64]rune
	var runes []rune
	capEstimate := d.len - d.pos
	if capEstimate <= 64 {
		runes = runeStack[:0]
	} else {
		runes = make([]rune, 0, capEstimate)
	}
	// ...existing loop logic...
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

// ----------------------------------------------------------------------------
// Unmarshal with same signature as json.Unmarshal (without reflect for core types)
// ----------------------------------------------------------------------------

func Unmarshal(data []byte, v any) error {
	// If v implements JSONUnmarshaler, delegate.
	if um, ok := v.(JSONUnmarshaler); ok {
		return um.UnmarshalJSONCustom(data)
	}
	d := newDecoder(data)
	d.skipWhitespace()
	switch target := v.(type) {
	case *map[string]any:
		obj, err := d.decodeObject()
		if err != nil {
			return err
		}
		*target = obj
		return nil
	case *[]map[string]any:
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
		s, err := d.decodeString()
		if err != nil {
			return err
		}
		*target = s
		return nil
	case *float64:
		n, err := d.decodeNumber()
		if err != nil {
			return err
		}
		*target = n
		return nil
	case *int:
		n, err := d.decodeNumber()
		if err != nil {
			return err
		}
		*target = int(n)
		return nil
	case *bool:
		b, err := d.decodeBool()
		if err != nil {
			return err
		}
		*target = b
		return nil
	default:
		return errors.New("unsupported type: only *map[string]any, *[]map[string]any, *string, *float64, *int, *bool or JSONUnmarshaler are supported")
	}
}

// ----------------------------------------------------------------------------
// Zero Allocation Hot Path Get & Set
//
// The following implementations are basic examples.
// A truly zero‐allocation solution would avoid full decoding/re‐encoding,
// instead scanning the JSON bytes in place (as done in libraries like gjson/sjson).
// Here we illustrate a simple approach using our custom Unmarshal/Marshal.
// ----------------------------------------------------------------------------

// Get retrieves a value from JSON bytes by a dot‑notation key path.
// For example: Get(data, "nested.obj.inner")
func Get(data []byte, keys ...string) (any, error) {
	var m map[string]any
	if err := Unmarshal(data, &m); err != nil {
		return nil, err
	}
	var current any = m
	for _, k := range keys {
		mp, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("path %q: not an object", k)
		}
		val, exists := mp[k]
		if !exists {
			return nil, fmt.Errorf("path %q: key not found", k)
		}
		current = val
	}
	return current, nil
}

// Set updates a value in JSON bytes by a dot‑notation key path and returns new JSON bytes.
// For example: Set(data, "nested.obj.inner", "newvalue")
// Note: This implementation decodes the entire JSON, modifies the in‑memory map, and re‑encodes.
// A truly zero‐allocation solution would modify the JSON bytes in place if possible.
func Set(data []byte, key string, value any) ([]byte, error) {
	var m map[string]any
	if err := Unmarshal(data, &m); err != nil {
		return nil, err
	}
	// Split key by dot.
	parts := strings.Split(key, ".")
	current := m
	for i, k := range parts {
		if i == len(parts)-1 {
			// Set value.
			current[k] = value
		} else {
			// Traverse nested object.
			sub, ok := current[k].(map[string]any)
			if !ok {
				// Create new nested object if missing.
				sub = make(map[string]any)
				current[k] = sub
			}
			current = sub
		}
	}
	return Marshal(m)
}

// ----------------------------------------------------------------------------
// Minimal JSON Marshal (for our supported types)
// ----------------------------------------------------------------------------

func Marshal(v any) ([]byte, error) {
	// This simple encoder supports map[string]any, []any, string, float64, bool and nil.
	switch vv := v.(type) {
	case nil:
		return []byte("null"), nil
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
		// Rough estimation: braces plus each key/value pair.
		estCapacity := 2 + len(vv)*20
		buf.Grow(estCapacity)
		buf.WriteByte('{')
		first := true
		for k, val := range vv {
			if !first {
				buf.WriteByte(',')
			}
			first = false
			// Write key.
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
	default:
		return nil, fmt.Errorf("unsupported type for marshal: %T", vv)
	}
}

// Updated escapeString to use a single-pass strings.Builder loop.
func escapeString(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	// ...replacing multiple ReplaceAll calls with a single loop...
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

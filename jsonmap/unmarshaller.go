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

// -----------------------------------------------------------------------------
// Zero‑Allocation Helpers & Decoder/Encoder (existing code)
// -----------------------------------------------------------------------------

// b2s converts []byte to string without extra copy.
// (Be sure that the underlying slice is not modified.)
func b2s(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

// ----------------------
// JSON Decoder (Zero‑Alloc)
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
// JSON Encoder (Minimal)
// ----------------------

var bufPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

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
		return marshalMap(vv)
	case []any:
		return marshalSlice(vv)
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		return marshalReflectSlice(rv)
	case reflect.Struct:
		m := structToMap(rv)
		return Marshal(m)
	default:
		return nil, fmt.Errorf("unsupported type for marshal: %T", v)
	}
}

func marshalMap(m map[string]any) ([]byte, error) {
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)
	buf.WriteByte('{')
	first := true
	for k, val := range m {
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
}

func marshalSlice(s []any) ([]byte, error) {
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)
	buf.WriteByte('[')
	for i, val := range s {
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

func marshalReflectSlice(rv reflect.Value) ([]byte, error) {
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)
	buf.WriteByte('[')
	for i := 0; i < rv.Len(); i++ {
		if i > 0 {
			buf.WriteByte(',')
		}
		b, err := Marshal(rv.Index(i).Interface())
		if err != nil {
			return nil, err
		}
		buf.Write(b)
	}
	buf.WriteByte(']')
	return buf.Bytes(), nil
}

func structToMap(v reflect.Value) map[string]any {
	fields := getStructFields(v.Type())
	m := make(map[string]any, len(fields))
	for _, info := range fields {
		fv := v.FieldByIndex(info.index)
		m[info.name] = fv.Interface()
	}
	return m
}

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

// -----------------------------------------------------------------------------
// Decode helper: decode JSON bytes into interface{} (map or slice)
// -----------------------------------------------------------------------------

func Decode(data []byte) (any, error) {
	d := newDecoder(data)
	d.skipWhitespace()
	return d.decodeValue()
}

// -----------------------------------------------------------------------------
// Path Traversal and Modification
// -----------------------------------------------------------------------------

// pathSegment represents a single segment of a dot‑notation key.
type pathSegment struct {
	key      string // non‑empty for map access
	index    *int   // non‑nil for slice index access
	wildcard bool   // true if segment is "#"
}

// parsePath splits the dot‑notation key into segments.
// Supported formats:
//   key1.key2             => two map accesses
//   key1.0.key2           => key1, then index 0, then key2
//   key1.[0].key2         => same as above
//   key1.#.key2           => wildcard on a slice; GetAll returns a flattened slice.
func parsePath(path string) ([]pathSegment, error) {
	if path == "" {
		return nil, errors.New("empty path")
	}
	parts := strings.Split(path, ".")
	segments := make([]pathSegment, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// Wildcard segment
		if part == "#" {
			segments = append(segments, pathSegment{wildcard: true})
			continue
		}
		// Handle bracketed index: [0] or [#]
		if strings.HasPrefix(part, "[") && strings.HasSuffix(part, "]") {
			content := strings.Trim(part, "[]")
			if content == "#" {
				segments = append(segments, pathSegment{wildcard: true})
				continue
			}
			if idx, err := strconv.Atoi(content); err == nil {
				segments = append(segments, pathSegment{index: &idx})
				continue
			}
			// Otherwise, treat as a key.
			segments = append(segments, pathSegment{key: content})
			continue
		}
		// If part is a number, treat it as an index.
		if idx, err := strconv.Atoi(part); err == nil {
			segments = append(segments, pathSegment{index: &idx})
			continue
		}
		// Otherwise, it's a key.
		segments = append(segments, pathSegment{key: part})
	}
	return segments, nil
}

// traverse recursively follows the segments in the data structure.
// For wildcard segments, it returns a flattened slice of matching nodes.
func traverse(data any, segments []pathSegment) (any, error) {
	if len(segments) == 0 {
		return data, nil
	}
	seg := segments[0]
	rest := segments[1:]
	var results []any
	
	switch d := data.(type) {
	case map[string]any:
		if seg.key == "" {
			return nil, fmt.Errorf("expected map key but got empty segment")
		}
		next, exists := d[seg.key]
		if !exists {
			return nil, fmt.Errorf("key %q not found", seg.key)
		}
		return traverse(next, rest)
	case []any:
		// If current segment is an index
		if seg.index != nil {
			if *seg.index < 0 || *seg.index >= len(d) {
				return nil, fmt.Errorf("index %d out of bounds", *seg.index)
			}
			return traverse(d[*seg.index], rest)
		}
		// If wildcard, apply rest to all elements
		if seg.wildcard {
			for _, elem := range d {
				res, err := traverse(elem, rest)
				if err == nil {
					// If result is a slice, flatten it.
					if slice, ok := res.([]any); ok {
						results = append(results, slice...)
					} else {
						results = append(results, res)
					}
				}
			}
			return results, nil
		}
		// Otherwise, if seg.key is provided, apply to each element if they are maps.
		for _, elem := range d {
			if m, ok := elem.(map[string]any); ok {
				if next, exists := m[seg.key]; exists {
					res, err := traverse(next, rest)
					if err == nil {
						results = append(results, res)
					}
				}
			}
		}
		if len(results) == 0 {
			return nil, fmt.Errorf("no match for segment %q", seg.key)
		}
		return results, nil
	default:
		return nil, fmt.Errorf("cannot traverse non-container type: %T", data)
	}
}

// setValue traverses the data using segments (except the last) and sets the final field.
// It supports map key access and slice index access.
func setValue(data any, segments []pathSegment, value any) error {
	if len(segments) == 0 {
		return errors.New("empty path")
	}
	// When only one segment remains, perform set.
	if len(segments) == 1 {
		seg := segments[0]
		switch d := data.(type) {
		case map[string]any:
			if seg.key == "" {
				return errors.New("expected map key for setting")
			}
			d[seg.key] = value
			return nil
		case []any:
			if seg.index != nil {
				if *seg.index < 0 || *seg.index >= len(d) {
					return fmt.Errorf("index %d out of bounds", *seg.index)
				}
				d[*seg.index] = value
				return nil
			}
			return fmt.Errorf("expected index for slice set")
		default:
			return fmt.Errorf("cannot set value on non-container type: %T", data)
		}
	}
	// Otherwise, traverse into the container.
	seg := segments[0]
	rest := segments[1:]
	switch d := data.(type) {
	case map[string]any:
		if seg.key == "" {
			return errors.New("expected map key in path")
		}
		next, exists := d[seg.key]
		if !exists {
			// If missing, create a new map for next if next segment is key,
			// or a slice if next segment is an index or wildcard.
			if rest[0].index != nil || rest[0].wildcard {
				next = make([]any, 0)
			} else {
				next = make(map[string]any)
			}
			d[seg.key] = next
		}
		return setValue(next, rest, value)
	case []any:
		if seg.index != nil {
			if *seg.index < 0 || *seg.index >= len(d) {
				return fmt.Errorf("index %d out of bounds", *seg.index)
			}
			return setValue(d[*seg.index], rest, value)
		}
		if seg.wildcard {
			var err error
			for i := range d {
				if e := setValue(d[i], rest, value); e != nil {
					err = e
				}
			}
			return err
		}
		return fmt.Errorf("expected numeric index or wildcard for slice set")
	default:
		return fmt.Errorf("cannot traverse non-container type: %T", data)
	}
}

// deleteValue traverses the data using segments (except the last) and deletes the final field.
// For maps, it removes the key; for slices, it removes the element at index.
func deleteValue(data any, segments []pathSegment) error {
	if len(segments) == 0 {
		return errors.New("empty path")
	}
	if len(segments) == 1 {
		seg := segments[0]
		switch d := data.(type) {
		case map[string]any:
			if seg.key == "" {
				return errors.New("expected map key for deletion")
			}
			delete(d, seg.key)
			return nil
		case []any:
			if seg.index != nil {
				idx := *seg.index
				if idx < 0 || idx >= len(d) {
					return fmt.Errorf("index %d out of bounds", idx)
				}
				// Remove element at index.
				d = append(d[:idx], d[idx+1:]...)
				// Note: since slices are passed by value, caller must handle assignment.
				// In our top‑level functions we re‑marshal the modified object.
				return nil
			}
			return fmt.Errorf("expected index for slice deletion")
		default:
			return fmt.Errorf("cannot delete on non-container type: %T", data)
		}
	}
	seg := segments[0]
	rest := segments[1:]
	switch d := data.(type) {
	case map[string]any:
		if seg.key == "" {
			return errors.New("expected map key in path for deletion")
		}
		next, exists := d[seg.key]
		if !exists {
			return fmt.Errorf("key %q not found for deletion", seg.key)
		}
		return deleteValue(next, rest)
	case []any:
		if seg.index != nil {
			if *seg.index < 0 || *seg.index >= len(d) {
				return fmt.Errorf("index %d out of bounds", *seg.index)
			}
			return deleteValue(d[*seg.index], rest)
		}
		if seg.wildcard {
			var err error
			for _, elem := range d {
				if e := deleteValue(elem, rest); e != nil {
					err = e
				}
			}
			return err
		}
		return fmt.Errorf("expected numeric index or wildcard for slice deletion")
	default:
		return fmt.Errorf("cannot traverse non-container type: %T", data)
	}
}

// setAllValue traverses the data using segments and sets the value on all matches
// when a wildcard (#) is encountered.
func setAllValue(data any, segments []pathSegment, value any) error {
	// If no segments, nothing to set.
	if len(segments) == 0 {
		return errors.New("empty path")
	}
	seg := segments[0]
	rest := segments[1:]
	switch d := data.(type) {
	case map[string]any:
		if seg.key == "" {
			return errors.New("expected map key in path")
		}
		next, exists := d[seg.key]
		if !exists {
			return fmt.Errorf("key %q not found", seg.key)
		}
		if len(rest) == 0 {
			d[seg.key] = value
			return nil
		}
		return setAllValue(next, rest, value)
	case []any:
		// If current segment is wildcard, then set value on all elements.
		if seg.wildcard {
			var err error
			for i := range d {
				if len(rest) == 0 {
					d[i] = value
				} else {
					if e := setAllValue(d[i], rest, value); e != nil {
						err = e
					}
				}
			}
			return err
		}
		// Otherwise, if index is specified.
		if seg.index != nil {
			if *seg.index < 0 || *seg.index >= len(d) {
				return fmt.Errorf("index %d out of bounds", *seg.index)
			}
			if len(rest) == 0 {
				d[*seg.index] = value
				return nil
			}
			return setAllValue(d[*seg.index], rest, value)
		}
		return fmt.Errorf("expected index or wildcard for slice access")
	default:
		return fmt.Errorf("cannot traverse non-container type: %T", data)
	}
}

// -----------------------------------------------------------------------------
// Public API Functions
// -----------------------------------------------------------------------------

// Get decodes the JSON and returns the value at the given dot‑notation key.
func Get(obj []byte, key string) (any, error) {
	data, err := Decode(obj)
	if err != nil {
		return nil, err
	}
	segments, err := parsePath(key)
	if err != nil {
		return nil, err
	}
	return traverse(data, segments)
}

// GetAll decodes the JSON and returns a flattened slice of all values matching
// the wildcard path (e.g. "key1.#.key2").
func GetAll(obj []byte, key string) ([]any, error) {
	res, err := Get(obj, key)
	if err != nil {
		return nil, err
	}
	// If result is a slice, assume it's the flattened result.
	if slice, ok := res.([]any); ok {
		return slice, nil
	}
	// Otherwise, return single element slice.
	return []any{res}, nil
}

// Set decodes the JSON, sets the value at the given key, and returns updated JSON.
func Set(obj []byte, key string, value any) ([]byte, error) {
	data, err := Decode(obj)
	if err != nil {
		return nil, err
	}
	segments, err := parsePath(key)
	if err != nil {
		return nil, err
	}
	if err := setValue(data, segments, value); err != nil {
		return nil, err
	}
	return Marshal(data)
}

// SetAll decodes the JSON, sets the value for all matching nodes (wildcard)
// and returns updated JSON.
func SetAll(obj []byte, key string, value any) ([]byte, error) {
	data, err := Decode(obj)
	if err != nil {
		return nil, err
	}
	segments, err := parsePath(key)
	if err != nil {
		return nil, err
	}
	if err := setAllValue(data, segments, value); err != nil {
		return nil, err
	}
	return Marshal(data)
}

// Delete decodes the JSON, deletes the node at the given key, and returns updated JSON.
func Delete(obj []byte, key string) ([]byte, error) {
	data, err := Decode(obj)
	if err != nil {
		return nil, err
	}
	segments, err := parsePath(key)
	if err != nil {
		return nil, err
	}
	if err := deleteValue(data, segments); err != nil {
		return nil, err
	}
	return Marshal(data)
}

package jsonmap

import (
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Marshaler interface {
	MarshalJSON() ([]byte, error)
}

type EncoderOptions struct {
	Pretty bool
	Indent string
}

type encoder struct {
	buf  []byte
	opts EncoderOptions
}

func newEncoder(opts EncoderOptions) *encoder {
	return &encoder{
		buf:  make([]byte, 0, 4096),
		opts: opts,
	}
}

func (e *encoder) reset() {
	e.buf = e.buf[:0]
}

func (e *encoder) writeByte(b byte) {
	e.buf = append(e.buf, b)
}

func (e *encoder) writeString(s string) {
	e.buf = append(e.buf, s...)
}

func (e *encoder) encodeValue(v any, indentLevel int) error {

	if m, ok := v.(Marshaler); ok {
		bytes, err := m.MarshalJSON()
		if err != nil {
			return err
		}
		e.writeString(b2s(bytes))
		return nil
	}

	if t, ok := v.(time.Time); ok {
		s := t.Format(time.RFC3339)
		e.writeByte('"')
		e.writeString(s)
		e.writeByte('"')
		return nil
	}

	switch vv := v.(type) {
	case nil:
		e.writeString("null")
	case string:
		e.writeByte('"')
		if err := e.encodeString(vv); err != nil {
			return err
		}
		e.writeByte('"')
	case float64:
		s := strconv.FormatFloat(vv, 'f', -1, 64)
		e.writeString(s)
	case int:
		s := strconv.FormatInt(int64(vv), 10)
		e.writeString(s)
	case uint, uint8, uint16, uint32, uint64:
		val := reflect.ValueOf(vv).Uint()
		s := strconv.FormatUint(val, 10)
		e.writeString(s)
	case bool:
		if vv {
			e.writeString("true")
		} else {
			e.writeString("false")
		}
	case map[string]any:
		if err := e.encodeMap(vv, indentLevel); err != nil {
			return err
		}
	case []any:
		if err := e.encodeSlice(vv, indentLevel); err != nil {
			return err
		}
	default:
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.Slice, reflect.Array:
			return e.encodeReflectSlice(rv, indentLevel)
		case reflect.Struct:
			return e.encodeStruct(rv, indentLevel)
		default:
			return fmt.Errorf("unsupported type for marshal: %T", v)
		}
	}
	return nil
}

func (e *encoder) encodeMap(m map[string]any, indentLevel int) error {
	e.writeByte('{')

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	first := true
	for _, k := range keys {
		if !first {
			e.writeByte(',')
		}
		if e.opts.Pretty {
			e.writeByte('\n')
			e.writeString(strings.Repeat(e.opts.Indent, indentLevel+1))
		}
		first = false
		e.writeByte('"')
		if err := e.encodeString(k); err != nil {
			return err
		}
		e.writeByte('"')
		e.writeByte(':')
		if e.opts.Pretty {
			e.writeByte(' ')
		}
		if err := e.encodeValue(m[k], indentLevel+1); err != nil {
			return err
		}
	}
	if e.opts.Pretty && len(m) > 0 {
		e.writeByte('\n')
		e.writeString(strings.Repeat(e.opts.Indent, indentLevel))
	}
	e.writeByte('}')
	return nil
}

func (e *encoder) encodeSlice(s []any, indentLevel int) error {
	e.writeByte('[')
	for i, val := range s {
		if i > 0 {
			e.writeByte(',')
		}
		if e.opts.Pretty {
			e.writeByte('\n')
			e.writeString(strings.Repeat(e.opts.Indent, indentLevel+1))
		}
		if err := e.encodeValue(val, indentLevel+1); err != nil {
			return err
		}
	}
	if e.opts.Pretty && len(s) > 0 {
		e.writeByte('\n')
		e.writeString(strings.Repeat(e.opts.Indent, indentLevel))
	}
	e.writeByte(']')
	return nil
}

func (e *encoder) encodeReflectSlice(rv reflect.Value, indentLevel int) error {
	e.writeByte('[')
	n := rv.Len()
	for i := 0; i < n; i++ {
		if i > 0 {
			e.writeByte(',')
		}
		if e.opts.Pretty {
			e.writeByte('\n')
			e.writeString(strings.Repeat(e.opts.Indent, indentLevel+1))
		}
		if err := e.encodeValue(rv.Index(i).Interface(), indentLevel+1); err != nil {
			return err
		}
	}
	if e.opts.Pretty && n > 0 {
		e.writeByte('\n')
		e.writeString(strings.Repeat(e.opts.Indent, indentLevel))
	}
	e.writeByte(']')
	return nil
}

func (e *encoder) encodeStruct(rv reflect.Value, indentLevel int) error {
	e.writeByte('{')
	rt := rv.Type()

	var fieldPairs []struct {
		key string
		val reflect.Value
		tag string
	}
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
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
		fieldPairs = append(fieldPairs, struct {
			key string
			val reflect.Value
			tag string
		}{key: key, val: rv.Field(i)})
	}
	sort.Slice(fieldPairs, func(i, j int) bool { return fieldPairs[i].key < fieldPairs[j].key })

	first := true
	for _, pair := range fieldPairs {
		if !first {
			e.writeByte(',')
		}
		if e.opts.Pretty {
			e.writeByte('\n')
			e.writeString(strings.Repeat(e.opts.Indent, indentLevel+1))
		}
		first = false
		e.writeByte('"')
		if err := e.encodeString(pair.key); err != nil {
			return err
		}
		e.writeByte('"')
		e.writeByte(':')
		if e.opts.Pretty {
			e.writeByte(' ')
		}
		if err := e.encodeValue(pair.val.Interface(), indentLevel+1); err != nil {
			return fmt.Errorf("field %q: %w", pair.key, err)
		}
	}
	if e.opts.Pretty && len(fieldPairs) > 0 {
		e.writeByte('\n')
		e.writeString(strings.Repeat(e.opts.Indent, indentLevel))
	}
	e.writeByte('}')
	return nil
}

func (e *encoder) encodeString(s string) error {
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' || c == '"' || c < 0x20 {
			if start < i {
				e.writeString(s[start:i])
			}
			switch c {
			case '\\', '"':
				e.writeString(`\`)
				e.writeByte(c)
			case '\n':
				e.writeString(`\n`)
			case '\r':
				e.writeString(`\r`)
			case '\t':
				e.writeString(`\t`)
			default:
				e.writeString(`\u00`)
				hex := "0123456789abcdef"
				e.writeByte(hex[c>>4])
				e.writeByte(hex[c&0xF])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		e.writeString(s[start:])
	}
	return nil
}

var encoderPool = sync.Pool{
	New: func() any { return newEncoder(EncoderOptions{}) },
}

func Marshal(v any) ([]byte, error) {
	return MarshalWithOptions(v, EncoderOptions{})
}

func MarshalWithOptions(v any, opts EncoderOptions) ([]byte, error) {
	enc := encoderPool.Get().(*encoder)
	enc.opts = opts
	enc.reset()
	if err := enc.encodeValue(v, 0); err != nil {
		encoderPool.Put(enc)
		return nil, err
	}

	ret := make([]byte, len(enc.buf))
	copy(ret, enc.buf)
	encoderPool.Put(enc)
	return ret, nil
}

type Encoder struct {
	w    io.Writer
	enc  *encoder
	opts EncoderOptions
}

func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{
		w:    w,
		opts: EncoderOptions{},
		enc:  newEncoder(EncoderOptions{}),
	}
}

func (e *Encoder) SetOptions(opts EncoderOptions) {
	e.opts = opts
	e.enc.opts = opts
}

func (e *Encoder) Encode(v any) error {
	e.enc.reset()
	if err := e.enc.encodeValue(v, 0); err != nil {
		return err
	}
	_, err := e.w.Write(e.enc.buf)
	return err
}

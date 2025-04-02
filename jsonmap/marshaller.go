package jsonmap

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

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

// New streaming Encoder implementation.
type Encoder struct {
	w   io.Writer
	enc *encoder
}

func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{
		w:   w,
		enc: newEncoder(),
	}
}

func (e *Encoder) Encode(v any) error {
	e.enc.reset()
	if err := e.enc.encode(v); err != nil {
		return err
	}
	_, err := e.w.Write(e.enc.buf)
	return err
}

package v2

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"sync"
)

var bufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

func canonicalize(v any) (string, error) {
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	if err := canonicalizeToBuffer(buf, v); err != nil {
		bufferPool.Put(buf)
		return "", err
	}
	result := buf.String()
	bufferPool.Put(buf)
	return result, nil
}

func canonicalizeToBuffer(buf *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case map[string]any:
		buf.WriteByte('{')

		var keys []string
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}

			b, err := json.Marshal(k)
			if err != nil {
				return err
			}
			buf.Write(b)
			buf.WriteByte(':')
			if err := canonicalizeToBuffer(buf, t[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	case []any:
		buf.WriteByte('[')
		for i, elem := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := canonicalizeToBuffer(buf, elem); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	default:

		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		buf.Write(b)
	}
	return nil
}

func computeCacheKey(v any) (string, error) {
	canonical, err := canonicalize(v)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(h[:]), nil
}

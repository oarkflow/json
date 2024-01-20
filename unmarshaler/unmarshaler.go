package unmarshaler

import (
	"encoding/json"
)

type Unmarshaler func([]byte, any) error

var (
	unmarshaler Unmarshaler
)

func init() {
	unmarshaler = json.Unmarshal
}

func SetUnmarshaler(m Unmarshaler) {
	unmarshaler = m
}

func Instance() Unmarshaler {
	return unmarshaler
}

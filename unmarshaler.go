package json

import (
	"encoding/json"
)

type Unmarshaler func([]byte, any) error

var (
	unmarshaler Unmarshaler
)

func DefaultUnmarshaler() {
	unmarshaler = json.Unmarshal
}

func SetUnmarshaler(m Unmarshaler) {
	unmarshaler = m
}

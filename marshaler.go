package json

import (
	"encoding/json"
)

type Marshaler func(any) ([]byte, error)
type Indenter func(v any, prefix, indent string) ([]byte, error)
type Unmarshaler func([]byte, any) error

var (
	marshaler   Marshaler
	indenter    Indenter
	unmarshaler Unmarshaler
)

func DefaultMarshaler() {
	marshaler = json.Marshal
}

func SetMarshaler(m Marshaler) {
	marshaler = m
}

func DefaultUnmarshaler() {
	unmarshaler = json.Unmarshal
}

func SetUnmarshaler(m Unmarshaler) {
	unmarshaler = m
}

func DefaultIndenter() {
	indenter = json.MarshalIndent
}

func SetIndenter(m Indenter) {
	indenter = m
}

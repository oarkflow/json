package json

import (
	"encoding/json"
)

type Marshaler func(any) ([]byte, error)

var (
	marshaler Marshaler
)

func DefaultMarshaler() {
	marshaler = json.Marshal
}

func SetMarshaler(m Marshaler) {
	marshaler = m
}

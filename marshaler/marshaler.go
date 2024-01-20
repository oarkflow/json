package marshaler

import (
	"encoding/json"
)

type Marshaler func(any) ([]byte, error)

var (
	marshaler Marshaler
)

func init() {
	marshaler = json.Marshal
}

func SetMarshaler(m Marshaler) {
	marshaler = m
}

func Instance() Marshaler {
	return marshaler
}

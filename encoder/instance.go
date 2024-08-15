package encoder

import (
	"encoding/json"
	"io"
)

type IEncoder interface {
	Encode(any) error
}

type Factory func(io.Writer) IEncoder

var encoderFactory Factory

// Initialize the package with the standard library's JSON encoder by default.
func init() {
	encoderFactory = func(w io.Writer) IEncoder {
		return json.NewEncoder(w)
	}
}

// SetEncoder allows you to set a custom encoder factory.
func SetEncoder(factory Factory) {
	encoderFactory = factory
}

// NewEncoder creates a new encoder using the currently set encoder factory.
func NewEncoder(w io.Writer) IEncoder {
	return encoderFactory(w)
}

func Instance() Factory {
	return encoderFactory
}

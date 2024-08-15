package decoder

import (
	"encoding/json"
	"io"
)

type IDecoder interface {
	Decode(any) error
}

type Factory func(io.Reader) IDecoder

var encoderFactory Factory

// Initialize the package with the standard library's JSON encoder by default.
func init() {
	encoderFactory = func(w io.Reader) IDecoder {
		return json.NewDecoder(w)
	}
}

// SetDecoder allows you to set a custom encoder factory.
func SetDecoder(factory Factory) {
	encoderFactory = factory
}

// NewDecoder creates a new encoder using the currently set encoder factory.
func NewDecoder(w io.Reader) IDecoder {
	return encoderFactory(w)
}

func Instance() Factory {
	return encoderFactory
}

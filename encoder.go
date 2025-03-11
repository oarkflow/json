package json

import (
	"encoding/json"
	"io"
)

type IEncoder interface {
	Encode(any) error
	SetIndent(prefix, indent string)
	SetEscapeHTML(on bool)
}

type EncoderFactory func(io.Writer) IEncoder

var encoderFactory EncoderFactory

// DefaultEncoder Initialize the package with the standard library's JSON encoder by default.
func DefaultEncoder() {
	encoderFactory = func(w io.Writer) IEncoder {
		return json.NewEncoder(w)
	}
}

// SetEncoder allows you to set a custom encoder factory.
func SetEncoder(factory EncoderFactory) {
	encoderFactory = factory
}

// NewEncoder creates a new encoder using the currently set encoder factory.
func NewEncoder(w io.Writer) IEncoder {
	return encoderFactory(w)
}

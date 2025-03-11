package json

import (
	"encoding/json"
	"io"
)

type IDecoder interface {
	Decode(any) error
	Token() (json.Token, error)
	More() bool
	UseNumber()
	Buffered() io.Reader
	DisallowUnknownFields()
}

type DecoderFactory func(io.Reader) IDecoder

var decoderFactory DecoderFactory

// DefaultDecoder Initialize the package with the standard library's JSON encoder by default.
func DefaultDecoder() {
	decoderFactory = func(w io.Reader) IDecoder {
		return json.NewDecoder(w)
	}
}

// SetDecoder allows you to set a custom encoder factory.
func SetDecoder(factory DecoderFactory) {
	decoderFactory = factory
}

// NewDecoder creates a new encoder using the currently set encoder factory.
func NewDecoder(w io.Reader) IDecoder {
	return decoderFactory(w)
}

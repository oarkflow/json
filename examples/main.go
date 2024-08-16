package main

import (
	json2 "encoding/json"
	"fmt"
	"io"
	"runtime"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/decoder"
	"github.com/bytedance/sonic/encoder"
	goccy "github.com/goccy/go-json"

	"github.com/oarkflow/json"
)

var data = []byte(`{"user_id": "1"}`)
var schemeBytes = []byte(`{"type":"object","description":"users","properties":{"avatar":{"type":"string","maxLength":255},"created_at":{"type":"string","default":"now()"},"created_by":{"type":"integer","maxLength":64},"deleted_at":{"type":"string"},"email":{"type":"string","maxLength":255,"default":"'s.baniya.np@gmail.com'"},"email_verified_at":{"type":"string"},"first_name":{"type":"string","maxLength":255},"is_active":{"type":"boolean","default":"false"},"last_name":{"type":"string","maxLength":255},"middle_name":{"type":"string","maxLength":255},"status":{"type":"string","maxLength":30},"title":{"type":"string","maxLength":10},"updated_at":{"type":"string","default":"now()"},"updated_by":{"type":"integer","maxLength":64},"user_id":{"type":"integer","maxLength":64},"verification_token":{"type":"string","maxLength":255}},"required":["email"],"primaryKeys":["user_id"]}`)

func main() {
	defaultJson()
	bytedanceSonic()
	goccyJSON()
}

func printStackTrace() {
	pc := make([]uintptr, 20)
	n := runtime.Callers(2, pc)
	frames := runtime.CallersFrames(pc[:n])
	for {
		frame, more := frames.Next()

		// Extract package name from the function name
		packageName := extractPackageName(frame.Function)

		fmt.Printf("Function: %s\n\tPackage: %s\n\tFile: %s:%d\n", frame.Function, packageName, frame.File, frame.Line)

		if !more {
			break
		}
	}
}

// Helper function to extract the package name from the fully qualified function name
func extractPackageName(function string) string {
	// Split the function name by slashes and dots
	parts := strings.Split(function, "/")
	// Get the last part, which is package + function
	if len(parts) > 0 {
		parts = strings.Split(parts[len(parts)-1], ".")
		if len(parts) > 1 {
			return parts[0] // Return only the package name
		}
	}
	return ""
}

func defaultJson() {
	json.SetMarshaler(json2.Marshal)
	json.SetUnmarshaler(json2.Unmarshal)
	json.SetDecoder(func(w io.Reader) json.IDecoder {
		return json2.NewDecoder(w)
	})
	json.SetEncoder(func(w io.Writer) json.IEncoder {
		return json2.NewEncoder(w)
	})
	handle()
}

func bytedanceSonic() {
	json.SetMarshaler(sonic.Marshal)
	json.SetUnmarshaler(sonic.Unmarshal)
	json.SetDecoder(func(w io.Reader) json.IDecoder {
		return decoder.NewStreamDecoder(w)
	})
	json.SetEncoder(func(w io.Writer) json.IEncoder {
		return encoder.NewStreamEncoder(w)
	})
	handle()
}

func goccyJSON() {
	json.SetMarshaler(goccy.Marshal)
	json.SetUnmarshaler(goccy.Unmarshal)
	json.SetDecoder(func(w io.Reader) json.IDecoder {
		return goccy.NewDecoder(w)
	})
	json.SetEncoder(func(w io.Writer) json.IEncoder {
		return goccy.NewEncoder(w)
	})
	handle()
}

func handle() {
	var d1 map[string]any
	err := json.Unmarshal(data, &d1)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(d1)
	var d map[string]any
	err = json.Unmarshal(data, &d, schemeBytes)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(d)
	fmt.Println("Goccy JSON")
	printStackTrace()
}

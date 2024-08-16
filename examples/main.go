package main

import (
	json2 "encoding/json"
	"fmt"
	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/decoder"
	"github.com/bytedance/sonic/encoder"
	goccy "github.com/goccy/go-json"
	"io"

	"github.com/oarkflow/json"
)

var data = []byte(`{"user_id": "1"}`)
var schemeBytes = []byte(`{"type":"object","description":"users","properties":{"avatar":{"type":"string","maxLength":255},"created_at":{"type":"string","default":"now()"},"created_by":{"type":"integer","maxLength":64},"deleted_at":{"type":"string"},"email":{"type":"string","maxLength":255,"default":"'s.baniya.np@gmail.com'"},"email_verified_at":{"type":"string"},"first_name":{"type":"string","maxLength":255},"is_active":{"type":"boolean","default":"false"},"last_name":{"type":"string","maxLength":255},"middle_name":{"type":"string","maxLength":255},"status":{"type":"string","maxLength":30},"title":{"type":"string","maxLength":10},"updated_at":{"type":"string","default":"now()"},"updated_by":{"type":"integer","maxLength":64},"user_id":{"type":"integer","maxLength":64},"verification_token":{"type":"string","maxLength":255}},"required":["email"],"primaryKeys":["user_id"]}`)

func main() {
	defaultJson()
	bytedanceSonic()
	goccyJSON()
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
}

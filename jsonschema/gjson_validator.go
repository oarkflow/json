package jsonschema

import (
	"github.com/oarkflow/json/sjson"
)

type GValidator interface {
	GValidate(ctx *ValidateCtx, val *sjson.Result)
}

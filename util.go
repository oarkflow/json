package json

import (
	"runtime"

	"github.com/goccy/go-reflect"
)

func FunctionPath(fn any) string {
	ptr := reflect.ValueOf(fn).Pointer()
	funcInfo := runtime.FuncForPC(ptr)
	if funcInfo != nil {
		return funcInfo.Name()
	}
	return ""
}

package json

import (
	"runtime"
	"strconv"

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

// A Number represents a JSON number literal.
type Number string

// String returns the literal text of the number.
func (n Number) String() string { return string(n) }

// Float64 returns the number as a float64.
func (n Number) Float64() (float64, error) {
	return strconv.ParseFloat(string(n), 64)
}

// Int64 returns the number as an int64.
func (n Number) Int64() (int64, error) {
	return strconv.ParseInt(string(n), 10, 64)
}

package v2

import (
	"encoding/json"
	"strings"

	"github.com/oarkflow/expr"
)

func evaluateExpression(exprStr string) (any, error) {
	if strings.HasPrefix(exprStr, "{{") && strings.HasSuffix(exprStr, "}}") {
		jsonStr := strings.ReplaceAll(exprStr, "'", "\"")
		var m any
		if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
			return nil, err
		}
		return m, nil
	}
	vm, err := expr.Parse(exprStr)
	if err != nil {
		return nil, err
	}
	return vm.Eval(nil)
}

func prepareDefault(def any) (any, error) {
	if def == nil {
		return nil, nil
	}
	defStr, ok := def.(string)
	if !ok {
		return def, nil
	}
	if strings.HasPrefix(defStr, "{{") && strings.HasSuffix(defStr, "}}") {
		trimmed := strings.TrimPrefix(defStr, "{{")
		trimmed = strings.TrimSuffix(trimmed, "}}")
		return evaluateExpression(trimmed)
	}
	return def, nil
}

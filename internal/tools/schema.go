package tools

import (
	"encoding/json"
	"fmt"
	"math"
)

func Validate(schema Schema, raw json.RawMessage) (map[string]any, *ToolError) {
	var value any
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, &ToolError{Type: ErrorValidation, Message: "arguments must be valid JSON", Details: map[string]any{"cause": err.Error()}}
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return nil, &ToolError{Type: ErrorValidation, Message: "arguments must be a JSON object"}
	}
	if typ, _ := schema["type"].(string); typ != "" && typ != "object" {
		return nil, &ToolError{Type: ErrorValidation, Message: "tool schema must be an object"}
	}
	for _, name := range stringSlice(schema["required"]) {
		if _, ok := obj[name]; !ok {
			return nil, &ToolError{Type: ErrorValidation, Message: fmt.Sprintf("missing required parameter %q", name), Details: map[string]any{"parameter": name}}
		}
	}
	props, _ := schema["properties"].(map[string]any)
	for name, rawProp := range props {
		value, exists := obj[name]
		if !exists {
			continue
		}
		prop, _ := rawProp.(map[string]any)
		if err := validateValue(name, value, prop); err != nil {
			return nil, err
		}
	}
	return obj, nil
}

func validateValue(name string, value any, prop map[string]any) *ToolError {
	if typ, _ := prop["type"].(string); typ != "" {
		if !matchesType(value, typ) {
			return &ToolError{Type: ErrorValidation, Message: fmt.Sprintf("parameter %q must be %s", name, typ), Details: map[string]any{"parameter": name, "type": typ}}
		}
	}
	if enum, ok := prop["enum"].([]any); ok && len(enum) > 0 {
		found := false
		for _, allowed := range enum {
			if fmt.Sprint(allowed) == fmt.Sprint(value) {
				found = true
				break
			}
		}
		if !found {
			return &ToolError{Type: ErrorValidation, Message: fmt.Sprintf("parameter %q has an unsupported value", name), Details: map[string]any{"parameter": name}}
		}
	}
	if n, ok := numeric(value); ok {
		if min, ok := numeric(prop["minimum"]); ok && n < min {
			return &ToolError{Type: ErrorValidation, Message: fmt.Sprintf("parameter %q is below minimum", name), Details: map[string]any{"parameter": name, "minimum": min}}
		}
		if max, ok := numeric(prop["maximum"]); ok && n > max {
			return &ToolError{Type: ErrorValidation, Message: fmt.Sprintf("parameter %q is above maximum", name), Details: map[string]any{"parameter": name, "maximum": max}}
		}
	}
	return nil
}

func matchesType(value any, typ string) bool {
	switch typ {
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "integer":
		n, ok := numeric(value)
		return ok && math.Trunc(n) == n
	case "number":
		_, ok := numeric(value)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	default:
		return true
	}
}

func numeric(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	default:
		return 0, false
	}
}

func stringSlice(value any) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

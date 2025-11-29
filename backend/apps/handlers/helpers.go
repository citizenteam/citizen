package handlers

import (
	"encoding/json"
	"strings"
)

// stringSliceFromInterface converts interface{} to []string
func stringSliceFromInterface(value interface{}) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if str, ok := item.(string); ok && strings.TrimSpace(str) != "" {
				result = append(result, str)
			}
		}
		return result
	default:
		return nil
	}
}

// hasPositiveValue checks if a value is positive (for various numeric types)
func hasPositiveValue(value interface{}) bool {
	switch n := value.(type) {
	case int:
		return n > 0
	case int32:
		return n > 0
	case int64:
		return n > 0
	case uint:
		return n > 0
	case uint32:
		return n > 0
	case uint64:
		return n > 0
	case float32:
		return n > 0
	case float64:
		return n > 0
	case json.Number:
		if f, err := n.Float64(); err == nil {
			return f > 0
		}
	}
	return false
}

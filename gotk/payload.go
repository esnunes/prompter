package gotk

import (
	"fmt"
	"strconv"
)

// Payload wraps the command payload with typed accessors.
// Missing keys return zero values. String values from the DOM are coerced.
type Payload struct {
	data map[string]any
}

// NewPayload creates a Payload from a map.
func NewPayload(data map[string]any) Payload {
	if data == nil {
		data = map[string]any{}
	}
	return Payload{data: data}
}

// String returns the string value for key, or "".
func (p Payload) String(key string) string {
	v, ok := p.data[key]
	if !ok {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	default:
		return fmt.Sprintf("%v", val)
	}
}

// Int returns the int value for key, or 0. Coerces strings and floats.
func (p Payload) Int(key string) int {
	v, ok := p.data[key]
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case string:
		n, _ := strconv.Atoi(val)
		return n
	default:
		return 0
	}
}

// Float returns the float64 value for key, or 0.
func (p Payload) Float(key string) float64 {
	v, ok := p.data[key]
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	default:
		return 0
	}
}

// Bool returns the bool value for key, or false. Treats "true"/"1" as true.
func (p Payload) Bool(key string) bool {
	v, ok := p.data[key]
	if !ok {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return val == "true" || val == "1"
	default:
		return false
	}
}

// Map returns the raw underlying map.
func (p Payload) Map() map[string]any {
	return p.data
}

// Package conv provides type conversion utilities.
package conv

// ToFloat64 converts various numeric types to float64.
// Returns the value and true if conversion succeeded, or 0 and false otherwise.
func ToFloat64(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	}
	return 0, false
}

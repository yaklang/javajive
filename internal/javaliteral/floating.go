// Package javaliteral renders primitive Java constant expressions without
// introducing class names, field lookups or initialization dependencies.
package javaliteral

import (
	"math"
	"strconv"
)

func Float32(value float32) string { return floating(float64(value), 32, "F") }
func Float64(value float64) string { return floating(value, 64, "D") }

func floating(value float64, bits int, suffix string) string {
	// Java floating division is total and a constant expression. It remains
	// valid in annotation defaults and ConstantValue declarations when wrapper
	// names are shadowed, unlike Float.NaN / Double.POSITIVE_INFINITY.
	if math.IsNaN(value) {
		return "(0.0" + suffix + "/0.0" + suffix + ")"
	}
	if math.IsInf(value, 1) {
		return "(1.0" + suffix + "/0.0" + suffix + ")"
	}
	if math.IsInf(value, -1) {
		return "(-1.0" + suffix + "/0.0" + suffix + ")"
	}
	return strconv.FormatFloat(value, 'g', -1, bits) + suffix
}

// Package javaliteral renders primitive Java literals. Constant-expression
// contexts avoid class references; runtime operands can preserve raw NaN words
// through the standard bit-conversion intrinsics.
package javaliteral

import (
	"math"
	"strconv"
)

func Float32(value float32) string { return floating(float64(value), 32, "F") }
func Float64(value float64) string { return floating(value, 64, "D") }

// RuntimeFloat32 and RuntimeFloat64 preserve the original NaN word when it is
// observable through the raw-bit APIs. Arithmetic constant expressions collapse
// its sign/payload. Keep Float32/Float64 for contexts that require a Java constant
// expression (ConstantValue and annotation defaults); a bit conversion is not a
// constant expression and must not silently add a class initializer there.
func RuntimeFloat32(value float32, typeName func(string) string) string {
	return RuntimeFloat32Call(value, func(owner, _, _ string) string { return runtimeOwner(owner, typeName) + "." })
}

// RuntimeFloat32Call and RuntimeFloat64Call bind a newly introduced intrinsic
// by its exact owner, method and JVM descriptor. A source method's type
// parameters may obscure both a simple wrapper type and its package root;
// callers can then use a proved static import instead of inventing a type name.
// Finite and canonical values do not request a call or introduce an import.
func RuntimeFloat32Call(value float32, callPrefix func(string, string, string) string) string {
	bits := math.Float32bits(value)
	if math.IsNaN(float64(value)) && bits != 0x7fc00000 {
		return runtimeCallPrefix("java.lang.Float", "intBitsToFloat", "(I)F", callPrefix) + "intBitsToFloat(0x" + strconv.FormatUint(uint64(bits), 16) + ")"
	}
	return Float32(value)
}

func RuntimeFloat64(value float64, typeName func(string) string) string {
	return RuntimeFloat64Call(value, func(owner, _, _ string) string { return runtimeOwner(owner, typeName) + "." })
}

func RuntimeFloat64Call(value float64, callPrefix func(string, string, string) string) string {
	bits := math.Float64bits(value)
	if math.IsNaN(value) && bits != 0x7ff8000000000000 {
		return runtimeCallPrefix("java.lang.Double", "longBitsToDouble", "(J)D", callPrefix) + "longBitsToDouble(0x" + strconv.FormatUint(bits, 16) + "L)"
	}
	return Float64(value)
}

func runtimeCallPrefix(owner, member, descriptor string, callPrefix func(string, string, string) string) string {
	if callPrefix != nil {
		return callPrefix(owner, member, descriptor)
	}
	return owner + "."
}

func runtimeOwner(name string, typeName func(string) string) string {
	if typeName != nil {
		return typeName(name)
	}
	return name
}

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

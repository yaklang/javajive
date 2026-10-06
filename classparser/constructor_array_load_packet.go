package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
)

// Array loads consume an original array reference and a computational int
// index. Their component descriptor establishes the result type, never a
// guessed source cast or a value sampled at runtime. BALOAD distinguishes
// boolean storage from signed byte storage; AALOAD also handles array-valued
// components without discarding their remaining dimensions.
func constructorArrayLoadPacketResult(opcode int, array, index string) (string, bool) {
	if index != "I" && index != "B" && index != "S" && index != "C" || !strings.HasPrefix(array, "[") {
		return "", false
	}
	params, _, err := callbinding.Descriptor("(" + array + ")V")
	if err != nil || len(params) != 1 || params[0] != array {
		return "", false
	}
	component := array[1:]
	if opcode == core.OP_AALOAD {
		return component, callbinding.Reference(component)
	}
	if opcode < core.OP_IALOAD || opcode > core.OP_SALOAD {
		return "", false
	}
	const components = "IJFDLBCS"
	// The numeric opcodes have one reserved reference position and one
	// dual-storage position. Validate the component rather than accepting
	// every array that happens to have the same JVM word width.
	expected := string(components[opcode-core.OP_IALOAD])
	if opcode == core.OP_BALOAD && component == "Z" {
		return "Z", true
	}
	return component, component == expected
}

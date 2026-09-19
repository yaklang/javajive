package javaclassparser

import "testing"

// A local defined only in a condition still needs a declaration. Its type now
// comes from the semantic value instead of a source-level return-type guess.
func TestReturnLocalDeclPreservesCoreDeclaration(t *testing.T) {
	assertDecompileBothPreserve(t, "testdata/regression/ReturnLocalDeclSynthesis.class", "JDEC_RETURN_DECL_FIX_OFF",
		"LocalDateTime var2 = null;", "var2 = Helper.parse(", "return var2;")
}

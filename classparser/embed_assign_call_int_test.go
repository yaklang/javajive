package javaclassparser

import "testing"

// Embedded assignment targets retain the call descriptor type even when the
// legacy source-level declaration repair is disabled.
func TestEmbedAssignCallReadIntPreservesCoreDeclaration(t *testing.T) {
	assertDecompileBothPreserve(t, "testdata/regression/EmbedAssignCallReadSeed.class", "JDEC_NO_EMBED_ASSIGN_INT",
		"int var5 = 0;", "(var5 = this.read()) != (-1)", "var1.update((byte)(var5));")
}

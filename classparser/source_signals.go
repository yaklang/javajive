package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// Closure checks can also receive historical/source-only method records. Keep
// their explicit stub sentinel and lexically inspect their emitted diagnostics;
// typed render failures remain authoritative even without diagnostic text.
func (body *dumpedMethods) sourceReconstructionFailed() bool {
	return body == nil || body.isStub || body.bodyCode == "stub" || body.incompleteValue || body.malformedTry || legacySourceHasFailureMarker(body.code)
}

// Structured rendering receipts are authoritative for actual generated bodies.
// A legacy source-only closure check must also retain its rejection of bare
// placeholders and generated failure comments, without treating literal data as
// a diagnostic. This is a diagnostic compatibility check, not a syntax proof.
func legacySourceHasFailureMarker(source string) bool {
	if !strings.Contains(source, DecompileStubMarker) && !strings.Contains(source, values.EmptySlotValuePlaceholder) {
		return false
	}
	found := false
	rewriteJavaSourceSegments(source, func(code string) string {
		found = strings.Contains(code, DecompileStubMarker) || strings.Contains(code, values.EmptySlotValuePlaceholder)
		return code
	}, false)
	return found
}

// Diagnostic sentinels are code tokens, never substrings of user literals,
// text blocks or comments. Reuse the lexical shield used by source rewrites.
func hasExceptionSentinel(src string) bool {
	found := false
	rewriteJavaCode(src, func(code string) string {
		found = strings.Contains(code, "= Exception;") || strings.Contains(code, "= Exception\n")
		return code
	})
	return found
}

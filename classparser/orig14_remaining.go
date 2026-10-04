package javaclassparser

import (
	"github.com/yaklang/javajive/internal/jdecenv"
)

// fixOrig14RemainderReconstructs applies shape-based reconstructs for the
// leftover original-14 tree sites. Kill-switch: JDEC_ORIG14_REMAINING_OFF=1.
func fixOrig14RemainderReconstructs(body string) string {
	if jdecenv.Get("JDEC_ORIG14_REMAINING_OFF") == "1" {
		return body
	}
	body = fixIntBareIf(body)
	body = fixBoolOrAccumulator(body)
	// Exception alternatives belong to the original handler table. A Class[]
	// argument or a reflection-like method name cannot establish a throws clause.
	body = fixObjectGetKeyAssignedToInt(body)
	body = fixUncheckedAwaitNanos(body)
	body = fixEmptySyncInTrailingElse(body)
	return body
}

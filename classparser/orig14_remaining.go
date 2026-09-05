package javaclassparser

import (
	"os"
)

// fixOrig14RemainderReconstructs applies shape-based reconstructs for the
// leftover original-14 tree sites. Kill-switch: JDEC_ORIG14_REMAINING_OFF=1.
func fixOrig14RemainderReconstructs(body string) string {
	if os.Getenv("JDEC_ORIG14_REMAINING_OFF") == "1" {
		return body
	}
	body = fixIntBareIf(body)
	body = fixBoolOrAccumulator(body)
	body = fixMissingNSMECatch(body)
	body = fixObjectGetKeyAssignedToInt(body)
	body = fixUncheckedAwaitNanos(body)
	body = fixEmptySyncInTrailingElse(body)
	return body
}

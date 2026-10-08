package javaclassparser

import (
	"strings"
	"testing"
)

// The original driver calculates raw words independently of the selected
// producers. These variants exercise actual F/D branch STORE joins, not just
// a source type annotation. Signed zero, infinities, subnormals, NaN payloads,
// eager partial failure and repeated lazy calls remain in the unchanged bank.
func TestAdversarialMemberLambdaJoinedCaptureRetainsFloatingWords(t *testing.T) {
	for _, kind := range []string{"float", "double", "both"} {
		t.Run(kind, func(t *testing.T) {
			fixture := memberLambdaPrimitiveCaptureFixture
			if kind == "float" || kind == "both" {
				fixture = strings.Replace(fixture,
					"final float fractional=WordCaptureEffects.fractional(floatBits);",
					"final float fractional;if((input&1)==0){fractional=WordCaptureEffects.fractional(floatBits);}else{fractional=WordCaptureEffects.fractional(floatBits^0x80000000);}", 1)
				fixture = strings.Replace(fixture, "^fractional^precise;", "^(fractional^((integer&1)==0?0:0x80000000))^precise;", 1)
			}
			if kind == "double" || kind == "both" {
				fixture = strings.Replace(fixture,
					"final double precise=WordCaptureEffects.precise(doubleBits);",
					"final double precise;if((input&1)==0){precise=WordCaptureEffects.precise(doubleBits);}else{precise=WordCaptureEffects.precise(doubleBits^0x8000000000000000L);}", 1)
				fixture = strings.Replace(fixture, "^precise;", "^(precise^((integer&1)==0?0L:0x8000000000000000L));", 1)
			}
			testNativeIndependentFamilyFixture(t, fixture, []string{"WordCaptureOwner"}, "WordCaptureDriver", "8004:word:local:capture:bits:order:identity\n", nativeLexicalExactSignatures)
		})
	}
}

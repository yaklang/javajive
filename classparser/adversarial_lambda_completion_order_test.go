package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// The original binary oracle calls both the functional interface and its
// reflected implementation. It distinguishes an early cast, a null receiver,
// user exceptions, value identity and declaration ABI. A name-only metadata
// pass cannot reject the pending checked-input obligation or apply it twice.
func TestAdversarialCheckedLambdaInputPrecedesDeclarationMetadata(t *testing.T) {
	for _, owner := range []string{"InputOwner", "OtherCheckedOwner"} {
		for _, profile := range []SourceCompilerProfile{NativeJavac8, ModernJavac} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(fmt.Sprintf("%s/%s/%s", owner, profile, debug), func(t *testing.T) {
					t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
					fixture := strings.ReplaceAll(checkedInputFixture, "InputOwner", owner)
					fixture = strings.ReplaceAll(fixture, "InputDriver", "SuffixDriver")
					constructorSuffixFamilyWithOriginalCompilerRoundTrip(t, fixture, debug, []string{owner, owner + "$Caller"}, "input:cast:null:effect:identity:physical-ABI\n", 8, 8, ModernJavac, profile)
				})
			}
		}
	}
}

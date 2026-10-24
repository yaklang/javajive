package javaclassparser

import (
	"strings"
	"testing"
)

// A Java 8 adapter's physical parameter table is observable metadata, but it
// does not change the method-reference operation. Cross-compiler rebuilding
// must retain that table without copying the old packing body into new Code.
func TestAdversarialArtifactVarargsReferencePreservesParameterTableAndBehavior(t *testing.T) {
	for _, family := range []string{"ordinary", "covariant", "covariant private competitor"} {
		t.Run(family, func(t *testing.T) {
			fixture := strings.Replace(localVarargsReceiverFixture, "$BODY", "LocalReceiverTarget receiver=LocalReceiverTarget.create(mode);return receiver::append;", 1)
			if family != "ordinary" {
				fixture = strings.Replace(fixture, "class LocalReceiverTarget {", "class LocalReceiverBase {public Object append(Object...items){throw new AssertionError(\"parent dispatch\");}}\nclass LocalReceiverTarget extends LocalReceiverBase {", 1)
			}
			if family == "covariant private competitor" {
				fixture = strings.Replace(fixture, "class LocalReceiverTarget extends LocalReceiverBase {", "class LocalReceiverTarget extends LocalReceiverBase {private Object append(Object item){throw new AssertionError(\"private competitor\");}", 1)
			}
			fixture = strings.ReplaceAll(fixture, "LocalReceiverOwner", "ArtifactLambdaOwner")
			fixture = strings.ReplaceAll(fixture, "LocalReceiverDriver", "ArtifactLambdaDriver")
			testArtifactLambdaMetadata(t, fixture, []bool{false, true}, "9:local:bound:reference:evaluation:identity\n")
		})
	}
}

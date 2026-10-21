package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialGuardedFinallyLambdaInMemberSourceKeepsFactoryBindings(t *testing.T) {
	// A named declaration gives the same method the complete member-family
	// publication path. Its factory/capture obligations must agree with the
	// already verified plain source path; no binary child may be borrowed.
	fixture := strings.ReplaceAll(guardedFinallyLambdaFixture, "EXTRA_NESTED_METHOD", "public static class Marker { public long echo(long value){return value;} }")
	fixture = strings.ReplaceAll(fixture, "EXTRA_NESTED_CHECK", "if(new ArtifactLambdaOwner.Marker().echo(value)!=value)throw new AssertionError(\"owned member\");")
	testArtifactLambdaMetadata(t, fixture, []bool{false, true}, "30:guarded-finally:single-factory:overflow:alias:exception\n")
}

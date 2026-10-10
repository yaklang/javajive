package javaclassparser

import (
	"strings"
	"testing"
)

// Java 11 removes synthetic constructor access bridges. Reconstruct the
// complete original nest rather than moving captures after an observable
// superclass call or widening a private declaration.
func TestAdversarialModernNestAnonymousMemberAllocationRoundTrip(t *testing.T) {
	testSourceTargetReleaseFamilyFixture(t, anonymousMemberAllocationFixture, "AllocationOwner", "AllocationDriver", "10:anonymous:member:allocation:effects:identity\n", "11", []int{11, 16})
}

func TestAdversarialModernNestObservableCaptureRoundTrip(t *testing.T) {
	testSourceTargetReleaseFamilyFixture(t, sourceTargetCaptureFixture, "SourceCaptureOwner", "CaptureDriver", "20:source-target:capture:observation:publication:exception\n", "11", []int{11, 16})
}

func TestAdversarialModernNestPrivateConstructorObservableCaptureRoundTrip(t *testing.T) {
	fixture := strings.Replace(sourceTargetCaptureFixture, "public Child(int", "private Child(int", 1)
	testSourceTargetReleaseFamilyFixture(t, fixture, "SourceCaptureOwner", "CaptureDriver", "20:source-target:capture:observation:publication:exception\n", "11", []int{11, 16})
}

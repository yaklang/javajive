package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialAnonymousNestedPrivateMemberSuperKeepsCaptureRoundTrip(t *testing.T) {
	fixture := anonymousNestedPrivateMemberSuperFixture()
	testSourceTargetReleaseFamilyFixture(t, fixture, "AllocationOwner", "AllocationDriver", "10:anonymous:member:allocation:effects:identity\n", "8", []int{8, 11, 16})
}

func anonymousNestedPrivateMemberSuperFixture() string {
	return strings.Replace(anonymousMemberAllocationFixture, "Entry read(int n){return new Entry(AllocationEffects.arg(n));}", "Entry read(int n){Reader inner=new Reader(){Object enclosing(){return AllocationOwner.this;}Entry read(int m){return new Entry(AllocationEffects.arg(m));}};return inner.read(n);}", 1)
}

func anonymousThirdLevelPrivateMemberSuperFixture() string {
	return strings.Replace(anonymousNestedPrivateMemberSuperFixture(), "Entry read(int m){return new Entry(AllocationEffects.arg(m));}", "Entry read(int m){Reader deeper=new Reader(){Object enclosing(){return AllocationOwner.this;}Entry read(int k){return new Entry(AllocationEffects.arg(k));}};return deeper.read(m);}", 1)
}
func TestAdversarialAnonymousThirdLevelPrivateMemberSuperKeepsCaptureRoundTrip(t *testing.T) {
	testSourceTargetReleaseFamilyFixture(t, anonymousThirdLevelPrivateMemberSuperFixture(), "AllocationOwner", "AllocationDriver", "10:anonymous:member:allocation:effects:identity\n", "8", []int{8, 11, 16})
}

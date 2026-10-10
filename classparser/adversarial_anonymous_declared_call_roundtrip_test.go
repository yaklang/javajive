package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialAnonymousConcreteDeclaredArgumentsKeepBindingRoundTrip(t *testing.T) {
	fixture := anonymousConcreteArgumentsFixture()
	testSourceTargetReleaseFamilyFixture(t, fixture, "AllocationOwner", "AllocationDriver", "10:anonymous:member:allocation:effects:identity\n", "8", []int{8, 11, 16})
}

func TestAdversarialAnonymousConcreteDeclaredNullKeepBindingRoundTrip(t *testing.T) {
	fixture := strings.Replace("interface AllocationThunk{Object take(Object n);}"+anonymousMemberAllocationFixture, "Entry read(int n){return new Entry(AllocationEffects.arg(n));}", "Entry read(final int n){return (Entry)new AllocationThunk(){public Object take(Object m){return new Entry(AllocationEffects.arg(n));}public Object take(String unused){AllocationEffects.trace+=\"WRONG\";return null;}}.take((Object)null);}", 1)
	testSourceTargetReleaseFamilyFixture(t, fixture, "AllocationOwner", "AllocationDriver", "10:anonymous:member:allocation:effects:identity\n", "8", []int{8, 11, 16})
}

func anonymousConcreteArgumentsFixture() string {
	return strings.Replace("interface AllocationThunk{Object take(int n);}"+anonymousMemberAllocationFixture, "Entry read(int n){return new Entry(AllocationEffects.arg(n));}", "Entry read(int n){return (Entry)new AllocationThunk(){public Object take(int m){return new Entry(AllocationEffects.arg(m));}public Object take(Object unused){AllocationEffects.trace+=\"WRONG\";return null;}}.take(n);}", 1)
}

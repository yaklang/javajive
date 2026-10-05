package javaclassparser

import (
	"strings"
	"testing"
)

// The ancestor has an independent source family. A type-name certificate must
// also project each construction's hidden enclosing operand without giving
// the subclass membership in the ancestor's private lexical scope.
func nativeInheritedDependencyAllocationFixture() string {
	f := strings.Replace(nativeInheritedDependencyFixture, "Object read(){", "Value makeOwn(Object token){return new Value(token);}Object read(){", 1)
	return strings.Replace(f, "try{new DependencyDerived(null)", "DependencyBase.Value own=derived.makeOwn(token);if(own.token!=token||own.outer()!=derived||own.getClass().getDeclaringClass()!=DependencyBase.class)throw new AssertionError(\"inherited allocation binding\");try{new DependencyDerived(null)", 1)
}

func TestNativeInheritedIndependentFamilyAllocationRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, nativeInheritedDependencyAllocationFixture(), []string{"DependencyBase", "DependencyDerived"}, "InheritedDependencyDriver", "2:ancestor:dependency:separate:outer\n", nativeLexicalExactSignatures)
}
func TestNativeInheritedIndependentFamilyAllocationRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeInheritedDependencyAllocationFixture(), "DependencyBase", "AllocatorOwner")
	f = strings.ReplaceAll(f, "DependencyDerived", "AllocationUser")
	f = strings.ReplaceAll(f, "Value", "Element")
	testNativeIndependentFamilyFixture(t, f, []string{"AllocatorOwner", "AllocationUser"}, "InheritedDependencyDriver", "2:ancestor:dependency:separate:outer\n", nativeLexicalExactSignatures)
}

func TestNativeInheritedIndependentFamilyAllocationGenericRoundTrip(t *testing.T) {
	f := strings.Replace(nativeInheritedDependencyAllocationFixture(), "class DependencyBase{", "class DependencyBase<T>{", 1)
	f = strings.Replace(f, "final Object token;Value(Object token)", "final T token;Value(T token)", 1)
	f = strings.Replace(f, "Value make(Object token)", "Value make(T token)", 1)
	f = strings.Replace(f, "class DependencyDerived extends DependencyBase", "class DependencyDerived<T> extends DependencyBase<T>", 1)
	f = strings.Replace(f, "Value makeOwn(Object token)", "Value makeOwn(T token)", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"DependencyBase", "DependencyDerived"}, "InheritedDependencyDriver", "2:ancestor:dependency:separate:outer\n", nativeLexicalExactSignatures)
}
func TestNativeInheritedIndependentFamilyAllocationPackagedRoundTrip(t *testing.T) {
	f := "package independent.allocation;\n" + nativeInheritedDependencyAllocationFixture()
	testNativeIndependentFamilyFixture(t, f, []string{"independent/allocation/DependencyBase", "independent/allocation/DependencyDerived"}, "independent.allocation.InheritedDependencyDriver", "2:ancestor:dependency:separate:outer\n", nativeLexicalExactSignatures)
}

func TestNativeInheritedIndependentFamilyAllocationQualifiedNullRoundTrip(t *testing.T) {
	f := strings.Replace(nativeInheritedDependencyAllocationFixture(), "Value makeOwn(Object token){return new Value(token);}", "Value makeOwn(Object token){return new Value(token);}Value makeOn(DependencyBase outer,Object token){return outer.new Value(token);}", 1)
	f = strings.Replace(f, "try{new DependencyDerived(null)", "DependencyBase.Value on=derived.makeOn(base,token);if(on.token!=token||on.outer()!=base)throw new AssertionError(\"qualified binding\");try{derived.makeOn(null,token);throw new AssertionError(\"lost qualifier check\");}catch(NullPointerException expected){}try{new DependencyDerived(null)", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"DependencyBase", "DependencyDerived"}, "InheritedDependencyDriver", "2:ancestor:dependency:separate:outer\n", nativeLexicalExactSignatures)
}

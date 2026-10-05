package javaclassparser

import (
	"strings"
	"testing"
)

// The inherited member is a declaration dependency, not a new private-access
// owner. The value belongs to a different actual outer instance; that identity
// must survive completing both source families.
const nativeInheritedDependencyFixture = `class DependencyBase{class Value{final Object token;Value(Object token){this.token=token;}Object outer(){return DependencyBase.this;}}Value make(Object token){return new Value(token);}}
class DependencyDerived extends DependencyBase{final Value value;DependencyDerived(Value value){this.value=value;}class Item{Object read(Value value){return value.token;}}Object read(){return new Item().read(value);}}
class InheritedDependencyDriver{public static void main(String[]args){int rows=0;for(Object token:new Object[]{null,new Object()}){DependencyBase base=new DependencyBase();DependencyBase.Value value=base.make(token);DependencyDerived derived=new DependencyDerived(value);if(derived.read()!=token||value.outer()!=base||value.outer()==derived||value.getClass().getDeclaringClass()!=DependencyBase.class||derived.new Item().getClass().getDeclaringClass()!=DependencyDerived.class)throw new AssertionError("ancestor source name/independent enclosing instance");try{new DependencyDerived(null).read();throw new AssertionError("lost dereference");}catch(NullPointerException expected){}rows++;}System.out.println(rows+":ancestor:dependency:separate:outer");}}`

func TestNativeInheritedNonstaticDeclarationDependencyRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, nativeInheritedDependencyFixture, []string{"DependencyBase", "DependencyDerived"}, "InheritedDependencyDriver", "2:ancestor:dependency:separate:outer\n", nativeLexicalExactSignatures)
}

func TestNativeInheritedNonstaticDeclarationDependencyRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeInheritedDependencyFixture, "DependencyBase", "DeclaringLexicalOwner")
	f = strings.ReplaceAll(f, "DependencyDerived", "CurrentLexicalOwner")
	f = strings.ReplaceAll(f, "Value", "Cursor")
	testNativeIndependentFamilyFixture(t, f, []string{"DeclaringLexicalOwner", "CurrentLexicalOwner"}, "InheritedDependencyDriver", "2:ancestor:dependency:separate:outer\n", nativeLexicalExactSignatures)
}

func TestNativeInheritedNonstaticDeclarationDependencyGenericSignatureRoundTrip(t *testing.T) {
	f := strings.Replace(nativeInheritedDependencyFixture, "class DependencyBase{", "class DependencyBase<T>{", 1)
	f = strings.Replace(f, "final Object token;Value(Object token)", "final T token;Value(T token)", 1)
	f = strings.Replace(f, "Value make(Object token)", "Value make(T token)", 1)
	f = strings.Replace(f, "class DependencyDerived extends DependencyBase", "class DependencyDerived<T> extends DependencyBase<T>", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"DependencyBase", "DependencyDerived"}, "InheritedDependencyDriver", "2:ancestor:dependency:separate:outer\n", nativeLexicalExactSignatures)
}

func TestNativeInheritedNonstaticDeclarationDependencyPrivateConstructorRoundTrip(t *testing.T) {
	f := strings.Replace(nativeInheritedDependencyFixture, "Value(Object token)", "private Value(Object token)", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"DependencyBase", "DependencyDerived"}, "InheritedDependencyDriver", "2:ancestor:dependency:separate:outer\n", nativeLexicalExactSignatures)
}

func TestNativeInheritedNonstaticDeclarationDependencyPackagedRoundTrip(t *testing.T) {
	f := "package ancestor.binding;\n" + nativeInheritedDependencyFixture
	testNativeIndependentFamilyFixture(t, f, []string{"ancestor/binding/DependencyBase", "ancestor/binding/DependencyDerived"}, "ancestor.binding.InheritedDependencyDriver", "2:ancestor:dependency:separate:outer\n", nativeLexicalExactSignatures)
}

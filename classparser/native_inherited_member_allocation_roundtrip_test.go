package javaclassparser

import (
	"slices"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

const nativeInheritedMemberAllocationFixture = `class InheritedAllocationEffects{static String trace="";static Object arg(Object token){trace+="A";return token;}static final java.io.IOException failure=new java.io.IOException("same");}
class InheritedAllocationOwner{class Child{final Object token;final long number;Child(Object token,long number)throws java.io.IOException{InheritedAllocationEffects.trace+="C";if(token==null)throw InheritedAllocationEffects.failure;this.token=token;this.number=number;}Object outer(){return InheritedAllocationOwner.this;}}Child make(Object token,long number)throws java.io.IOException{return new Child(InheritedAllocationEffects.arg(token),number);}}
class InheritedAllocationDerived extends InheritedAllocationOwner{Child build(Object token,long number)throws java.io.IOException{return new Child(InheritedAllocationEffects.arg(token),number);}}
class InheritedAllocationDriver{public static void main(String[]args)throws Exception{Object token=new Object();int rows=0;for(InheritedAllocationDerived owner:new InheritedAllocationDerived[]{new InheritedAllocationDerived(),new InheritedAllocationDerived()}){for(long n:new long[]{Long.MIN_VALUE,-1,0,Long.MAX_VALUE}){InheritedAllocationEffects.trace="";InheritedAllocationOwner.Child child=owner.build(token,n);if(child.outer()!=owner||child.token!=token||child.number!=n||!InheritedAllocationEffects.trace.equals("AC"))throw new AssertionError("THIS origin/widening/width/order");if(child.getClass().getDeclaringClass()!=InheritedAllocationOwner.class)throw new AssertionError("member declaration");rows++;}InheritedAllocationEffects.trace="";try{owner.build(null,0);throw new AssertionError("missing original checked failure");}catch(java.io.IOException failure){if(failure!=InheritedAllocationEffects.failure||!InheritedAllocationEffects.trace.equals("AC"))throw new AssertionError("failure identity/order");}}System.out.println(rows+":inherited:member:allocation:identity");}}`

func TestNativeInheritedMemberAllocationExternalSubclassRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, nativeInheritedMemberAllocationFixture, []string{"InheritedAllocationOwner", "InheritedAllocationDerived"}, "InheritedAllocationDriver", "8:inherited:member:allocation:identity\n", nativeLexicalExactSignatures, nativeInheritedAllocationExactCasts)
}

func TestNativeInheritedMemberAllocationRenamedRoundTrip(t *testing.T) {
	fixture := strings.ReplaceAll(nativeInheritedMemberAllocationFixture, "InheritedAllocationOwner", "BaseAllocationScope")
	fixture = strings.ReplaceAll(fixture, "InheritedAllocationDerived", "CurrentAllocationScope")
	fixture = strings.ReplaceAll(fixture, "Child", "Item")
	testNativeIndependentFamilyFixture(t, fixture, []string{"BaseAllocationScope", "CurrentAllocationScope"}, "InheritedAllocationDriver", "8:inherited:member:allocation:identity\n", nativeLexicalExactSignatures, nativeInheritedAllocationExactCasts)
}

func TestNativeInheritedMemberAllocationTwoAncestorsRoundTrip(t *testing.T) {
	fixture := strings.Replace(nativeInheritedMemberAllocationFixture, "class InheritedAllocationDerived extends InheritedAllocationOwner", "class InheritedAllocationMiddle extends InheritedAllocationOwner{}class InheritedAllocationDerived extends InheritedAllocationMiddle", 1)
	testNativeIndependentFamilyFixture(t, fixture, []string{"InheritedAllocationOwner", "InheritedAllocationDerived"}, "InheritedAllocationDriver", "8:inherited:member:allocation:identity\n", nativeLexicalExactSignatures, nativeInheritedAllocationExactCasts)
}

func TestNativeInheritedMemberAllocationPackagedRoundTrip(t *testing.T) {
	fixture := "package inherited.binding;\n" + nativeInheritedMemberAllocationFixture
	testNativeIndependentFamilyFixture(t, fixture, []string{"inherited/binding/InheritedAllocationOwner", "inherited/binding/InheritedAllocationDerived"}, "inherited.binding.InheritedAllocationDriver", "8:inherited:member:allocation:identity\n", nativeLexicalExactSignatures, nativeInheritedAllocationExactCasts)
}

func TestNativeInheritedMemberAllocationGenericErasureRoundTrip(t *testing.T) {
	fixture := strings.Replace(nativeInheritedMemberAllocationFixture, "class InheritedAllocationOwner{class Child{final Object token;", "class InheritedAllocationOwner<Outer>{class Child{final Outer token;", 1)
	fixture = strings.Replace(fixture, "Child(Object token,long number)", "Child(Outer token,long number)", 1)
	fixture = strings.Replace(fixture, "new Child(InheritedAllocationEffects.arg(token),number)", "new Child((Outer)InheritedAllocationEffects.arg(token),number)", 1)
	fixture = strings.Replace(fixture, "class InheritedAllocationDerived extends InheritedAllocationOwner", "class InheritedAllocationDerived<Item> extends InheritedAllocationOwner<Item>", 1)
	fixture = strings.Replace(fixture, "new Child(InheritedAllocationEffects.arg(token),number)", "new Child((Item)InheritedAllocationEffects.arg(token),number)", 1)
	testNativeIndependentFamilyFixture(t, fixture, []string{"InheritedAllocationOwner", "InheritedAllocationDerived"}, "InheritedAllocationDriver", "8:inherited:member:allocation:identity\n", nativeLexicalExactSignatures, nativeInheritedAllocationExactCasts)
}

func nativeInheritedAllocationExactCasts(t *testing.T, name string, original, rebuilt []byte) {
	t.Helper()
	casts := func(raw []byte) []string {
		obj, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		var result []string
		for _, method := range obj.Methods {
			n, _ := sourceBridgeUTF8(obj, method.NameIndex)
			if n != "build" {
				continue
			}
			for _, attribute := range method.Attributes {
				if code, ok := attribute.(*CodeAttribute); ok {
					decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
					if err := decoder.ParseOpcode(); err != nil {
						t.Fatal(err)
					}
					for _, op := range decoder.Opcodes() {
						if op.Instr.OpCode != core.OP_CHECKCAST {
							continue
						}
						target, known := sourceBridgeClassName(obj, core.Convert2bytesToInt(op.Data))
						if !known {
							t.Fatal("original cast class")
						}
						result = append(result, target)
					}
				}
			}
		}
		return result
	}
	old, next := casts(original), casts(rebuilt)
	if !slices.Equal(old, next) {
		t.Fatalf("inherited allocation changed runtime casts %s: %v -> %v", name, old, next)
	}
}

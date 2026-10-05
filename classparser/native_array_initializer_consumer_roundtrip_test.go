package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"strings"
	"testing"
)

func TestNativeEnumWideNestedArrayInitializerConsumerRoundTrip(t *testing.T) {
	const f = `enum WideArrayChoice{LEFT(0x1020304050607080L,0.25,java.util.Arrays.<java.util.Map<String,? super Integer>>asList(new java.util.HashMap<String,Object>()),"a","bbb");final long result;WideArrayChoice(long value,double ratio,java.util.List<java.util.Map<String,? super Integer>> rows,String... tags){rows.get(0).put("count",tags.length);result=value+(long)(ratio*4)+tags[1].length()+((Number)rows.get(0).get("count")).intValue();}}class WideArrayDriver{public static void main(String[]args){if(WideArrayChoice.LEFT.result!=0x1020304050607086L)throw new AssertionError("wide nested array operand binding");System.out.println("array:wide:nested:constructor");}}`
	testNativeIndependentFamilyFixture(t, f, []string{"WideArrayChoice"}, "WideArrayDriver", "array:wide:nested:constructor\n", nativeLexicalExactSignatures)
}
func TestNativeEnumMultiplePrivateArrayEffectOrderRoundTrip(t *testing.T) {
	const f = `class ArrayEffects{static String trace="";static Object token(String name){trace+=name;return name;}}enum ArrayOrderChoice{LEFT(new Object[]{ArrayEffects.token("A"),ArrayEffects.token("B")},new Object[]{ArrayEffects.token("C"),ArrayEffects.token("D")});final Object[] first,second;ArrayOrderChoice(Object[] a,Object[] b){first=a;second=b;ArrayEffects.token("S");}}class ArrayOrderDriver{public static void main(String[]args){if(!ArrayOrderChoice.LEFT.first[0].equals("A")||!ArrayOrderChoice.LEFT.first[1].equals("B")||!ArrayOrderChoice.LEFT.second[0].equals("C")||!ArrayOrderChoice.LEFT.second[1].equals("D")||!ArrayEffects.trace.equals("ABCDS")||ArrayOrderChoice.LEFT.first==ArrayOrderChoice.LEFT.second)throw new AssertionError("allocation identity/array effects/order");System.out.println("array:multiple:identity:effects:order");}}`
	testNativeIndependentFamilyFixture(t, f, []string{"ArrayOrderChoice"}, "ArrayOrderDriver", "array:multiple:identity:effects:order\n", nativeLexicalExactSignatures)
}
func TestNativeEnumPrivateArrayFailureRetainsOriginalPrefixEffectsRoundTrip(t *testing.T) {
	const f = `class ArrayFailureEffects{static String trace="";static final RuntimeException failure=new RuntimeException("original");static Object token(String name){trace+=name;if(name.equals("B"))throw failure;return name;}}enum ArrayFailureChoice{LEFT(new Object[]{ArrayFailureEffects.token("A"),ArrayFailureEffects.token("B")},new Object[]{ArrayFailureEffects.token("C")});ArrayFailureChoice(Object[] a,Object[] b){ArrayFailureEffects.trace+="S";}}class ArrayFailureDriver{public static void main(String[]args){try{ArrayFailureChoice.values();throw new AssertionError("missing original array fill failure");}catch(ExceptionInInitializerError e){if(e.getCause()!=ArrayFailureEffects.failure||!ArrayFailureEffects.trace.equals("AB"))throw new AssertionError("failure identity/partial prefix effects");}System.out.println("array:failure:identity:partial-effects");}}`
	testNativeIndependentFamilyFixture(t, f, []string{"ArrayFailureChoice"}, "ArrayFailureDriver", "array:failure:identity:partial-effects\n", nativeLexicalExactSignatures)
}

// Publishing a later argument leaves it as a separate source statement. Moving
// only the first private array across that statement would reverse A/B and C/D.
func TestNativeArrayPublishedLaterOperandKeepsEffectOrderRoundTrip(t *testing.T) {
	const f = `class PublishedArrayEffects{static String trace="";static Object[] saved;static Object token(String name){trace+=name;return name;}static PublishedArrayResult consume(Object[] a,Object[] b){return new PublishedArrayResult(a,b);}}class PublishedArrayResult{final Object[] first,second;PublishedArrayResult(Object[] a,Object[] b){first=a;second=b;PublishedArrayEffects.token("S");}}class PublishedArrayOwner{static PublishedArrayResult result=PublishedArrayEffects.consume(new Object[]{PublishedArrayEffects.token("A"),PublishedArrayEffects.token("B")},PublishedArrayEffects.saved=new Object[]{PublishedArrayEffects.token("C"),PublishedArrayEffects.token("D")});}class PublishedArrayDriver{public static void main(String[]args){PublishedArrayResult r=PublishedArrayOwner.result;if(!PublishedArrayEffects.trace.equals("ABCDS")||r.first==r.second||r.second!=PublishedArrayEffects.saved||!r.first[0].equals("A")||!r.second[1].equals("D"))throw new AssertionError("publication/identity/effect order:"+PublishedArrayEffects.trace);System.out.println("array:publication:identity:effect-order");}}`
	testNativeIndependentFamilyFixture(t, f, []string{"PublishedArrayOwner"}, "PublishedArrayDriver", "array:publication:identity:effect-order\n", nativeLexicalExactSignatures)
}

func TestNativeConstructorArrayPublishedLaterOperandKeepsEffectOrderRoundTrip(t *testing.T) {
	const f = `class PublishedArrayEffects{static String trace="";static Object[] saved;static Object token(String name){trace+=name;return name;}static PublishedArrayResult consume(Object[] a,Object[] b){return new PublishedArrayResult(a,b);}}class PublishedArrayResult{final Object[] first,second;PublishedArrayResult(Object[] a,Object[] b){first=a;second=b;PublishedArrayEffects.token("S");}}class PublishedArrayOwner{static PublishedArrayResult result=new PublishedArrayResult(new Object[]{PublishedArrayEffects.token("A"),PublishedArrayEffects.token("B")},PublishedArrayEffects.saved=new Object[]{PublishedArrayEffects.token("C"),PublishedArrayEffects.token("D")});}class PublishedArrayDriver{public static void main(String[]args){PublishedArrayResult r=PublishedArrayOwner.result;if(!PublishedArrayEffects.trace.equals("ABCDS")||r.first==r.second||r.second!=PublishedArrayEffects.saved||!r.first[0].equals("A")||!r.second[1].equals("D"))throw new AssertionError("publication/identity/effect order:"+PublishedArrayEffects.trace);System.out.println("array:publication:identity:effect-order");}}`
	testNativeIndependentFamilyFixture(t, f, []string{"PublishedArrayOwner"}, "PublishedArrayDriver", "array:publication:identity:effect-order\n", nativeLexicalExactSignatures)
}

// A valid stack-only array may be followed by an independent void effect before
// its consumer. Single use and linear control flow alone do not permit motion.
func TestNativeArrayIndependentBarrierKeepsOriginalEffectOrderRoundTrip(t *testing.T) {
	testNativeArrayBarrierEffectFixture(t, false)
}

func TestNativeConstructorArrayIndependentBarrierKeepsOriginalEffectOrderRoundTrip(t *testing.T) {
	testNativeArrayBarrierEffectFixture(t, true)
}

func testNativeArrayBarrierEffectFixture(t *testing.T, constructor bool) {
	f := `class ArrayBarrierEffects{static String trace="";static Object token(String name){trace+=name;return name;}static void barrier(){trace+="X";}static ArrayBarrierResult consume(Object[] a){return new ArrayBarrierResult(a);}}class ArrayBarrierResult{final Object[] array;ArrayBarrierResult(Object[] a){array=a;ArrayBarrierEffects.trace+="S";}}class ArrayBarrierOwner{static ArrayBarrierResult result=ArrayBarrierEffects.consume(new Object[]{ArrayBarrierEffects.token("A"),ArrayBarrierEffects.token("B")});}class ArrayBarrierDriver{public static void main(String[]args){ArrayBarrierResult r=ArrayBarrierOwner.result;if(!ArrayBarrierEffects.trace.equals("ABXS")||!r.array[0].equals("A")||!r.array[1].equals("B"))throw new AssertionError("independent barrier:"+ArrayBarrierEffects.trace);System.out.println("array:independent:barrier:effect-order");}}`
	if constructor {
		f = strings.Replace(f, "ArrayBarrierEffects.consume(new Object[]", "new ArrayBarrierResult(new Object[]", 1)
	}
	mutate := func(t *testing.T, files map[string][]byte) {
		obj, err := Parse(files["ArrayBarrierOwner.class"])
		if err != nil {
			t.Fatal(err)
		}
		pool := NewConstantPoolWithConstant(&obj.ConstantPool)
		idx := pool.AddNewMethodInfo("ArrayBarrierEffects", "barrier", "()V")
		changed := false
		for _, m := range obj.Methods {
			name, _ := sourceBridgeUTF8(obj, m.NameIndex)
			if name != "<clinit>" {
				continue
			}
			ops, known := nativeEnumMethodOps(obj, m, nil)
			if !known {
				t.Fatal("original opcodes")
			}
			pc := -1
			for _, op := range ops {
				if (!constructor && nativeEnumMemberOperand(obj, op, core.OP_INVOKESTATIC, "ArrayBarrierEffects", "consume", "([Ljava/lang/Object;)LArrayBarrierResult;")) || (constructor && nativeEnumMemberOperand(obj, op, core.OP_INVOKESPECIAL, "ArrayBarrierResult", "<init>", "([Ljava/lang/Object;)V")) {
					pc = int(op.CurrentOffset)
				}
			}
			if pc < 0 {
				t.Fatal("original consumer")
			}
			for _, a := range m.Attributes {
				if c, ok := a.(*CodeAttribute); ok {
					if len(c.ExceptionTable) != 0 {
						t.Fatal("unexpected handlers")
					}
					code := append([]byte(nil), c.Code[:pc]...)
					code = append(code, byte(core.OP_INVOKESTATIC), byte(idx>>8), byte(idx))
					code = append(code, c.Code[pc:]...)
					c.Code, c.Attributes = code, nil
					c.AttrLen = uint32(12 + len(code))
					changed = true
				}
			}
		}
		if !changed {
			t.Fatal("missing original initializer")
		}
		files["ArrayBarrierOwner.class"] = obj.Bytes()
	}
	testNativeIndependentMutatedFamilyFixture(t, f, []string{"ArrayBarrierOwner"}, "ArrayBarrierDriver", "array:independent:barrier:effect-order\n", mutate, nativeLexicalExactSignatures)
}

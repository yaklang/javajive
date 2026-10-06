package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

const constructorNestedAllocationPacketFixture = `
class AllocationToken {final Object value;AllocationToken(Object value){this.value=value;}}
class AllocationPacketParent {final AllocationToken[] result;AllocationPacketParent(AllocationToken[] result){this.result=result;}}
class AllocationPacketAdapter {static AllocationToken[] forward(AllocationToken[] values){return values;}}
class AllocationPacketSubject extends AllocationPacketParent {
 AllocationPacketSubject(String[] payload,boolean flag){super(new AllocationToken[]{new AllocationToken(payload),new AllocationToken(flag?payload:new String[]{"d"})});}
}

class AllocationPacketDriver {
 public static void main(String[]args){
  for(String[] payload:new String[][]{null,new String[0],new String[]{"a","z"}})
   for(boolean flag:new boolean[]{false,true}){
    AllocationPacketSubject subject=new AllocationPacketSubject(payload,flag);
    if(subject.result.getClass()!=AllocationToken[].class||subject.result[0].value!=payload)throw new AssertionError("array and element identity");
    if(flag){if(subject.result[1].value!=payload)throw new AssertionError("conditional identity");}
    else {String[] fallback=(String[])subject.result[1].value;if(fallback.length!=1||!"d".equals(fallback[0]))throw new AssertionError("conditional fallback");}
    System.out.println((payload==null?-1:payload.length)+":"+flag+":true");
   }
 }
}`

func TestAdversarialConstructorNestedAllocationPacketsRoundTrip(t *testing.T) {
	for _, variant := range []string{"direct", "static consumer", "renamed"} {
		t.Run(variant, func(t *testing.T) {
			f := constructorNestedAllocationPacketFixture
			subject := "AllocationPacketSubject"
			if variant == "static consumer" {
				f = strings.ReplaceAll(f, `super(new AllocationToken[]{new AllocationToken(payload),new AllocationToken(flag?payload:new String[]{"d"})})`, `super(AllocationPacketAdapter.forward(new AllocationToken[]{new AllocationToken(payload),new AllocationToken(flag?payload:new String[]{"d"})}))`)
			}
			if variant == "renamed" {
				f = strings.ReplaceAll(f, "AllocationPacketSubject", "FreshOperandSubject")
				f = strings.ReplaceAll(f, "AllocationToken", "FreshOperand")
				subject = "FreshOperandSubject"
			}
			testNativeIndependentFamilyFixture(t, f, []string{subject}, "AllocationPacketDriver",
				"-1:false:true\n-1:true:true\n0:false:true\n0:true:true\n2:false:true\n2:true:true\n", nativeLexicalExactSignatures)
		})
	}
}

const constructorNestedAllocationEffectsFixture = `
class AllocationTrace {
 static String trace="";static int fail=-1;
 static final Failure failure=new Failure();static class Failure extends RuntimeException {}
 static void mark(String event,int stage){trace+=event;if(fail==stage)throw failure;}
 static boolean choose(boolean flag){mark("C",1);return flag;}
 static String fallback(){mark("F",2);return "d";}
}
class EffectAllocationToken {final Object value;EffectAllocationToken(Object value,String event,int stage){AllocationTrace.mark(event,stage);this.value=value;}}
class EffectAllocationParent {final EffectAllocationToken[] result;EffectAllocationParent(EffectAllocationToken[] result){AllocationTrace.mark("P",4);this.result=result;}}
class EffectAllocationAdapter {static EffectAllocationToken[] forward(EffectAllocationToken[] values){return values;}}
class EffectAllocationSubject extends EffectAllocationParent {
 EffectAllocationSubject(String[] payload,boolean flag){super(new EffectAllocationToken[]{new EffectAllocationToken(payload,"A",0),new EffectAllocationToken(AllocationTrace.choose(flag)?payload:new String[]{AllocationTrace.fallback()},"B",3)});}
}
class EffectAllocationDriver {
 public static void main(String[]args){
  for(String[] payload:new String[][]{null,new String[0],new String[]{"a","z"}})
   for(boolean flag:new boolean[]{false,true})for(int fail=-1;fail<=4;fail++){
    AllocationTrace.trace="";AllocationTrace.fail=fail;String result="OK";
    try{
     EffectAllocationSubject subject=new EffectAllocationSubject(payload,flag);
     if(subject.result.getClass()!=EffectAllocationToken[].class||subject.result[0].value!=payload)throw new AssertionError("array/element identity");
     if(flag){if(subject.result[1].value!=payload)throw new AssertionError("conditional identity");}
     else {String[] fallback=(String[])subject.result[1].value;if(fallback.length!=1||!"d".equals(fallback[0]))throw new AssertionError("conditional fallback");}
    }catch(AllocationTrace.Failure failure){if(failure!=AllocationTrace.failure)throw new AssertionError("exception identity");result="X";}
    System.out.println((payload==null?-1:payload.length)+":"+flag+":"+fail+":"+AllocationTrace.trace+":"+result);
   }
 }
}`

// Independent event order includes both object initializations, the condition,
// the selected fresh-array initializer and the original superclass boundary.
// An exception must stop at that exact event and retain the original object.
func TestAdversarialConstructorNestedAllocationEffectsRoundTrip(t *testing.T) {
	var expected strings.Builder
	for _, length := range []int{-1, 0, 2} {
		for _, flag := range []bool{false, true} {
			for fail := -1; fail <= 4; fail++ {
				events := "ACFBP"
				if flag {
					events = "ACBP"
				}
				trace, result := "", "OK"
				for _, event := range events {
					trace += string(event)
					if map[rune]int{'A': 0, 'C': 1, 'F': 2, 'B': 3, 'P': 4}[event] == fail {
						result = "X"
						break
					}
				}
				fmt.Fprintf(&expected, "%d:%t:%d:%s:%s\n", length, flag, fail, trace, result)
			}
		}
	}
	for _, variant := range []string{"direct", "static consumer", "renamed"} {
		t.Run(variant, func(t *testing.T) {
			f := constructorNestedAllocationEffectsFixture
			subject := "EffectAllocationSubject"
			if variant == "static consumer" {
				f = strings.ReplaceAll(f, `super(new EffectAllocationToken[]{new EffectAllocationToken(payload,"A",0),new EffectAllocationToken(AllocationTrace.choose(flag)?payload:new String[]{AllocationTrace.fallback()},"B",3)})`, `super(EffectAllocationAdapter.forward(new EffectAllocationToken[]{new EffectAllocationToken(payload,"A",0),new EffectAllocationToken(AllocationTrace.choose(flag)?payload:new String[]{AllocationTrace.fallback()},"B",3)}))`)
			}
			if variant == "renamed" {
				f = strings.ReplaceAll(f, "EffectAllocationSubject", "FreshEffectsSubject")
				f = strings.ReplaceAll(f, "AllocationTrace", "FreshEffectsTrace")
				f = strings.ReplaceAll(f, "EffectAllocationToken", "FreshEffectsToken")
				subject = "FreshEffectsSubject"
			}
			testNativeIndependentFamilyFixture(t, f, []string{subject}, "EffectAllocationDriver", expected.String(), nativeLexicalExactSignatures)
		})
	}
}

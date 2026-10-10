package javaclassparser

import (
	"strings"
	"testing"
)

const constructorArrayComponentPacketFixture = `
class ComponentPacketParent {final Object[][] result;ComponentPacketParent(Object[][] result){this.result=result;}}
class ComponentPacketAdapter {static Object[][] forward(Object[][] values){return values;}}
class ComponentPacketSubject extends ComponentPacketParent {
 ComponentPacketSubject(String[] payload,boolean flag){super(ComponentPacketAdapter.forward(new Object[][]{payload,flag?null:payload}));}
}
class ComponentPacketDriver {
 public static void main(String[]args){
  for(String[] payload:new String[][]{null,new String[0],new String[]{"a","z"}})
   for(boolean flag:new boolean[]{false,true}){
    ComponentPacketSubject subject=new ComponentPacketSubject(payload,flag);
    if(subject.result.getClass()!=Object[][].class||subject.result[0]!=payload||
      subject.result[1]!=(flag?null:payload))throw new AssertionError("component/rank/reference identity");
    System.out.println((payload==null?-1:payload.length)+":"+flag+":"+(subject.result[0]==payload));
   }
 }
}`

// AASTORE in a reference-array allocation consumes an immediate component,
// which may itself be an array. Preserve that descriptor rather than dropping
// rank to a class name. Driver, adapter and superclass are original JVM code.
func TestAdversarialConstructorArrayComponentPacketsRoundTrip(t *testing.T) {
	for _, variant := range []string{"reference component", "primitive nested payload", "higher rank", "renamed"} {
		t.Run(variant, func(t *testing.T) {
			f := constructorArrayComponentPacketFixture
			subject := "ComponentPacketSubject"
			switch variant {
			case "primitive nested payload":
				f = strings.ReplaceAll(f, "String[] payload", "int[][] payload")
				f = strings.ReplaceAll(f, `new String[][]{null,new String[0],new String[]{"a","z"}}`, `new int[][][]{null,new int[0][],new int[][]{new int[]{Integer.MIN_VALUE,Integer.MAX_VALUE},null}}`)
			case "higher rank":
				f = strings.ReplaceAll(f, "Object[][]", "Object[][][]")
				f = strings.ReplaceAll(f, "String[] payload", "String[][] payload")
				f = strings.ReplaceAll(f, `new String[][]{null,new String[0],new String[]{"a","z"}}`, `new String[][][]{null,new String[0][],new String[][]{new String[]{"a","z"},null}}`)
			case "renamed":
				f = strings.ReplaceAll(f, "ComponentPacketSubject", "RankOperandSubject")
				f = strings.ReplaceAll(f, "ComponentPacketAdapter", "RankIdentity")
				f = strings.ReplaceAll(f, "forward", "preserve")
				subject = "RankOperandSubject"
			}
			testNativeIndependentFamilyFixture(t, f, []string{subject}, "ComponentPacketDriver",
				"-1:false:true\n-1:true:true\n0:false:true\n0:true:true\n2:false:true\n2:true:true\n", nativeLexicalExactSignatures)
		})
	}
}

package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// A primitive predicate and its chosen CHECKCAST arm stay after super. The
// parent changes the predicate and observes default child fields; the caller
// checks every fault phase, a malformed erased element and assertion ownership.
const numericArmInitializerFixture = `
class NumericArmEffects{static String trace="";static int mode;static NumericArmList active;static final RuntimeException error=new RuntimeException("phase");static int finish(){trace+="D";if((mode&16)!=0)throw error;return 7;}}
class NumericArmList extends java.util.AbstractList{int count;Object payload;NumericArmList(int c,Object p){count=c;payload=p;}public int size(){NumericArmEffects.trace+="P";if((NumericArmEffects.mode&4)!=0)throw NumericArmEffects.error;return count;}public Object get(int i){NumericArmEffects.trace+="R";if((NumericArmEffects.mode&8)!=0)throw NumericArmEffects.error;return payload;}}
abstract class NumericArmBase{NumericArmBase(){NumericArmEffects.trace+="B";if(get()!=null)throw new AssertionError("initializer ran before super");assert NumericArmEffects.active!=null;if((NumericArmEffects.mode&2)!=0)throw NumericArmEffects.error;if((NumericArmEffects.mode&1)!=0)NumericArmEffects.active.count=0;}abstract byte[] get();abstract int stamp();}
class NumericArmOwner{private NumericArmList blocks;
 NumericArmBase first(NumericArmList input){blocks=input;NumericArmEffects.active=input;return new NumericArmBase(){public byte[] get(){assert blocks!=null;return null;}public int stamp(){return 0;}};}
 NumericArmBase second(NumericArmList input){blocks=input;NumericArmEffects.active=input;return new NumericArmBase(){final byte[] data=blocks.size()==0?null:(byte[])blocks.get(0);final int completed=NumericArmEffects.finish();public byte[] get(){assert data==null||data.length>=0;return data;}public int stamp(){return completed;}};}
}
class NumericArmDriver{public static void main(String[]args){ClassLoader.getSystemClassLoader().setClassAssertionStatus("NumericArmOwner",ASSERT_STATUS);NumericArmOwner owner=new NumericArmOwner();NumericArmEffects.mode=0;NumericArmBase sibling=owner.first(new NumericArmList(0,null));if(!sibling.getClass().isAnonymousClass()||!sibling.getClass().getName().equals("NumericArmOwner$1"))throw new AssertionError("first anonymous sibling ownership");int rows=0;
 for(int count:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(Object payload:new Object[]{null,new byte[]{-128,0,127},"wrong erased type"})for(int mode=0;mode<32;mode++){
  NumericArmEffects.mode=mode;NumericArmEffects.trace="";NumericArmList list=new NumericArmList(count,payload);
  int c=(mode&1)!=0?0:count,sign=Integer.compare(c,0);boolean selected=SELECTED;
  String trace="B";int failure=(mode&2)!=0?2:0;if(failure==0){trace+="P";if((mode&4)!=0)failure=4;}
  if(failure==0&&selected){trace+="R";if((mode&8)!=0)failure=8;else if(payload!=null&&!(payload instanceof byte[]))failure=32;}
  if(failure==0){trace+="D";if((mode&16)!=0)failure=16;}
  Throwable thrown=null;NumericArmBase value=null;try{value=owner.second(list);}catch(Throwable e){thrown=e;}
  if(!NumericArmEffects.trace.equals(trace)||failure==0&&thrown!=null||failure==32&&!(thrown instanceof ClassCastException)||failure!=0&&failure!=32&&thrown!=NumericArmEffects.error)throw new AssertionError("predicate/arm/failure phase count="+count+" mode="+mode+" trace="+NumericArmEffects.trace+" expected="+trace,thrown);
  if(failure==0){Object expected=selected?payload:null;list.payload=new byte[]{42};list.count=17;if(value.get()!=expected||value.get()!=value.get()||value.stamp()!=7||!value.getClass().isAnonymousClass()||!value.getClass().getName().equals("NumericArmOwner$2")||!value.getClass().getEnclosingMethod().getName().equals("second"))throw new AssertionError("chosen arm identity/post-super field snapshot/ownership");}
  rows++;
 }if(rows!=480)throw new AssertionError("world census");System.out.println(rows+":primitive-predicate:chosen-cast:super:failure-phases");}}
`

func TestAdversarialAnonymousPrimitivePredicateInitializerKeepsChosenCast(t *testing.T) {
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("actual javac8 required")
	}
	for _, compiler := range []struct {
		name, javac string
		profile     SourceCompilerProfile
	}{{"native8", native, NativeJavac8}, {"modern8", modern, ModernJavac}} {
		for i, predicate := range []struct{ op, selected string }{{"==", "sign!=0"}, {"!=", "sign==0"}, {"<", "sign>=0"}, {"<=", "sign>0"}, {">", "sign<=0"}, {">=", "sign<0"}} {
			for _, status := range []string{"false", "true"} {
				t.Run(compiler.name+"/"+predicate.op+"/assertions="+status, func(t *testing.T) {
					source := strings.NewReplacer("blocks.size()==0", "blocks.size()"+predicate.op+"0", "ASSERT_STATUS", status, "SELECTED", predicate.selected).Replace(numericArmInitializerFixture)
					owner := "NumericArmOwner"
					if i%2 != 0 {
						source = strings.ReplaceAll(source, owner, "OtherPredicateOwner")
						owner = "OtherPredicateOwner"
					}
					compile := func(debug string) map[string][]byte {
						if compiler.profile == NativeJavac8 {
							return localGetterCompileJavac8(t, map[string]string{owner + ".java": source}, debug)
						}
						return nativeCompileDebugClasses(t, source, debug)
					}
					testNativeIndependentCompilerFamilyWithPolicies(t, compile, compiler.profile, compiler.javac, []string{owner}, "NumericArmDriver", "480:primitive-predicate:chosen-cast:super:failure-phases\n", nil, nil, []string{"normal"}, nativeLexicalExactSignatures)
				})
			}
		}
	}
}

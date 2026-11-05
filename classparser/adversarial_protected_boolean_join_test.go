package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// Both boolean definitions reach one return; a caught exception reuses the
// physical slot but does not define that returned value. The original driver
// checks values, selected effects, null precedence and exact wrapped failures.
const protectedBooleanJoinFixture = `
class BooleanEffects {static int trace;static final IllegalArgumentException first=new IllegalArgumentException("first");static final IllegalAccessException second=new IllegalAccessException("second");static final NullPointerException missing=new NullPointerException("missing");}
class BooleanOwner {
 boolean first(Object value,int faults) {BooleanEffects.trace=1;if((faults&1)!=0)throw BooleanEffects.first;return ((Integer)value).intValue()<0;}
 boolean second(Object value,int faults) throws IllegalAccessException {BooleanEffects.trace=2;if((faults&2)!=0)throw BooleanEffects.second;return (((Integer)value).intValue()&1)!=0;}
 boolean run(boolean choose,Object value,int faults) {
  if(value==null)throw BooleanEffects.missing;
  try { boolean answer;if(choose)answer=first(value,faults);else answer=second(value,faults);return answer; }
  catch(IllegalArgumentException|IllegalAccessException error){throw new IllegalStateException(error);}
 }
}
class BooleanDriver {
 public static void main(String[] args) {int rows=0;for(boolean choose:new boolean[]{false,true})for(Integer value:new Integer[]{null,Integer.MIN_VALUE,-3,-2,-1,0,1,2,3,Integer.MAX_VALUE})for(int faults=0;faults<4;faults++){
 BooleanEffects.trace=0;boolean result=false;Throwable error=null;try{result=new BooleanOwner().run(choose,value,faults);}catch(Throwable e){error=e;}
 Throwable expected=value==null?BooleanEffects.missing:choose?(faults&1)!=0?BooleanEffects.first:null:(faults&2)!=0?BooleanEffects.second:null;
 if(value==null){if(error!=expected)throw new AssertionError("null precedence",error);}else if(expected!=null){if(error==null||error.getClass()!=IllegalStateException.class||error.getCause()!=expected)throw new AssertionError("exception identity",error);}else if(error!=null||result!=(choose?value.longValue()<0:Math.floorMod(value.longValue(),2)==1))throw new AssertionError("branch value",error);
 if(BooleanEffects.trace!=(value==null?0:choose?1:2))throw new AssertionError("effects");rows++;
 }if(rows!=80)throw new AssertionError("inventory");System.out.println(rows+":boolean-join:values:effects:failure-identity");}
}`

func TestAdversarialProtectedBooleanDefinitionsKeepJoinedReturn(t *testing.T) {
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("actual javac8 required")
	}
	for _, compiler := range []struct {
		name, javac string
		profile     SourceCompilerProfile
	}{{"native8", native, NativeJavac8}, {"modern8", modern, ModernJavac}} {
		for _, owner := range []string{"BooleanOwner", "OtherChoiceOwner"} {
			for _, recovery := range []string{"default", "strict"} {
				t.Run(compiler.name+"/"+owner+"/"+recovery, func(t *testing.T) {
					if recovery == "strict" {
						t.Setenv("JDEC_INIT_PROX_SPLIT_OFF", "1")
					}
					source := strings.ReplaceAll(protectedBooleanJoinFixture, "BooleanOwner", owner)
					compile := func(debug string) map[string][]byte {
						if compiler.profile == NativeJavac8 {
							return localGetterCompileJavac8(t, map[string]string{owner + ".java": source}, debug)
						}
						return nativeCompileDebugClasses(t, source, debug)
					}
					testNativeIndependentCompilerFamilyWithPolicies(t, compile, compiler.profile, compiler.javac, []string{owner}, "BooleanDriver", "80:boolean-join:values:effects:failure-identity\n", nil, nil, []string{"normal", "no-source-rewrites", "no-core-cleanups"}, nativeLexicalExactSignatures)
				})
			}
		}
	}
}

package javaclassparser

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

const ownedAbstractEnumSwitchFixture = `class OwnedEnumSwitch {
 static enum Mode{ADD{long apply(long x){return x+7;}},XOR{long apply(long x){return x^13;}};abstract long apply(long x);}
 static long run(Mode mode,long x){switch(mode){case ADD:return mode.apply(x)+31;case XOR:return mode.apply(x)-17;default:throw new AssertionError("unknown constant");}}
}
class OwnedEnumSwitchDriver{public static void main(String[]args){int rows=0;for(OwnedEnumSwitch.Mode mode:OwnedEnumSwitch.Mode.values())for(long x:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){long expected=mode==OwnedEnumSwitch.Mode.ADD?x+7+31:(x^13)-17;if(OwnedEnumSwitch.run(mode,x)!=expected)throw new AssertionError("constant body dispatch/switch");if(OwnedEnumSwitch.Mode.valueOf(mode.name())!=mode||mode.getDeclaringClass()!=OwnedEnumSwitch.Mode.class||!mode.getClass().isAnonymousClass())throw new AssertionError("enum factory/body identities");rows++;}if(OwnedEnumSwitch.Mode.class.getDeclaringClass()!=OwnedEnumSwitch.class||!OwnedEnumSwitch.Mode.class.isMemberClass()||!java.lang.reflect.Modifier.isAbstract(OwnedEnumSwitch.Mode.class.getModifiers()))throw new AssertionError("abstract original declaration scope");try{OwnedEnumSwitch.run(null,0);throw new AssertionError("missing null ordinal failure");}catch(NullPointerException expected){}System.out.println(rows+":owned-abstract-enum:switch:dispatch:scope");}}
`

func ownedEnumSwitchVariant(abstract, deep, wide, rename bool) (string, string) {
	s, owner := ownedAbstractEnumSwitchFixture, "OwnedEnumSwitch"
	if !abstract {
		s = strings.NewReplacer("abstract long apply(long x);", "long apply(long x){return x;}", "!java.lang.reflect.Modifier.isAbstract(", "java.lang.reflect.Modifier.isAbstract(").Replace(s)
	}
	if wide {
		s = strings.NewReplacer("ADD{long apply", "ADD(Long.MIN_VALUE,-0.0){long apply", "XOR{long apply", "XOR(Long.MAX_VALUE,2.0){long apply", ";abstract long apply", ";final long step;final double bias;Mode(long step,double bias){this.step=step;this.bias=bias;}abstract long apply", ";long apply(long x){return x;}", ";final long step;final double bias;Mode(long step,double bias){this.step=step;this.bias=bias;}long apply(long x){return x;}", "rows++;", "if(mode.step!=(mode==OwnedEnumSwitch.Mode.ADD?Long.MIN_VALUE:Long.MAX_VALUE)||Double.doubleToRawLongBits(mode.bias)!=Double.doubleToRawLongBits(mode==OwnedEnumSwitch.Mode.ADD?-0.0:2.0))throw new AssertionError(\"source argument slots\");rows++;").Replace(s)
	}
	if deep {
		s = strings.Replace(s, "class OwnedEnumSwitch {", "class OwnedEnumSwitch {static class Container {", 1)
		s = strings.Replace(s, "}\nclass OwnedEnumSwitchDriver", "}}\nclass OwnedEnumSwitchDriver", 1)
		s = strings.ReplaceAll(s, "OwnedEnumSwitch.Mode", "OwnedEnumSwitch.Container.Mode")
		s = strings.ReplaceAll(s, "OwnedEnumSwitch.run", "OwnedEnumSwitch.Container.run")
		s = strings.ReplaceAll(s, "!=OwnedEnumSwitch.class", "!=OwnedEnumSwitch.Container.class")
	}
	if rename {
		owner = "OriginalOperationScope"
		s = strings.NewReplacer("OwnedEnumSwitch", owner, "Mode", "Choice", "apply", "evaluate", "ADD", "PLUS", "XOR", "MASK").Replace(s)
	}
	return s, owner
}

// The unchanged driver runs on the original first and has no original target
// classes on its candidate classpath. Abstractness, enum factories, anonymous
// bodies and declaration scope are separate observable obligations.
func TestAdversarialOwnedEnumSwitchConstantBodyNativeCompiler(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Fatal("JAVA8_JAVAC required")
	}
	for _, abstract := range []bool{false, true} {
		for _, deep := range []bool{false, true} {
			for _, wide := range []bool{false, true} {
				for _, rename := range []bool{false, true} {
					t.Run(fmt.Sprintf("abstract=%t/deep=%t/wide=%t/rename=%t", abstract, deep, wide, rename), func(t *testing.T) {
						s, owner := ownedEnumSwitchVariant(abstract, deep, wide, rename)
						driver := strings.ReplaceAll("OwnedEnumSwitchDriver", "OwnedEnumSwitch", owner)
						testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte { return nativePrivateEnumCompile(t, s, owner, debug) }, NativeJavac8, javac, []string{owner}, driver, "10:owned-abstract-enum:switch:dispatch:scope\n", nil, nativeLexicalExactSignatures)
					})
				}
			}
		}
	}
}

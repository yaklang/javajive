package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

const nativeAnonymousNullArgumentTemplate = `interface NullableAction{Object apply(Object token);Object peek();}
class NullableEffects{static String trace="";static boolean fail;static Object arg(Object v){trace+="A";return v;}static final RuntimeException fault=new IllegalStateException("identity"),wrong=new IllegalArgumentException("wrong overload");}
class NullableOwner{NullableAction make(final TYPE seed){return new NullableAction(){private TYPE value=seed;public Object peek(){return value;}private Object accept(TYPE v){NullableEffects.trace+="O";if(NullableEffects.fail)throw NullableEffects.fault;return v;}private Object accept(String v){NullableEffects.trace+="S";throw NullableEffects.wrong;}public Object apply(Object token){value=(TYPE)token;Runnable step=new Runnable(){public void run(){value=null;accept((TYPE)null);}};step.run();return value;}};}}
class NullableDriver{public static void main(String[]a){Object identity=IDENTITY;int rows=0;for(Object seed:new Object[]{null,identity}){NullableAction action=new NullableOwner().make((TYPE)seed);if(action.peek()!=seed)throw new AssertionError("initial identity");for(Object token:new Object[]{null,identity})for(boolean fail:new boolean[]{false,true}){NullableEffects.fail=fail;NullableEffects.trace="";try{if(action.apply(token)!=null||fail)throw new AssertionError("missing result/failure");}catch(RuntimeException e){if(!fail||e!=NullableEffects.fault)throw new AssertionError("exception identity/overload",e);}if(action.peek()!=null||!NullableEffects.trace.equals("TRACE"))throw new AssertionError("clearing/order/overload");rows++;}}System.out.println(rows+":null:private:clear:overload:effects");}}
`

func TestAdversarialAnonymousOriginalNullOperands(t *testing.T) {
	for _, shape := range []string{"object", "reference", "array", "nested call", "setter only", "call only", "renamed"} {
		t.Run(shape, func(t *testing.T) {
			typ, identity, trace := "Object", "new Object()", "O"
			if shape == "reference" {
				typ, identity = "CharSequence", "new StringBuilder()"
			}
			if shape == "array" {
				typ, identity = "Object[]", "new Object[]{new Object()}"
			}
			source := strings.NewReplacer("TYPE", typ, "IDENTITY", identity).Replace(nativeAnonymousNullArgumentTemplate)
			if shape == "nested call" {
				source = strings.Replace(source, "accept((Object)null)", "accept(NullableEffects.arg((Object)null))", 1)
				trace = "AO"
			}
			if shape == "setter only" {
				source = strings.Replace(source, "accept((Object)null);", "NullableEffects.trace+=\"O\";if(NullableEffects.fail)throw NullableEffects.fault;", 1)
			}
			if shape == "call only" {
				source = strings.Replace(source, "value=null;accept((Object)null);", "accept((Object)null);", 1)
				source = strings.Replace(source, "NullableEffects.trace+=\"O\";if(NullableEffects.fail)", "value=null;NullableEffects.trace+=\"O\";if(NullableEffects.fail)", 1)
			}
			source = strings.ReplaceAll(source, "TRACE", trace)
			root, driver := "NullableOwner", "NullableDriver"
			if shape == "renamed" {
				source = strings.NewReplacer("Nullable", "Different", "accept", "consume", "value", "stored").Replace(source)
				root, driver = "DifferentOwner", "DifferentDriver"
			}
			for _, native := range []bool{false, true} {
				name := "modern"
				if native {
					name = "native8"
				}
				t.Run(name, func(t *testing.T) {
					if native {
						testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
							return localGetterCompileJavac8(t, map[string]string{root + ".java": source}, debug)
						}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{root}, driver, "8:null:private:clear:overload:effects\n", nil, nativeLexicalExactSignatures)
					} else {
						testNativePrivateSetterCompiledFixture(t, root, driver, "8:null:private:clear:overload:effects\n", func(t *testing.T, debug string) map[string][]byte {
							return nativeCompileSourceReleaseClasses(t, map[string]string{root + ".java": source}, debug, "8")
						}, nativeLexicalExactSignatures)
					}
				})
			}
		})
	}
}

package javaclassparser

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// One original anonymous declaration plays two compiler roles: the marker
// parameter of a private-super constructor bridge and a bound lambda receiver.
// Delayed reads, throwable identity and reflection observe both compositions.
const sharedAnonymousBridgeLambdaFixture = `class SharedMarkerEffects {static String trace="";static boolean fail;static final IllegalArgumentException error=new IllegalArgumentException("original");}
class SharedMarkerOwner {
 static abstract class Base {private Base(){} abstract java.util.function.Supplier<Object> supplier();abstract void set(Object value);}
 static Base make(final Object token){return new Base(){private Object current=token;public void set(Object value){SharedMarkerEffects.trace+="M";current=value;}public java.util.function.Supplier<Object> supplier(){SharedMarkerEffects.trace+="F";return ()->{SharedMarkerEffects.trace+="L";if(SharedMarkerEffects.fail)throw SharedMarkerEffects.error;return this.current;};}};}
}
class SharedMarkerDriver{public static void main(String[]args){int rows=0;Object shared=new Object();for(Object first:new Object[]{null,shared,new String("first")})for(Object next:new Object[]{null,shared,new String("next")}){SharedMarkerOwner.Base receiver=SharedMarkerOwner.make(first);SharedMarkerEffects.trace="";SharedMarkerEffects.fail=false;java.util.function.Supplier<Object> op=receiver.supplier(),other=receiver.supplier();if(op.get()!=first)throw new AssertionError("initial identity");receiver.set(next);if(op.get()!=next||other.get()!=next)throw new AssertionError("delayed receiver");SharedMarkerEffects.fail=true;try{other.get();throw new AssertionError("missing lambda failure");}catch(IllegalArgumentException e){if(e!=SharedMarkerEffects.error)throw new AssertionError("lambda failure identity",e);}if(!SharedMarkerEffects.trace.equals("FFLMLLL"))throw new AssertionError("effects/order");if(!receiver.getClass().isAnonymousClass()||receiver.getClass().getEnclosingClass()!=SharedMarkerOwner.class||SharedMarkerOwner.Base.class.getDeclaringClass()!=SharedMarkerOwner.class)throw new AssertionError("original scopes");rows++;}System.out.println(rows+":shared-marker:lambda:identity:scope");}}
`

func sharedAnonymousBridgeLambdaVariant(deep, wide, rename bool) (string, string) {
	s, owner := sharedAnonymousBridgeLambdaFixture, "SharedMarkerOwner"
	if wide {
		s = strings.NewReplacer("supplier();", "supplier(Object capture,long salt);", "supplier(){", "supplier(Object capture,long salt){", "static boolean fail;", "static boolean fail;static Object captured;static long observed;", "SharedMarkerEffects.trace+=\"L\";", "SharedMarkerEffects.trace+=\"L\";SharedMarkerEffects.captured=capture;SharedMarkerEffects.observed=salt;", "op=receiver.supplier(),other=receiver.supplier()", "op=receiver.supplier(first,Long.MIN_VALUE),other=receiver.supplier(next,Long.MAX_VALUE)", "if(op.get()!=first)", "if(op.get()!=first||SharedMarkerEffects.captured!=first||SharedMarkerEffects.observed!=Long.MIN_VALUE)", "if(op.get()!=next||other.get()!=next)", "if(op.get()!=next||SharedMarkerEffects.captured!=first||SharedMarkerEffects.observed!=Long.MIN_VALUE||other.get()!=next||SharedMarkerEffects.captured!=next||SharedMarkerEffects.observed!=Long.MAX_VALUE)").Replace(s)
	}
	if deep {
		s = strings.Replace(s, "class SharedMarkerOwner {", "class SharedMarkerOwner {static class Holder {", 1)
		s = strings.Replace(s, "}\nclass SharedMarkerDriver", "}}\nclass SharedMarkerDriver", 1)
		s = strings.ReplaceAll(s, "SharedMarkerOwner.Base", "SharedMarkerOwner.Holder.Base")
		s = strings.ReplaceAll(s, "SharedMarkerOwner.make", "SharedMarkerOwner.Holder.make")
		s = strings.ReplaceAll(s, "!=SharedMarkerOwner.class", "!=SharedMarkerOwner.Holder.class")
	}
	if rename {
		owner = "DeferredSharedCompilerScope"
		s = strings.NewReplacer("SharedMarkerOwner", owner, "current", "payload").Replace(s)
	}
	return s, owner
}
func TestAdversarialSharedAnonymousBridgeLambdaRoundTrip(t *testing.T) {
	for _, deep := range []bool{false, true} {
		for _, wide := range []bool{false, true} {
			for _, rename := range []bool{false, true} {
				t.Run(fmt.Sprintf("deep=%t/wide=%t/renamed=%t", deep, wide, rename), func(t *testing.T) {
					s, owner := sharedAnonymousBridgeLambdaVariant(deep, wide, rename)
					testNativePrivateSetterFixture(t, s, owner, "SharedMarkerDriver", "9:shared-marker:lambda:identity:scope\n")
				})
			}
		}
	}
}
func TestAdversarialSharedAnonymousBridgeLambdaNativeCompiler(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Fatal("JAVA8_JAVAC required")
	}
	for _, deep := range []bool{false, true} {
		for _, wide := range []bool{false, true} {
			t.Run(fmt.Sprintf("deep=%t/wide=%t", deep, wide), func(t *testing.T) {
				s, owner := sharedAnonymousBridgeLambdaVariant(deep, wide, false)
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte { return nativePrivateEnumCompile(t, s, owner, debug) }, NativeJavac8, javac, []string{owner}, "SharedMarkerDriver", "9:shared-marker:lambda:identity:scope\n", nil, nativeLexicalExactSignatures)
			})
		}
	}
}

package javaclassparser

import (
	"strings"
	"testing"
)

// An exact EnclosingMethod belongs to its actual nested named class, not the
// source compilation-unit root. A local owned by Root.Level.Inner must not
// borrow Root.make's scope, captured THIS or compiler registration ordinal.
func TestAdversarialNestedMethodLocalOwnerRoundTrip(t *testing.T) {
	for _, root := range []string{"NestedLocalOwner", "OtherNestedLocalOwner"} {
		t.Run(root, func(t *testing.T) {
			for _, scope := range []string{"instance chain", "static ancestor", "static owner", "deeper chain"} {
				t.Run(scope, func(t *testing.T) {
					source := `abstract class NestedLocalBase {static Object published,observed;static long observedLong;NestedLocalBase(){published=this;observedLong=get(0);observed=token();}abstract long get(long delta);abstract Object token();}
class NestedLocalOwner {
 class Level {class Inner {NestedLocalBase make(final long seed,final Object token){class Entry extends NestedLocalBase{long get(long delta){return seed+delta;}Object token(){return token;}}return new Entry();}}}
 NestedLocalBase make(final long seed,final Object token){class Entry extends NestedLocalBase{long get(long delta){return seed-delta;}Object token(){return token;}}return new Entry();}
 NestedLocalBase nested(long seed,Object token){return new Level().new Inner().make(seed,token);}}
class NestedLocalDriver {public static void main(String[] args){int count=0;Object identity=new Object();for(long seed:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(long delta:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(Object token:new Object[]{null,identity})for(boolean nested:new boolean[]{false,true}){NestedLocalOwner owner=new NestedLocalOwner();NestedLocalBase value=nested?owner.nested(seed,token):owner.make(seed,token);java.math.BigInteger want=java.math.BigInteger.valueOf(seed);want=nested?want.add(java.math.BigInteger.valueOf(delta)):want.subtract(java.math.BigInteger.valueOf(delta));if(value.get(delta)!=want.longValue()||value.token()!=token||NestedLocalBase.published!=value||NestedLocalBase.observedLong!=seed||NestedLocalBase.observed!=token)throw new AssertionError("original captured values before superclass observation");Class<?> kind=value.getClass(),enclosing=nested?NestedLocalOwner.Level.Inner.class:NestedLocalOwner.class;if(!kind.isLocalClass()||kind.getDeclaringClass()!=null||kind.getEnclosingClass()!=enclosing||kind.getEnclosingMethod().getDeclaringClass()!=enclosing||!kind.getEnclosingMethod().getName().equals("make")||!kind.getName().equals(nested?"NestedLocalOwner$Level$Inner$1Entry":"NestedLocalOwner$1Entry")||kind.getDeclaredConstructors()[0].getParameterCount()!=3)throw new AssertionError("actual declaring owner/method/ordinal/capture ABI");count++;}System.out.println(count+":nested:local:owner:wide:identity:pre-super");}}`
					source = strings.ReplaceAll(source, "NestedLocalOwner", root)
					switch scope {
					case "static ancestor":
						source = strings.Replace(source, "class Level", "static class Level", 1)
					case "static owner":
						source = strings.Replace(source, "class Level", "static class Level", 1)
						source = strings.Replace(source, "class Inner", "static class Inner", 1)
						source = strings.Replace(source, "new Level().new Inner()", "new Level.Inner()", 1)
					case "deeper chain":
						source = strings.Replace(source, "class Level {class Inner", "class Level {class Middle {class Inner", 1)
						source = strings.Replace(source, "return new Entry();}}}", "return new Entry();}}}}", 1)
						source = strings.Replace(source, "new Level().new Inner()", "new Level().new Middle().new Inner()", 1)
						source = strings.ReplaceAll(source, "Level.Inner.class", "Level.Middle.Inner.class")
						source = strings.ReplaceAll(source, "$Level$Inner$1Entry", "$Level$Middle$Inner$1Entry")
					}
					testNativeIndependentFamilyFixture(t, source, []string{root}, "NestedLocalDriver", "100:nested:local:owner:wide:identity:pre-super\n")
				})
			}
		})
	}
}

package javaclassparser

import (
	"strings"
	"testing"
)

// The original JVM checks identities, wide overflow and physical local-class
// ABI before the rebuilt program is executed. Generic capture fields are
// erased by javac even though their declaring method has a Signature.
func TestAdversarialMethodLocalParameterizedCaptureRoundTrip(t *testing.T) {
	for _, root := range []string{"LocalListOwner", "OtherLocalListOwner"} {
		t.Run(root, func(t *testing.T) {
			source := `interface LocalListValue {long get(long delta);Object token();java.util.List<String> items();}
class LocalListOwner<T extends Number>{
 <T extends CharSequence> LocalListValue make(final T token,final java.util.List<String> items,final long seed){class Entry implements LocalListValue{public long get(long delta){return seed+delta;}public T token(){return token;}public java.util.List<String> items(){return items;}}return new Entry();}
 static <T extends CharSequence> LocalListValue stat(final T token,final java.util.List<String> items,final long seed){class Node implements LocalListValue{public long get(long delta){return seed-delta;}public T token(){return token;}public java.util.List<String> items(){return items;}}return new Node();}}
class LocalListDriver{public static void main(String[] args){int count=0;CharSequence identity=new StringBuilder("identity");java.util.List<String> items=new java.util.ArrayList<String>();items.add("unchanged");for(long seed:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(long delta:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(CharSequence token:new CharSequence[]{null,identity})for(java.util.List<String> input:new java.util.List[]{null,items})for(int mode=0;mode<2;mode++){LocalListValue value=mode==0?new LocalListOwner<Integer>().make(token,input,seed):LocalListOwner.stat(token,input,seed);java.math.BigInteger want=java.math.BigInteger.valueOf(seed);want=mode==0?want.add(java.math.BigInteger.valueOf(delta)):want.subtract(java.math.BigInteger.valueOf(delta));if(value.get(delta)!=want.longValue()||value.token()!=token||value.items()!=input)throw new AssertionError("parameterized capture/identity/wide overflow");Class<?> kind=value.getClass();if(!kind.isLocalClass()||kind.getDeclaringClass()!=null||kind.getEnclosingClass()!=LocalListOwner.class||!kind.getEnclosingMethod().getName().equals(mode==0?"make":"stat")||kind.getDeclaredConstructors()[0].getParameterCount()!=(mode==0?4:3))throw new AssertionError("local owner/constructor ABI");for(java.lang.reflect.Field f:kind.getDeclaredFields())if(f.isSynthetic()&&f.getGenericType()!=f.getType())throw new AssertionError("hidden capture Signature changed");count++;}if(items.size()!=1||!items.get(0).equals("unchanged"))throw new AssertionError("capture mutated input");System.out.println(count+":local:parameterized:static:shadow:wide:identity:field-ABI");}}`
			source = strings.ReplaceAll(source, "LocalListOwner", root)
			testNativeIndependentFamilyFixture(t, source, []string{root}, "LocalListDriver", "200:local:parameterized:static:shadow:wide:identity:field-ABI\n")
		})
	}
}

package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialMethodLocalGenericStaticAndFinalScopes(t *testing.T) {
	for _, owner := range []string{"GenericLocalScopeOwner", "OtherGenericLocalScopeOwner"} {
		t.Run(owner, func(t *testing.T) {
			source := `interface GenericLocalValue{long get(long delta);Object token();}
class GenericLocalBase<U extends CharSequence> implements GenericLocalValue {public long get(long delta){return delta;}public Object token(){return null;}}
class GenericLocalScopeOwner<T extends Number>{private final long bias;GenericLocalScopeOwner(long b){bias=b;}
<T extends CharSequence> GenericLocalValue make(final T token,final long seed,boolean flag){final class Entry<U extends CharSequence> extends GenericLocalBase<U>{public long get(long delta){return bias+seed+delta;}public T token(){return token;}}if(flag)return new Entry<StringBuilder>();return new Entry<StringBuilder>();}
static <T extends CharSequence> GenericLocalValue stat(final T token,final long seed){final class Entry<U extends CharSequence> extends GenericLocalBase<U>{public long get(long delta){return seed-delta;}public T token(){return token;}}return new Entry<StringBuilder>();}}
class GenericLocalScopeDriver {public static void main(String[] args){int count=0;CharSequence identity=new StringBuilder("identity");Class<?>[] kinds=new Class<?>[2];for(long seed:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(long delta:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(CharSequence token:new CharSequence[]{null,identity})for(boolean flag:new boolean[]{false,true})for(int mode=0;mode<2;mode++){GenericLocalScopeOwner<Integer> owner=new GenericLocalScopeOwner<Integer>(Long.MAX_VALUE);GenericLocalValue value=mode==0?owner.make(token,seed,flag):GenericLocalScopeOwner.stat(token,seed);java.math.BigInteger want=java.math.BigInteger.valueOf(seed);want=mode==0?want.add(java.math.BigInteger.valueOf(Long.MAX_VALUE)).add(java.math.BigInteger.valueOf(delta)):want.subtract(java.math.BigInteger.valueOf(delta));if(value.get(delta)!=want.longValue()||value.token()!=token)throw new AssertionError("generic scopes/capture/wide/identity");kinds[mode]=value.getClass();count++;}for(int mode=0;mode<2;mode++){Class<?> c=kinds[mode];if(!c.isLocalClass()||c.getDeclaringClass()!=null||!c.getEnclosingMethod().getName().equals(mode==0?"make":"stat")||!c.getName().equals("GenericLocalScopeOwner$"+(mode+1)+"Entry")||!java.lang.reflect.Modifier.isFinal(c.getModifiers())||c.getTypeParameters().length!=1||c.getDeclaredConstructors()[0].getParameterCount()!=(mode==0?3:2))throw new AssertionError("local/static/final/generic/constructor ABI");}System.out.println(count+":local:generic-shadow:static:final:wide:repeat:identity");}}`
			source = strings.ReplaceAll(source, "GenericLocalScopeOwner", owner)
			testNativeIndependentFamilyFixture(t, source, []string{owner}, "GenericLocalScopeDriver", "200:local:generic-shadow:static:final:wide:repeat:identity\n")
		})
	}
}

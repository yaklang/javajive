package javaclassparser

import (
	"strings"
	"testing"
)

// Authored failure inventory for a distinct lexical domain. A named local
// class is owned by an original method, not a member or an anonymous ordinal.
// The source transaction must preserve that complete lexical domain.
func TestAdversarialMethodLocalScopeCaptureAndOrdinalRoundTrip(t *testing.T) {
	for _, root := range []string{"LocalCaptureOwner", "OtherLocalCaptureOwner"} {
		t.Run(root, func(t *testing.T) {
			source := `interface LocalMvpValue {long get(long delta);Object token();}
class LocalCaptureOwner<T> {
 private final long bias;LocalCaptureOwner(long bias){this.bias=bias;}
 <T> LocalMvpValue first(final T token,final long seed){class Entry implements LocalMvpValue {public long get(long delta){return bias+seed+delta;}public T token(){return token;}}return new Entry();}
 <T> LocalMvpValue second(final T token,final long seed){class Entry implements LocalMvpValue {public long get(long delta){return bias-seed-delta;}public T token(){return token;}}return new Entry();}
}
class LocalMvpDriver {public static void main(String[] args){int count=0;Class<?>[] kinds=new Class<?>[2];Object identity=new Object();for(long bias:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(long seed:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(long delta:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(Object token:new Object[]{null,identity})for(int mode=0;mode<2;mode++){LocalCaptureOwner<String> owner=new LocalCaptureOwner<String>(bias);LocalMvpValue value=mode==0?owner.first(token,seed):owner.second(token,seed);java.math.BigInteger want=java.math.BigInteger.valueOf(bias);want=mode==0?want.add(java.math.BigInteger.valueOf(seed)).add(java.math.BigInteger.valueOf(delta)):want.subtract(java.math.BigInteger.valueOf(seed)).subtract(java.math.BigInteger.valueOf(delta));Class<?> cls=value.getClass();if(value.get(delta)!=want.longValue()||value.token()!=token)throw new AssertionError("capture/generic-shadow/overflow/identity");kinds[mode]=cls;count++;}System.out.println(count+":values checked");for(int mode=0;mode<2;mode++){Class<?> cls=kinds[mode];if(!cls.isLocalClass()||cls.isAnonymousClass()||cls.getDeclaringClass()!=null||cls.getEnclosingClass()!=LocalCaptureOwner.class||!cls.getEnclosingMethod().getName().equals(mode==0?"first":"second")||!cls.getSimpleName().equals("Entry")||!cls.getName().equals("LocalCaptureOwner$"+(mode+1)+"Entry")||cls.getDeclaredConstructors().length!=1||cls.getDeclaredConstructors()[0].getParameterCount()!=3)throw new AssertionError("method scope/ordinal/constructor ABI");}System.out.println(count+":method-local:capture:generic-shadow:ordinal:overflow");}}`
			source = strings.ReplaceAll(source, "LocalCaptureOwner", root)
			testNativeIndependentFamilyFixture(t, source, []string{root}, "LocalMvpDriver", "500:values checked\n500:method-local:capture:generic-shadow:ordinal:overflow\n")
		})
	}
}

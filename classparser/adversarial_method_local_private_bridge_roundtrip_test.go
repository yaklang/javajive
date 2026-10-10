package javaclassparser

import (
	"regexp"
	"strings"
	"testing"
)

const nativeMethodLocalPrivateBridgeFixture = `interface OwnedLocalPrivateView{long eval(int width);Object packet();}
class OwnedLocalPrivateRoot{static String trace="";static final RuntimeException error=new RuntimeException("identity");private static class Value{final long word;private Value(long word){trace+="V";if(word==Long.MIN_VALUE)throw error;this.word=word;}long eval(int width){return word&((1L<<(width&63))-1L);}}OwnedLocalPrivateView make(long seed){class Entry implements OwnedLocalPrivateView{public long eval(int width){return new Value(seed).eval(width);}public Object packet(){return new Value(seed);}}return new Entry();}}
class OwnedLocalPrivateDriver{public static void main(String[]args){int rows=0;OwnedLocalPrivateRoot root=new OwnedLocalPrivateRoot();for(long seed:new long[]{-1,0,1,Long.MAX_VALUE,Long.MIN_VALUE}){OwnedLocalPrivateView view=root.make(seed);if(view.getClass().getEnclosingMethod().getDeclaringClass()!=OwnedLocalPrivateRoot.class||view.getClass().getDeclaringClass()!=null)throw new AssertionError("local identity");for(int width:new int[]{Integer.MIN_VALUE,-1,0,1,31,32,63,64,65,Integer.MAX_VALUE}){OwnedLocalPrivateRoot.trace="";try{long got=view.eval(width);long expected=java.math.BigInteger.valueOf(seed).and(java.math.BigInteger.ONE.shiftLeft(width&63).subtract(java.math.BigInteger.ONE)).longValue();if(seed==Long.MIN_VALUE||got!=expected||!OwnedLocalPrivateRoot.trace.equals("V"))throw new AssertionError("word/order");}catch(RuntimeException e){if(seed!=Long.MIN_VALUE||e!=OwnedLocalPrivateRoot.error||!OwnedLocalPrivateRoot.trace.equals("V"))throw new AssertionError("failure identity");}rows++;}if(seed!=Long.MIN_VALUE){Object packet=view.packet();if(packet.getClass().getDeclaringClass()!=OwnedLocalPrivateRoot.class)throw new AssertionError("private allocation identity");}}System.out.println(rows+":local:private:word:identity:effects:failure");}}
`

func TestAdversarialMethodLocalPrivateConstructorBridgeRoundTrip(t *testing.T) {
	for _, profile := range []string{"ordinary", "renamed", "static local", "generic capture"} {
		fixture := nativeMethodLocalPrivateBridgeFixture
		owner := "OwnedLocalPrivateRoot"
		driver := "OwnedLocalPrivateDriver"
		want := "50:local:private:word:identity:effects:failure\n"
		switch profile {
		case "renamed":
			fixture = strings.NewReplacer("OwnedLocalPrivateRoot", "IndependentCaptureScope", "OwnedLocalPrivateDriver", "IndependentCaptureDriver", "Entry", "Record", "seed", "salt").Replace(fixture)
			fixture = regexp.MustCompile(`\bValue\b`).ReplaceAllString(fixture, "Payload")
			owner = "IndependentCaptureScope"
			driver = "IndependentCaptureDriver"
		case "static local":
			fixture = strings.ReplaceAll(fixture, "OwnedLocalPrivateView make(long seed)", "static OwnedLocalPrivateView make(long seed)")
		case "generic capture":
			fixture = strings.ReplaceAll(fixture, "class OwnedLocalPrivateRoot{", "class OwnedLocalPrivateRoot<T>{")
			fixture = strings.ReplaceAll(fixture, "final long word;private Value(long word)", "final Object token;final long word;private Value(long word,Object token)")
			fixture = strings.ReplaceAll(fixture, "this.word=word;", "this.word=word;this.token=token;")
			fixture = strings.ReplaceAll(fixture, "OwnedLocalPrivateView make(long seed)", "OwnedLocalPrivateView make(T token,long seed)")
			fixture = strings.ReplaceAll(fixture, "new Value(seed)", "new Value(seed,token)")
			fixture = strings.ReplaceAll(fixture, "main(String[]args){", "main(String[]args)throws Exception{")
			fixture = strings.ReplaceAll(fixture, "for(long seed:new long[]", "for(Object token:new Object[]{null,new Object()})for(long seed:new long[]")
			fixture = strings.ReplaceAll(fixture, "root.make(seed)", "root.make(token,seed)")
			fixture = strings.ReplaceAll(fixture, "if(packet.getClass().getDeclaringClass()", "java.lang.reflect.Field field=packet.getClass().getDeclaredField(\"token\");field.setAccessible(true);if(field.get(packet)!=token)throw new AssertionError(\"captured generic argument identity\");if(packet.getClass().getDeclaringClass()")
			want = "100:local:private:word:identity:effects:failure\n"
		}
		t.Run(profile, func(t *testing.T) {
			testNativeIndependentFamilyFixture(t, fixture, []string{owner}, driver, want, nativeLexicalExactSignatures)
		})
	}
}

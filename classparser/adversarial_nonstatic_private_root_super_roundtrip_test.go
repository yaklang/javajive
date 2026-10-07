package javaclassparser

import (
	"strings"
	"testing"
)

const nativeNonstaticPrivateRootSuperFixture = `class RootSuperEffects{static String trace="";static int fail;static Object published;static final RuntimeException error=new RuntimeException("identity");static long arg(long n){trace+="A";if(fail==1)throw error;return n;}}
class CapturedRootSuper{final long word;final Object token;final Object observed;private CapturedRootSuper(long word,Object token){RootSuperEffects.trace+="P";this.word=word;this.token=token;observed=observe();RootSuperEffects.published=this;if(RootSuperEffects.fail==2)throw RootSuperEffects.error;}private CapturedRootSuper(Long word,Object token){throw new AssertionError("wrong boxed target");}Object observe(){return token;}static CapturedRootSuper outer(Object token){return new CapturedRootSuper(0L,token);}final class Member extends CapturedRootSuper{Member(long word,Object argument){super(RootSuperEffects.arg(word),argument);RootSuperEffects.trace+="M";}Object observe(){return CapturedRootSuper.this.token;}Object outer(){return CapturedRootSuper.this;}}}
class CapturedRootSuperDriver{public static void main(String[]args){int rows=0;Object token=new Object();for(Object outside:new Object[]{null,token})for(Object argument:new Object[]{null,token})for(long word:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){RootSuperEffects.fail=0;CapturedRootSuper outer=CapturedRootSuper.outer(outside);RootSuperEffects.trace="";RootSuperEffects.published=null;CapturedRootSuper.Member member=outer.new Member(word,argument);if(member.word!=word||member.token!=argument||member.observed!=outside||member.outer()!=outer||RootSuperEffects.published!=member||!RootSuperEffects.trace.equals("APM")||member.getClass().getDeclaringClass()!=CapturedRootSuper.class)throw new AssertionError("wide/identity/binding/pre-super");for(int width:new int[]{Integer.MIN_VALUE,-1,0,1,31,63,64,Integer.MAX_VALUE}){long expected=java.math.BigInteger.valueOf(word).and(java.math.BigInteger.ONE.shiftLeft(width&63).subtract(java.math.BigInteger.ONE)).longValue();if((member.word&((1L<<(width&63))-1L))!=expected)throw new AssertionError("word oracle");rows++;}for(int f=1;f<=2;f++){RootSuperEffects.trace="";RootSuperEffects.published=null;RootSuperEffects.fail=f;try{outer.new Member(word,argument);throw new AssertionError("missing failure");}catch(RuntimeException e){if(e!=RootSuperEffects.error||!RootSuperEffects.trace.equals(f==1?"A":"AP"))throw new AssertionError("failure order/identity");if(f==1&&RootSuperEffects.published!=null)throw new AssertionError("early publication");if(f==2){CapturedRootSuper.Member failed=(CapturedRootSuper.Member)RootSuperEffects.published;if(failed.word!=word||failed.token!=argument||failed.observed!=outside||failed.outer()!=outer)throw new AssertionError("partial capture state");}}}RootSuperEffects.fail=0;}System.out.println(rows+":nonstatic:private-root:wide:callback:identity:failure");}}
`

func TestAdversarialNonstaticPrivateRootSuperRoundTrip(t *testing.T) {
	for _, profile := range []string{"ordinary", "renamed", "generic", "THIS chain", "deep owner", "static parent"} {
		fixture := nativeNonstaticPrivateRootSuperFixture
		owner := "CapturedRootSuper"
		driver := "CapturedRootSuperDriver"
		switch profile {
		case "generic":
			fixture = strings.ReplaceAll(fixture, "class CapturedRootSuper{", "class CapturedRootSuper<T>{")
			fixture = strings.ReplaceAll(fixture, "final Object token;", "final T token;")
			fixture = strings.ReplaceAll(fixture, "CapturedRootSuper(long word,Object token)", "CapturedRootSuper(long word,T token)")
			fixture = strings.ReplaceAll(fixture, "CapturedRootSuper(Long word,Object token)", "CapturedRootSuper(Long word,T token)")
			fixture = strings.ReplaceAll(fixture, "static CapturedRootSuper outer(Object token){return new CapturedRootSuper(0L,token);}", "static <V> CapturedRootSuper<V> outer(V token){return new CapturedRootSuper<V>(0L,token);}")
			fixture = strings.ReplaceAll(fixture, "Member extends CapturedRootSuper{Member(long word,Object argument)", "Member extends CapturedRootSuper<T>{Member(long word,T argument)")
		case "THIS chain":
			fixture = strings.ReplaceAll(fixture, "Member(long word,Object argument){super(", "Member(long word,Object argument){this(word,argument,true);}private Member(long word,Object argument,boolean ignored){super(")
		case "deep owner":
			fixture = strings.ReplaceAll(fixture, "final class Member extends", "class Layer{final class Member extends")
			fixture = strings.ReplaceAll(fixture, "return CapturedRootSuper.this;}}}", "return CapturedRootSuper.this;}}}}")
			fixture = strings.ReplaceAll(fixture, "CapturedRootSuper.Member", "CapturedRootSuper.Layer.Member")
			fixture = strings.ReplaceAll(fixture, "outer.new Member(", "outer.new Layer().new Member(")
			fixture = strings.ReplaceAll(fixture, "getDeclaringClass()!=CapturedRootSuper.class", "getDeclaringClass()!=CapturedRootSuper.Layer.class")
		case "static parent":
			parent := `static class Parent{final long word;final Object token;final Object observed;private Parent(long word,Object token){RootSuperEffects.trace+="P";this.word=word;this.token=token;observed=observe();RootSuperEffects.published=this;if(RootSuperEffects.fail==2)throw RootSuperEffects.error;}private Parent(Long word,Object token){throw new AssertionError("wrong boxed target");}Object observe(){return token;}}`
			fixture = strings.ReplaceAll(fixture, "final class Member extends CapturedRootSuper", parent+"final class Member extends Parent")
		}
		if profile == "renamed" {
			fixture = strings.NewReplacer("CapturedRootSuperDriver", "IndependentDelegationDriver", "CapturedRootSuper", "IndependentDelegationOwner", "Member", "Child", "word", "salt", "argument", "request").Replace(fixture)
			owner = "IndependentDelegationOwner"
			driver = "IndependentDelegationDriver"
		}
		t.Run(profile, func(t *testing.T) {
			testNativeIndependentFamilyFixture(t, fixture, []string{owner}, driver, "160:nonstatic:private-root:wide:callback:identity:failure\n", nativeLexicalExactSignatures)
		})
	}
}

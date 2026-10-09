package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialIndependentStaticRootRetainsEarlyNamedCapture(t *testing.T) {
	fixtures := []struct{ name, source, want string }{
		{"integer words", `abstract class ScopeParent{final int observed;ScopeParent(){observed=read();}abstract int read();}class ScopeOwner{static class Capsule{final int base;Capsule(int base){this.base=base;}final class Worker extends ScopeParent{final int captured;Worker(int captured){this.captured=captured;}int read(){return Capsule.this.base;}int result(){return Capsule.this.base+captured;}}ScopeParent make(int captured){return new Worker(captured);}}static Runnable unrelated(){return new Runnable(){public void run(){}};}}
class ScopeDriver{public static void main(String[]args){int rows=0;for(int base:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(int captured:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){ScopeOwner.Capsule owner=new ScopeOwner.Capsule(base);ScopeOwner.Capsule.Worker value;try{value=(ScopeOwner.Capsule.Worker)owner.make(captured);}catch(NullPointerException e){throw new AssertionError("early static-root capture timing",e);}if(value.getClass().getDeclaringClass()!=owner.getClass()||value.observed!=base||value.read()!=base||value.captured!=captured||value.result()!=java.math.BigInteger.valueOf(base).add(java.math.BigInteger.valueOf(captured)).intValue())throw new AssertionError("static boundary original early outer capture");Runnable spare=ScopeOwner.unrelated();if(spare.getClass().getDeclaredConstructors().length!=1||spare.getClass().getDeclaredConstructors()[0].getModifiers()!=0)throw new AssertionError("original default constructor access");spare.run();rows++;}System.out.println(rows+":static:integer:root");}}`, "25:static:integer:root\n"},
		{"wide words", `abstract class ScopeParent{final long observed;ScopeParent(){observed=read();}abstract long read();}class ScopeOwner{public static final class Capsule{final long left;final double right;Capsule(long left,double right){this.left=left;this.right=right;}final class Worker extends ScopeParent{final long captured;Worker(long captured){this.captured=captured;}long read(){return Capsule.this.left^Double.doubleToRawLongBits(Capsule.this.right);}long result(){return read()^captured;}}ScopeParent make(long captured){return new Worker(captured);}}static Runnable unrelated(){return new Runnable(){public void run(){}};}}
class ScopeDriver{public static void main(String[]args){int rows=0;for(long left:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(long bits:new long[]{0L,Long.MIN_VALUE,0x7ff0000000000000L,0x7ff8000000000042L,0xfff0000000000000L}){ScopeOwner.Capsule owner=new ScopeOwner.Capsule(left,Double.longBitsToDouble(bits));ScopeOwner.Capsule.Worker value;try{value=(ScopeOwner.Capsule.Worker)owner.make(~left);}catch(NullPointerException e){throw new AssertionError("early static-root capture timing",e);}if(value.getClass().getDeclaringClass()!=owner.getClass()||value.observed!=(left^bits)||value.read()!=(left^bits)||value.captured!=~left||value.result()!=((left^bits)^~left))throw new AssertionError("static root category-2 capture words");Runnable spare=ScopeOwner.unrelated();if(spare.getClass().getDeclaredConstructors().length!=1||spare.getClass().getDeclaredConstructors()[0].getModifiers()!=0)throw new AssertionError("original default constructor access");spare.run();rows++;}System.out.println(rows+":static:wide:root");}}`, "25:static:wide:root\n"},
		{"closed generic identities", `abstract class ScopeParent<T>{final T observed;ScopeParent(){observed=read();}abstract T read();}class ScopeOwner<U>{static class Capsule<T>{final T token;Capsule(T token){this.token=token;}final class Worker extends ScopeParent<T>{final T captured;Worker(T captured){this.captured=captured;}T read(){return Capsule.this.token;}T saved(){return captured;}}ScopeParent<T> make(T captured){return new Worker(captured);}}static Runnable unrelated(){return new Runnable(){public void run(){}};}}
class ScopeDriver{public static void main(String[]args){int rows=0;String[] shared=new String[]{"same"};for(String[]token:new String[][]{null,shared,new String[0],new String[]{"same"}})for(String[]captured:new String[][]{null,shared,new String[0],new String[]{"same"}}){ScopeOwner.Capsule<String[]>owner=new ScopeOwner.Capsule<String[]>(token);ScopeOwner.Capsule<String[]>.Worker value;try{value=(ScopeOwner.Capsule<String[]>.Worker)owner.make(captured);}catch(NullPointerException e){throw new AssertionError("early static-root capture timing",e);}if(value.getClass().getDeclaringClass()!=owner.getClass()||value.observed!=token||value.read()!=token||value.saved()!=captured)throw new AssertionError("closed own formal and equal-type identity");Runnable spare=ScopeOwner.unrelated();if(spare.getClass().getDeclaredConstructors().length!=1||spare.getClass().getDeclaredConstructors()[0].getModifiers()!=0)throw new AssertionError("original default constructor access");spare.run();rows++;}System.out.println(rows+":static:generic:root");}}`, "16:static:generic:root\n"},
		{"nested static boundaries", `abstract class ScopeParent{final Object seen;ScopeParent(){seen=read();}abstract Object read();}class ScopeOwner{static class Capsule{static final class TokenBox{final Object token;TokenBox(Object token){this.token=token;}final class Leaf extends ScopeParent{Object read(){return TokenBox.this.token;}}ScopeParent make(){return new Leaf();}}ScopeParent make(Object token){return new TokenBox(token).make();}}static Runnable unrelated(){return new Runnable(){public void run(){}};}}
class ScopeDriver{public static void main(String[]args){int rows=0;Object same=new Object();for(Object token:new Object[]{null,same,"same",new String("same")}){ScopeOwner.Capsule owner=new ScopeOwner.Capsule();ScopeParent p;try{p=owner.make(token);}catch(NullPointerException e){throw new AssertionError("early static-root capture timing",e);}if(p.seen!=token||p.read()!=token||p.getClass().getDeclaringClass()!=ScopeOwner.Capsule.TokenBox.class)throw new AssertionError("nested static cut instance identity");Runnable spare=ScopeOwner.unrelated();if(spare.getClass().getDeclaredConstructors().length!=1||spare.getClass().getDeclaredConstructors()[0].getModifiers()!=0)throw new AssertionError("original default constructor access");spare.run();rows++;}System.out.println(rows+":static:nested:root");}}`, "4:static:nested:root\n"},
		{"static interface namespace", `abstract class ScopeParent{final Object seen;ScopeParent(){seen=read();}abstract Object read();}class ScopeOwner{interface Capsule{class TokenBox{final Object token;TokenBox(Object token){this.token=token;}final class Leaf extends ScopeParent{Object read(){return TokenBox.this.token;}}ScopeParent make(){return new Leaf();}}}static Runnable unrelated(){return new Runnable(){public void run(){}};}}
class ScopeDriver{public static void main(String[]args){int rows=0;Object same=new Object();for(Object token:new Object[]{null,same,"same",new String("same")}){ScopeOwner.Capsule.TokenBox owner=new ScopeOwner.Capsule.TokenBox(token);ScopeParent p;try{p=owner.make();}catch(NullPointerException e){throw new AssertionError("early static-root capture timing",e);}if(p.seen!=token||p.read()!=token||p.getClass().getDeclaringClass()!=ScopeOwner.Capsule.TokenBox.class)throw new AssertionError("interface namespace and static instance identity");Runnable spare=ScopeOwner.unrelated();if(spare.getClass().getDeclaredConstructors().length!=1||spare.getClass().getDeclaredConstructors()[0].getModifiers()!=0)throw new AssertionError("original default constructor access");spare.run();rows++;}System.out.println(rows+":static:interface:root");}}`, "4:static:interface:root\n"},
	}
	for _, root := range []string{"ScopeOwner", "RenamedStaticBoundary"} {
		for _, level := range []string{"7", "8"} {
			for _, fixture := range fixtures {
				t.Run(root+"/source"+level+"/"+fixture.name, func(t *testing.T) {
					source := strings.ReplaceAll(fixture.source, "ScopeOwner", root)
					testNativePrivateSetterCompiledFixtureWithShape(t, root, "ScopeDriver", fixture.want, func(t *testing.T, debug string) map[string][]byte {
						return nativeCompileIndependentRootFixture(t, root, source, debug, level)
					}, nativeStaticIndependentRootRepresentationShape(root))
				})
			}
		}
	}
}

// Only already-independent roots/outer anonymous tails use the established
// flattened representation contract. Every regenerated nonstatic child keeps
// exact physical capture/constructor ABI and original named ownership.
func nativeStaticIndependentRootRepresentationShape(root string) func(*testing.T, string, []byte, []byte) {
	return func(t *testing.T, name string, original, rebuilt []byte) {
		t.Helper()
		old, err := Parse(original)
		if err != nil {
			t.Fatal(err)
		}
		if old.GetClassName() == root+"$Capsule" || old.GetClassName() == root+"$1" {
			nativeAnonymousPrefixRepresentationShape(t, name, original, rebuilt)
			return
		}
		if a, b := nativeBinaryShape(t, original), nativeBinaryShape(t, rebuilt); a != b {
			t.Fatalf("exact native child ABI %s\n%s\n%s", name, a, b)
		}
		if a, b := nativeAnonymousAccessorShape(t, original), nativeAnonymousAccessorShape(t, rebuilt); a != b {
			t.Fatalf("exact native child ownership %s\n%s\n%s", name, a, b)
		}
	}
}

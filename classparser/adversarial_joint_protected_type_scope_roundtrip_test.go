package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

func jointProtectedTypeScopeSources() map[string]string {
	return map[string]string{
		"access/base/ScopeBase.java":   `package access.base;public class ScopeBase {public static String trace="";public static boolean fail;public static final RuntimeException error=new RuntimeException("identity");public final Object token;public ScopeBase(Object x){token=x;}public static abstract class Parent {public final Object seen;protected Parent(){trace+="P";seen=observe();if(fail)throw error;}public abstract Object observe();}public class Capture extends Parent {public Object observe(){return ScopeBase.this.token;}}public Capture make(){return new Capture();}protected static class Item {protected final Object value;public Item(Object x){trace+="I";value=x;}}public access.use.ScopeDerived.Child peer;}`,
		"access/use/ScopeDerived.java": `package access.use;public class ScopeDerived extends access.base.ScopeBase {public ScopeDerived(Object x){super(x);}public static class Child extends Item {public Child(Object x){super(x);}public Object read(){return value;}}}`,
		"access/use/ScopeDriver.java":  `package access.use;public class ScopeDriver {public static void main(String[]args)throws Exception{Object token=new Object();int rows=0;for(Object x:new Object[]{null,token})for(boolean abrupt:new boolean[]{false,true}){access.base.ScopeBase owner=new access.base.ScopeBase(x);access.base.ScopeBase.trace="";access.base.ScopeBase.fail=abrupt;try{access.base.ScopeBase.Capture c=owner.make();if(abrupt||c.seen!=x||c.getClass().getDeclaringClass()!=access.base.ScopeBase.class)throw new AssertionError("callback capture");}catch(RuntimeException e){if(!abrupt||e!=access.base.ScopeBase.error)throw new AssertionError("failure identity",e);}if(!access.base.ScopeBase.trace.equals("P"))throw new AssertionError("callback order");ScopeDerived derived=new ScopeDerived(x);ScopeDerived.Child c=new ScopeDerived.Child(x);owner.peer=c;if(c.read()!=x||owner.peer!=c||derived.token!=x||c.getClass().getDeclaringClass()!=ScopeDerived.class||c.getClass().getSuperclass().getDeclaringClass()!=access.base.ScopeBase.class||!access.base.ScopeBase.class.getField("peer").getType().equals(c.getClass()))throw new AssertionError("protected lexical declaration identity");if(!access.base.ScopeBase.trace.equals("PI"))throw new AssertionError("constructor order");rows++;}System.out.println(rows+":joint-protected:lexical:identity:callback:failure");}}`}
}
func TestAdversarialJointProtectedTypeScopeRoundTrip(t *testing.T) {
	for _, renamed := range []bool{false, true} {
		for _, deep := range []bool{false, true} {
			t.Run(fmt.Sprintf("renamed%v/deep%v", renamed, deep), func(t *testing.T) {
				sources := jointProtectedTypeScopeSources()
				if deep {
					sources["access/use/ScopeDerived.java"] = strings.Replace(sources["access/use/ScopeDerived.java"], "public static class Child", "public static class Box {public static class Child", 1) + "}"
					for n, text := range sources {
						sources[n] = strings.ReplaceAll(text, "ScopeDerived.Child", "ScopeDerived.Box.Child")
					}
					sources["access/use/ScopeDriver.java"] = strings.Replace(sources["access/use/ScopeDriver.java"], "getDeclaringClass()!=ScopeDerived.class", "getDeclaringClass()!=ScopeDerived.Box.class||ScopeDerived.Box.class.getDeclaringClass()!=ScopeDerived.class", 1)
				}
				owners := []string{"access/base/ScopeBase", "access/use/ScopeDerived"}
				driver := "access.use.ScopeDriver"
				if renamed {
					next := map[string]string{}
					for n, text := range sources {
						n = strings.ReplaceAll(n, "ScopeBase", "OriginalParent")
						n = strings.ReplaceAll(n, "ScopeDerived", "LexicalSubclass")
						text = strings.ReplaceAll(text, "ScopeBase", "OriginalParent")
						text = strings.ReplaceAll(text, "ScopeDerived", "LexicalSubclass")
						next[n] = text
					}
					sources = next
					owners = []string{"access/base/OriginalParent", "access/use/LexicalSubclass"}
				}
				testNativeIndependentCompiledFamilyFixture(t, func(debug string) map[string][]byte { return nativeCompileSourceReleaseClasses(t, sources, debug, "8") }, owners, driver, "4:joint-protected:lexical:identity:callback:failure\n", nil, nativeLexicalExactSignatures)
			})
		}
	}
}

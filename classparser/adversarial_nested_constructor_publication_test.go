package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// A constructor obligation belongs to the anonymous allocation's lexical
// scope. Transporting it through enclosing named declarations must preserve
// the exact packet without requiring the owner to use a lambda protocol.
func TestAdversarialNestedConstructorPublicationKeepsLexicalCapture(t *testing.T) {
	const fixture = `class ForestEffects {static Object value;static Object owner;static long word;static String trace;}
class ForestOwner<T> {final T value;ForestOwner(T value){this.value=value;}class View {Runnable make(final long word){return new Runnable(){public void run(){ForestEffects.trace+="R";ForestEffects.value=value;ForestEffects.owner=ForestOwner.this;ForestEffects.word=word;}};}}View view(){return new View();}}
class ForestDriver {public static void main(String[]args)throws Exception{int rows=0;Object token=new Object();for(Object value:new Object[]{null,token})for(long word:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){ForestOwner<Object> root=new ForestOwner<Object>(value);ForestOwner<Object>.View view=root.view();Runnable action=view.make(word);java.lang.reflect.Constructor<?>ctor=action.getClass().getDeclaredConstructors()[0];Class<?>[]p=ctor.getParameterTypes();java.lang.reflect.Type[]g=ctor.getGenericParameterTypes();if(p.length!=2||g.length!=2||p[0]!=view.getClass()||p[1]!=long.class||g[0]!=p[0]||g[1]!=p[1])throw new AssertionError("constructor ports");ForestEffects.trace="";action.run();if(ForestEffects.owner!=root||ForestEffects.value!=value||ForestEffects.word!=word||!ForestEffects.trace.equals("R"))throw new AssertionError("lexical capture");rows++;}System.out.println(rows+":nested:constructor:owner:identity:wide");}}`
	for _, name := range []string{"ForestOwner", "RenamedForestOwner"} {
		for _, original := range []int{7, 8} {
			for _, target := range []int{8, 16} {
				t.Run(fmt.Sprintf("%s/original%d/target%d", name, original, target), func(t *testing.T) {
					roundTripConstructorBridgeArtifact(t, strings.ReplaceAll(fixture, "ForestOwner", name), name, "ForestDriver", "6:nested:constructor:owner:identity:wide\n", "none", target, original)
				})
			}
		}
	}
}

// The registration solver must interleave Reader before Scope.read while
// preserving an anonymous capture publication owned by the same Scope.
func TestAdversarialNestedRegistrationTransportsConstructorPublication(t *testing.T) {
	fixture := strings.Replace(nativeAccessorNestedLayoutFixture, `static String trace="";`, `static String trace="";static Object token;`, 1)
	fixture = strings.Replace(fixture, `Reader reader(){`, `Runnable action(final Object token){return new Runnable(){public void run(){NestedLayoutEffects.token=token;}};}Reader reader(){`, 1)
	fixture = strings.Replace(fixture, `rows++;`, `Runnable action=scope.action(right);action.run();java.lang.reflect.Constructor<?>ctor=action.getClass().getDeclaredConstructors()[0];Class<?>[]physical=ctor.getParameterTypes();java.lang.reflect.Type[]generic=ctor.getGenericParameterTypes();if(NestedLayoutEffects.token!=right||physical.length!=2||generic.length!=2||generic[0]!=physical[0]||generic[1]!=physical[1])throw new AssertionError("reordered scope capture metadata");rows++;`, 1)
	for _, owner := range []string{"NestedLayoutOwner", "RenamedLayoutOwner"} {
		for _, original := range []int{7, 8} {
			t.Run(fmt.Sprintf("%s/original%d", owner, original), func(t *testing.T) {
				roundTripConstructorBridgeArtifact(t, strings.ReplaceAll(fixture, "NestedLayoutOwner", owner), owner, "NestedLayoutDriver", "12:nested:ordinals:identity:LRFS\n", "none", 8, original)
			})
		}
	}
}

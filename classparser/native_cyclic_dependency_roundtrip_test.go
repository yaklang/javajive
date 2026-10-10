package javaclassparser

import (
	"strings"
	"testing"
)

// Both foreign member-type edges are actual reflected field declarations.
// They cannot be removed as unused CP entries or assigned to one private nest.
// A correct extension needs a transaction that completes both source families.
const nativeCyclicDependencyFixture = `class CycleScopeA{static class Value{final Object token;CycleScopeB.Value peer;Value(Object token){this.token=token;}}Value make(Object token){return new Value(token);}}
class CycleScopeB{static class Value{final Object token;CycleScopeA.Value peer;Value(Object token){this.token=token;}}Value make(Object token){return new Value(token);}}
class CyclicDependencyDriver{public static void main(String[]args)throws Exception{int rows=0;for(Object a:new Object[]{null,new Object()})for(Object b:new Object[]{null,a}){CycleScopeA.Value left=new CycleScopeA().make(a);CycleScopeB.Value right=new CycleScopeB().make(b);left.peer=right;right.peer=left;if(left.token!=a||right.token!=b||left.peer.peer!=left||right.peer.peer!=right||left.getClass().getDeclaringClass()!=CycleScopeA.class||right.getClass().getDeclaringClass()!=CycleScopeB.class||left.getClass().getDeclaredField("peer").getType()!=CycleScopeB.Value.class||right.getClass().getDeclaredField("peer").getType()!=CycleScopeA.Value.class)throw new AssertionError("cyclic declaration identities/independent owners");rows++;}System.out.println(rows+":cyclic:declarations:independent:owners");}}`

func TestNativeCyclicIndependentDeclarationFamiliesRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, nativeCyclicDependencyFixture, []string{"CycleScopeA", "CycleScopeB"}, "CyclicDependencyDriver", "4:cyclic:declarations:independent:owners\n", nativeLexicalExactSignatures)
}

func TestNativeCyclicIndependentDeclarationFamiliesRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeCyclicDependencyFixture, "CycleScopeA", "LeftBindingOwner")
	f = strings.ReplaceAll(f, "CycleScopeB", "RightBindingOwner")
	f = strings.ReplaceAll(f, "Value", "Cursor")
	testNativeIndependentFamilyFixture(t, f, []string{"LeftBindingOwner", "RightBindingOwner"}, "CyclicDependencyDriver", "4:cyclic:declarations:independent:owners\n", nativeLexicalExactSignatures)
}

func TestNativeCyclicIndependentPrivateScopesRoundTrip(t *testing.T) {
	f := strings.Replace(nativeCyclicDependencyFixture, "class CycleScopeA{", "class CycleScopeA{private static Object secret(Object token){return token;}", 1)
	f = strings.Replace(f, "class CycleScopeB{", "class CycleScopeB{private static Object secret(Object token){return token;}", 1)
	f = strings.Replace(f, "this.token=token;", "this.token=CycleScopeA.secret(token);", 1)
	f = strings.Replace(f, "this.token=token;", "this.token=CycleScopeB.secret(token);", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"CycleScopeA", "CycleScopeB"}, "CyclicDependencyDriver", "4:cyclic:declarations:independent:owners\n", nativeLexicalExactSignatures)
}

func TestNativeCyclicIndependentGenericScopesRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeCyclicDependencyFixture, "static class Value{final Object token;", "static class Value<T>{final T token;")
	f = strings.ReplaceAll(f, "CycleScopeA.Value peer", "CycleScopeA.Value<T> peer")
	f = strings.ReplaceAll(f, "CycleScopeB.Value peer", "CycleScopeB.Value<T> peer")
	f = strings.ReplaceAll(f, "Value(Object token)", "Value(T token)")
	f = strings.ReplaceAll(f, "Value make(Object token){return new Value(token);}", "<T>Value<T> make(T token){return new Value<T>(token);}")
	testNativeIndependentFamilyFixture(t, f, []string{"CycleScopeA", "CycleScopeB"}, "CyclicDependencyDriver", "4:cyclic:declarations:independent:owners\n", nativeLexicalExactSignatures)
}

func TestNativeCyclicCondensationDependencyRoundTrip(t *testing.T) {
	f := strings.Replace(nativeCyclicDependencyFixture, "class CyclicDependencyDriver{", "class CycleClient{static class Item{CycleScopeA.Value identity(CycleScopeA.Value value){return value;}}}class CyclicDependencyDriver{", 1)
	f = strings.Replace(f, "left.peer=right;", "if(new CycleClient.Item().identity(left)!=left||CycleClient.Item.class.getDeclaringClass()!=CycleClient.class)throw new AssertionError(\"condensation dependency binding\");left.peer=right;", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"CycleScopeA", "CycleScopeB", "CycleClient"}, "CyclicDependencyDriver", "4:cyclic:declarations:independent:owners\n", nativeLexicalExactSignatures)
}

const nativeThreeCyclicDependencyFixture = `class StageOne{static int initialized=ThreeCycleDriver.mark("A");static class Node{final Object token;StageTwo.Node next;Node(Object token){this.token=token;}}Node make(Object token){return new Node(token);}}
class StageTwo{static int initialized=ThreeCycleDriver.mark("B");static class Node{final Object token;StageThree.Node next;Node(Object token){this.token=token;}}Node make(Object token){return new Node(token);}}
class StageThree{static int initialized=ThreeCycleDriver.mark("C");static class Node{final Object token;StageOne.Node next;Node(Object token){this.token=token;}}Node make(Object token){return new Node(token);}}
class ThreeCycleDriver{static String events="";static int mark(String value){events+=value;return 1;}public static void main(String[]args)throws Exception{int rows=0;for(Object a:new Object[]{null,new Object()})for(Object b:new Object[]{null,a}){StageOne.Node left=new StageOne().make(a);StageTwo.Node middle=new StageTwo().make(b);StageThree.Node right=new StageThree().make(b);left.next=middle;middle.next=right;right.next=left;if(left.token!=a||middle.token!=b||right.token!=b||left.next.next.next!=left||middle.next.next.next!=middle||right.next.next.next!=right||left.getClass().getDeclaringClass()!=StageOne.class||middle.getClass().getDeclaringClass()!=StageTwo.class||right.getClass().getDeclaringClass()!=StageThree.class||left.getClass().getDeclaredField("next").getType()!=StageTwo.Node.class||middle.getClass().getDeclaredField("next").getType()!=StageThree.Node.class||right.getClass().getDeclaredField("next").getType()!=StageOne.Node.class)throw new AssertionError("three independent original declarations");rows++;}if(!events.equals("ABC"))throw new AssertionError("source initialization order:"+events);System.out.println(rows+":three:independent:cycle:init:"+events);}}`

func TestNativeThreeCyclicDeclarationFamiliesInitializationRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, nativeThreeCyclicDependencyFixture, []string{"StageOne", "StageTwo", "StageThree"}, "ThreeCycleDriver", "4:three:independent:cycle:init:ABC\n", nativeLexicalExactSignatures)
}

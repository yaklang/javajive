package javaclassparser

import (
	"regexp"
	"strings"
	"testing"
)

const constructorEnclosingParameterFixture = `
class EnclosingEvents {
 static String trace="";static int fail;static Object actor,target;
 static final RuntimeException error=new IllegalArgumentException("original");
 static Object produce(Object value){trace+="V";if(fail==2)throw error;return value;}
}
class ActorParent {
 final Object observed;ActorParent(long n){EnclosingEvents.trace+="P";observed=owner();EnclosingEvents.actor=this;if(EnclosingEvents.fail==1)throw EnclosingEvents.error;}
 Object owner(){return null;}
}
class TargetParent {
 final Object observed;TargetParent(long n){EnclosingEvents.trace+="Q";observed=owner();EnclosingEvents.target=this;if(EnclosingEvents.fail==3)throw EnclosingEvents.error;}
 Object owner(){return null;}
}
class ConstructorScope {
 class Target extends TargetParent {
  final Object value;final long wide;
  Target(Object value,long wide){super(wide);EnclosingEvents.trace+="T";this.value=value;this.wide=wide;}
  Object owner(){return ConstructorScope.this;}
 }
 class Actor extends ActorParent {
  final Target result;
  Actor(Object value,long wide){super(wide);result=new Target(EnclosingEvents.produce(value),wide);EnclosingEvents.trace+="A";}
  Actor(long wide){this(null,wide);}
  Object owner(){return ConstructorScope.this;}
 }
}
class ConstructorEnclosingDriver {
 public static void main(String[]args)throws Exception {
  ConstructorScope owner=new ConstructorScope();Object token=new Object();int rows=0;
  java.lang.reflect.Constructor<?> direct=ConstructorScope.Actor.class.getDeclaredConstructor(ConstructorScope.class,Object.class,long.class);
  java.lang.reflect.Constructor<?> chained=ConstructorScope.Actor.class.getDeclaredConstructor(ConstructorScope.class,long.class);
  direct.setAccessible(true);chained.setAccessible(true);
  for(boolean nullOuter:new boolean[]{false,true})for(boolean delegated:new boolean[]{false,true})
   for(long wide:new long[]{Long.MIN_VALUE,0L,Long.MAX_VALUE})for(Object value:new Object[]{null,token})for(int fail:new int[]{0,1,2,3}) {
    Object enclosing=nullOuter?null:owner;Object expected=delegated?null:value;
    EnclosingEvents.trace="";EnclosingEvents.actor=null;EnclosingEvents.target=null;EnclosingEvents.fail=fail;
    try {
     ConstructorScope.Actor actor=(ConstructorScope.Actor)(delegated?chained.newInstance(enclosing,wide):direct.newInstance(enclosing,value,wide));
     ConstructorScope.Target target=actor.result;
     if(fail!=0||actor.owner()!=enclosing||actor.observed!=enclosing||target.owner()!=enclosing||target.observed!=enclosing||target.value!=expected||target.wide!=wide||EnclosingEvents.actor!=actor||EnclosingEvents.target!=target||!EnclosingEvents.trace.equals("PVQTA"))throw new AssertionError("enclosing identity/evaluation order");
    }catch(java.lang.reflect.InvocationTargetException failure){
     ConstructorScope.Actor actor=(ConstructorScope.Actor)EnclosingEvents.actor;
     String trace=fail==1?"P":fail==2?"PV":"PVQ";
     if(fail==0||failure.getCause()!=EnclosingEvents.error||actor==null||actor.owner()!=enclosing||actor.observed!=enclosing||actor.result!=null||!EnclosingEvents.trace.equals(trace))throw new AssertionError("actor publication/failure priority",failure);
     if(fail!=3&&EnclosingEvents.target!=null)throw new AssertionError("premature target");
     if(fail==3){ConstructorScope.Target target=(ConstructorScope.Target)EnclosingEvents.target;if(target==null||target.owner()!=enclosing||target.observed!=enclosing||target.value!=null||target.wide!=0L)throw new AssertionError("target capture/default fields");}
    }
    rows++;
   }
  System.out.println(rows+":constructor:lexical:identity:failure-order");
 }
}
`

// javac can reuse the unchanged enclosing constructor parameter after SUPER.
// That parameter is a nullable lexical identity, so inserting a qualified-new
// null check changes reflection, callback, publication and exception behavior.
func TestAdversarialConstructorSiblingAllocationUsesOriginalEnclosingParameter(t *testing.T) {
	testNativeIndependentFamilyFixture(t, constructorEnclosingParameterFixture, []string{"ConstructorScope"}, "ConstructorEnclosingDriver", "96:constructor:lexical:identity:failure-order\n", nativeLexicalExactSignatures)
}

func TestAdversarialConstructorSiblingAllocationEnclosingParameterRenamed(t *testing.T) {
	fixture := strings.ReplaceAll(constructorEnclosingParameterFixture, "ConstructorScope", "InstanceEnvironment")
	fixture = strings.ReplaceAll(fixture, "TargetParent", "ResultParent")
	fixture = regexp.MustCompile(`\bTarget\b`).ReplaceAllString(fixture, "ResultNode")
	testNativeIndependentFamilyFixture(t, fixture, []string{"InstanceEnvironment"}, "ConstructorEnclosingDriver", "96:constructor:lexical:identity:failure-order\n", nativeLexicalExactSignatures)
}

func TestAdversarialConstructorSiblingAllocationEnclosingParameterGeneric(t *testing.T) {
	fixture := strings.ReplaceAll(constructorEnclosingParameterFixture, "class ConstructorScope {", "class ConstructorScope<K> {")
	fixture = strings.ReplaceAll(fixture, "class Target extends", "class Target<U> extends")
	fixture = strings.ReplaceAll(fixture, "final Object value;final long wide;", "final U value;final long wide;")
	fixture = strings.ReplaceAll(fixture, "Target(Object value,long wide)", "Target(U value,long wide)")
	fixture = strings.ReplaceAll(fixture, "class Actor extends", "class Actor<Z> extends")
	fixture = strings.ReplaceAll(fixture, "final Target result;", "final Target<Object> result;")
	fixture = strings.ReplaceAll(fixture, "result=new Target(", "result=new Target<Object>(")
	testNativeIndependentFamilyFixture(t, fixture, []string{"ConstructorScope"}, "ConstructorEnclosingDriver", "96:constructor:lexical:identity:failure-order\n", nativeLexicalExactSignatures)
}

func TestAdversarialConstructorSiblingAllocationEnclosingParameterAfterLambda(t *testing.T) {
	fixture := strings.ReplaceAll(constructorEnclosingParameterFixture, "result=new Target(EnclosingEvents.produce(value),wide);", "java.util.function.Supplier<Object> producer=()->EnclosingEvents.produce(value);result=new Target(producer.get(),wide);")
	testNativeIndependentFamilyFixture(t, fixture, []string{"ConstructorScope"}, "ConstructorEnclosingDriver", "96:constructor:lexical:identity:failure-order\n", nativeLexicalExactSignatures)
}

func TestAdversarialConstructorSiblingAllocationEnclosingParameterKeepsOriginalTypeBinding(t *testing.T) {
	fixture := strings.ReplaceAll(constructorEnclosingParameterFixture, "class Actor extends ActorParent {", "class Actor extends ActorParent {class Target {Target(String unrelated){}}")
	fixture = strings.ReplaceAll(fixture, "final Target result;", "final ConstructorScope.Target result;")
	fixture = strings.ReplaceAll(fixture, "result=new Target(", "result=new ConstructorScope.Target(")
	testNativeIndependentFamilyFixture(t, fixture, []string{"ConstructorScope"}, "ConstructorEnclosingDriver", "96:constructor:lexical:identity:failure-order\n", nativeLexicalExactSignatures)
}

func TestAdversarialConstructorSiblingAllocationEnclosingParameterKeepsGenericTypeBinding(t *testing.T) {
	fixture := strings.ReplaceAll(constructorEnclosingParameterFixture, "class ConstructorScope {", "class ConstructorScope<K> {")
	fixture = strings.ReplaceAll(fixture, "class Target extends", "class Target<U> extends")
	fixture = strings.ReplaceAll(fixture, "final Object value;final long wide;", "final U value;final long wide;")
	fixture = strings.ReplaceAll(fixture, "Target(Object value,long wide)", "Target(U value,long wide)")
	fixture = strings.ReplaceAll(fixture, "class Actor extends ActorParent {", "class Actor<Z> extends ActorParent {class Target {Target(String unrelated){}}")
	fixture = strings.ReplaceAll(fixture, "final Target result;", "final ConstructorScope<K>.Target<Object> result;")
	fixture = strings.ReplaceAll(fixture, "result=new Target(", "result=new ConstructorScope<K>.Target<Object>(")
	testNativeIndependentFamilyFixture(t, fixture, []string{"ConstructorScope"}, "ConstructorEnclosingDriver", "96:constructor:lexical:identity:failure-order\n", nativeLexicalExactSignatures)
}

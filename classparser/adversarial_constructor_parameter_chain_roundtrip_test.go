package javaclassparser

import (
	"testing"
)

const constructorParameterChainFixture = `class ParameterChainEffects {static String trace="";static Object published;static boolean fail;static final RuntimeException error=new RuntimeException("identity");}
class ParameterChainOwner {
 final long rootWord;ParameterChainOwner(long word){rootWord=word;}
 abstract class Base {final Object observed;Base(){ParameterChainEffects.trace+="P";ParameterChainEffects.published=this;observed=origin();if(ParameterChainEffects.fail)throw ParameterChainEffects.error;}abstract Object origin();}
 class Layer {final Object token;Layer(Object token){this.token=token;}class Leaf extends Base {final long word;Leaf(long delta){super();word=ParameterChainOwner.this.rootWord+delta;ParameterChainEffects.trace+="C";}Object origin(){return Layer.this;}long evaluate(long delta){return ParameterChainOwner.this.rootWord+delta;}}Leaf make(long delta){return new Leaf(delta);}}
 Layer layer(Object token){return new Layer(token);}
}
class ParameterChainDriver {public static void main(String[]args)throws Exception{Object token=new Object();int rows=0;for(long seed:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(long delta:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(boolean fail:new boolean[]{false,true}){ParameterChainOwner root=new ParameterChainOwner(seed);ParameterChainOwner.Layer layer=root.layer(token);ParameterChainEffects.trace="";ParameterChainEffects.published=null;ParameterChainEffects.fail=fail;try{ParameterChainOwner.Layer.Leaf leaf=layer.make(delta);if(fail||leaf.observed!=layer||leaf.origin()!=layer||leaf.word!=java.math.BigInteger.valueOf(seed).add(java.math.BigInteger.valueOf(delta)).longValue()||leaf.evaluate(delta)!=leaf.word||leaf!=ParameterChainEffects.published||!ParameterChainEffects.trace.equals("PC"))throw new AssertionError("word/owner/effects");}catch(RuntimeException error){ParameterChainOwner.Layer.Leaf leaf=(ParameterChainOwner.Layer.Leaf)ParameterChainEffects.published;if(!fail||error!=ParameterChainEffects.error||leaf==null||leaf.observed!=layer||leaf.word!=0||!ParameterChainEffects.trace.equals("P"))throw new AssertionError("error/publication",error);}rows++;}
 ParameterChainEffects.fail=false;java.lang.reflect.Constructor<?>ctor=ParameterChainOwner.Layer.Leaf.class.getDeclaredConstructor(ParameterChainOwner.Layer.class,long.class);ctor.setAccessible(true);ParameterChainEffects.trace="";ParameterChainEffects.published=null;try{ctor.newInstance(null,5L);throw new AssertionError("null enclosing");}catch(java.lang.reflect.InvocationTargetException e){if(!(e.getCause() instanceof NullPointerException)||ParameterChainEffects.published!=null||!ParameterChainEffects.trace.equals(""))throw new AssertionError("enclosing dereference ordering",e);}ParameterChainOwner.Layer detached=new ParameterChainOwner(9).layer(token);int changed=0;for(java.lang.reflect.Field f:detached.getClass().getDeclaredFields())if(f.getType()==ParameterChainOwner.class){f.setAccessible(true);f.set(detached,null);changed++;}if(changed!=1)throw new AssertionError("original capture field");ParameterChainEffects.trace="";ParameterChainEffects.published=null;try{detached.make(5L);throw new AssertionError("missing body NPE");}catch(NullPointerException expected){ParameterChainOwner.Layer.Leaf leaf=(ParameterChainOwner.Layer.Leaf)ParameterChainEffects.published;if(leaf==null||leaf.observed!=detached||leaf.word!=0||!ParameterChainEffects.trace.equals("P"))throw new AssertionError("body dereference stays after publication");}System.out.println(rows+":parameter-chain:word:identity:callback:failure");}}
`

// Original bytecode keeps slot 1 live after SUPER. No assignment may move
// across the callback, and the lexical root remains distinct from the parent's
// already-captured root when reflective clients clear an enclosing link.
func TestAdversarialConstructorBodyEnclosingParameterRoundTrip(t *testing.T) {
	testSourceTargetReleaseFamilyFixture(t, constructorParameterChainFixture, "ParameterChainOwner", "ParameterChainDriver", "50:parameter-chain:word:identity:callback:failure\n", "8", []int{8, 11, 16})
}
func TestAdversarialModernConstructorBodyEnclosingParameterRoundTrip(t *testing.T) {
	testSourceTargetReleaseFamilyFixture(t, constructorParameterChainFixture, "ParameterChainOwner", "ParameterChainDriver", "50:parameter-chain:word:identity:callback:failure\n", "11", []int{11, 16})
}

package javaclassparser

import "testing"

// The foreign parent's enclosing object and the subclass's SDK argument have
// independent proof obligations. The original driver observes both enclosures,
// producer-before-SUPER failures and a superclass callback before the body.
func TestAdversarialMemberConstructorSDKForeignSuperRoundTrip(t *testing.T) {
	fixture := `
class SDKSuperEffects {static String trace="";static Object parent,child;static boolean fail;static final RuntimeException failure=new IllegalStateException("super");}
class SDKSuperBean {public long field;}
class SDKSuperBase {final Object token;SDKSuperBase(Object t){token=t;}
 class Handler {final Class type;final boolean flag;Handler(Class t,boolean b){type=t;flag=b;SDKSuperEffects.parent=SDKSuperBase.this.token;SDKSuperEffects.child=capture();SDKSuperEffects.trace+="S";if(SDKSuperEffects.fail)throw SDKSuperEffects.failure;}Object capture(){return null;}}
}
public class SDKForeignOwner extends SDKSuperBase {SDKForeignOwner(Object t){super(t);}
 class Child extends Handler {final java.lang.reflect.Field field;Child(java.lang.reflect.Field f){super(f.getType(),f!=null);field=f;SDKSuperEffects.trace+="B";}Object capture(){return SDKForeignOwner.this.token;}}
 Child make(java.lang.reflect.Field f){return new Child(f);}
}
class SDKForeignDriver {public static void main(String[]args)throws Exception{Object token=new Object();SDKForeignOwner owner=new SDKForeignOwner(token);java.lang.reflect.Field f=SDKSuperBean.class.getField("field");
 SDKSuperEffects.trace="";SDKSuperEffects.fail=false;SDKForeignOwner.Child c=owner.make(f);if(c.field!=f||c.type!=long.class||!c.flag||SDKSuperEffects.parent!=token||SDKSuperEffects.child!=token||!SDKSuperEffects.trace.equals("SB"))throw new AssertionError("foreign SUPER/capture/binding/order");
 SDKSuperEffects.trace="";SDKSuperEffects.parent=null;SDKSuperEffects.child=null;try{owner.make(null);throw new AssertionError("lost SDK null");}catch(NullPointerException e){if(!SDKSuperEffects.trace.isEmpty()||SDKSuperEffects.parent!=null||SDKSuperEffects.child!=null)throw new AssertionError("eager SUPER");}
 SDKSuperEffects.trace="";SDKSuperEffects.fail=true;try{owner.make(f);throw new AssertionError("lost SUPER failure");}catch(RuntimeException e){if(e!=SDKSuperEffects.failure||SDKSuperEffects.parent!=token||SDKSuperEffects.child!=token||!SDKSuperEffects.trace.equals("S"))throw new AssertionError("failure identity/capture/order");}
 System.out.println("3:foreign:enclosing:SDK:callback:failure-order");}}
`
	testNativePrivateSetterSourceFixture(t, map[string]string{"SDKForeignOwner.java": fixture}, "SDKForeignOwner", "SDKForeignDriver", "3:foreign:enclosing:SDK:callback:failure-order\n")
}

func TestAdversarialMemberConstructorSDKLexicalSuperRoundTrip(t *testing.T) {
	fixture := `
class SDKLexicalEffects {static String trace="";static Object parent,child;}
class SDKLexicalBean {public long field;}
public class SDKLexicalOwner {final Object token;SDKLexicalOwner(Object t){token=t;}
 class Parent {final Class type;Parent(Class t){type=t;SDKLexicalEffects.parent=SDKLexicalOwner.this.token;SDKLexicalEffects.child=capture();SDKLexicalEffects.trace+="S";}Object capture(){return null;}}
 class Layer {final Object local;Layer(Object t){local=t;}
  class Child extends Parent {final java.lang.reflect.Field field;Child(java.lang.reflect.Field f){super(f.getType());field=f;SDKLexicalEffects.trace+="B";}Object capture(){return SDKLexicalOwner.Layer.this.local;}}
  Child make(java.lang.reflect.Field f){return new Child(f);}
 }
 Layer layer(Object token){return new Layer(token);}
}
class SDKLexicalDriver {public static void main(String[]args)throws Exception{Object outer=new Object(),inner=new Object();SDKLexicalOwner owner=new SDKLexicalOwner(outer);SDKLexicalOwner.Layer layer=owner.layer(inner);java.lang.reflect.Field f=SDKLexicalBean.class.getField("field");
 SDKLexicalEffects.trace="";SDKLexicalOwner.Layer.Child c=layer.make(f);if(c.type!=long.class||c.field!=f||SDKLexicalEffects.parent!=outer||SDKLexicalEffects.child!=inner||!SDKLexicalEffects.trace.equals("SB"))throw new AssertionError("distinct lexical captures/order");
 SDKLexicalEffects.trace="";SDKLexicalEffects.parent=null;SDKLexicalEffects.child=null;try{layer.make(null);throw new AssertionError("lost SDK null");}catch(NullPointerException e){if(!SDKLexicalEffects.trace.isEmpty()||SDKLexicalEffects.parent!=null||SDKLexicalEffects.child!=null)throw new AssertionError("eager lexical SUPER");}
 System.out.println("2:lexical:distinct-captures:SDK:callback:failure-order");}}
`
	testNativePrivateSetterSourceFixture(t, map[string]string{"SDKLexicalOwner.java": fixture}, "SDKLexicalOwner", "SDKLexicalDriver", "2:lexical:distinct-captures:SDK:callback:failure-order\n")
}

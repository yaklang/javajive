package javaclassparser

import "testing"

// Reads in the original delegation arguments must keep their identity, order
// and failures. Their receiver is external to the not-yet-initialized child.
func TestAdversarialConstructorCaptureFieldOperandsRoundTrip(t *testing.T) {
	roundTripGenericFlowUnitsClasspath(t, "FieldOperandDriver", `
class FieldOperandEffects {static int touched;}
class FieldOperandState {static final Object value=init();static Object init(){FieldOperandEffects.touched++;return new Object();}}
class FieldOperandFailureEffects {static int touched;}
class FieldOperandFailure {static final Object value=init();static Object init(){FieldOperandFailureEffects.touched++;throw new IllegalStateException("initializer");}}
class FieldOperandBox {volatile Object value;volatile long wide;FieldOperandBox(Object v,long w){value=v;wide=w;}}
class FieldOperandParent {final Object a,b;final long wide;FieldOperandParent(Object x,Object y,long w){a=x;b=y;wide=w;}Object capture(){return null;}}
class FieldOperandOwner {
 final Object token;FieldOperandOwner(Object t){token=t;}
 final class ReadFirst extends FieldOperandParent {ReadFirst(FieldOperandBox box){super(box.value,FieldOperandState.value,box.wide);}Object capture(){return FieldOperandOwner.this.token;}}
 final class StaticFirst extends FieldOperandParent {StaticFirst(FieldOperandBox box){super(FieldOperandState.value,box.value,box.wide);}Object capture(){return FieldOperandOwner.this.token;}}
 final class ReadBeforeFailure extends FieldOperandParent {ReadBeforeFailure(FieldOperandBox box){super(box.value,FieldOperandFailure.value,0);}}
 final class FailureBeforeRead extends FieldOperandParent {FailureBeforeRead(FieldOperandBox box){super(FieldOperandFailure.value,box.value,0);}}
 ReadBeforeFailure before(FieldOperandBox box){return new ReadBeforeFailure(box);}FailureBeforeRead after(FieldOperandBox box){return new FailureBeforeRead(box);}
 ReadFirst first(FieldOperandBox box){return new ReadFirst(box);}StaticFirst second(FieldOperandBox box){return new StaticFirst(box);}
}
class FieldOperandOracle {static void run(){Object token=new Object();Object value=new Object();FieldOperandOwner owner=new FieldOperandOwner(token);
 try{owner.first(null);throw new AssertionError("missing null failure");}catch(NullPointerException expected){if(FieldOperandEffects.touched!=0)throw new AssertionError("static read moved before null failure");System.out.println("null-before-static");}
 try{owner.second(null);throw new AssertionError("missing null failure");}catch(NullPointerException expected){if(FieldOperandEffects.touched!=1)throw new AssertionError("static read did not precede null failure");System.out.println("static-before-null");}
 try{owner.before(null);throw new AssertionError("missing read failure");}catch(NullPointerException expected){if(FieldOperandFailureEffects.touched!=0)throw new AssertionError("failed initializer ran too early");System.out.println("read-before-failed-init");}
 try{owner.after(null);throw new AssertionError("missing initializer failure");}catch(ExceptionInInitializerError expected){if(!(expected.getCause() instanceof IllegalStateException)||FieldOperandFailureEffects.touched!=1)throw new AssertionError("initializer failure priority");System.out.println("failed-init-before-read");}
 try{owner.before(null);throw new AssertionError("missing repeated read failure");}catch(NullPointerException expected){System.out.println("read-before-erroneous-class");}
 try{owner.after(null);throw new AssertionError("missing erroneous-class failure");}catch(NoClassDefFoundError expected){if(FieldOperandFailureEffects.touched!=1)throw new AssertionError("reinitialized erroneous class");System.out.println("erroneous-class-before-read");}
 for(Object v:new Object[]{null,value})for(long w:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){FieldOperandBox box=new FieldOperandBox(v,w);FieldOperandOwner.ReadFirst a=owner.first(box);FieldOperandOwner.StaticFirst b=owner.second(box);
 if(a.a!=v||a.b!=FieldOperandState.value||b.a!=FieldOperandState.value||b.b!=v||a.wide!=w||b.wide!=w||a.capture()!=token||b.capture()!=token||FieldOperandEffects.touched!=1)throw new AssertionError("field/capture identity or width");System.out.println((v==null)+":"+w);}
}}
public class FieldOperandDriver {public static void main(String[]args){FieldOperandOracle.run();}}
`, nil, []string{"FieldOperandOwner", "FieldOperandOwner$ReadFirst", "FieldOperandOwner$StaticFirst", "FieldOperandOwner$ReadBeforeFailure", "FieldOperandOwner$FailureBeforeRead"}, true, Precision, Compatibility, "legacy")
}

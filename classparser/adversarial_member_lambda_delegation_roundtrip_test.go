package javaclassparser

import "testing"

// A compiler-generated implementation is a bootstrap target, not an ordinary
// source static member declaration. Preserve its physical capture words and
// the source constructor's enclosing callback independently of its lazy body.
const memberLambdaDelegationFixture = `
class LambdaDelegateEvents {static String trace="";}
class LambdaDelegateBean {public long field;}
class LambdaDelegateParent {final Class type;final Object observed;LambdaDelegateParent(Class t){type=t;observed=owner();LambdaDelegateEvents.trace+="S";}Object owner(){return null;}}
public class LambdaDelegateOwner {final Object token;LambdaDelegateOwner(Object t){token=t;}
 class Child extends LambdaDelegateParent {Child(java.lang.reflect.Field f){super(f.getType());LambdaDelegateEvents.trace+="B";}
  Object owner(){return LambdaDelegateOwner.this.token;}
  java.util.function.Supplier<Object> self(final long n){return ()->n==Long.MIN_VALUE?this:null;}
  java.util.function.Supplier<Integer> zero(){return ()->42;}
  java.util.function.Function<String,Integer> mapping(final long n){return s->{LambdaDelegateEvents.trace+="M";return n==Long.MIN_VALUE?s.length():-1;};}
  java.util.function.Supplier<Class<?>> type(final java.lang.reflect.Field f,final long n){return ()->{LambdaDelegateEvents.trace+="L";return n==Long.MIN_VALUE?f.getType():null;};}
 }
 Child make(java.lang.reflect.Field f){return new Child(f);}
}
class LambdaDelegateDriver {public static void main(String[]args)throws Exception{Object token=new Object();LambdaDelegateOwner owner=new LambdaDelegateOwner(token);java.lang.reflect.Field f=LambdaDelegateBean.class.getField("field");int rows=0;
 for(long n:new long[]{Long.MIN_VALUE,-1,0,Long.MAX_VALUE}){LambdaDelegateEvents.trace="";LambdaDelegateOwner.Child child=owner.make(f);
  java.util.function.Supplier<Class<?>> lambda=child.type(f,n);if(child.observed!=token||child.type!=long.class||!LambdaDelegateEvents.trace.equals("SB"))throw new AssertionError("constructor capture and lazy lambda");
  Class<?> expected=n==Long.MIN_VALUE?long.class:null;if(lambda.get()!=expected||!LambdaDelegateEvents.trace.equals("SBL"))throw new AssertionError("original implementation/captured wide word");
  if(child.self(n).get()!=(n==Long.MIN_VALUE?child:null)||child.zero().get()!=42)throw new AssertionError("instance receiver or zero capture");
  java.util.function.Function<String,Integer> mapping=child.mapping(n);if(!LambdaDelegateEvents.trace.equals("SBL"))throw new AssertionError("eager SAM");
  if(mapping.apply("\u03a9\u00e9")!=(n==Long.MIN_VALUE?2:-1)||!LambdaDelegateEvents.trace.equals("SBLM"))throw new AssertionError("SAM parameter or wide capture");
  LambdaDelegateEvents.trace="";try{((java.util.function.Function)mapping).apply(42);throw new AssertionError("missing SAM entry cast");}catch(ClassCastException e){if(!LambdaDelegateEvents.trace.isEmpty())throw new AssertionError("body before cast");}rows++;
 }
 LambdaDelegateOwner.Child child=owner.make(f);LambdaDelegateEvents.trace="";java.util.function.Supplier<Class<?>> bad=child.type(null,Long.MIN_VALUE);if(!LambdaDelegateEvents.trace.isEmpty())throw new AssertionError("eager body");
 try{bad.get();throw new AssertionError("missing lazy null");}catch(NullPointerException e){if(!LambdaDelegateEvents.trace.equals("L"))throw new AssertionError("failure order");}
 LambdaDelegateEvents.trace="";try{owner.make(null);throw new AssertionError("missing constructor null");}catch(NullPointerException e){if(!LambdaDelegateEvents.trace.isEmpty())throw new AssertionError("eager SUPER");}
 System.out.println(rows+":member-lambda:static-implementation:wide-capture:lazy:SUPER-identity");}}
`

func TestAdversarialMemberConstructorLambdaImplementationsRoundTrip(t *testing.T) {
	testNativePrivateSetterSourceFixture(t, map[string]string{"LambdaDelegateOwner.java": memberLambdaDelegationFixture}, "LambdaDelegateOwner", "LambdaDelegateDriver", "4:member-lambda:static-implementation:wide-capture:lazy:SUPER-identity\n")
}

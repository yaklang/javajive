package javaclassparser

import "testing"

func TestAdversarialConstructorPrivateArrayReadRoundTrip(t *testing.T) {
	fixture := `
class PrivateReadEffects {static String trace="";static Object observed;static boolean fail;static final java.io.IOException failure=new java.io.IOException("element");
 static java.lang.reflect.Field element(java.lang.reflect.Field f)throws java.io.IOException{trace+="A";if(fail)throw failure;return f;}
 static java.lang.reflect.Field element(Object f){trace+="X";return (java.lang.reflect.Field)f;}
 static int index(int n){trace+="I";return n;}}
class PrivateReadBean {public long field;}
class PrivateReadParent {final Class type;PrivateReadParent(Class t){type=t;PrivateReadEffects.trace+="S";PrivateReadEffects.observed=capture();}Object capture(){return null;}}
public class PrivateReadOwner {final Object token;PrivateReadOwner(Object t){token=t;}
 class Child extends PrivateReadParent {Child(java.lang.reflect.Field f,int n,boolean choose)throws java.io.IOException{super(choose?new java.lang.reflect.Field[]{PrivateReadEffects.element(f)}[PrivateReadEffects.index(n)].getType():String.class);PrivateReadEffects.trace+="B";}Object capture(){return PrivateReadOwner.this.token;}}
 Child make(java.lang.reflect.Field f,int n,boolean choose)throws java.io.IOException{return new Child(f,n,choose);}}
class PrivateReadDriver {
 static void reset(boolean fail){PrivateReadEffects.trace="";PrivateReadEffects.observed=null;PrivateReadEffects.fail=fail;}
 public static void main(String[]args)throws Exception{Object token=new Object();PrivateReadOwner owner=new PrivateReadOwner(token);java.lang.reflect.Field field=PrivateReadBean.class.getField("field");
  reset(false);PrivateReadOwner.Child ok=owner.make(field,0,true);if(ok.type!=long.class||PrivateReadEffects.observed!=token||!PrivateReadEffects.trace.equals("AISB"))throw new AssertionError("read/capture/effects");
  for(int n:new int[]{-1,1,Integer.MIN_VALUE,Integer.MAX_VALUE}){reset(false);try{owner.make(field,n,true);throw new AssertionError("lost bounds");}catch(ArrayIndexOutOfBoundsException e){if(PrivateReadEffects.observed!=null||!PrivateReadEffects.trace.equals("AI"))throw new AssertionError("bounds order");}}
  reset(false);try{owner.make(null,0,true);throw new AssertionError("lost loaded-receiver NPE");}catch(NullPointerException e){if(PrivateReadEffects.observed!=null||!PrivateReadEffects.trace.equals("AI"))throw new AssertionError("read null order");}
  reset(true);try{owner.make(field,1,true);throw new AssertionError("lost element failure");}catch(java.io.IOException e){if(e!=PrivateReadEffects.failure||PrivateReadEffects.observed!=null||!PrivateReadEffects.trace.equals("A"))throw new AssertionError("element before index/bounds/super");}
  reset(true);PrivateReadOwner.Child lazy=owner.make(null,Integer.MAX_VALUE,false);if(lazy.type!=String.class||PrivateReadEffects.observed!=token||!PrivateReadEffects.trace.equals("SB"))throw new AssertionError("unselected allocation/element/index");
  System.out.println("8:private-array-read:lazy:checked:bounds:null:capture:order");}}
`
	testNativePrivateSetterSourceFixture(t, map[string]string{"PrivateReadOwner.java": fixture}, "PrivateReadOwner", "PrivateReadDriver", "8:private-array-read:lazy:checked:bounds:null:capture:order\n")
}

func TestAdversarialConstructorPrivateArrayReadNewElementHoldout(t *testing.T) {
	fixture := `
class NewReadEffects {static String trace="";static Object observed;static boolean fail;static final java.io.IOException failure=new java.io.IOException("new-element");}
class NewReadToken<T> {final T value;NewReadToken(T t)throws java.io.IOException{NewReadEffects.trace+="N";if(NewReadEffects.fail)throw NewReadEffects.failure;value=t;}T read(){NewReadEffects.trace+="R";return value;}}
class NewReadParent {final Object value;NewReadParent(Object v){NewReadEffects.trace+="O";value=v;NewReadEffects.observed=capture();}NewReadParent(String wrong){throw new AssertionError("wrong source overload");}Object capture(){return null;}}
public class NewReadOwner {final Object token;NewReadOwner(Object t){token=t;}
 class Child extends NewReadParent {Child(Object value,int index,boolean yes)throws java.io.IOException{super(yes?new NewReadToken[]{new NewReadToken<Object>(value)}[index].read():value);NewReadEffects.trace+="B";}Object capture(){return NewReadOwner.this.token;}}
 Child make(Object v,int n,boolean yes)throws java.io.IOException{return new Child(v,n,yes);}}
class NewReadDriver {
 static void reset(boolean fail){NewReadEffects.trace="";NewReadEffects.observed=null;NewReadEffects.fail=fail;}
 public static void main(String[]args)throws Exception{Object token=new Object(),payload=new Object();NewReadOwner owner=new NewReadOwner(token);
  for(Object value:new Object[]{null,payload,"text"}){reset(false);NewReadOwner.Child h=owner.make(value,0,true);if(h.value!=value||NewReadEffects.observed!=token||!NewReadEffects.trace.equals("NROB"))throw new AssertionError("NEW/read/erasure/overload/capture");}
  reset(false);try{owner.make(payload,1,true);throw new AssertionError("lost bounds");}catch(ArrayIndexOutOfBoundsException e){if(NewReadEffects.observed!=null||!NewReadEffects.trace.equals("N"))throw new AssertionError("bounds before read/super");}
  reset(true);try{owner.make(payload,-1,true);throw new AssertionError("lost new failure");}catch(java.io.IOException e){if(e!=NewReadEffects.failure||NewReadEffects.observed!=null||!NewReadEffects.trace.equals("N"))throw new AssertionError("allocation constructor before bounds");}
  reset(true);NewReadOwner.Child h=owner.make(payload,Integer.MAX_VALUE,false);if(h.value!=payload||NewReadEffects.observed!=token||!NewReadEffects.trace.equals("OB"))throw new AssertionError("lazy NEW/reads");
  System.out.println("6:private-array-read:NEW:raw-formal:identity:overload:lazy:checked");}}
`
	testNativePrivateSetterSourceFixture(t, map[string]string{"NewReadOwner.java": fixture}, "NewReadOwner", "NewReadDriver", "6:private-array-read:NEW:raw-formal:identity:overload:lazy:checked\n")
}

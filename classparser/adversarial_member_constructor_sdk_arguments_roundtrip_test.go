package javaclassparser

import "testing"

// An external producer in a superclass argument need not have a catalog entry
// to identify the original uninitialized THIS. The enclosing capture must be
// regenerated before that argument and before a superclass callback or failure.
// The driver and the observer/descriptor implementations remain original.
func TestAdversarialMemberConstructorSDKArgumentsRoundTrip(t *testing.T) {
	fixture := `
class PacketEffects {
 static String trace="";static Object observed;static boolean fail;
 static final RuntimeException failure=new IllegalStateException("super-failure");
 static final RuntimeException producerFailure=new IllegalArgumentException("producer-failure");
}

class PacketParent {
 final Class type;final boolean readable,writable;
 PacketParent(Class t,boolean r,boolean w){type=t;readable=r;writable=w;PacketEffects.trace+="S";PacketEffects.observed=capture();if(PacketEffects.fail)throw PacketEffects.failure;}
 Object capture(){return null;}
}
class PacketBean {public int getValue(){return 0;}public void setValue(int n){}public long field;}
class PacketDescriptor extends java.beans.PropertyDescriptor {
 boolean read,write,fail,armed;PacketDescriptor()throws Exception{super("value",PacketBean.class.getMethod("getValue"),PacketBean.class.getMethod("setValue",int.class));armed=true;}
 public Class getPropertyType(){if(!armed)return super.getPropertyType();PacketEffects.trace+="T";if(fail)throw PacketEffects.producerFailure;return int.class;}
 public java.lang.reflect.Method getReadMethod(){if(!armed)return super.getReadMethod();PacketEffects.trace+="R";return read?super.getReadMethod():null;}
 public java.lang.reflect.Method getWriteMethod(){if(!armed)return super.getWriteMethod();PacketEffects.trace+="W";return write?super.getWriteMethod():null;}
}
public class PacketConstructorOwner {
 final Object token;PacketConstructorOwner(Object t){token=t;}
 class BeanHandler extends PacketParent {
  final java.beans.PropertyDescriptor descriptor;
  BeanHandler(java.beans.PropertyDescriptor d){super(d.getPropertyType(),d.getReadMethod()!=null,d.getWriteMethod()!=null);descriptor=d;PacketEffects.trace+="B";}
  Object capture(){return PacketConstructorOwner.this.token;}
 }
 class FieldHandler extends PacketParent {
  final java.lang.reflect.Field field;
  FieldHandler(java.lang.reflect.Field f){super(f.getType(),true,true);field=f;PacketEffects.trace+="F";}
  Object capture(){return PacketConstructorOwner.this.token;}
 }
 BeanHandler bean(java.beans.PropertyDescriptor d){return new BeanHandler(d);}
 FieldHandler field(java.lang.reflect.Field f){return new FieldHandler(f);}
}
class PacketConstructorDriver {
 static void reset(boolean fail){PacketEffects.trace="";PacketEffects.observed=null;PacketEffects.fail=fail;}
 public static void main(String[] args)throws Exception{
  Object token=new Object();PacketConstructorOwner owner=new PacketConstructorOwner(token);PacketDescriptor d=new PacketDescriptor();
  for(int flags=0;flags<4;flags++){d.read=(flags&1)!=0;d.write=(flags&2)!=0;reset(false);PacketConstructorOwner.BeanHandler h=owner.bean(d);
   if(h.descriptor!=d||h.type!=int.class||h.readable!=d.read||h.writable!=d.write||PacketEffects.observed!=token||!PacketEffects.trace.equals("TRWSB"))throw new AssertionError("bean identity/conditional/order/capture:"+PacketEffects.trace);
  }
  java.lang.reflect.Field f=PacketBean.class.getField("field");reset(false);PacketConstructorOwner.FieldHandler h=owner.field(f);
  if(h.field!=f||h.type!=long.class||!h.readable||!h.writable||PacketEffects.observed!=token||!PacketEffects.trace.equals("SF"))throw new AssertionError("field identity/capture/order");
  reset(true);try{owner.bean(d);throw new AssertionError("lost super failure");}catch(RuntimeException e){if(e!=PacketEffects.failure||PacketEffects.observed!=token||!PacketEffects.trace.equals("TRWS"))throw new AssertionError("super failure/capture/order");}
  reset(true);try{owner.field(f);throw new AssertionError("lost field super failure");}catch(RuntimeException e){if(e!=PacketEffects.failure||PacketEffects.observed!=token||!PacketEffects.trace.equals("S"))throw new AssertionError("field super failure");}
  d.fail=true;reset(true);try{owner.bean(d);throw new AssertionError("lost producer failure");}catch(RuntimeException e){if(e!=PacketEffects.producerFailure||PacketEffects.observed!=null||!PacketEffects.trace.equals("T"))throw new AssertionError("producer before super");}
  reset(true);try{owner.bean(null);throw new AssertionError("lost descriptor NPE");}catch(NullPointerException e){if(PacketEffects.observed!=null||!PacketEffects.trace.isEmpty())throw new AssertionError("null before super");}
  reset(true);try{owner.field(null);throw new AssertionError("lost field NPE");}catch(NullPointerException e){if(PacketEffects.observed!=null||!PacketEffects.trace.isEmpty())throw new AssertionError("field null before super");}
  System.out.println("10:SDK:constructor:conditional:capture:identity:failure-order");
 }
}`
	testNativePrivateSetterSourceFixture(t, map[string]string{"PacketConstructorOwner.java": fixture}, "PacketConstructorOwner", "PacketConstructorDriver", "10:SDK:constructor:conditional:capture:identity:failure-order\n")
}

// Exercise source narrow words independently of the verifier's int category.
// The original driver computes the Java truncation/ranges itself; both source
// branches, signed extremes and category-two operands precede the SUPER call.
func TestAdversarialMemberConstructorNarrowArgumentsRoundTrip(t *testing.T) {
	fixture := `
class NarrowPacketEffects {
 static String trace="";static Object observed;
 static byte b;static char c;static short s;static boolean z;
 static byte byteValue(){trace+="b";return b;}static char charValue(){trace+="c";return c;}
 static short shortValue(){trace+="s";return s;}static boolean boolValue(){trace+="z";return z;}
}
class NarrowPacketParent {
 final Class type;final long wide;final double decimal;final boolean z;final byte b;final char c;final short s;
 NarrowPacketParent(Class t,long w,double d,boolean v,byte x,char y,short q){type=t;wide=w;decimal=d;z=v;b=x;c=y;s=q;NarrowPacketEffects.trace+="S";NarrowPacketEffects.observed=capture();}
 Object capture(){return null;}
}
class NarrowPacketBean {public long value;}
public class NarrowPacketOwner {
 final Object token;NarrowPacketOwner(Object t){token=t;}
 class Constants extends NarrowPacketParent {
  Constants(java.lang.reflect.Field f,boolean flag,long w,double d){super(f.getType(),w,d,flag,flag?(byte)-128:(byte)127,flag?(char)256:(char)65535,flag?(short)-32768:(short)32767);}
  Object capture(){return NarrowPacketOwner.this.token;}
 }
 class Truncations extends NarrowPacketParent {
  Truncations(java.lang.reflect.Field f,int n,long w,double d){super(f.getType(),w,d,n!=0,(byte)n,(char)n,(short)n);}
  Object capture(){return NarrowPacketOwner.this.token;}
 }
 class Producers extends NarrowPacketParent {
  Producers(java.lang.reflect.Field f,boolean flag,long w,double d){super(f.getType(),w,d,flag?NarrowPacketEffects.boolValue():NarrowPacketEffects.z,flag?NarrowPacketEffects.byteValue():NarrowPacketEffects.b,flag?NarrowPacketEffects.charValue():NarrowPacketEffects.c,flag?NarrowPacketEffects.shortValue():NarrowPacketEffects.s);}
  Object capture(){return NarrowPacketOwner.this.token;}
 }
}
class NarrowPacketDriver {
 static void check(NarrowPacketParent p,Object token,long wide,double d,boolean z,int b,int c,int s,String trace){
  if(p.type!=long.class||p.wide!=wide||Double.doubleToRawLongBits(p.decimal)!=Double.doubleToRawLongBits(d)||p.z!=z||p.b!=b||p.c!=c||p.s!=s||NarrowPacketEffects.observed!=token||!NarrowPacketEffects.trace.equals(trace))throw new AssertionError("narrow words/capture/order:"+NarrowPacketEffects.trace);
 }
 public static void main(String[] args)throws Exception{
  Object token=new Object();NarrowPacketOwner owner=new NarrowPacketOwner(token);java.lang.reflect.Field f=NarrowPacketBean.class.getField("value");
  int[] words={Integer.MIN_VALUE,-65537,-32769,-32768,-129,-128,-1,0,1,2,127,128,255,256,32767,32768,65535,65536,Integer.MAX_VALUE};
  int count=0;for(int n:words){long w=((long)n<<32)^0xFEDCBA98L;double d=Double.longBitsToDouble(0x7ff8000000000042L);
   NarrowPacketEffects.trace="";NarrowPacketEffects.observed=null;check(owner.new Truncations(f,n,w,d),token,w,d,n!=0,(byte)n,(char)n,(short)n,"S");count++;
   NarrowPacketEffects.b=(byte)n;NarrowPacketEffects.c=(char)n;NarrowPacketEffects.s=(short)n;NarrowPacketEffects.z=n<0;
   for(int k=0;k<2;k++){boolean flag=k!=0;NarrowPacketEffects.trace="";NarrowPacketEffects.observed=null;check(owner.new Constants(f,flag,w,d),token,w,d,flag,flag?-128:127,flag?256:65535,flag?-32768:32767,"S");count++;
    NarrowPacketEffects.trace="";NarrowPacketEffects.observed=null;check(owner.new Producers(f,flag,w,d),token,w,d,n<0,(byte)n,(char)n,(short)n,flag?"zbcsS":"S");count++;
   }
  }
  if(count!=95)throw new AssertionError("missing observations");System.out.println("95:narrow:boundaries:phi:truncation:wide:capture:order");
 }
}`
	testNativePrivateSetterSourceFixture(t, map[string]string{"NarrowPacketOwner.java": fixture}, "NarrowPacketOwner", "NarrowPacketDriver", "95:narrow:boundaries:phi:truncation:wide:capture:order\n")
}

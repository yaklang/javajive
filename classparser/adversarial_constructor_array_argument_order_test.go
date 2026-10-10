package javaclassparser

import "testing"

// Cross every array-arm/middle-failure/later-failure tuple. The unchanged
// original driver checks the selected arm, precise exception identity, lazy
// suffix, enclosing callback and category-2 words independently of the body.
func TestAdversarialConstructorPrivateArrayMiddleArgumentOrderRoundTrip(t *testing.T) {
	fixture := `
class PacketEffects {static String trace="";static final java.io.IOException middleError=new java.io.IOException("middle"),lastError=new java.io.IOException("last");
 static String left(String s){trace+="L";return "left:"+s;}static String left(Object o){throw new AssertionError("rival left overload");}
 static String right(String s){trace+="R";return "right:"+s;}static String right(Object o){throw new AssertionError("rival right overload");}
 static Object middle(boolean fail)throws java.io.IOException{trace+="M";if(fail)throw middleError;return middleError;}
 static Object last(boolean fail)throws java.io.IOException{trace+="T";if(fail)throw lastError;return lastError;}}
class PacketParent {final String[] array;final Object middle,last,capture;final long n;final double d;
 PacketParent(String[] a,Object m,long n,Object t,double d){PacketEffects.trace+="S";array=a;middle=m;last=t;this.n=n;this.d=d;capture=owner();}Object owner(){return null;}}
public class PacketOwner {final Object token;PacketOwner(Object t){token=t;}
 class Child extends PacketParent {Child(boolean arm,boolean failMiddle,boolean failLast,String s,long n,double d)throws java.io.IOException{
 super(new String[]{"head",arm?PacketEffects.left(s):PacketEffects.right(s)},PacketEffects.middle(failMiddle),n,PacketEffects.last(failLast),d);PacketEffects.trace+="B";}Object owner(){return PacketOwner.this.token;}}
 Child make(boolean a,boolean m,boolean l,String s,long n,double d)throws java.io.IOException{return new Child(a,m,l,s,n,d);}}
class PacketDriver {public static void main(String[]args)throws Exception{Object token=new Object();PacketOwner owner=new PacketOwner(token);long[] words={Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE};long[] doubleBits={0x8000000000000000L,0x7ff8000000000013L,0x7ff0000000000000L,0xfff0000000000000L,0x3ff0000000000000L};int rows=0;
 for(int mask=0;mask<8;mask++)for(int i=0;i<words.length;i++){boolean a=(mask&1)!=0,m=(mask&2)!=0,l=(mask&4)!=0;String s=i%2==0?null:"payload";double d=Double.longBitsToDouble(doubleBits[i]);PacketEffects.trace="";
  try{PacketOwner.Child c=owner.make(a,m,l,s,words[i],d);if(m||l)throw new AssertionError("missing suffix failure");
   if(!c.array[0].equals("head")||!c.array[1].equals((a?"left:":"right:")+s)||c.middle!=PacketEffects.middleError||c.last!=PacketEffects.lastError||c.capture!=token||c.n!=words[i]||Double.doubleToRawLongBits(c.d)!=doubleBits[i]||!PacketEffects.trace.equals((a?"L":"R")+"MTSB"))throw new AssertionError("source position/order/binding/capture/raw words");}
  catch(java.io.IOException e){if(e!=(m?PacketEffects.middleError:PacketEffects.lastError)||!PacketEffects.trace.equals((a?"L":"R")+(m?"M":"MT")))throw new AssertionError("exact failure identity and lazy suffix");}rows++;}
 System.out.println(rows+":private-array:middle-argument:all-tuples:failure-order:wide-identity");}}
`
	testNativePrivateSetterSourceFixture(t, map[string]string{"PacketOwner.java": fixture}, "PacketOwner", "PacketDriver", "40:private-array:middle-argument:all-tuples:failure-order:wide-identity\n")
}

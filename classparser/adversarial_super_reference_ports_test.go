package javaclassparser

import (
	"strings"
	"testing"
)

const superReferencePortsFixture = `interface CheckedAction<T>{void accept(T value)throws java.io.IOException;}
interface CheckedMap<T,R>{R apply(T value)throws java.io.IOException;}
class ReferenceBase {
 int calls;long state;boolean fail;final java.io.IOException error=new java.io.IOException("identity");
 public void take(int value)throws java.io.IOException{calls++;state=value;if(fail)throw error;}
 public long step(long value)throws java.io.IOException{calls++;state=value*31L+7;if(fail)throw error;return state;}
}
public class SuffixOwner extends ReferenceBase {
 public void take(int value){throw new AssertionError("virtual target");}
 public long step(long value){throw new AssertionError("virtual target");}
 CheckedAction<Integer> action(){return super::take;}
 CheckedMap<Long,Long> mapping(){return super::step;}
}
class SuffixDriver {public static void main(String[]args)throws Exception {
 SuffixOwner object=new SuffixOwner();CheckedAction<Integer> action=object.action();CheckedMap<Long,Long> mapping=object.mapping();int rows=0;
 for(boolean fail:new boolean[]{false,true})for(Integer value:new Integer[]{null,Integer.MIN_VALUE,-1,0,Integer.MAX_VALUE}){
  object.fail=fail;object.calls=0;object.state=913;Throwable caught=null;try{action.accept(value);}catch(Throwable e){caught=e;}
  if(value==null){if(!(caught instanceof NullPointerException)||object.calls!=0||object.state!=913)throw new AssertionError("unbox timing");}
  else if(caught!=(fail?object.error:null)||object.calls!=1||object.state!=value.longValue())throw new AssertionError("super dispatch / checked exception",caught);rows++;
 }
 for(boolean fail:new boolean[]{false,true})for(Long value:new Long[]{null,Long.MIN_VALUE,-1L,0L,Long.MAX_VALUE}){
  object.fail=fail;object.calls=0;object.state=913;Throwable caught=null;Long result=null;try{result=mapping.apply(value);}catch(Throwable e){caught=e;}
  if(value==null){if(!(caught instanceof NullPointerException)||object.calls!=0||object.state!=913)throw new AssertionError("wide unbox timing");}
  else{long expected=java.math.BigInteger.valueOf(value).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(7)).longValue();if(caught!=(fail?object.error:null)||object.calls!=1||object.state!=expected||!fail&&(result==null||result.longValue()!=expected))throw new AssertionError("super wide boxing / overflow / failure",caught);}rows++;
 }
 System.out.println(rows+":super:reference:physical:ports:effects");}}
`

func TestAdversarialSuperMethodReferenceRetainsPrimitiveImplementationPorts(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			constructorSuffixFamilyRoundTrip(t, superReferencePortsFixture, debug, []string{"SuffixOwner"}, "20:super:reference:physical:ports:effects\n")
		})
	}
}

func TestAdversarialSuperReferenceRetainsNarrowAndRenamedPorts(t *testing.T) {
	for _, profile := range []SourceCompilerProfile{NativeJavac8, ModernJavac} {
		for _, width := range []string{"byte", "short", "char"} {
			t.Run(string(profile)+"/"+width, func(t *testing.T) {
				boxed := map[string]string{"byte": "Byte", "short": "Short", "char": "Character"}[width]
				fixture := strings.NewReplacer("take", "acceptValue", "ReferenceBase", "ParentDispatch", "int value", width+" value", "Integer", boxed).Replace(superReferencePortsFixture)
				fixture = strings.ReplaceAll(fixture, "Big"+boxed, "BigInteger")
				fixture = strings.ReplaceAll(fixture, "MIN_VALUE,-1,0,", "MIN_VALUE,("+width+")1,("+width+")0,")
				if width == "char" {
					fixture = strings.ReplaceAll(fixture, "value.longValue()", "value.charValue()")
				}
				constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, fixture, "source,lines,vars", []string{"SuffixOwner"}, "20:super:reference:physical:ports:effects\n", 8, 8, profile)
			})
		}
	}
}

const superReferenceWidePortsFixture = `interface CheckedFive<A,B,C,D,E,R>{R apply(A a,B b,C c,D d,E e)throws java.io.IOException;}
class ReferenceBase {
 int calls;boolean fail;Object seen;final java.io.IOException error=new java.io.IOException("identity");
 public String mix(double a,long b,char c,Object d,int[] e)throws java.io.IOException{calls++;seen=d;if(fail)throw error;return Long.toHexString(Double.doubleToRawLongBits(a))+":"+b+":"+(int)c+":"+(e==null?"null":Integer.toString(e[0]));}
}
public class SuffixOwner extends ReferenceBase {
 public String mix(double a,long b,char c,Object d,int[] e){throw new AssertionError("virtual target");}
 CheckedFive<Double,Long,Character,Object,int[],String> operation(){return super::mix;}
}
class SuffixDriver {public static void main(String[]args)throws Exception{
 SuffixOwner object=new SuffixOwner();CheckedFive<Double,Long,Character,Object,int[],String> f=object.operation();int rows=0;
 for(Double a:new Double[]{null,-0.0d,Double.longBitsToDouble(0x7ff8000000000042L),Double.NEGATIVE_INFINITY})for(Long b:new Long[]{null,Long.MIN_VALUE,Long.MAX_VALUE})for(Character c:new Character[]{null,'\u0000','\uffff'})for(boolean fail:new boolean[]{false,true}){
  Object token=new Object();int[] array=(rows&1)==0?null:new int[]{Integer.MIN_VALUE};object.calls=0;object.seen=null;object.fail=fail;Throwable error=null;String result=null;try{result=f.apply(a,b,c,token,array);}catch(Throwable e){error=e;}
  if(a==null||b==null||c==null){if(!(error instanceof NullPointerException)||object.calls!=0||object.seen!=null)throw new AssertionError("unboxing before invocation",error);}
  else{if(error!=(fail?object.error:null)||object.calls!=1||object.seen!=token)throw new AssertionError("special dispatch / cause / identity",error);if(!fail){long bits=java.nio.ByteBuffer.allocate(8).putDouble(a.doubleValue()).getLong(0);String expected=Long.toHexString(bits)+":"+b+":"+(int)c.charValue()+":"+(array==null?"null":Integer.toString(array[0]));if(!expected.equals(result))throw new AssertionError("ordered wide / reference / array ports");}}
  rows++;
 }
 System.out.println(rows+":super:wide:ordered:boxing:effects");}}
`

func TestAdversarialSuperReferenceRetainsWideSlotOrderAndFloatingBits(t *testing.T) {
	for _, profile := range []SourceCompilerProfile{NativeJavac8, ModernJavac} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(string(profile)+"/"+debug, func(t *testing.T) {
				constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, superReferenceWidePortsFixture, debug, []string{"SuffixOwner"}, "72:super:wide:ordered:boxing:effects\n", 8, 8, profile)
			})
		}
	}
}

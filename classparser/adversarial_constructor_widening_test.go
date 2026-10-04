package javaclassparser

import "testing"

// Widening changes neither identity nor evaluation order. Competing overloads
// must still bind to the original invocation descriptor, including null and
// covariant arrays, rather than whichever source overload appears more specific.
func TestAdversarialConstructorCaptureWideningKeepsOriginalOverload(t *testing.T) {
	roundTripGenericFlowUnitsClasspath(t, "CaptureWideningDriver", `
class CaptureWideningParent {final Object value;final int tag;CaptureWideningParent(Object x){value=x;tag=1;}CaptureWideningParent(String x){value=x;tag=2;}Object capture(){return null;}}
class CaptureWideningInterfaceParent {final Object value;final int tag;CaptureWideningInterfaceParent(CharSequence x){value=x;tag=3;}CaptureWideningInterfaceParent(String x){value=x;tag=4;}Object capture(){return null;}}
class CaptureWideningArrayParent {final Object value;final int tag;CaptureWideningArrayParent(Object[] x){value=x;tag=5;}CaptureWideningArrayParent(String[] x){value=x;tag=6;}Object capture(){return null;}}
class CaptureWideningOwner {
 final Object token;CaptureWideningOwner(Object t){token=t;}
 final class ObjectMember extends CaptureWideningParent {ObjectMember(String x){super((Object)x);}Object capture(){return CaptureWideningOwner.this.token;}}
 final class InterfaceMember extends CaptureWideningInterfaceParent {InterfaceMember(String x){super((CharSequence)x);}Object capture(){return CaptureWideningOwner.this.token;}}
 final class DowncastMember extends CaptureWideningParent {DowncastMember(Object x){super((String)x);}Object capture(){return CaptureWideningOwner.this.token;}}
 final class ArrayMember extends CaptureWideningArrayParent {ArrayMember(String[] x){super((Object[])x);}Object capture(){return CaptureWideningOwner.this.token;}}
 DowncastMember downcast(Object x){return new DowncastMember(x);}
 ObjectMember object(String x){return new ObjectMember(x);}InterfaceMember inter(String x){return new InterfaceMember(x);}ArrayMember array(String[] x){return new ArrayMember(x);}
}
class CaptureWideningOracle {static void run(){Object token=new Object();for(Object t:new Object[]{null,token}){
 CaptureWideningOwner owner=new CaptureWideningOwner(t);
 for(String value:new String[]{null,"text"}){CaptureWideningOwner.ObjectMember a=owner.object(value);CaptureWideningOwner.InterfaceMember b=owner.inter(value);
 if(a.value!=value||b.value!=value||a.tag!=1||b.tag!=3||a.capture()!=t||b.capture()!=t)throw new AssertionError("reference identity/overload");System.out.println((value==null)+":"+a.tag+":"+b.tag+":"+(t==token));}
 for(Object value:new Object[]{null,"text",new Object()}){try{CaptureWideningOwner.DowncastMember a=owner.downcast(value);if(value!=null&&!(value instanceof String)||a.value!=value||a.tag!=2||a.capture()!=t)throw new AssertionError("cast identity/overload");System.out.println("downcast:"+(value==null));}catch(ClassCastException failure){if(value==null||value instanceof String)throw new AssertionError("unexpected cast failure",failure);System.out.println("cast");}}
 for(String[] value:new String[][]{null,new String[0],new String[]{"item",null}}){CaptureWideningOwner.ArrayMember a=owner.array(value);if(a.value!=value||a.tag!=5||a.capture()!=t)throw new AssertionError("array identity/overload");System.out.println((value==null)+":"+a.tag+":"+(t==token));}
}}}
public class CaptureWideningDriver {public static void main(String[]args){CaptureWideningOracle.run();}}
`, nil, []string{"CaptureWideningOwner", "CaptureWideningOwner$ObjectMember", "CaptureWideningOwner$InterfaceMember", "CaptureWideningOwner$ArrayMember", "CaptureWideningOwner$DowncastMember"}, true, Precision, Compatibility, "legacy")
}

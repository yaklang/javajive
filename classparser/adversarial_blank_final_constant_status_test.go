package javaclassparser

import "testing"

// A literal assignment to a blank final is not a ConstantValue declaration.
// The parent callback and failed receiver observe the original default fields;
// the rebuilt reader must still trigger the carrier's class initializer.
func TestAdversarialBlankFinalStoresKeepRuntimeConstantStatus(t *testing.T) {
	roundTripGenericFlowUnitsClasspath(t, "BlankConstantDriver", `
class BlankConstantEffects {static Object published;static int initialized;static RuntimeException failure=new IllegalArgumentException("parent");}
class BlankConstantParent {final String before;BlankConstantParent(boolean fail){before=snapshot();BlankConstantEffects.published=this;if(fail)throw BlankConstantEffects.failure;}String snapshot(){return "parent";}}
class BlankConstantChild extends BlankConstantParent {
 final boolean z;final byte b;final char c;final short s;final int i;final long j;final float f;final double d;final String text;final int genuine=11;
 BlankConstantChild(boolean fail){super(fail);z=true;b=-7;c=65535;s=-300;i=7;j=Long.MIN_VALUE;f=-0.0f;d=Double.NaN;text="runtime";}
 String snapshot(){return new StringBuilder().append(z).append(':').append(b).append(':').append((int)c).append(':').append(s).append(':').append(i).append(':').append(j).append(':').append(Float.floatToRawIntBits(f)).append(':').append(Double.doubleToRawLongBits(d)).append(':').append(text).append(':').append(genuine).toString();}
}
class BlankConstantCarrier {static final int genuine=29;static final int number;static {number=23;BlankConstantEffects.initialized++;}}
class BlankConstantReader {static int genuine(){return BlankConstantCarrier.genuine;}static int read(){return BlankConstantCarrier.number;}}
public class BlankConstantDriver {public static void main(String[]args){if(BlankConstantReader.genuine()!=29||BlankConstantEffects.initialized!=0)throw new AssertionError("original constant initialization");if(BlankConstantReader.read()!=23||BlankConstantEffects.initialized!=1)throw new AssertionError("runtime field read lost class initialization");for(boolean fail:new boolean[]{false,true}){BlankConstantEffects.published=null;try{BlankConstantChild c=new BlankConstantChild(fail);if(fail||!c.before.equals("false:0:0:0:0:0:0:0:null:11")||!c.snapshot().equals("true:-7:65535:-300:7:-9223372036854775808:-2147483648:9221120237041090560:runtime:11"))throw new AssertionError("field observation");System.out.println(c.before+"/"+c.snapshot());}catch(RuntimeException e){if(!fail||e!=BlankConstantEffects.failure)throw new AssertionError("exception identity",e);BlankConstantChild c=(BlankConstantChild)BlankConstantEffects.published;if(c==null||!c.before.equals("false:0:0:0:0:0:0:0:null:11")||!c.snapshot().equals(c.before))throw new AssertionError("published default fields");System.out.println(c.snapshot());}}}}
`, nil, []string{"BlankConstantChild", "BlankConstantCarrier", "BlankConstantReader"}, true, Precision, Compatibility, "legacy")
}

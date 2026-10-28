package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialRoundedGuardRetainsUnassignedBypass(t *testing.T) {
	t.Setenv("JDEC_INIT_PROX_SPLIT_OFF", "1")
	const source = `
class RoundedOutcome{static int created;final long value,selected;final boolean exact;final int route;RoundedOutcome(long n,boolean e,int r){created++;selected=n;exact=e;route=r;value=r==0||r==4?n:n^(e?4919L:31L)^r;}}
public class DecimalChoice {
 public static Object run(Number original,int flags,boolean enabled,double low,double high){
  Class kind=original.getClass();if(kind==Integer.class){int word=original.intValue();return word<0?new RoundedOutcome(word,false,3):original;}else if(kind==Long.class){long word=original.longValue();return word<0L?new RoundedOutcome(word,false,3):original;}
  else if(kind==Double.class){double input=original.doubleValue();
   rounded:{if(!enabled)break rounded;if(input>9000000000000000D||input< -9000000000000000D)break rounded;
    long word=original.longValue();double remainder=input-(double)word;boolean whole;
    if(remainder==0D)whole=true;
    else if(remainder>0D){if(remainder<low)whole=false;else if(remainder>high){whole=false;word++;}else break rounded;}
    else{if(remainder>-low)whole=false;else if(remainder<-high){whole=false;word--;}else break rounded;}
    if((flags&1)!=0&&word<=127L&&word>=-128L)return new RoundedOutcome(word,false,1);
    else if((flags&2)!=0&&word<=32767L&&word>=-32768L)return new RoundedOutcome(word,false,2);
    else if((flags&4)!=0&&word<=2147483647L&&word>=-2147483648L){int narrowed=(int)word;return (flags&16)!=0&&narrowed>=-16777216&&narrowed<=16777216?new RoundedOutcome(narrowed,false,5):new RoundedOutcome(narrowed,false,6);}
    else if((flags&8)!=0){if(whole)return new RoundedOutcome(word,false,7);else if(word<=2147483647L&&word>=-2147483648L)return new RoundedOutcome(word,false,7);}
   }
   if((flags&16)!=0&&input>=-Float.MAX_VALUE&&input<=Float.MAX_VALUE){return new RoundedOutcome(((long)Float.floatToRawIntBits((float)input))&4294967295L,false,4);}
   return original;
  }else if(kind==Float.class){float word=original.floatValue();return word<0F?new RoundedOutcome((long)word,false,3):original;}else return original;
 }
}
class DecimalChoiceDriver{public static void main(String[]args){int rows=0;
 double[] inputs={Double.NaN,Double.NEGATIVE_INFINITY,Double.POSITIVE_INFINITY,-0D,0D,-128.9999999D,-128.875D,-128.5D,-128.125D,-128.0000001D,-128D,-1.9999999D,-1.875D,-1.5D,-1.125D,-1.0000001D,-1D,0.0000001D,0.125D,0.5D,0.875D,0.9999999D,1D,127.0000001D,127.125D,127.5D,127.875D,127.9999999D,2147483648.5D,9000000000000000D,9000000000000001D};
 for(double input:inputs)for(int flags=0;flags<32;flags++)for(boolean enabled:new boolean[]{false,true})for(int mode=1;mode<2;mode++)for(int window=0;window<2;window++){
  double low=window==0?0.125D:0.000001D,high=1D-low;long word=(long)input;double rem=input-(double)word;
  boolean precise=rem==0D,nearLow=rem>0D&&rem<low||rem<0D&&rem>-low,nearHigh=rem>high||rem<-high;
  boolean valid=enabled&&input<=9000000000000000D&&input>=-9000000000000000D&&(precise||nearLow||nearHigh);
  if(nearHigh)word+=rem>0D?1L:-1L;
  int route=0;if(mode==2){route=3;word=(long)input;precise=false;}else if(mode==1&&valid){if((flags&1)!=0&&word<=127L&&word>=-128L)route=1;else if((flags&2)!=0&&word<=32767L&&word>=-32768L)route=2;else if((flags&4)!=0&&word<=2147483647L&&word>=-2147483648L)route=(flags&16)!=0&&word>=-16777216L&&word<=16777216L?5:6;else if((flags&8)!=0&&(precise||word<=2147483647L&&word>=-2147483648L))route=7;}
  boolean asFloat=route==0&&mode==1&&(flags&16)!=0&&input>=-Float.MAX_VALUE&&input<=Float.MAX_VALUE;
  boolean returnedExact=false;long expected=route!=0?word^(returnedExact?4919L:31L)^route:asFloat?((long)Float.floatToRawIntBits((float)input))&4294967295L:Double.doubleToRawLongBits(input);
  RoundedOutcome.created=0;Number original=Double.valueOf(input);Object result=DecimalChoice.run(original,flags,enabled,low,high);boolean identity=route==0&&!asFloat;
  if(identity){if(result!=original||RoundedOutcome.created!=0)throw new AssertionError("fallback identity");rows++;continue;}
  if(!(result instanceof RoundedOutcome))throw new AssertionError("result type");RoundedOutcome got=(RoundedOutcome)result;
  if(got.value!=expected||got.route!=(asFloat?4:route)||RoundedOutcome.created!=1)throw new AssertionError("routing "+input+":"+flags+":"+mode);
  if(route!=0&&(got.selected!=word||got.exact!=returnedExact))throw new AssertionError("selected binding");rows++;
 }
 Number[] others={Integer.valueOf(-23),Integer.valueOf(23),Long.valueOf(-23),Float.valueOf(-23),Float.valueOf(0.5F),Short.valueOf((short)4)};
 for(Number input:others)for(int flags:new int[]{0,31})for(boolean enabled:new boolean[]{false,true}){
  boolean selected=(input instanceof Integer||input instanceof Long||input instanceof Float)&&input.doubleValue()<0D;RoundedOutcome.created=0;
  Object result=DecimalChoice.run(input,flags,enabled,0.125D,0.875D);
  if(selected){if(!(result instanceof RoundedOutcome))throw new AssertionError("other result type");RoundedOutcome r=(RoundedOutcome)result;if(r.route!=3||r.selected!=input.longValue()||r.exact||r.value!=(input.longValue()^31L^3L))throw new AssertionError("other selected route");}
  else if(result!=input)throw new AssertionError("other identity");
  if(RoundedOutcome.created!=(selected?1:0))throw new AssertionError("other allocation count");rows++;
 }
 System.out.println(rows+":rounded:shared-fallback");}}
`
	for _, name := range []string{"DecimalChoice", "QuantizedSelection"} {
		t.Run(name, func(t *testing.T) {
			testAuthoredOwnerAcrossSourceModes(t, strings.ReplaceAll(source, "DecimalChoice", name), name, name+"Driver", "3992:rounded:shared-fallback\n")
		})
	}
}

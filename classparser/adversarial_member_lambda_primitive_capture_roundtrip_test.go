package javaclassparser

import "testing"

// Computational int, category-2 words and raw floating bits must stay in their
// original declarations. The same driver covers boundary values, NaN payloads,
// signed zero, eager partial failure and lazy repeated SAM calls.
const memberLambdaPrimitiveCaptureFixture = `
class WordCaptureEffects {
 static String trace="";static int fail;static final RuntimeException failure=new RuntimeException("original");
 static int integer(int word){trace+="I";if(fail==1)throw failure;return word^0x76543210;}
 static long wide(long word){trace+="L";if(fail==2)throw failure;return word^0x123456789abcdefL;}
 static float fractional(int bits){trace+="F";if(fail==3)throw failure;return Float.intBitsToFloat(bits);}
 static double precise(long bits){trace+="D";if(fail==4)throw failure;return Double.longBitsToDouble(bits);}
}
class WordCaptureOwner {
 static class View {
  java.util.function.LongUnaryOperator operation(int input,long raw,int floatBits,long doubleBits){
   final int integer=WordCaptureEffects.integer(input);final long wide=WordCaptureEffects.wide(raw);
   final float fractional=WordCaptureEffects.fractional(floatBits);final double precise=WordCaptureEffects.precise(doubleBits);
   return argument->{WordCaptureEffects.trace+="B";return argument^integer^wide^Float.floatToRawIntBits(fractional)^Double.doubleToRawLongBits(precise);};
  }
 }
 static View view(){return new View();}
}
class WordCaptureDriver {
 public static void main(String[] args){int rows=0;WordCaptureOwner.View owner=WordCaptureOwner.view();
  if(!owner.getClass().isMemberClass()||owner.getClass().getDeclaringClass()!=WordCaptureOwner.class)throw new AssertionError("member identity");
  int[] integers={Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE};long[] longs={Long.MIN_VALUE,-1L,0L,1L,Long.MAX_VALUE};
  int[] floats={0,0x80000000,0x7f800000,0xff800000,1,0x7f7fffff,0x7fc12345,0xffc54321};
  long[] doubles={0L,0x8000000000000000L,0x7ff0000000000000L,0xfff0000000000000L,1L,0x7fefffffffffffffL,0x7ff8123456789abcL,0xfff8abcdef123456L};
  for(int integer:integers)for(long wide:longs)for(int fractional:floats)for(long precise:doubles){
   WordCaptureEffects.trace="";WordCaptureEffects.fail=0;java.util.function.LongUnaryOperator operation=owner.operation(integer,wide,fractional,precise);
   if(!WordCaptureEffects.trace.equals("ILFD"))throw new AssertionError("eager producer order/lazy body");
   for(long argument:longs){WordCaptureEffects.trace="";long expected=argument^(integer^0x76543210)^(wide^0x123456789abcdefL)^fractional^precise;
    if(operation.applyAsLong(argument)!=expected||!WordCaptureEffects.trace.equals("B"))throw new AssertionError("captured word bits or body count");rows++;
   }
  }
  for(int fail=1;fail<=4;fail++){WordCaptureEffects.trace="";WordCaptureEffects.fail=fail;
   try{owner.operation(1,2L,0x80000000,0x7ff8123456789abcL);throw new AssertionError("missing eager failure");}
   catch(RuntimeException error){if(error!=WordCaptureEffects.failure||!WordCaptureEffects.trace.equals("ILFD".substring(0,fail)))throw new AssertionError("partial failure order/identity",error);rows++;}
  }
  System.out.println(rows+":word:local:capture:bits:order:identity");
 }
}`

func TestAdversarialMemberLambdaLocalCaptureRetainsPrimitiveWords(t *testing.T) {
	testNativeIndependentFamilyFixture(t, memberLambdaPrimitiveCaptureFixture, []string{"WordCaptureOwner"}, "WordCaptureDriver", "8004:word:local:capture:bits:order:identity\n", nativeLexicalExactSignatures)
}

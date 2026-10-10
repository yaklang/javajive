package javaclassparser

import (
	"strings"
	"testing"
)

// The original parent and driver stay compiled. In addition to capture value
// identity, the parent observes the outer receiver before the source-level
// constructor body, and reflection distinguishes the original anonymous scope.
const nativeAnonymousLambdaCaptureFixture = `abstract class LambdaObserverParent {
 final Object seen; LambdaObserverParent(){seen=observe();}
 abstract Object observe(); abstract Object[] apply(Object argument,long number);
}
class LambdaCaptureOwner {
 final Object token; LambdaCaptureOwner(Object token){this.token=token;}
 LambdaObserverParent make(final Object first,final Object second,final long salt,final Object[] array){
  return new LambdaObserverParent(){
   Object observe(){return LambdaCaptureOwner.this.token;}
   Object[] apply(Object argument,long number){
    java.util.function.Supplier<Object> field=()->first;
    java.util.function.Supplier<Object[]> parameter=()->new Object[]{argument};
    java.util.function.Supplier<Object> pair=()->first==second?first:second;
    java.util.function.LongSupplier wide=()->salt^number;
    java.util.function.Supplier<Object[]> referenceArray=()->array;
    java.util.function.IntSupplier empty=()->17;
    return new Object[]{field.get(),parameter.get()[0],pair.get(),wide.getAsLong(),referenceArray.get(),empty.getAsInt()};
   }
  };
 }
}
class LambdaCaptureDriver {
 public static void main(String[]args){Object shared=new Object();int rows=0;
  Object[] tokens={null,shared,"same",new String("same")};
  long[] numbers={0,1,-1,Long.MIN_VALUE,Long.MAX_VALUE,0x100000001L};
  for(Object outer:tokens)for(Object first:tokens)for(Object second:tokens)for(long salt:numbers){
   Object[] array={first,second};LambdaCaptureOwner owner=new LambdaCaptureOwner(outer);LambdaObserverParent value;
   try{value=owner.make(first,second,salt,array);}catch(NullPointerException e){throw new AssertionError("anonymous lambda early capture timing",e);}
   array[0]=outer;
   for(long number:numbers){
    Object[] got=value.apply(shared,number);
    if(value.seen!=outer||value.observe()!=outer||!value.getClass().isAnonymousClass()||
       got.length!=6||got[0]!=first||got[1]!=shared||got[2]!=(first==second?first:second)||
       ((Long)got[3]).longValue()!=(salt^number)||got[4]!=array||((Integer)got[5]).intValue()!=17)
     throw new AssertionError("anonymous lambda producer order/identity/wide capture/original ownership");rows++;
   }
  }
  System.out.println(rows+":anonymous:lambda:identity");
 }
}`

func TestAdversarialAnonymousLambdaOriginalCapturesRoundTrip(t *testing.T) {
	for _, rename := range []string{"original", "renamed"} {
		t.Run(rename, func(t *testing.T) {
			source, owner, driver := nativeAnonymousLambdaCaptureFixture, "LambdaCaptureOwner", "LambdaCaptureDriver"
			if rename == "renamed" {
				replace := strings.NewReplacer("LambdaCaptureOwner", "LexicalEnvelope", "LambdaObserverParent", "ObservingBase", "first", "alpha", "second", "beta", "salt", "bias", "array", "buffer")
				source, owner = replace.Replace(source), replace.Replace(owner)
			}
			testNativePrivateSetterFixture(t, source, owner, driver, "2304:anonymous:lambda:identity\n")
		})
	}
}

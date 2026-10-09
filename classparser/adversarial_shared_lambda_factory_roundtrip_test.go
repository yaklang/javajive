package javaclassparser

import (
	"strings"
	"testing"
)

// Both identity lambdas use one original private helper. Their captures have
// the same erasure but distinct physical origins and must remain distinct.
const nativeSharedAnonymousLambdaCaptureFixture = `abstract class LambdaObserverParent {
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
    java.util.function.Supplier<Object> parameter=()->argument;
    java.util.function.Supplier<Object> pair=()->first==second?first:second;
    java.util.function.LongSupplier wide=()->salt^number;
    java.util.function.Supplier<Object[]> referenceArray=()->array;
    java.util.function.IntSupplier empty=()->17;
    return new Object[]{field.get(),parameter.get(),pair.get(),wide.getAsLong(),referenceArray.get(),empty.getAsInt()};
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

func TestAdversarialAnonymousSharedLambdaFactoryCapturesRoundTrip(t *testing.T) {
	for _, rename := range []string{"original", "renamed"} {
		t.Run(rename, func(t *testing.T) {
			source, owner, driver := nativeSharedAnonymousLambdaCaptureFixture, "LambdaCaptureOwner", "LambdaCaptureDriver"
			if rename == "renamed" {
				replace := strings.NewReplacer("LambdaCaptureOwner", "LexicalEnvelope", "LambdaObserverParent", "ObservingBase", "first", "alpha", "second", "beta", "salt", "bias", "array", "buffer")
				source, owner = replace.Replace(source), replace.Replace(owner)
			}
			testNativePrivateSetterFixture(t, source, owner, driver, "2304:anonymous:lambda:identity\n")
		})
	}
}

// Shared no-capture and category-2 helpers retain independent factory PCs.
func TestAdversarialAnonymousSharedLambdaWideAndEmptyFactories(t *testing.T) {
	source := strings.Replace(nativeSharedAnonymousLambdaCaptureFixture, "java.util.function.LongSupplier wide=()->salt^number;", "java.util.function.LongSupplier wide=()->salt; java.util.function.LongSupplier word=()->number;", 1)
	source = strings.Replace(source, "java.util.function.IntSupplier empty=()->17;", "java.util.function.IntSupplier empty=()->17; java.util.function.IntSupplier other=()->17;", 1)
	source = strings.Replace(source, "wide.getAsLong(),referenceArray.get(),empty.getAsInt()", "wide.getAsLong()^word.getAsLong(),referenceArray.get(),empty.getAsInt()+other.getAsInt()", 1)
	source = strings.Replace(source, "((Integer)got[5]).intValue()!=17", "((Integer)got[5]).intValue()!=34", 1)
	testNativePrivateSetterFixture(t, source, "LambdaCaptureOwner", "LambdaCaptureDriver", "2304:anonymous:lambda:identity\n")
}

// Factories in different original methods can have the same bytecode PC and
// erased capture type. Their method/code identity remains part of the binding.
const nativeNamedSharedLambdaFactoryFixture = `class SharedFactoryOwner {
 static class Cell {
  Object first(Object value){java.util.function.Supplier<Object> s=()->value;return s.get();}
  Object second(Object other){java.util.function.Supplier<Object> s=()->other;return s.get();}
  long firstWord(long value){java.util.function.LongSupplier s=()->value;return s.getAsLong();}
  long secondWord(long other){java.util.function.LongSupplier s=()->other;return s.getAsLong();}
  int firstEmpty(){java.util.function.IntSupplier s=()->31;return s.getAsInt();}
  int secondEmpty(){java.util.function.IntSupplier s=()->31;return s.getAsInt();}
 }
}
class SharedFactoryDriver {public static void main(String[] args){
 Object shared=new Object();Object[] values={null,shared,"same",new String("same")};
 long[] words={0,1,-1,Long.MIN_VALUE,Long.MAX_VALUE,0x100000001L};int rows=0;
 SharedFactoryOwner.Cell cell=new SharedFactoryOwner.Cell();
 for(Object a:values)for(Object b:values)for(long word:words){
  if(cell.first(a)!=a||cell.second(b)!=b||cell.firstWord(word)!=word||cell.secondWord(~word)!=~word||cell.firstEmpty()!=31||cell.secondEmpty()!=31||!cell.getClass().isMemberClass())throw new AssertionError("shared factory original method/code/pc binding");rows++;
 }
 System.out.println(rows+":shared:method:bindings");
}}`

func TestAdversarialNamedSharedLambdaFactoryBindings(t *testing.T) {
	for _, rename := range []string{"original", "renamed"} {
		t.Run(rename, func(t *testing.T) {
			source, owner, driver := nativeNamedSharedLambdaFactoryFixture, "SharedFactoryOwner", "SharedFactoryDriver"
			if rename == "renamed" {
				r := strings.NewReplacer("SharedFactoryOwner", "MethodEnvelope", "Cell", "Worker", "first", "alpha", "second", "beta", "value", "item", "other", "candidate")
				source, owner = r.Replace(source), r.Replace(owner)
			}
			testNativePrivateSetterFixture(t, source, owner, driver, "96:shared:method:bindings\n")
		})
	}
}

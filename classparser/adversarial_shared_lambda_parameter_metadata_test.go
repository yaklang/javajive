package javaclassparser

import (
	"strings"
	"testing"
)

// Several factories can copy the same effectively-final local. Its mutable
// referent stays shared, but each factory and its implementation metadata must
// retain their original identity. Equal types or source spellings are not proof
// that independently initialized locals can share a capture binder.
const sharedLambdaParameterFixture = `
import java.util.*;import java.util.function.*;
public class UncapturedLambdaNames {
 public int bias;public UncapturedLambdaNames(int initial){bias=initial;}
 public IntUnaryOperator make(int seed){
  ArrayList<Integer> chunks=new ArrayList<Integer>();chunks.add(Integer.valueOf(seed));
  IntUnaryOperator first=(int sample)->{chunks.add(Integer.valueOf(sample));return chunks.get(0).intValue()+chunks.get(chunks.size()-1).intValue()+chunks.size()+this.bias;};
  return(int sample)->first.applyAsInt(sample)+chunks.size()+this.bias;
 }
}
class LambdaParameterDriver {
 public static void main(String[]args)throws Exception{
  int rows=0;for(int initial:new int[]{-3,0,4})for(int seed:new int[]{-1,0,Integer.MAX_VALUE}){
   UncapturedLambdaNames owner=new UncapturedLambdaNames(initial);IntUnaryOperator left=owner.make(seed),right=owner.make(seed);
   int step=0;for(int sample:new int[]{Integer.MIN_VALUE,-1,0,1,2,Integer.MAX_VALUE}){
    owner.bias=initial+step;int size=step+2;int wantLeft=(int)((long)seed+(long)sample+2L*size+2L*owner.bias);int other=-sample;int wantRight=(int)((long)seed+(long)other+2L*size+2L*owner.bias);
    if(left.applyAsInt(sample)!=wantLeft||right.applyAsInt(other)!=wantRight)throw new AssertionError("shared binder, fresh factory state, live receiver or overflow "+rows);rows+=2;step++;
   }
  }
  List<String> declarations=new ArrayList<>();for(java.lang.reflect.Method method:UncapturedLambdaNames.class.getDeclaredMethods()){
   StringBuilder row=new StringBuilder(method.getName()).append(':').append(method.getReturnType().getName()).append(':').append(method.getModifiers());for(java.lang.reflect.Parameter parameter:method.getParameters())row.append('/').append(parameter.getType().getName()).append(':').append(parameter.getName()).append(':').append(parameter.getModifiers()).append(':').append(parameter.isNamePresent());declarations.add(row.toString());
  }
  Collections.sort(declarations);java.security.MessageDigest hash=java.security.MessageDigest.getInstance("SHA-256");for(String declaration:declarations)hash.update((declaration+"\n").getBytes("UTF-8"));System.out.println(rows+":uncaptured:lambda:parameters:"+Base64.getEncoder().encodeToString(hash.digest()));
 }
}`

func TestAdversarialSharedLambdaLocalParameterMetadataKeepsOneOriginalBinder(t *testing.T) {
	testUncapturedLambdaParameterMetadataRoundTrip(t, sharedLambdaParameterFixture, 2, "108")
}

func TestAdversarialGeneratedSharedLambdaLocalMetadataKeepsOriginalCaptureNames(t *testing.T) {
	testUncapturedLambdaParameterMetadataRoundTrip(t, strings.ReplaceAll(sharedLambdaParameterFixture, "chunks", "var0"), 2, "108")
}

func sharedWideLambdaParameterFixture() string {
	source := strings.Replace(sharedLambdaParameterFixture, " public int bias;", `
 public static LongUnaryOperator wide(long initial){
  long shared=initial^0x100000001L;long pinned=7L;
  LongUnaryOperator first=(long delta)->shared+delta+pinned;
  return(long delta)->pinned+first.applyAsLong(delta)+shared;
 }
 public int bias;`, 1)
	source = strings.Replace(source, "  List<String> declarations=", `
  for(long initial:new long[]{Long.MIN_VALUE,-1L,0L,1L,Long.MAX_VALUE})for(long delta:new long[]{Long.MIN_VALUE,-1L,0L,1L,Long.MAX_VALUE}){
   long expected=java.math.BigInteger.valueOf(initial).xor(java.math.BigInteger.valueOf(0x100000001L)).multiply(java.math.BigInteger.valueOf(2)).add(java.math.BigInteger.valueOf(delta)).add(java.math.BigInteger.valueOf(14)).longValue();
   if(UncapturedLambdaNames.wide(initial).applyAsLong(delta)!=expected)throw new AssertionError("category-2 shared locals, constant capture, descriptor order or overflow");rows++;
  }
  List<String> declarations=`, 1)
	return source
}

func TestAdversarialGeneratedSharedWideLambdaMetadataCannotTurnLiteralIntoConstant(t *testing.T) {
	source := strings.ReplaceAll(sharedWideLambdaParameterFixture(), "pinned", "var0")
	testUncapturedLambdaParameterMetadataRoundTrip(t, source, 4, "133")
}

func TestAdversarialSharedWideLambdaLocalParameterMetadataKeepsValuesAndCaptureRows(t *testing.T) {
	source := sharedWideLambdaParameterFixture()
	for _, named := range []bool{true, false} {
		name := "named"
		if !named {
			name = "absent"
		}
		t.Run(name, func(t *testing.T) {
			testUncapturedLambdaParameterMetadataRoundTrip(t, source, 4, "133", named)
		})
	}
}

package javaclassparser

import (
	"fmt"
	"testing"
)

// One compiler/runtime checks source-level boundaries; this is not a runtime
// version matrix. A requested higher source release must retain the original
// factory/counter graph, including nested captured bodies and wide ports.
func TestAdversarialModernLambdaSourceLevelKeepsOriginalFactoriesAndImplementationTypes(t *testing.T) {
	const fixture = `import java.util.*;import java.util.function.*;import java.math.BigInteger;
class DefaultCompilerCases {
 static final IntUnaryOperator WORD=value->value+1;
 static final LongUnaryOperator WIDE=value->value^0x8123456789abcdefL;
 static IntUnaryOperator nested(int base){return value->{IntUnaryOperator child=other->base+(other^value);return child.applyAsInt(value*3);};}
}
class DefaultCompilerDriver {
 public static void main(String[]args)throws Exception{
  int rows=0;
  for(int base:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(int value:new int[]{Integer.MIN_VALUE,-33,-1,0,1,31,32,Integer.MAX_VALUE}){
   int product=BigInteger.valueOf(value).multiply(BigInteger.valueOf(3)).intValue();
   int expected=BigInteger.valueOf(product).xor(BigInteger.valueOf(value)).add(BigInteger.valueOf(base)).intValue();
   if(DefaultCompilerCases.nested(base).applyAsInt(value)!=expected||DefaultCompilerCases.WORD.applyAsInt(value)!=BigInteger.valueOf(value).add(BigInteger.ONE).intValue())throw new AssertionError("nested integer model");rows++;
  }
  for(long value:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){if(DefaultCompilerCases.WIDE.applyAsLong(value)!=BigInteger.valueOf(value).xor(BigInteger.valueOf(0x8123456789abcdefL)).longValue())throw new AssertionError("wide model");rows++;}
  List<String> methods=new ArrayList<String>();for(java.lang.reflect.Method method:DefaultCompilerCases.class.getDeclaredMethods())if(method.isSynthetic())methods.add(method.toGenericString());
  Collections.sort(methods);System.out.println(rows+":modern-level:"+methods);
 }
}`
	for _, pair := range [][2]int{{8, 8}, {8, 11}, {8, 16}, {11, 11}, {17, 17}, {21, 21}, {21, 0}} {
		t.Run(fmt.Sprintf("original%d/source%d", pair[0], pair[1]), func(t *testing.T) {
			assertDefaultCompilerRegistrationOracleAtReleases(t, fixture, "45:modern-level:", true, pair[0], pair[1])
		})
	}
}

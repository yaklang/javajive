package javaclassparser

import "testing"

// Each source occurrence consumes a compiler ordinal even when javac folds
// equal translated bodies into one private implementation. Different target
// interfaces, wide ports and void effects must retain the original factory
// graph. The unchanged driver also observes the actual implementation ABI.
const sharedLambdaOccurrenceFixture = `
import java.util.*;import java.util.function.*;import java.math.BigInteger;
class DefaultCompilerCases {
 static final IntUnaryOperator FIRST=value->value+1;
 static final SharedWord SECOND=value->value+1;
 static final IntUnaryOperator DIFFERENT=value->value-1;
 static final LongUnaryOperator WIDE_FIRST=value->value^0x8123456789abcdefL;
 static final LongUnaryOperator WIDE_SECOND=value->value^0x8123456789abcdefL;
 static int total;
 static final IntConsumer EFFECT_FIRST=value->{total+=value;};
 static final IntConsumer EFFECT_SECOND=value->{total+=value;};
 static final IntUnaryOperator LAST=value->{if(value<0)throw new IllegalArgumentException("negative");return value;};
}
interface SharedWord {int apply(int value);}
class DefaultCompilerDriver {
 public static void main(String[]args)throws Exception{
  int rows=0;BigInteger total=BigInteger.ZERO;
  for(int value:new int[]{Integer.MIN_VALUE,-17,-1,0,1,17,Integer.MAX_VALUE}){
   int plus=BigInteger.valueOf(value).add(BigInteger.ONE).intValue();
   int minus=BigInteger.valueOf(value).subtract(BigInteger.ONE).intValue();
   if(DefaultCompilerCases.FIRST.applyAsInt(value)!=plus||DefaultCompilerCases.SECOND.apply(value)!=plus||DefaultCompilerCases.DIFFERENT.applyAsInt(value)!=minus)throw new AssertionError("distinct factories and target types");rows++;
   DefaultCompilerCases.EFFECT_FIRST.accept(value);DefaultCompilerCases.EFFECT_SECOND.accept(value);
   total=total.add(BigInteger.valueOf(value).multiply(BigInteger.valueOf(2)));
   if(DefaultCompilerCases.total!=total.intValue())throw new AssertionError("void effects");rows++;
   try{int result=DefaultCompilerCases.LAST.applyAsInt(value);if(value<0||result!=value)throw new AssertionError("lost exception");}catch(IllegalArgumentException expected){if(value>=0||!"negative".equals(expected.getMessage()))throw new AssertionError("wrong exception");}rows++;
  }
  for(long value:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){
   long expected=BigInteger.valueOf(value).xor(BigInteger.valueOf(0x8123456789abcdefL)).longValue();
   if(DefaultCompilerCases.WIDE_FIRST.applyAsLong(value)!=expected||DefaultCompilerCases.WIDE_SECOND.applyAsLong(value)!=expected)throw new AssertionError("wide ports");rows++;
  }
  List<String> methods=new ArrayList<String>();
  for(java.lang.reflect.Method method:DefaultCompilerCases.class.getDeclaredMethods())if(method.isSynthetic()){
   String signature=method.getName()+":"+method.getModifiers()+":"+method.getReturnType().getName();
   for(Class<?> parameter:method.getParameterTypes())signature+=":"+parameter.getName();methods.add(signature);
  }
  Collections.sort(methods);System.out.println(rows+":shared-factories:"+methods);
 }
}`

func TestAdversarialSharedLambdaOccurrencesKeepPortsEffectsAndImplementationABI(t *testing.T) {
	assertDefaultCompilerRegistrationOracle(t, sharedLambdaOccurrenceFixture, "26:shared-factories:")
}

func TestAdversarialSharedGenericLambdaOccurrencesKeepActualGenericImplementationTypes(t *testing.T) {
	assertDefaultCompilerRegistrationOracle(t, `
import java.util.*;import java.util.function.*;
class DefaultCompilerCases {
 static final SharedThree<String,Object,Map<String,Object>> FIRST=(key,value,map)->map.put(key,value);
 static final SharedReverse<Map<String,Object>,Object,String> SECOND=(key,value,map)->map.put(key,value);
 static final IntUnaryOperator LAST=value->value+1;
 static void accept(String key,Object value,Map<String,Object> map){FIRST.accept(key,value,map);SECOND.accept(key,value,map);}
}
interface SharedThree<A,B,C>{void accept(A first,B second,C third);}
interface SharedReverse<A,B,C>{void accept(C first,B second,A third);}
class CountingMap extends HashMap<String,Object>{int writes;public Object put(String key,Object value){writes++;return super.put(key,value);}}
class DefaultCompilerDriver {
 public static void main(String[]args)throws Exception{
  int rows=0;
  for(String key:new String[]{null,"","λ"})for(Object value:new Object[]{null,Integer.valueOf(7),"x"}){
   CountingMap actual=new CountingMap();DefaultCompilerCases.accept(key,value,actual);
   if(actual.writes!=2||actual.size()!=1||!actual.containsKey(key)||actual.get(key)!=value)throw new AssertionError("generic target mapping and effects");rows++;
  }
  List<String> methods=new ArrayList<String>();
  for(java.lang.reflect.Method method:DefaultCompilerCases.class.getDeclaredMethods())if(method.isSynthetic())methods.add(method.toGenericString());
  Collections.sort(methods);System.out.println(rows+":shared-generic-factories:"+methods);
 }
}`, "9:shared-generic-factories:")
}

func TestAdversarialSharedEnumLambdaOccurrencesKeepInitializationAndImplementationABI(t *testing.T) {
	// Generated enum valueOf parameter names have a separate unsupported
	// compiler-metadata contract. Keep this factory/counter experiment free of
	// that unrelated obligation; its existing refusal test remains unchanged.
	assertDefaultCompilerRegistrationOracleConfigured(t, `
import java.util.*;import java.util.function.*;import java.math.BigInteger;
enum DefaultCompilerCases {
 FIRST(value->value+1),SECOND(value->value+1),LAST(value->{if(value<0)throw new IllegalArgumentException("negative");return value;});
 final IntUnaryOperator operation;
 DefaultCompilerCases(IntUnaryOperator operation){this.operation=operation;}
}
class DefaultCompilerDriver {
 public static void main(String[]args)throws Exception{
  int rows=0;
  for(DefaultCompilerCases entry:DefaultCompilerCases.values())for(int value:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){
   if(DefaultCompilerCases.valueOf(entry.name())!=entry||DefaultCompilerCases.values()[entry.ordinal()]!=entry)throw new AssertionError("enum initialization order");
   try{int result=entry.operation.applyAsInt(value);int expected=entry.ordinal()<2?BigInteger.valueOf(value).add(BigInteger.ONE).intValue():value;if(entry.ordinal()==2&&value<0||result!=expected)throw new AssertionError("enum argument arithmetic");}
   catch(IllegalArgumentException expected){if(entry.ordinal()!=2||value>=0||!"negative".equals(expected.getMessage()))throw new AssertionError("enum argument exception");}rows++;
  }
  List<String> methods=new ArrayList<String>();
  for(java.lang.reflect.Method method:DefaultCompilerCases.class.getDeclaredMethods())if(method.isSynthetic()){
   String signature=method.getName()+":"+method.getModifiers()+":"+method.getReturnType().getName();
   for(Class<?> parameter:method.getParameterTypes())signature+=":"+parameter.getName();methods.add(signature);
  }
  Collections.sort(methods);System.out.println(rows+":shared-enum-factories:"+methods);
 }
}`, "15:shared-enum-factories:", false)
}

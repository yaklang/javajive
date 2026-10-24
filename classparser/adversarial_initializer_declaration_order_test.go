package javaclassparser

import "testing"

// Constructor declaration order affects compiler artifact numbering, while
// instance initialization still runs before the selected constructor body.
// Moving declarations must preserve field/static initialization order, exact
// this(...) delegation and every executable statement inside each packet.
func TestAdversarialConstructorDeclarationLambdaOrderPreservesInitialization(t *testing.T) {
	assertDefaultCompilerRegistrationOracle(t, `
import java.util.*;import java.util.function.*;import java.math.BigInteger;
class DefaultCompilerCases {
 DefaultCompilerCases(){this(7);}
 DefaultCompilerCases(int base){mark("C");seed=base;operation=value->base*7+value;}
 DefaultCompilerCases(long base){this((int)base);mark("W");}
 static String trace="";
 static int first=mark("S");
 static final IntUnaryOperator FIELD=value->value^17;
 int initialized=mark("I");
 final int seed;
 final IntUnaryOperator operation;
 static int mark(String event){trace+=event;return 17;}
}
class DefaultCompilerDriver {
 public static void main(String[]args)throws Exception{
  int rows=0;String expectedTrace="S";
  for(int base:new int[]{Integer.MIN_VALUE,-17,-1,0,1,Integer.MAX_VALUE})for(int value:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){
   DefaultCompilerCases object=new DefaultCompilerCases(base);expectedTrace+="IC";
   int expected=BigInteger.valueOf(base).multiply(BigInteger.valueOf(7)).add(BigInteger.valueOf(value)).intValue();
   if(object.seed!=base||object.initialized!=17||object.operation.applyAsInt(value)!=expected||DefaultCompilerCases.FIELD.applyAsInt(value)!=BigInteger.valueOf(value).xor(BigInteger.valueOf(17)).intValue())throw new AssertionError("constructor capture and word model");rows++;
  }
  for(long base:new long[]{Long.MIN_VALUE,-1,0,Long.MAX_VALUE}){
   DefaultCompilerCases object=new DefaultCompilerCases(base);expectedTrace+="ICW";
   if(object.seed!=(int)base||object.operation.applyAsInt(0)!=BigInteger.valueOf((int)base).multiply(BigInteger.valueOf(7)).intValue())throw new AssertionError("wide constructor delegation");rows++;
  }
  DefaultCompilerCases ordinary=new DefaultCompilerCases();expectedTrace+="IC";
  if(ordinary.seed!=7||ordinary.operation.applyAsInt(0)!=49||!expectedTrace.equals(DefaultCompilerCases.trace))throw new AssertionError("initialization/delegation effect order");rows++;
  List<String> methods=new ArrayList<String>();
  for(java.lang.reflect.Method method:DefaultCompilerCases.class.getDeclaredMethods())if(method.isSynthetic())methods.add(method.toGenericString());
  for(java.lang.reflect.Constructor<?> constructor:DefaultCompilerCases.class.getDeclaredConstructors()){
   String signature=constructor.toGenericString();for(java.lang.reflect.Parameter parameter:constructor.getParameters())signature+=":"+parameter.getName()+":"+parameter.getModifiers();methods.add(signature);
  }
  Collections.sort(methods);System.out.println(rows+":constructor-declarations:"+methods);
 }
}`, "35:constructor-declarations:")
}

func TestAdversarialInterfaceLambdaInitializerRetainsOriginalOwner(t *testing.T) {
	assertDefaultCompilerRegistrationOracle(t, `
import java.util.*;import java.util.function.*;import java.math.BigInteger;
interface DefaultCompilerCases {
 IntUnaryOperator WORD=value->value+1;
 Function<Object,String> FORMAT=DefaultCompilerCases::format;
 static String format(Object value){return String.valueOf(value);}
}
class DefaultCompilerDriver {
 public static void main(String[]args)throws Exception{
  int rows=0;
  for(int value:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){if(DefaultCompilerCases.WORD.applyAsInt(value)!=BigInteger.valueOf(value).add(BigInteger.ONE).intValue())throw new AssertionError("interface field word model");rows++;}
  for(Object value:new Object[]{null,"λ",Integer.valueOf(7)}){String expected=value==null?"null":value.toString();if(!expected.equals(DefaultCompilerCases.FORMAT.apply(value)))throw new AssertionError("ordinary method reference binding");rows++;}
  List<String> methods=new ArrayList<String>();
  for(java.lang.reflect.Method method:DefaultCompilerCases.class.getDeclaredMethods())if(method.isSynthetic())methods.add(method.toGenericString());
  Collections.sort(methods);System.out.println(rows+":interface-owner:"+methods);
 }
}`, "8:interface-owner:")
}

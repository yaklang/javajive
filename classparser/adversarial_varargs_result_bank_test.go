package javaclassparser

import (
	"os"
	"testing"
)

// The oracle checks values independently of the reconstructed expression.
// Primitive boundary/raw-NaN bits, target exception identity, pack freshness,
// and the two null boundaries distinguish the result and entry conversions.
const varargsResultBank = `import java.util.function.*;
class ResultKindsTarget {
 static Object pack;static int calls;static boolean fail;
 static final RuntimeException failure=new RuntimeException("result target payload");
 void enter(Object x){if(x==pack)throw new AssertionError("reused result pack");pack=x;calls++;if(fail)throw failure;}
 public boolean bools(boolean... x){enter(x);return x[0];}
 public byte bytes(byte... x){enter(x);return x[0];}
 public char chars(char... x){enter(x);return x[0];}
 public short shorts(short... x){enter(x);return x[0];}
 public int ints(int... x){enter(x);return x[0];}
 public long longs(long... x){enter(x);return x[0];}
 public float floats(float... x){enter(x);return x[0];}
 public double doubles(double... x){enter(x);return x[0];}
 public String strings(String... x){enter(x);return x[0];}
 public String[] arrays(String[]... x){enter(x);return x[0];}
}
interface ResultFloatDouble {double apply(float x);}
interface ResultByteInt {int apply(byte x);}
interface ResultShortLong {long apply(short x);}
interface ResultCharDouble {double apply(char x);}
interface ResultIntFloat {float apply(int x);}
interface ResultLongFloat {float apply(long x);}
class ResultBoolOwner {static Function<Boolean,Boolean> make(ResultKindsTarget x){return x::bools;}}
class ResultByteOwner {static Function<Byte,Byte> make(ResultKindsTarget x){return x::bytes;}}
class ResultCharOwner {static Function<Character,Character> make(ResultKindsTarget x){return x::chars;}}
class ResultShortOwner {static Function<Short,Short> make(ResultKindsTarget x){return x::shorts;}}
class ResultIntOwner {static Function<Integer,Integer> make(ResultKindsTarget x){return x::ints;}}
class ResultLongOwner {static Function<Long,Long> make(ResultKindsTarget x){return x::longs;}}
class ResultFloatOwner {static Function<Float,Float> make(ResultKindsTarget x){return x::floats;}}
class ResultDoubleOwner {static Function<Double,Double> make(ResultKindsTarget x){return x::doubles;}}
class ResultIntLongOwner {static IntToLongFunction make(ResultKindsTarget x){return x::ints;}}
class ResultIntDoubleOwner {static IntToDoubleFunction make(ResultKindsTarget x){return x::ints;}}
class ResultLongDoubleOwner {static LongToDoubleFunction make(ResultKindsTarget x){return x::longs;}}
class ResultFloatDoubleOwner {static ResultFloatDouble make(ResultKindsTarget x){return x::floats;}}
class ResultStringObjectOwner {static Function<String,Object> make(ResultKindsTarget x){return x::strings;}}
class ResultArrayObjectOwner {static Function<String[],Object[]> make(ResultKindsTarget x){return x::arrays;}}
class ResultIntNumberOwner {static IntFunction<Number> make(ResultKindsTarget x){return x::ints;}}
class ResultLongDiscardOwner {static LongConsumer make(ResultKindsTarget x){return x::longs;}}
class ResultDoubleDiscardOwner {static DoubleConsumer make(ResultKindsTarget x){return x::doubles;}}
class ResultByteIntOwner {static ResultByteInt make(ResultKindsTarget x){return x::bytes;}}
class ResultShortLongOwner {static ResultShortLong make(ResultKindsTarget x){return x::shorts;}}
class ResultCharDoubleOwner {static ResultCharDouble make(ResultKindsTarget x){return x::chars;}}
class ResultIntFloatOwner {static ResultIntFloat make(ResultKindsTarget x){return x::ints;}}
class ResultLongFloatOwner {static ResultLongFloat make(ResultKindsTarget x){return x::longs;}}
class ResultKindsDriver {
 static void require(boolean b){if(!b)throw new AssertionError("independent typed result frontier");}
 static void same(Object a,Object b){require(a.getClass()==b.getClass());if(a instanceof Float)require(Float.floatToRawIntBits((Float)a)==Float.floatToRawIntBits((Float)b));else if(a instanceof Double)require(Double.doubleToRawLongBits((Double)a)==Double.doubleToRawLongBits((Double)b));else require(a.equals(b));}
 public static void main(String[] ignored){ResultKindsTarget target=new ResultKindsTarget();int rows=0;
  int[] ints={Integer.MIN_VALUE,-1,0,129,Integer.MAX_VALUE};long[] longs={Long.MIN_VALUE,-1,0,129,Long.MAX_VALUE};
  float[] floats={0f,-0f,Float.MIN_VALUE,Float.MAX_VALUE,Float.intBitsToFloat(0x7fc00001),Float.intBitsToFloat(0xffc01234),Float.POSITIVE_INFINITY,Float.NEGATIVE_INFINITY};
  double[] doubles={0d,-0d,Double.MIN_VALUE,Double.MAX_VALUE,Double.longBitsToDouble(0x7ff8000000000001L),Double.longBitsToDouble(0xfff8000000001234L),Double.POSITIVE_INFINITY,Double.NEGATIVE_INFINITY};
  Object[][] samples={{false,true},{Byte.MIN_VALUE,(byte)-1,(byte)0,(byte)1,Byte.MAX_VALUE},{Character.MIN_VALUE,(char)1,(char)0xd800,(char)0xdfff,Character.MAX_VALUE},{Short.MIN_VALUE,(short)-1,(short)0,(short)1,Short.MAX_VALUE},{ints[0],ints[1],ints[2],ints[3],ints[4]},{longs[0],longs[1],longs[2],longs[3],longs[4]},{floats[0],floats[1],floats[2],floats[3],floats[4],floats[5],floats[6],floats[7]},{doubles[0],doubles[1],doubles[2],doubles[3],doubles[4],doubles[5],doubles[6],doubles[7]}};
  Function[] actions={ResultBoolOwner.make(target),ResultByteOwner.make(target),ResultCharOwner.make(target),ResultShortOwner.make(target),ResultIntOwner.make(target),ResultLongOwner.make(target),ResultFloatOwner.make(target),ResultDoubleOwner.make(target)};
  for(int k=0;k<actions.length;k++){
   for(Object input:samples[k])for(int repeat=0;repeat<2;repeat++){int before=ResultKindsTarget.calls;same(input,actions[k].apply(input));require(ResultKindsTarget.calls==before+1&&java.lang.reflect.Array.getLength(ResultKindsTarget.pack)==1);rows++;}
   int before=ResultKindsTarget.calls;Throwable caught=null;try{actions[k].apply(null);}catch(Throwable t){caught=t;}require(caught instanceof NullPointerException&&ResultKindsTarget.calls==before);rows++;
   before=ResultKindsTarget.calls;caught=null;ResultKindsTarget.fail=true;try{actions[k].apply(samples[k][0]);}catch(Throwable t){caught=t;}finally{ResultKindsTarget.fail=false;}require(caught==ResultKindsTarget.failure&&ResultKindsTarget.calls==before+1);rows++;
  }
  for(int route=0;route<8;route++){int before=ResultKindsTarget.calls;Throwable caught=null;try{switch(route){case 0:ResultBoolOwner.make(null);break;case 1:ResultByteOwner.make(null);break;case 2:ResultCharOwner.make(null);break;case 3:ResultShortOwner.make(null);break;case 4:ResultIntOwner.make(null);break;case 5:ResultLongOwner.make(null);break;case 6:ResultFloatOwner.make(null);break;case 7:ResultDoubleOwner.make(null);break;default:throw new AssertionError();}}catch(Throwable t){caught=t;}require(caught instanceof NullPointerException&&ResultKindsTarget.calls==before);rows++;}
  for(int input:ints){require(ResultIntLongOwner.make(target).applyAsLong(input)==(long)input);rows++;require(Double.doubleToRawLongBits(ResultIntDoubleOwner.make(target).applyAsDouble(input))==Double.doubleToRawLongBits((double)input));rows++;require(ResultIntNumberOwner.make(target).apply(input).longValue()==(long)input);rows++;require(Float.floatToRawIntBits(ResultIntFloatOwner.make(target).apply(input))==Float.floatToRawIntBits((float)input));rows++;}
  for(long input:longs){require(Double.doubleToRawLongBits(ResultLongDoubleOwner.make(target).applyAsDouble(input))==Double.doubleToRawLongBits((double)input));rows++;require(Float.floatToRawIntBits(ResultLongFloatOwner.make(target).apply(input))==Float.floatToRawIntBits((float)input));rows++;ResultLongDiscardOwner.make(target).accept(input);require(((long[])ResultKindsTarget.pack)[0]==input);rows++;}
  for(float input:floats){require(Double.doubleToRawLongBits(ResultFloatDoubleOwner.make(target).apply(input))==Double.doubleToRawLongBits((double)input));rows++;}
  for(double input:doubles){ResultDoubleDiscardOwner.make(target).accept(input);require(Double.doubleToRawLongBits(((double[])ResultKindsTarget.pack)[0])==Double.doubleToRawLongBits(input));rows++;}
  for(Object input:samples[1]){byte x=(Byte)input;require(ResultByteIntOwner.make(target).apply(x)==(int)x);rows++;}
  for(Object input:samples[3]){short x=(Short)input;require(ResultShortLongOwner.make(target).apply(x)==(long)x);rows++;}
  for(Object input:samples[2]){char x=(Character)input;require(ResultCharDoubleOwner.make(target).apply(x)==(double)x);rows++;}
  for(String input:new String[]{null,"",new String("identity")}){require(ResultStringObjectOwner.make(target).apply(input)==input);rows++;}
  for(String[] input:new String[][]{null,new String[0],new String[]{"identity"}}){require(ResultArrayObjectOwner.make(target).apply(input)==input);rows++;}
  require(rows==182);System.out.println("182:result:typed:bits:null:payload:pack");
 }
}`

func TestAdversarialVarargsResultConversionBankPreservesTypedInvocationFrontier(t *testing.T) {
	modern, _ := t04Tools(t)
	owners := []string{"ResultBoolOwner", "ResultByteOwner", "ResultCharOwner", "ResultShortOwner", "ResultIntOwner", "ResultLongOwner", "ResultFloatOwner", "ResultDoubleOwner", "ResultIntLongOwner", "ResultIntDoubleOwner", "ResultLongDoubleOwner", "ResultFloatDoubleOwner", "ResultStringObjectOwner", "ResultArrayObjectOwner", "ResultIntNumberOwner", "ResultLongDiscardOwner", "ResultDoubleDiscardOwner", "ResultByteIntOwner", "ResultShortLongOwner", "ResultCharDoubleOwner", "ResultIntFloatOwner", "ResultLongFloatOwner"}
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required")
				}
			}
			for _, named := range []bool{false, true} {
				label := "absent"
				if named {
					label = "named"
				}
				t.Run(label, func(t *testing.T) {
					testNativeIndependentCompilerFamilyWithParameterFlag(t, func(debug string) map[string][]byte {
						return compileLambdaParameterMetadataFixture(t, compiler, profile, varargsResultBank, "ResultKindsTarget", debug, named)
					}, profile, compiler, owners, "ResultKindsDriver", "182:result:typed:bits:null:payload:pack\n", nil, func(t *testing.T, files map[string][]byte) *JarFS { return nativeEnumOwnUnitArchive(t, files, profile) }, []string{"normal", "no-source-rewrites", "no-core-cleanups"}, named, nativeLexicalExactSignatures, func(t *testing.T, name string, a, b []byte) {
						if x, y := lambdaParameterMetadataTables(t, a), lambdaParameterMetadataTables(t, b); x != y {
							t.Fatalf("result conversion changed original adapter metadata %s\n%s\n%s", name, x, y)
						}
					})
				})
			}
		})
	}
}

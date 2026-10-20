package javaclassparser

import (
	"os"
	"testing"
)

// Independent executable entry model: conversions belong to invocation,
// while the bound receiver's null check belongs to factory creation. Target
// counters, payload identity, exact array element values and fresh pack identity
// distinguish the two boundaries without testing rendered source spelling.
const varargsConvertedEntryBank = `import java.util.function.*;
class ConvertedEntryTarget {
 static Object pack;static ConvertedEntryTarget receiver;static int calls;static long fixed;
 static boolean fail,dispatch;static final RuntimeException failure=new RuntimeException("target payload");
 void enter(Object values){if(values==pack)throw new AssertionError("reused pack");pack=values;receiver=this;calls++;dispatch=false;if(fail)throw failure;}
 public void ints(int...v){enter(v);}
 public void longs(long...v){enter(v);}
 public void bools(boolean...v){enter(v);}
 public void objects(Object...v){enter(v);}
 public void doubles(double...v){enter(v);}
 public void mixed(long prefix,double...v){fixed=prefix;enter(v);}
 public void arrays(Object[]...v){enter(v);}
}
class ConvertedEntryChild extends ConvertedEntryTarget {public void ints(int...v){super.ints(v);dispatch=true;}}
interface ConvertedFloatPort {void accept(float value);}
class ConvertedIntegerOwner {static Consumer<Integer> make(ConvertedEntryTarget x){return x::ints;}}
class ConvertedByteOwner {static Consumer<Byte> make(ConvertedEntryTarget x){return x::ints;}}
class ConvertedShortOwner {static Consumer<Short> make(ConvertedEntryTarget x){return x::longs;}}
class ConvertedCharacterOwner {static Consumer<Character> make(ConvertedEntryTarget x){return x::ints;}}
class ConvertedBooleanOwner {static Consumer<Boolean> make(ConvertedEntryTarget x){return x::bools;}}
class ConvertedStringOwner {static Consumer<String> make(ConvertedEntryTarget x){return x::objects;}}
class ConvertedBoxOwner {static IntConsumer make(ConvertedEntryTarget x){return x::objects;}}
class ConvertedWidenOwner {static IntConsumer make(ConvertedEntryTarget x){return x::doubles;}}
class ConvertedFloatOwner {static ConvertedFloatPort make(ConvertedEntryTarget x){return x::doubles;}}
class ConvertedMixedOwner {static BiConsumer<Integer,Long> make(ConvertedEntryTarget x){return x::mixed;}}
class ConvertedArrayOwner {static Consumer<String[]> make(ConvertedEntryTarget x){return x::arrays;}}
class ConvertedEntryDriver {
 static void require(boolean value){if(!value)throw new AssertionError("independent conversion frontier");}
 static void one(ConvertedEntryTarget x){require(ConvertedEntryTarget.receiver==x);require(java.lang.reflect.Array.getLength(ConvertedEntryTarget.pack)==1);}
 static void observe(int channel,Object input,ConvertedEntryTarget x){one(x);Object p=ConvertedEntryTarget.pack;
  switch(channel){case 0:case 1:require(((int[])p)[0]==((Number)input).intValue());break;
   case 2:require(((long[])p)[0]==((Short)input).longValue());break;
   case 3:require(((int[])p)[0]==((Character)input).charValue());break;
   case 4:require(((boolean[])p)[0]==((Boolean)input).booleanValue());break;
   case 5:require(((Object[])p)[0]==input);break;default:throw new AssertionError();}
  require(ConvertedEntryTarget.dispatch==((channel==0||channel==1||channel==3)&&x instanceof ConvertedEntryChild));
 }
 public static void main(String[]args){int rows=0;
  Object[][] samples={{Integer.MIN_VALUE,-1,0,129,Integer.MAX_VALUE},{Byte.MIN_VALUE,(byte)-1,(byte)0,(byte)1,Byte.MAX_VALUE},{Short.MIN_VALUE,(short)-1,(short)0,(short)1,Short.MAX_VALUE},{Character.MIN_VALUE,(char)1,(char)0xd800,(char)0xdfff,Character.MAX_VALUE},{false,true},{null,"",new String("payload")}};
  for(ConvertedEntryTarget x:new ConvertedEntryTarget[]{new ConvertedEntryTarget(),new ConvertedEntryChild()}){
   Consumer[] actions={ConvertedIntegerOwner.make(x),ConvertedByteOwner.make(x),ConvertedShortOwner.make(x),ConvertedCharacterOwner.make(x),ConvertedBooleanOwner.make(x),ConvertedStringOwner.make(x)};
   for(int channel=0;channel<actions.length;channel++){
    for(Object input:samples[channel])for(int repeat=0;repeat<2;repeat++){actions[channel].accept(input);observe(channel,input,x);rows++;}
    int before=ConvertedEntryTarget.calls;Throwable caught=null;try{actions[channel].accept(null);}catch(Throwable t){caught=t;}
    if(channel<5)require(caught instanceof NullPointerException&&ConvertedEntryTarget.calls==before);else{require(caught==null&&ConvertedEntryTarget.calls==before+1);observe(channel,null,x);}rows++;
    before=ConvertedEntryTarget.calls;caught=null;try{actions[channel].accept(new Object());}catch(Throwable t){caught=t;}require(caught instanceof ClassCastException&&ConvertedEntryTarget.calls==before);rows++;
    ConvertedEntryTarget.fail=true;caught=null;before=ConvertedEntryTarget.calls;try{actions[channel].accept(samples[channel][0]);}catch(Throwable t){caught=t;}finally{ConvertedEntryTarget.fail=false;}require(caught==ConvertedEntryTarget.failure&&ConvertedEntryTarget.calls==before+1);one(x);rows++;
   }
   IntConsumer box=ConvertedBoxOwner.make(x),widen=ConvertedWidenOwner.make(x);
   for(int input:new int[]{Integer.MIN_VALUE,-1,0,129,Integer.MAX_VALUE}){box.accept(input);one(x);require(((Object[])ConvertedEntryTarget.pack)[0].equals(input));rows++;widen.accept(input);one(x);require(Double.doubleToRawLongBits(((double[])ConvertedEntryTarget.pack)[0])==Double.doubleToRawLongBits((double)input));rows++;}
   ConvertedFloatPort floats=ConvertedFloatOwner.make(x);
   for(int bits:new int[]{0,0x80000000,0x7fc00001,0x7f800000,0xff800000}){float input=Float.intBitsToFloat(bits);floats.accept(input);one(x);require(Double.doubleToRawLongBits(((double[])ConvertedEntryTarget.pack)[0])==Double.doubleToRawLongBits((double)input));rows++;}
   BiConsumer<Integer,Long> mixed=ConvertedMixedOwner.make(x);
   for(int a:new int[]{Integer.MIN_VALUE,-1,0,129,Integer.MAX_VALUE})for(long b:new long[]{Long.MIN_VALUE,-1,0,129,Long.MAX_VALUE}){mixed.accept(a,b);one(x);require(ConvertedEntryTarget.fixed==(long)a&&Double.doubleToRawLongBits(((double[])ConvertedEntryTarget.pack)[0])==Double.doubleToRawLongBits((double)b));rows++;}
   Consumer<String[]> arrays=ConvertedArrayOwner.make(x);for(String[] value:new String[][]{null,new String[0],new String[]{"payload"}}){arrays.accept(value);one(x);require(((Object[][])ConvertedEntryTarget.pack)[0]==value);rows++;}
  }
  for(int route=0;route<11;route++){int before=ConvertedEntryTarget.calls;Throwable caught=null;try{switch(route){case 0:ConvertedIntegerOwner.make(null);break;case 1:ConvertedByteOwner.make(null);break;case 2:ConvertedShortOwner.make(null);break;case 3:ConvertedCharacterOwner.make(null);break;case 4:ConvertedBooleanOwner.make(null);break;case 5:ConvertedStringOwner.make(null);break;case 6:ConvertedBoxOwner.make(null);break;case 7:ConvertedWidenOwner.make(null);break;case 8:ConvertedFloatOwner.make(null);break;case 9:ConvertedMixedOwner.make(null);break;case 10:ConvertedArrayOwner.make(null);break;default:throw new AssertionError();}}catch(Throwable t){caught=t;}require(caught instanceof NullPointerException&&ConvertedEntryTarget.calls==before);rows++;}
  require(rows==233);System.out.println(rows+":entry:conversions:null:raw:identity:dispatch");
 }
}`

func TestAdversarialVarargsEntryConversionBankPreservesNullAndRawInvocationBoundaries(t *testing.T) {
	modern, _ := t04Tools(t)
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
						return compileLambdaParameterMetadataFixture(t, compiler, profile, varargsConvertedEntryBank, "ConvertedEntryTarget", debug, named)
					}, profile, compiler, []string{"ConvertedIntegerOwner", "ConvertedByteOwner", "ConvertedShortOwner", "ConvertedCharacterOwner", "ConvertedBooleanOwner", "ConvertedStringOwner", "ConvertedBoxOwner", "ConvertedWidenOwner", "ConvertedFloatOwner", "ConvertedMixedOwner", "ConvertedArrayOwner"}, "ConvertedEntryDriver", "233:entry:conversions:null:raw:identity:dispatch\n", nil, func(t *testing.T, files map[string][]byte) *JarFS { return nativeEnumOwnUnitArchive(t, files, profile) }, []string{"normal", "no-source-rewrites", "no-core-cleanups"}, named, nativeLexicalExactSignatures, func(t *testing.T, name string, a, b []byte) {
						if x, y := lambdaParameterMetadataTables(t, a), lambdaParameterMetadataTables(t, b); x != y {
							t.Fatalf("original typed adapter entry changed%s\n%s\n%s", name, x, y)
						}
					})
				})
			}
		})
	}
}

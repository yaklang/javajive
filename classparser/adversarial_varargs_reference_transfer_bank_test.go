package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// This independent executable bank observes values, identity and timing, not
// source spelling. The original compiler first supplies every expected world;
// rebuilding must preserve the complete method/parameter and lexical family.
const varargsReferenceTransferBank = `import java.util.function.*;
class VarargsTransferTarget {
 static Object lastPack;static VarargsTransferTarget lastTarget;static int calls;static long wideLong,wideDouble;static boolean dispatched;
 void fresh(Object pack){if(pack==lastPack)throw new AssertionError("reused argument pack");lastPack=pack;lastTarget=this;calls++;}
 public Object[] objects(Object...values){fresh(values);dispatched=false;return values;}
 public int ints(int...values){fresh(values);return values.length;}
 public long longs(long...values){fresh(values);return values.length;}
 public double doubles(double...values){fresh(values);return values[0];}
 public boolean bools(boolean...values){fresh(values);return values.length==1;}
 public Object[][] arrays(Object[]...values){fresh(values);return values;}
 public void mixed(long a,double b,Object...values){fresh(values);wideLong=a;wideDouble=Double.doubleToRawLongBits(b);}
}
class VarargsTransferChild extends VarargsTransferTarget {
 @Override public Object[] objects(Object...values){Object[] result=super.objects(values);dispatched=true;return result;}
}
public class VarargsTransferOwner {
 interface Mixed {void apply(long a,double b,Object c);}
 interface Bool {boolean apply(boolean value);}
 static Runnable empty(VarargsTransferTarget x){return x::objects;}
 static Consumer<Object> one(VarargsTransferTarget x){return x::objects;}
 static BiConsumer<Object,Object> two(VarargsTransferTarget x){return x::objects;}
 static Function<Object,Object[]> retained(VarargsTransferTarget x){return x::objects;}
 static IntUnaryOperator ints(VarargsTransferTarget x){return x::ints;}
 static LongUnaryOperator longs(VarargsTransferTarget x){return x::longs;}
 static Runnable emptyLong(VarargsTransferTarget x){return x::longs;}
 static LongConsumer discardLong(VarargsTransferTarget x){return x::longs;}
 static DoubleUnaryOperator doubles(VarargsTransferTarget x){return x::doubles;}
 static DoubleConsumer discardDouble(VarargsTransferTarget x){return x::doubles;}
 static Bool bools(VarargsTransferTarget x){return x::bools;}
 static BiFunction<Object[],Object[],Object[][]> arrays(VarargsTransferTarget x){return x::arrays;}
 static Mixed mixed(VarargsTransferTarget x){return x::mixed;}
}
class VarargsTransferDriver {
 static void require(boolean ok){if(!ok)throw new AssertionError("independent ordered transfer model");}
 static void receiver(VarargsTransferTarget target){require(VarargsTransferTarget.lastTarget==target);}
 static void objectPack(VarargsTransferTarget target,Object first,Object second,int length){
  receiver(target);Object[] pack=(Object[])VarargsTransferTarget.lastPack;require(pack.length==length);
  if(length>0)require(pack[0]==first);if(length>1)require(pack[1]==second);
  require(VarargsTransferTarget.dispatched==(target instanceof VarargsTransferChild));
 }
 public static void main(String[]args){int rows=0;Object shared=new Object(),other=new Object();
  for(VarargsTransferTarget target:new VarargsTransferTarget[]{new VarargsTransferTarget(),new VarargsTransferChild()}){
   VarargsTransferOwner.empty(target).run();objectPack(target,null,null,0);rows++;
   VarargsTransferOwner.emptyLong(target).run();receiver(target);require(((long[])VarargsTransferTarget.lastPack).length==0);rows++;
   for(Object a:new Object[]{null,shared,other})for(Object b:new Object[]{null,shared,other}){
    VarargsTransferOwner.one(target).accept(a);objectPack(target,a,null,1);rows++;
    VarargsTransferOwner.two(target).accept(a,b);objectPack(target,a,b,2);rows++;
    Object[] result=VarargsTransferOwner.retained(target).apply(a);objectPack(target,a,null,1);require(result==VarargsTransferTarget.lastPack);rows++;
    Object[] left=a==null?null:new Object[]{a},right=b==null?left:new Object[]{b};
    Object[][] outer=VarargsTransferOwner.arrays(target).apply(left,right);receiver(target);require(outer==VarargsTransferTarget.lastPack&&outer.length==2&&outer[0]==left&&outer[1]==right);rows++;
   }
   for(int value:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){require(VarargsTransferOwner.ints(target).applyAsInt(value)==1);receiver(target);int[] pack=(int[])VarargsTransferTarget.lastPack;require(pack.length==1&&pack[0]==value);rows++;}
   for(long value:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){require(VarargsTransferOwner.longs(target).applyAsLong(value)==1);receiver(target);long[] pack=(long[])VarargsTransferTarget.lastPack;require(pack.length==1&&pack[0]==value);rows++;}
   for(long value:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){VarargsTransferOwner.discardLong(target).accept(value);receiver(target);long[] pack=(long[])VarargsTransferTarget.lastPack;require(pack.length==1&&pack[0]==value);rows++;}
   for(long bits:new long[]{0L,0x8000000000000000L,0x7ff8000000000001L,0x7ff0000000000000L,0xfff0000000000000L}){double value=Double.longBitsToDouble(bits);double result=VarargsTransferOwner.doubles(target).applyAsDouble(value);receiver(target);double[] pack=(double[])VarargsTransferTarget.lastPack;require(pack.length==1&&Double.doubleToRawLongBits(pack[0])==bits&&Double.doubleToRawLongBits(result)==bits);rows++;VarargsTransferOwner.discardDouble(target).accept(value);receiver(target);double[] discarded=(double[])VarargsTransferTarget.lastPack;require(discarded.length==1&&Double.doubleToRawLongBits(discarded[0])==bits);rows++;}
   for(boolean value:new boolean[]{false,true}){require(VarargsTransferOwner.bools(target).apply(value));receiver(target);boolean[] pack=(boolean[])VarargsTransferTarget.lastPack;require(pack.length==1&&pack[0]==value);rows++;}
   long[] longs={Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE};long[] doubles={0L,0x8000000000000000L,0x7ff8000000000001L,0x7ff0000000000000L,0xfff0000000000000L};
   for(int i=0;i<longs.length;i++){VarargsTransferOwner.mixed(target).apply(longs[i],Double.longBitsToDouble(doubles[i]),shared);receiver(target);Object[] pack=(Object[])VarargsTransferTarget.lastPack;require(pack.length==1&&pack[0]==shared&&VarargsTransferTarget.wideLong==longs[i]&&VarargsTransferTarget.wideDouble==doubles[i]);rows++;}
  }
  for(int route=0;route<13;route++){Throwable caught=null;int before=VarargsTransferTarget.calls;try{switch(route){case 0:VarargsTransferOwner.empty(null);break;case 1:VarargsTransferOwner.one(null);break;case 2:VarargsTransferOwner.two(null);break;case 3:VarargsTransferOwner.retained(null);break;case 4:VarargsTransferOwner.ints(null);break;case 5:VarargsTransferOwner.longs(null);break;case 6:VarargsTransferOwner.bools(null);break;case 7:VarargsTransferOwner.arrays(null);break;case 8:VarargsTransferOwner.mixed(null);break;case 9:VarargsTransferOwner.emptyLong(null);break;case 10:VarargsTransferOwner.discardLong(null);break;case 11:VarargsTransferOwner.doubles(null);break;case 12:VarargsTransferOwner.discardDouble(null);break;default:throw new AssertionError();}}catch(Throwable error){caught=error;}require(caught instanceof NullPointerException&&VarargsTransferTarget.calls==before);rows++;}
  System.out.println(rows+":typed-varargs:ports:freshness:dispatch");
 }
}`

func TestAdversarialVarargsReferenceTransferBankPreservesTypedPortsAndArrayIdentity(t *testing.T) {
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
					fixture := strings.ReplaceAll(varargsReferenceTransferBank, "VarargsTransferOwner", "RenamedVarargsTransferOwner")
					testNativeIndependentCompilerFamilyWithParameterFlag(t, func(debug string) map[string][]byte {
						return compileLambdaParameterMetadataFixture(t, compiler, profile, fixture, "RenamedVarargsTransferOwner", debug, named)
					}, profile, compiler, []string{"RenamedVarargsTransferOwner"}, "VarargsTransferDriver", "153:typed-varargs:ports:freshness:dispatch\n", nil, func(t *testing.T, files map[string][]byte) *JarFS { return nativeEnumOwnUnitArchive(t, files, profile) }, []string{"normal", "no-source-rewrites", "no-core-cleanups"}, named, nativeLexicalExactSignatures, func(t *testing.T, name string, a, b []byte) {
						if x, y := lambdaParameterMetadataTables(t, a), lambdaParameterMetadataTables(t, b); x != y {
							t.Fatalf("original receiver/fixed/packed roles changed %s\n%s\n%s", name, x, y)
						}
					})
				})
			}
		})
	}
}

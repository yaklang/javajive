package javaclassparser

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

func nativeVisibilityBridgeParameterABI(t *testing.T, name string, original, rebuilt []byte) {
	t.Helper()
	shape := func(raw []byte) string {
		obj, e := Parse(append([]byte(nil), raw...))
		if e != nil {
			t.Fatal(e)
		}
		rows := []string{}
		for _, m := range obj.Methods {
			if m.AccessFlags&0x40 == 0 {
				continue
			}
			n, _ := sourceBridgeUTF8(obj, m.NameIndex)
			d, _ := sourceBridgeUTF8(obj, m.DescriptorIndex)
			info := "absent"
			for _, a := range m.Attributes {
				if p, ok := a.(*UnparsedAttribute); ok && p.Name == "MethodParameters" {
					info = fmt.Sprintf("%x", p.Info)
				}
			}
			rows = append(rows, n+d+":"+info)
		}
		sort.Strings(rows)
		return strings.Join(rows, "\n")
	}
	if a, b := shape(original), shape(rebuilt); a != b {
		t.Fatalf("bridge parameter metadata %s\noriginal %s\nrebuilt %s", name, a, b)
	}
}
func testNativeVisibilityBridgeEffects(t *testing.T, native bool) {
	for _, owner := range []string{"BridgeArithmetic", "RenamedBridgeArithmetic"} {
		t.Run(owner, func(t *testing.T) {
			source := `public class BridgeArithmetic<T extends Comparable<T>> extends HiddenArithmetic<T>{
 public class Gate {
  public long run(Object input,long a,double d,float f,Object[] payload){Comparable v=(Comparable)input;return BridgeArithmetic.this.fold(a,d,(T)v,HiddenArithmetic.mark(2),f,payload);}
  public void take(Object input){BridgeArithmetic.this.consume((T)(Comparable)input);}
  public Object read(Object input){return BridgeArithmetic.this.echo((T)(Comparable)input);}
 }
}
class HiddenArithmetic<X extends Comparable<X>>{
 static int trace,fail;static final RuntimeException fault=new IllegalStateException("original");
 static int mark(int phase){trace=trace*10+phase;if(fail==phase)throw fault;return phase;}
 public long fold(long a,double d,X item,int b,float f,Object[] payload){mark(3);return a+(long)d+b+(long)f+payload.length+(item==null?0:1);}
 public void consume(X item){mark(5);}
 public X echo(X item){mark(7);return item;}
}
class BridgeArithmeticDriver{public static void main(String[]args)throws Exception{int count=0;
 if(!BridgeArithmetic.class.getDeclaredMethod("fold",long.class,double.class,Comparable.class,int.class,float.class,Object[].class).isBridge())throw new AssertionError("missing public bridge");
 for(long a:new long[]{Long.MIN_VALUE,-1,0,Long.MAX_VALUE})for(double d:new double[]{Double.NEGATIVE_INFINITY,-1.0,Double.NaN,Double.POSITIVE_INFINITY})for(float f:new float[]{Float.NEGATIVE_INFINITY,-1.0f,Float.NaN,Float.POSITIVE_INFINITY})for(int phase:new int[]{0,2,3,5,7}){
 HiddenArithmetic.trace=0;HiddenArithmetic.fail=phase;String item=new String("identity");Object[]payload={item,item};BridgeArithmetic<String> root=new BridgeArithmetic<String>();BridgeArithmetic<String>.Gate gate=root.new Gate();
 try{long value=gate.run(item,a,d,f,payload);long want=java.math.BigInteger.valueOf(a).add(java.math.BigInteger.valueOf((long)d)).add(java.math.BigInteger.valueOf(2)).add(java.math.BigInteger.valueOf((long)f)).add(java.math.BigInteger.valueOf(payload.length)).add(java.math.BigInteger.ONE).longValue();if(value!=want)throw new AssertionError("wide/overflow/array inputs");gate.take(item);if(gate.read(item)!=item||phase!=0)throw new AssertionError("identity/completion");}
 catch(RuntimeException ex){if(phase==0||ex!=HiddenArithmetic.fault)throw new AssertionError("exception identity");}
 int wantTrace=phase==2?2:phase==3?23:phase==5?235:2357;if(HiddenArithmetic.trace!=wantTrace)throw new AssertionError("argument/target/void/reference order");count++;}
 System.out.println(count+":bridge:wide:overflow:effects:identity");}}
`
			source = strings.ReplaceAll(source, "BridgeArithmetic", owner)
			if native {
				compiler := os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required")
				}
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte { return nativePrivateEnumCompile(t, source, owner, debug) }, NativeJavac8, compiler, []string{owner}, owner+"Driver", "320:bridge:wide:overflow:effects:identity\n", nil, nativeLexicalExactSignatures, nativeCovariantBridgeABI, nativeVisibilityBridgeParameterABI)
			} else {
				testNativePrivateSetterCompiledFixture(t, owner, owner+"Driver", "320:bridge:wide:overflow:effects:identity\n", func(t *testing.T, debug string) map[string][]byte {
					return nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": source}, debug, "8")
				}, nativeLexicalExactSignatures, nativeCovariantBridgeABI, nativeVisibilityBridgeParameterABI)
			}
		})
	}
}
func TestAdversarialVisibilityBridgeEffects(t *testing.T) {
	testNativeVisibilityBridgeEffects(t, false)
}
func TestAdversarialVisibilityBridgeEffectsNativeCompiler(t *testing.T) {
	testNativeVisibilityBridgeEffects(t, true)
}

package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// This source is intentionally legal and constructor scheduling is changed
// only in the authored ORIGINAL fixture below. The unchanged caller is compiled
// before that change. It observes pre-SUPER captures through virtual callbacks,
// an escaped receiver after failure, and a further subtype's finalizer dispatch.
const constructorLoweringFixture = `class LoweringWitness {
 static Object token;static long wide;static double floating;static int[] array;static boolean flag;
 static LoweringParent published;static RuntimeException failure;static int finalized;
 static void check(LoweringParent p){
  if(p.token()!=token||p.wide()!=wide||Double.doubleToRawLongBits(p.floating())!=Double.doubleToRawLongBits(floating)||p.array()!=array||p.flag()!=flag)throw new AssertionError("pre-super captures");
  long want=java.math.BigInteger.valueOf(wide).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(7)).longValue();
  if(p.mix()!=want)throw new AssertionError("overflow oracle");
 }
}
abstract class LoweringParent {
 LoweringParent(Object input,int mode){LoweringWitness.check(this);LoweringWitness.published=this;if(mode==1)throw LoweringWitness.failure;if(input==null)throw new AssertionError("normal input");}
 abstract Object token();abstract long wide();abstract double floating();abstract int[] array();abstract boolean flag();abstract long mix();
 protected void finalize(){LoweringWitness.check(this);LoweringWitness.finalized++;}
 public void finish(){finalize();}
}
public class LoweringChild extends LoweringParent {
 final Object capturedToken;final long capturedWide;final double capturedFloating;final int[] capturedArray;final boolean capturedFlag;
 public LoweringChild(Object input,int mode,Object token,long wide,double floating,int[] array,boolean flag){super(input,mode);capturedToken=token;capturedWide=wide;capturedFloating=floating;capturedArray=array;capturedFlag=flag;}
 Object token(){return capturedToken;}long wide(){return capturedWide;}double floating(){return capturedFloating;}int[] array(){return capturedArray;}boolean flag(){return capturedFlag;}long mix(){return capturedWide*31L+7L;}
}
class LoweringDerived extends LoweringChild {
 LoweringDerived(Object input,int mode,Object token,long wide,double floating,int[] array,boolean flag){super(input,mode,token,wide,floating,array,flag);}
 protected void finalize(){LoweringWitness.check(this);LoweringWitness.finalized+=3;}
}
class LoweringDriver {
 public static void main(String[]args){int rows=0;
 for(Object token:new Object[]{new Object(),null,new IllegalArgumentException("token")})
 for(long wide:new long[]{Long.MIN_VALUE,-1,0,Long.MAX_VALUE})
 for(double floating:new double[]{-0.0,0.0,Double.longBitsToDouble(0x7ff8000000000042L),Double.NEGATIVE_INFINITY,Double.MAX_VALUE})
 for(int[] array:new int[][]{null,new int[0],new int[]{Integer.MIN_VALUE,7,Integer.MAX_VALUE}})
 for(boolean flag:new boolean[]{false,true})for(int mode=0;mode<2;mode++)for(int derived=0;derived<2;derived++){
  LoweringWitness.token=token;LoweringWitness.wide=wide;LoweringWitness.floating=floating;LoweringWitness.array=array;LoweringWitness.flag=flag;
  LoweringWitness.failure=new IllegalArgumentException("failure:"+rows);LoweringWitness.published=null;LoweringWitness.finalized=0;
  Object input=mode==0?new Object():null;Throwable error=null;LoweringParent value=null;
  try{value=derived==0?new LoweringChild(input,mode,token,wide,floating,array,flag):new LoweringDerived(input,mode,token,wide,floating,array,flag);}catch(Throwable e){error=e;}
  if(mode==0?(error!=null||value!=LoweringWitness.published):(error!=LoweringWitness.failure||value!=null))throw new AssertionError("abrupt/receiver/exception identity",error);
  LoweringWitness.check(LoweringWitness.published);LoweringWitness.published.finish();
  if(LoweringWitness.finalized!=(derived==0?1:3))throw new AssertionError("subtype finalizer dispatch");rows++;
 }System.out.println(rows+":captures:callback:publication:abrupt:subtype:wide:array:identity");}
}`

// Independent fixture construction: javac's first six bytes are this/input/
// mode/SUPER. Its remaining five field assignments are a closed trailing block.
// The test moves those fresh authored assignments to the prefix, as javac8
// does for anonymous captures. It does not call the production lowering proof.
func constructorLoweringOriginal(t *testing.T, classes map[string][]byte) map[string][]byte {
	t.Helper()
	original := make(map[string][]byte, len(classes))
	for name, raw := range classes {
		original[name] = bytes.Clone(raw)
	}
	// The class parser can retain Code slices of its input. A fixture mutation
	// must not mutate the separately retained legal compiler negative control.
	object, err := Parse(bytes.Clone(classes["LoweringChild.class"]))
	if err != nil {
		t.Fatal(err)
	}
	if len(object.Fields) != 5 {
		t.Fatal("independent authored field inventory")
	}
	for _, field := range object.Fields {
		field.AccessFlags |= syntheticFieldFlag
	}
	found := 0
	for _, method := range object.Methods {
		name, _ := object.getUtf8(method.NameIndex)
		if name != "<init>" {
			continue
		}
		found++
		code := constructorLoweringCode(method)
		if code == nil || len(code.ExceptionTable) != 0 || len(code.Code) != 36 || !bytes.Equal(code.Code[:3], []byte{0x2a, 0x2b, 0x1c}) || code.Code[3] != 0xb7 || code.Code[35] != 0xb1 {
			t.Fatalf("independent authored compiler packet changed: %x", code.Code)
		}
		fresh := bytes.Clone(code.Code)
		copy(code.Code, fresh[6:35])
		copy(code.Code[29:], fresh[:6])
		mapPC := func(pc uint16) uint16 {
			if pc < 6 {
				return pc + 29
			}
			if pc < 35 {
				return pc - 6
			}
			return pc
		}
		for _, attr := range code.Attributes {
			if lines, ok := attr.(*LineNumberTableAttribute); ok {
				for _, row := range lines.LineNumberTable {
					row.StartPc = mapPC(row.StartPc)
				}
			}
		}
	}
	if found != 1 {
		t.Fatal("independent original constructor count", found)
	}
	original["LoweringChild.class"] = object.Bytes()
	return original
}

func testConstructorLoweringRoundTrip(t *testing.T, compiler, java string, profile SourceCompilerProfile, debug string) {
	t.Helper()
	classes := compileLambdaParameterMetadataFixture(t, compiler, profile, constructorLoweringFixture, "LoweringChild", debug, false)
	original := constructorLoweringOriginal(t, classes)
	const want = "1440:captures:callback:publication:abrupt:subtype:wide:array:identity\n"
	run := func(directory string, succeeds bool) {
		t.Helper()
		for _, assertions := range []string{"-ea", "-da"} {
			out, err := exec.Command(java, assertions, "-Xverify:all", "-cp", directory, "LoweringDriver").CombinedOutput()
			if succeeds && (err != nil || string(out) != want) {
				t.Fatalf("independent runtime %s: %v\n%s", assertions, err, out)
			}
			if !succeeds && (err == nil || !strings.Contains(string(out), "pre-super captures")) {
				t.Fatalf("unlowered negative control inactive: %v\n%s", err, out)
			}
		}
	}
	run(sourceArtifactWriteFiles(t, original), true)
	jar := filepath.Join(t.TempDir(), "original.jar")
	if err := os.WriteFile(jar, t23Zip(t, map[string][]byte{"LoweringChild.class": original["LoweringChild.class"]}), 0600); err != nil {
		t.Fatal(err)
	}
	resolve := func(binary string) ([]byte, bool) { raw, ok := original[binary+".class"]; return raw, ok }
	plain, err := NewJarSourceArtifactFromLocal(jar, 8, profile, resolve)
	if err != nil {
		t.Fatal(err)
	}
	defer plain.Close()
	refused, err := plain.ReadFile("LoweringChild.class")
	if err != nil || !strings.Contains(string(refused), DecompileStubMarker) {
		t.Fatalf("ordinary source must retain receiver-observation gate: %v\n%s", err, refused)
	}
	a, err := NewJarReconstructionArtifactFromLocal(jar, 8, profile, resolve)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	source, err := a.ReadFile("LoweringChild.class")
	if err != nil || legacySourceHasFailureMarker(string(source)) {
		t.Fatalf("explicit reconstruction source: %v\n%s", err, source)
	}
	dependencies := map[string][]byte{}
	for name, raw := range original {
		if name != "LoweringChild.class" {
			dependencies[name] = raw
		}
	}
	out := sourceArtifactWriteFiles(t, dependencies)
	path := filepath.Join(out, "LoweringChild.java")
	if err := os.WriteFile(path, source, 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"-proc:none", "--release", "8", "-g:" + debug}
	if profile == NativeJavac8 {
		args = []string{"-proc:none", "-source", "8", "-target", "8", "-g:" + debug}
	}
	args = append(args, "-cp", out, "-sourcepath", out, "-d", out, path)
	if log, err := exec.Command(compiler, args...).CombinedOutput(); err != nil {
		t.Fatalf("compiler without original subject: %v\n%s\n%s", err, log, source)
	}
	run(out, false)
	compiled, err := os.ReadFile(filepath.Join(out, "LoweringChild.class"))
	if err != nil {
		t.Fatal(err)
	}
	input := bytes.Clone(compiled)
	finished, err := a.FinishClass("LoweringChild.class", compiled)
	if err != nil || !bytes.Equal(input, compiled) {
		t.Fatalf("complete reconstruction or input immutability: %v", err)
	}
	if err := sourceMethodMetadataClosed(mustParseLowering(t, original["LoweringChild.class"]), finished, nil); err != nil {
		t.Fatal("complete original method ABI", err)
	}
	wantFields, _ := sourceFieldIdentities(mustParseLowering(t, original["LoweringChild.class"]), nil)
	gotFields, _ := sourceFieldIdentities(mustParseLowering(t, finished), nil)
	if !equalLoweringFields(wantFields, gotFields) {
		t.Fatal("complete original field ABI")
	}
	before, after := mustParseLowering(t, compiled), mustParseLowering(t, finished)
	if before.AccessFlags != after.AccessFlags || len(before.Methods) != len(after.Methods) {
		t.Fatal("unlicensed declaration change")
	}
	for i, method := range before.Methods {
		name, _ := before.getUtf8(method.NameIndex)
		if name != "<init>" && !bytes.Equal(constructorLoweringCode(method).Code, constructorLoweringCode(after.Methods[i]).Code) {
			t.Fatal("lowering modified a nonconstructor", name)
		}
	}
	again, err := a.FinishClass("LoweringChild.class", finished)
	if err != nil || !bytes.Equal(finished, again) {
		t.Fatal("idempotent completion", err)
	}
	if err := os.WriteFile(filepath.Join(out, "LoweringChild.class"), finished, 0600); err != nil {
		t.Fatal(err)
	}
	run(out, true)
}

func mustParseLowering(t *testing.T, raw []byte) *ClassObject {
	t.Helper()
	obj, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return obj
}

func equalLoweringFields(a, b map[sourceFieldIdentity]uint16) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestAdversarialConstructorLoweringPreservesOriginalPreSuperCaptures(t *testing.T) {
	modern, java := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		compiler, runtime := modern, java
		if profile == NativeJavac8 {
			compiler = os.Getenv("JAVA8_JAVAC")
			if originalRuntime := os.Getenv("JAVA8_JAVA"); originalRuntime != "" {
				runtime = originalRuntime
			}
			if compiler == "" {
				t.Fatal("native compiler required")
			}
		}
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(string(profile)+"/"+debug, func(t *testing.T) {
				testConstructorLoweringRoundTrip(t, compiler, runtime, profile, debug)
			})
		}
	}
}

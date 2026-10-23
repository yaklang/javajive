package javaclassparser

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const constructorSuffixFixture = `class SuffixWitness {static Object token;static long seed;static SuffixBase escaped;static int visits;static RuntimeException failure;}
abstract class SuffixBase {
 SuffixBase(Object input){SuffixWitness.escaped=this;if(value()!=SuffixWitness.token||seed()!=SuffixWitness.seed||answer()!=0||SuffixWitness.visits!=0)throw new AssertionError("constructor cut observations");if(input==null)throw SuffixWitness.failure;}
 abstract Object value();abstract long seed();abstract long answer();}
class SuffixNumber {final long value;SuffixNumber(long value){this.value=value;}}
public class SuffixOwner {SuffixBase make(Object input,final Object token,final long seed,final int mode){return new SuffixBase(input){
 long result;{BODY}
 Object value(){return token;}long seed(){return seed;}long answer(){return result;}
 };}}
class SuffixDriver {public static void main(String[]args){int rows=0;SuffixOwner owner=new SuffixOwner();
 for(Object token:new Object[]{null,"token",new Object()})for(long seed:new long[]{Long.MIN_VALUE,-1,0,Long.MAX_VALUE})for(int mode:new int[]{-3,0,4})for(boolean fail:new boolean[]{false,true}){
 SuffixWitness.token=token;SuffixWitness.seed=seed;SuffixWitness.escaped=null;SuffixWitness.visits=0;SuffixWitness.failure=new IllegalArgumentException("identity");
 Throwable error=null;SuffixBase result=null;try{result=owner.make(fail?null:new Object(),token,seed,mode);}catch(Throwable t){error=t;}
 java.math.BigInteger oracle=java.math.BigInteger.valueOf(seed);for(int i=0;i<ROUNDS;i++)oracle=oracle.multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(mode));
 if(error!=(fail?SuffixWitness.failure:null)||SuffixWitness.escaped==null||SuffixWitness.escaped.value()!=token||SuffixWitness.escaped.seed()!=seed||SuffixWitness.escaped.answer()!=(fail?0:oracle.longValue())||SuffixWitness.visits!=(fail?0:1)||!fail&&result!=SuffixWitness.escaped)throw new AssertionError("capture/suffix/abrupt/order/numeric",error);
 rows++;}System.out.println(rows+":constructor:closed:suffix:oracle");}}
`

func TestAdversarialConstructorLoweringRetainsClosedControlFlowSuffix(t *testing.T) {
	for _, kind := range []string{"linear", "loop", "switch", "handler", "generic", "uninitialized"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(kind+"/"+debug, func(t *testing.T) {
				body, rounds := "", "1"
				switch kind {
				case "linear":
					body = `result=seed*31L+mode;SuffixWitness.visits++;`
				case "loop":
					body = `long r=seed;for(int k=0;k<5;k++){r=r*31L+mode;}result=r;SuffixWitness.visits++;`
					rounds = "5"
				case "switch":
					body = `switch(mode){case -3:result=seed*31L-3;break;case 0:result=seed*31L;break;default:result=seed*31L+mode;}SuffixWitness.visits++;`
				case "handler":
					body = `try{if(mode==0)throw SuffixWitness.failure;result=seed*31L+mode;}catch(RuntimeException e){if(e!=SuffixWitness.failure)throw e;result=seed*31L;}finally{SuffixWitness.visits++;}`
				case "generic":
					body = `java.util.List<String> tmp=new java.util.ArrayList<String>();tmp.add(""+mode);result=seed*31L+Long.parseLong(tmp.get(0));SuffixWitness.visits++;`
				case "uninitialized":
					body = `result=new SuffixNumber(mode==0?seed*31L:seed*31L+mode).value;SuffixWitness.visits++;`
				}
				fixture := strings.ReplaceAll(strings.Replace(constructorSuffixFixture, "BODY", body, 1), "ROUNDS", rounds)
				constructorSuffixRoundTrip(t, fixture, debug, "SuffixOwner$1", "72:constructor:closed:suffix:oracle\n")
			})
		}
	}
}

func constructorSuffixRoundTrip(t *testing.T, fixture, debug, owner, expected string, target ...int) ([]byte, []byte, *JarReconstructionArtifact) {
	t.Helper()
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("original native compiler required")
	}
	release := 8
	if len(target) == 1 {
		release = target[0]
	}
	original := compileLambdaParameterMetadataFixtureWithSourcesAtRelease(t, native, NativeJavac8, fixture, "SuffixOwner", debug, nil, release, false)
	if int(mustParseLowering(t, original[owner+".class"]).MajorVersion) != 44+release {
		t.Fatal("original compiler target mismatch")
	}
	run := func(dir string) {
		t.Helper()
		for _, mode := range []string{"-ea", "-da"} {
			out, err := enumFreshOracleRun(t, java, mode, dir, "SuffixDriver")
			if err != nil || string(out) != expected {
				t.Fatal("unchanged original suffix oracle", err, string(out))
			}
		}
	}
	run(sourceArtifactWriteFiles(t, original))
	jar := filepath.Join(t.TempDir(), "original.jar")
	if err := os.WriteFile(jar, t23Zip(t, map[string][]byte{owner + ".class": original[owner+".class"]}), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, func(n string) ([]byte, bool) { b, k := original[n+".class"]; return b, k })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	source, err := a.ReadFile(owner + ".class")
	if err != nil || legacySourceHasFailureMarker(string(source)) {
		t.Fatal("complete independent constructor source", err, string(source))
	}
	deps := map[string][]byte{}
	for n, b := range original {
		if n != owner+".class" {
			deps[n] = b
		}
	}
	out := sourceArtifactWriteFiles(t, deps)
	// The test reconstructs one independent physical unit. Do not let a
	// dependency's redundant lexical row enroll that unit as a nonstatic
	// member again and make javac add a second hidden enclosing parameter.
	// This is compiler-only dependency metadata: all original dependency bytes,
	// including the unchanged factory and oracle, are restored before execution.
	for name, raw := range deps {
		obj := mustParseLowering(t, raw)
		changed := false
		for _, attr := range obj.Attributes {
			if table, ok := attr.(*InnerClassesAttribute); ok {
				kept := table.Classes[:0]
				for _, row := range table.Classes {
					binary, _ := sourceBridgeClassName(obj, row.InnerClassInfoIndex)
					if binary == owner {
						changed = true
					} else {
						kept = append(kept, row)
					}
				}
				table.Classes = kept
				table.NumberOfClasses = uint16(len(kept))
				table.AttrLen = uint32(2 + len(kept)*8)
			}
		}
		if changed {
			compilerDependency := obj.Bytes()
			if _, err := Parse(compilerDependency); err != nil {
				t.Fatal("invalid compile-only lexical dependency", err)
			}
			if err := os.WriteFile(filepath.Join(out, name), compilerDependency, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	path := filepath.Join(out, owner+".java")
	if err := os.WriteFile(path, source, 0600); err != nil {
		t.Fatal(err)
	}
	if log, err := exec.Command(modern, "-proc:none", "--release", "8", "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out, path).CombinedOutput(); err != nil {
		t.Fatal("fresh constructor suffix compilation", err, string(log), string(source))
	}
	path = filepath.Join(out, owner+".class")
	fresh, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	saved := bytes.Clone(fresh)
	finished, err := a.FinishClass(owner+".class", fresh)
	if err != nil {
		for label, raw := range map[string][]byte{"original": original[owner+".class"], "fresh": fresh} {
			obj := mustParseLowering(t, raw)
			for _, method := range obj.Methods {
				name, _ := sourceBridgeUTF8(obj, method.NameIndex)
				if name == "<init>" {
					desc, _ := sourceBridgeUTF8(obj, method.DescriptorIndex)
					t.Logf("%s constructor descriptor %s", label, desc)
				}
			}
		}
		t.Fatal("constructor suffix completion", err, string(source))
	}
	if !bytes.Equal(fresh, saved) {
		t.Fatal("input mutated")
	}
	if again, err := a.FinishClass(owner+".class", finished); err != nil || !bytes.Equal(again, finished) {
		t.Fatal("completion not idempotent", err)
	}
	if err := os.WriteFile(path, finished, 0600); err != nil {
		t.Fatal(err)
	}
	for name, raw := range deps {
		if err := os.WriteFile(filepath.Join(out, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	run(out)
	return original[owner+".class"], fresh, a
}

func TestAdversarialConstructorLoweringOriginalVersionsRetainCaptureOrder(t *testing.T) {
	for _, release := range []int{5, 6, 7, 8} {
		for _, kind := range []string{"anonymous loop", "static reads", "reference widening"} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(fmt.Sprintf("release%d/%s/%s", release, kind, debug), func(t *testing.T) {
					owner := "SuffixOwner$Member"
					fixture := constructorStaticArgumentFixture()
					switch kind {
					case "anonymous loop":
						owner = "SuffixOwner$1"
						fixture = strings.ReplaceAll(strings.Replace(constructorSuffixFixture, "BODY", `long r=seed;for(int k=0;k<5;k++)r=r*31L+mode;result=r;SuffixWitness.visits++;`, 1), "ROUNDS", "5")
					case "reference widening":
						fixture = constructorWideningFixture("parameter")
					}
					constructorSuffixRoundTrip(t, fixture, debug, owner, "72:constructor:closed:suffix:oracle\n", release)
				})
			}
		}
	}
}

func TestAdversarialConstructorLoweringKeepsStaticArgumentReads(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			fixture := constructorStaticArgumentFixture()
			constructorSuffixRoundTrip(t, fixture, debug, "SuffixOwner$Member", "72:constructor:closed:suffix:oracle\n")
		})
	}
}

const constructorStaticFailureFixture = `class SuffixWitness {
 static int first,second,entered,body;static boolean fail=true;
 static RuntimeException failure=new IllegalArgumentException("same failure");}
class SuffixOptions {static volatile int first=init();static int init(){SuffixWitness.first++;return 37;}}
class SuffixOther {static volatile long second=init();static long init(){
 if(SuffixWitness.first!=1)throw new AssertionError("field read order");SuffixWitness.second++;
 if(SuffixWitness.fail)throw SuffixWitness.failure;return Long.MIN_VALUE;}}
class SuffixBase {SuffixBase(int first,long second){SuffixWitness.entered++;}}
public class SuffixOwner {
 SuffixBase make(){return new Member();}
 class Member extends SuffixBase {Member(){super(SuffixOptions.first,SuffixOther.second);SuffixWitness.body++;}Object owner(){return SuffixOwner.this;}}
}
class SuffixDriver {public static void main(String[]args){SuffixOwner owner=new SuffixOwner();
 for(int i=0;i<2;i++){
 Throwable caught=null;try{owner.make();}catch(Throwable t){caught=t;}
 if(i==0?!(caught instanceof ExceptionInInitializerError)||caught.getCause()!=SuffixWitness.failure:!(caught instanceof NoClassDefFoundError))throw new AssertionError("class initialization abrupt identity",caught);
 if(SuffixWitness.first!=1||SuffixWitness.second!=1||SuffixWitness.entered!=0||SuffixWitness.body!=0)throw new AssertionError("class initialization count/order");
 }System.out.println("2:constructor:static:initialization");}}
`

func TestAdversarialConstructorStaticOperandsKeepInitializationFailure(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			constructorSuffixRoundTrip(t, constructorStaticFailureFixture, debug, "SuffixOwner$Member", "2:constructor:static:initialization\n")
		})
	}
}

func constructorStaticArgumentFixture() string {
	fixture := strings.ReplaceAll(strings.Replace(constructorSuffixFixture, "BODY", `result=seed*31L+mode;SuffixWitness.visits++;`, 1), "ROUNDS", "1")
	fixture = strings.Replace(fixture, "SuffixBase(Object input){", "SuffixBase(Object input,int tag,long wide,Object reference,int[] array){if(tag!=SuffixWitness.tag||wide!=SuffixWitness.wide||reference!=SuffixWitness.reference||array!=SuffixWitness.array)throw new AssertionError(\"static argument identities\");", 1)
	// A named inner constructor evaluates SUPER operands inside its
	// own initialization prefix. Anonymous constructor arguments are
	// evaluated by the factory and merely passed as hidden parameters.
	start := strings.Index(fixture, "public class SuffixOwner")
	end := strings.Index(fixture, "class SuffixDriver")
	fixture = fixture[:start] + `public class SuffixOwner {Object token;long seed;
 SuffixBase make(Object input,final Object token,final long seed,final int mode){this.token=token;this.seed=seed;return new Member(input,seed,mode);}
 class Member extends SuffixBase {long result;Member(Object input,long seed,int mode){super(input,SuffixOptions.tag,SuffixOptions.wide,SuffixOptions.reference,SuffixOptions.array);result=seed*31L+mode;SuffixWitness.visits++;}
 Object value(){return SuffixOwner.this.token;}long seed(){return SuffixOwner.this.seed;}long answer(){return result;}}
}
` + fixture[end:]
	fixture = strings.Replace(fixture, "class SuffixWitness {", "class SuffixWitness {static int tag;static long wide;static Object reference;static int[] array;", 1)
	fixture = strings.Replace(fixture, "SuffixWitness.token=token;", "SuffixOptions.tag=SuffixWitness.tag=mode;SuffixOptions.wide=SuffixWitness.wide=seed;SuffixOptions.reference=SuffixWitness.reference=token;SuffixOptions.array=SuffixWitness.array=mode==0?null:new int[]{mode};SuffixWitness.token=token;", 1)
	fixture += "\nclass SuffixOptions {static volatile int tag;static volatile long wide;static volatile Object reference;static volatile int[] array;}\n"
	return fixture
}

func TestAdversarialConstructorLoweringKeepsReferenceWideningWithoutRetargeting(t *testing.T) {
	for _, kind := range []string{"parameter", "static field", "array"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(kind+"/"+debug, func(t *testing.T) {
				fixture := constructorWideningFixture(kind)
				constructorSuffixRoundTrip(t, fixture, debug, "SuffixOwner$Member", "72:constructor:closed:suffix:oracle\n")
			})
		}
	}
}

func constructorWideningFixture(kind string) string {
	fixture := constructorStaticArgumentFixture()
	switch kind {
	case "parameter":
		fixture = strings.Replace(fixture, "make(Object input,", "make(SuffixInput input,", 1)
		fixture = strings.Replace(fixture, "Member(Object input,", "Member(SuffixInput input,", 1)
		fixture = strings.Replace(fixture, "owner.make(fail?null:new Object(),", "owner.make(fail?null:new SuffixInput(),", 1)
		fixture = strings.Replace(fixture, "super(input,SuffixOptions.tag", "super((Object)input,SuffixOptions.tag", 1)
		fixture = strings.Replace(fixture, "abstract Object value();", `SuffixBase(SuffixInput input,int tag,long wide,Object reference,int[] array){throw new AssertionError("wrong more-specific constructor");}abstract Object value();`, 1)
		fixture += "\nclass SuffixInput {}\n"
	case "static field":
		fixture = strings.Replace(fixture, "long wide,Object reference,int[] array)", "long wide,Runnable reference,int[] array)", 1)
		fixture = strings.Replace(fixture, "static volatile Object reference", "static volatile SuffixToken reference", 1)
		fixture = strings.Replace(fixture, "SuffixOptions.reference=SuffixWitness.reference=token;", "SuffixWitness.reference=(SuffixOptions.reference=token==null?null:new SuffixToken());", 1)
		fixture += "\nclass SuffixToken implements Runnable {public void run(){throw new AssertionError(\"must not execute while widening\");}}\n"
	case "array":
		fixture = strings.Replace(fixture, "Object reference,int[] array)", "Object reference,Cloneable array)", 1)
	}
	return fixture
}

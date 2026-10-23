package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The unchanged binary observer checks the erased (String,int) header separately
// from user operands. Their category widths and nested allocations must not be
// mistaken for either the enum receiver or the backing-array boundary.
const enumArgumentHelperFixture = `public class ArgumentOwner {
 public enum Mode {
  FIRST(Effects.next(3),Long.MIN_VALUE,-0.0,new Box(Effects.next(5))),
  SECOND(Effects.next(7),Long.MAX_VALUE,Double.NaN,new Box(Effects.next(11)));
  final int tag;final long wide;final double real;final Box box;
  Mode(int tag,long wide,double real,Box box){this.tag=tag;this.wide=wide;this.real=real;this.box=java.util.Objects.requireNonNull(box);Effects.trace+="ctor:"+tag+";";}
  public long fold(long input){return input*wide+tag+box.value+Double.doubleToRawLongBits(real);}
 }
}
class Box {final int value;Box(int n){value=n;Effects.trace+="box:"+n+";";}}
class Effects {static String trace="";static int fail;static final RuntimeException failure=new RuntimeException("same failure");static int next(int n){trace+="arg:"+n+";";if(n==fail)throw failure;return n;}}
class ArgumentDriver {public static void main(String[]args)throws Exception {
 Effects.fail=Integer.parseInt(args[0]);
 if(!Effects.trace.isEmpty())throw new AssertionError("eager enum initialization");
 if(Effects.fail!=0){
  try{ArgumentOwner.Mode.values();throw new AssertionError("missing initializer failure");}catch(ExceptionInInitializerError e){if(e.getCause()!=Effects.failure)throw new AssertionError("exception identity",e);}
  String expected=Effects.fail==3?"arg:3;":"arg:3;arg:5;box:5;ctor:3;arg:7;";
  if(!Effects.trace.equals(expected))throw new AssertionError("abrupt evaluation order: "+Effects.trace);
  try{ArgumentOwner.Mode.values();throw new AssertionError("missing erroneous class state");}catch(NoClassDefFoundError e){}
  if(!Effects.trace.equals(expected))throw new AssertionError("repeated failed initialization");
  System.out.println("0:typed:enum:arguments:effects:binding");return;
 }
 ArgumentOwner.Mode[] modes=ArgumentOwner.Mode.values();
 if(!Effects.trace.equals("arg:3;arg:5;box:5;ctor:3;arg:7;arg:11;box:11;ctor:7;"))throw new AssertionError("argument order/identity: "+Effects.trace);
 if(modes.length!=2||modes[0]!=ArgumentOwner.Mode.FIRST||modes[1]!=ArgumentOwner.Mode.SECOND||modes[0].ordinal()!=0||modes[1].ordinal()!=1||ArgumentOwner.Mode.valueOf("SECOND")!=modes[1])throw new AssertionError("hidden name/ordinal/array");
 if(ArgumentOwner.Mode.class.getDeclaredMethods().length!=3||ArgumentOwner.Mode.class.getDeclaringClass()!=ArgumentOwner.class)throw new AssertionError("method/lexical universe");
 int rows=0;for(long value:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(int i=0;i<2;i++){
  long factor=i==0?Long.MIN_VALUE:Long.MAX_VALUE;int tag=i==0?3:7,box=i==0?5:11;double real=i==0?-0.0:Double.NaN;
  long expected=java.math.BigInteger.valueOf(value).multiply(java.math.BigInteger.valueOf(factor)).add(java.math.BigInteger.valueOf(tag+box)).add(java.math.BigInteger.valueOf(Double.doubleToRawLongBits(real))).longValue();
  if(modes[i].fold(value)!=expected)throw new AssertionError("category2/overflow/payload");rows++;
 }
 modes[0]=null;if(ArgumentOwner.Mode.values()[0]!=ArgumentOwner.Mode.FIRST)throw new AssertionError("array clone");
 System.out.println(rows+":typed:enum:arguments:effects:binding");
}}
`

func TestAdversarialEnumArgumentHelperRetainsTypedAllocationAndEffects(t *testing.T) {
	compiler, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	for _, owner := range []string{"ArgumentOwner", "RenamedArgumentOwner"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, protocol := range []string{"mixed", "delegating", "varargs"} {
				t.Run(owner+"/"+debug+"/"+protocol, func(t *testing.T) {
					fixture := strings.ReplaceAll(enumArgumentHelperFixture, "ArgumentOwner", owner)
					switch protocol {
					case "delegating":
						fixture = strings.Replace(fixture, "FIRST(Effects.next(3),Long.MIN_VALUE,-0.0,new Box(Effects.next(5)))", "FIRST(Effects.next(3),new Box(Effects.next(5)))", 1)
						fixture = strings.Replace(fixture, "Mode(int tag,long wide", "Mode(int tag,Box box){this(tag,Long.MIN_VALUE,-0.0,box);}\n  Mode(int tag,long wide", 1)
					case "varargs":
						fixture = strings.Replace(fixture, "double real,Box box)", "double real,Box... boxes)", 1)
						fixture = strings.Replace(fixture, "requireNonNull(box)", "requireNonNull(boxes[0])", 1)
					}
					files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, fixture, owner, debug, false)
					run := func(dir string) {
						t.Helper()
						for _, mode := range []string{"-ea", "-da"} {
							for _, failure := range []string{"0", "3", "7"} {
								out, err := enumFreshOracleRun(t, java, mode, dir, "ArgumentDriver", failure)
								rows := "0"
								if failure == "0" {
									rows = "10"
								}
								if err != nil || string(out) != rows+":typed:enum:arguments:effects:binding\n" {
									t.Fatal("unchanged original-first argument observer", err, string(out))
								}
							}
						}
					}
					run(sourceArtifactWriteFiles(t, files))
					owned := map[string][]byte{owner + ".class": files[owner+".class"], owner + "$Mode.class": files[owner+"$Mode.class"]}
					jar := filepath.Join(t.TempDir(), "typed.jar")
					if err := os.WriteFile(jar, t23Zip(t, owned), 0600); err != nil {
						t.Fatal(err)
					}
					resolve := func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return raw, ok }
					a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, resolve)
					if err != nil {
						t.Fatal(err)
					}
					defer a.Close()
					source, err := a.ReadFile(owner + ".class")
					if err != nil || legacySourceHasFailureMarker(string(source)) || !strings.Contains(string(source), "enum Mode") {
						t.Fatal("native source with typed user operands", err, string(source))
					}
					deps := map[string][]byte{}
					for name, raw := range files {
						if _, own := owned[name]; !own {
							deps[name] = raw
						}
					}
					out := sourceArtifactWriteFiles(t, deps)
					path := filepath.Join(out, owner+".java")
					if err := os.WriteFile(path, source, 0600); err != nil {
						t.Fatal(err)
					}
					if log, err := exec.Command(compiler, "-proc:none", "--release", "8", "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out, path).CombinedOutput(); err != nil {
						t.Fatal("compile without owned originals", err, string(log))
					}
					for name := range owned {
						p := filepath.Join(out, name)
						fresh, err := os.ReadFile(p)
						if err != nil {
							t.Fatal(err)
						}
						finished, err := a.FinishClass(name, fresh)
						if err != nil {
							t.Fatal("complete typed constant headers before fresh array", name, err)
						}
						if again, err := a.FinishClass(name, finished); err != nil || !bytes.Equal(again, finished) {
							t.Fatal("typed enum completion is not idempotent", err)
						}
						if err := os.WriteFile(p, finished, 0600); err != nil {
							t.Fatal(err)
						}
					}
					run(out)
				})
			}
		}
	}
}

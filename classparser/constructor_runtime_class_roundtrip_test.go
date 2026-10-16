package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Keep the original driver and superclass binaries. Recompile the complete
// physical owner/children set: leaving the original owner's InnerClasses table
// on javac's input path can make javac prepend a second enclosing parameter to
// a flat child. No original binary from the replaced set is copied to output.
// The unchanged driver observes captures, mirror identity, arithmetic and order.
func TestAdversarialConstructorRuntimeClassCaptureRoundTrip(t *testing.T) {
	testConstructorRuntimeClassCapture(t, false)
}

func TestAdversarialConstructorRuntimeClassCaptureOriginalJavac8(t *testing.T) {
	testConstructorRuntimeClassCapture(t, true)
}

func testConstructorRuntimeClassCapture(t *testing.T, native bool) {
	t.Helper()
	const fixture = `
class MirrorState {static String trace="";static Class<?> observed;}
class MirrorBase {
 final Class<?> kind;final long value;
 MirrorBase(int n,int divisor){
  MirrorState.trace+="P";kind=getClass();
  MirrorState.observed=kind;MirrorState.trace+="C";
  value=(((long)n)*3L+17L)/divisor;MirrorState.trace+="Q";
 }
 Object captured(){return null;}
}
class MirrorOwner {
 final Object token;MirrorOwner(Object t){token=t;}
 final class Child extends MirrorBase {
  Child(int n,int d){super(n,d);}
  Object captured(){return MirrorOwner.this.token;}
 }
 class Sealed extends MirrorBase {
  Sealed(int n,int d){super(n,d);}
  protected final void finalize(){}
  Object captured(){return MirrorOwner.this.token;}
 }
 MirrorBase make(int n,int d,boolean sealed){return sealed?new Sealed(n,d):new Child(n,d);}
}
class MirrorDriver {
 public static void main(String[] args)throws Exception{
  int rows=0;Object token=new Object();
  if(Class.class.getClass()!=Class.class)throw new AssertionError("mirror alias control");
  for(Object t:new Object[]{null,token})for(int n:new int[]{Integer.MIN_VALUE,-7,-1,0,1,7,Integer.MAX_VALUE})for(int d:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(boolean sealed:new boolean[]{false,true}){
   MirrorOwner owner=new MirrorOwner(t);MirrorState.trace="";MirrorState.observed=null;
   Class<?> expected=sealed?MirrorOwner.Sealed.class:MirrorOwner.Child.class;
   try{
    MirrorBase result=owner.make(n,d,sealed);
    long oracle=java.math.BigInteger.valueOf(n).multiply(java.math.BigInteger.valueOf(3)).add(java.math.BigInteger.valueOf(17)).divide(java.math.BigInteger.valueOf(d)).longValueExact();
    if(result.value!=oracle||result.kind!=expected||result.getClass()!=expected||MirrorState.observed!=expected||result.captured()!=t||!MirrorState.trace.equals("PCQ"))throw new AssertionError("value/capture/mirror/order");
    java.lang.reflect.Field capture=expected.getDeclaredField("this$0");capture.setAccessible(true);
    if(capture.get(result)!=owner||capture.getType()!=MirrorOwner.class)throw new AssertionError("capture storage");
   }catch(ArithmeticException failure){
    if(d!=0||!MirrorState.trace.equals("PC")||MirrorState.observed!=expected)throw new AssertionError("abrupt observation");
   }
   rows++;
  }
  System.out.println(rows+":mirror:capture:arithmetic:abrupt");
 }
}`
	javac, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			var files map[string][]byte
			if native {
				files = nativePrivateEnumCompile(t, fixture, "MirrorOwner", debug)
			} else {
				files = nativeCompileDebugClasses(t, fixture, debug)
			}
			original := t.TempDir()
			for name, raw := range files {
				if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			want := t04RunJava(t, java, original, "MirrorDriver")
			if want != "140:mirror:capture:arithmetic:abrupt\n" {
				t.Fatalf("independent original oracle: %q", want)
			}
			resolve := func(n string) ([]byte, bool) { b, ok := files[n+".class"]; return bytes.Clone(b), ok }
			for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
				t.Run(policy, func(t *testing.T) {
					if policy == "no-source-rewrites" {
						t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
					}
					if policy == "no-core-cleanups" {
						t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
					}
					for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
						t.Run(string(mode), func(t *testing.T) {
							out := t.TempDir()
							var sources []string
							for name, raw := range files {
								if name != "MirrorOwner.class" && name != "MirrorOwner$Child.class" && name != "MirrorOwner$Sealed.class" {
									if err := os.WriteFile(filepath.Join(out, name), raw, 0600); err != nil {
										t.Fatal(err)
									}
									continue
								}
								var source string
								var err error
								if mode == "legacy" {
									source, err = DecompileWithResolver(bytes.Clone(raw), resolve)
								} else {
									var result DecompileResult
									result, err = DecompileWithOptions(bytes.Clone(raw), DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
									source = result.Source
								}
								if err != nil || strings.Contains(source, DecompileStubMarker) {
									t.Fatalf("complete physical constructor %s: %v\n%s", name, err, source)
								}
								path := filepath.Join(out, strings.TrimSuffix(name, ".class")+".java")
								if err := os.WriteFile(path, []byte(source), 0600); err != nil {
									t.Fatal(err)
								}
								sources = append(sources, path)
							}
							args := append([]string{"-proc:none", "--release", "8", "-cp", out, "-d", out}, sources...)
							if log, err := exec.Command(javac, args...).CombinedOutput(); err != nil {
								t.Fatalf("rebuild: %v\n%s", err, log)
							}
							if got := t04RunJava(t, java, out, "MirrorDriver"); got != want {
								t.Fatalf("candidate oracle: %q want %q", got, want)
							}
						})
					}
				})
			}
		})
	}
}

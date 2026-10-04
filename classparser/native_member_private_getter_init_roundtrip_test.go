package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Leaf initialization must not eagerly initialize its lexical Root. The private
// getter invocation subsequently initializes Root even with a null argument:
// its failing class initializer precedes the receiver's GETFIELD NPE.
const nativeMemberGetterInitFixture = `class GetterInitEffects{static String trace="";static boolean fail;static final RuntimeException failure=new RuntimeException("same");static Object init(){trace+="I";if(fail)throw failure;return new Object();}}
class GetterInitRoot{static final Object initialized=GetterInitEffects.init();private Object token;class Layer{private Object layerToken;class Leaf{Object read(){return ((GetterInitRoot)null).token;}Object readLayer(){return Layer.this.layerToken;}}}}
class GetterInitDriver{public static void main(String[]args)throws Exception{GetterInitEffects.fail=args.length!=0;Class<?>c=Class.forName("GetterInitRoot$Layer$Leaf");java.lang.reflect.Constructor<?>ctor=c.getDeclaredConstructor(Class.forName("GetterInitRoot$Layer"));ctor.setAccessible(true);Object leaf=ctor.newInstance(new Object[]{null});if(!GetterInitEffects.trace.equals(""))throw new AssertionError("eager initialization");java.lang.reflect.Method read=c.getDeclaredMethod("read");read.setAccessible(true);for(int i=0;i<2;i++){try{read.invoke(leaf);throw new AssertionError("no exception");}catch(java.lang.reflect.InvocationTargetException e){Throwable cause=e.getCause();if(GetterInitEffects.fail){if(i==0){if(!(cause instanceof ExceptionInInitializerError)||cause.getCause()!=GetterInitEffects.failure)throw new AssertionError("initialization failure",cause);}else if(!(cause instanceof NoClassDefFoundError))throw new AssertionError("erroneous class",cause);}else if(!(cause instanceof NullPointerException))throw new AssertionError("receiver failure",cause);if(!GetterInitEffects.trace.equals("I"))throw new AssertionError("initialize once");}}System.out.println(GetterInitEffects.trace+":"+GetterInitEffects.fail);}}`

func TestNativeMemberPrivateGetterPreservesNullReceiverInitialization(t *testing.T) {
	javac, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, nativeMemberGetterInitFixture, debug)
			original := t.TempDir()
			for n, raw := range files {
				if e := os.WriteFile(filepath.Join(original, n), raw, 0600); e != nil {
					t.Fatal(e)
				}
			}
			oracles := []string{}
			for _, args := range [][]string{{}, {"fail"}} {
				out, e := exec.Command(java, append([]string{"-Xverify:all", "-cp", original, "GetterInitDriver"}, args...)...).CombinedOutput()
				if e != nil {
					t.Fatalf("original %v %s", e, out)
				}
				oracles = append(oracles, string(out))
			}
			for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
				t.Run(policy, func(t *testing.T) {
					if policy == "no-source-rewrites" {
						t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
					}
					if policy == "no-core-cleanups" {
						t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
					}
					z := nativeArchive(t, files)
					defer z.Close()
					for _, n := range []string{"GetterInitRoot$Layer", "GetterInitRoot$Layer$Leaf"} {
						raw, e := z.ReadFile(n + ".class")
						if e != nil || !strings.Contains(string(raw), "body owned by") {
							t.Fatalf("ownership %s %v\n%s", n, e, raw)
						}
					}
					src, e := z.ReadFile("GetterInitRoot.class")
					if e != nil || strings.Contains(string(src), DecompileStubMarker) {
						t.Fatalf("source %v\n%s", e, src)
					}
					output := t.TempDir()
					for n, raw := range files {
						if strings.HasPrefix(n, "GetterInitRoot") {
							continue
						}
						if e := os.WriteFile(filepath.Join(output, n), raw, 0600); e != nil {
							t.Fatal(e)
						}
					}
					file := filepath.Join(output, "GetterInitRoot.java")
					if e := os.WriteFile(file, src, 0600); e != nil {
						t.Fatal(e)
					}
					if out, e := exec.Command(javac, "-proc:none", "--release", "8", "-cp", output, "-d", output, file).CombinedOutput(); e != nil {
						t.Fatalf("rebuilt %v %s\n%s", e, out, src)
					}
					for i, args := range [][]string{{}, {"fail"}} {
						out, e := exec.Command(java, append([]string{"-Xverify:all", "-cp", output, "GetterInitDriver"}, args...)...).CombinedOutput()
						if e != nil || string(out) != oracles[i] {
							t.Fatalf("rebuilt %v %s != %s", e, out, oracles[i])
						}
					}
					for n, want := range files {
						if !strings.HasPrefix(n, "GetterInitRoot") {
							continue
						}
						raw, e := os.ReadFile(filepath.Join(output, n))
						if e != nil {
							t.Fatal(e)
						}
						if got := nativeBinaryShape(t, raw); got != nativeBinaryShape(t, want) {
							t.Fatalf("ABI %s\n%s\n%s", n, nativeBinaryShape(t, want), got)
						}
					}
				})
			}
		})
	}
}

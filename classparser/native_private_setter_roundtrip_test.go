package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Original callers check receiver/RHS ordering, failure identity, category-2
// assignment results and callback observation before constructor delegation.
// The independently compiled driver is reused without recompiling its ABI.
const nativePrivateSetterFixture = `class SetterEffects {
 static String trace="";static int fail;
 static final RuntimeException error=new RuntimeException("original");
 static SetterOwner receiver(SetterOwner o){trace+="R";if(fail==1)throw error;return o;}
 static Object payload(Object o){trace+="V";if(fail==2)throw error;return o;}
 static SetterOwner.Layer layer(SetterOwner.Layer o){trace+="L";if(fail==1)throw error;return o;}
 static long number(long n){trace+="N";if(fail==2)throw error;return n;}
}
abstract class SetterParent {final Object observed;SetterParent(){observed=observe();}abstract Object observe();}
class SetterOwner {
 private Object token;SetterOwner(Object token){this.token=token;}
 class Layer extends SetterParent {
  private long number;Object observe(){return SetterOwner.this.token;}
  class Leaf {
   Object put(SetterOwner o,Object v){return SetterEffects.receiver(o).token=SetterEffects.payload(v);}
   Object get(SetterOwner o){return o.token;}
void store(SetterOwner o,Object v){SetterEffects.receiver(o).token=SetterEffects.payload(v);}
   long putLong(Layer o,long n){return SetterEffects.layer(o).number=SetterEffects.number(n);}
   long getLong(Layer o){return o.number;}
void storeLong(Layer o,long n){SetterEffects.layer(o).number=SetterEffects.number(n);}
long add(Layer o,long n){return 7+(o.number=n);}
Object chain(SetterOwner o,Object v){return o.token=(o.token=v);}
  }
 }
}
class SetterDriver {public static void main(String[]args){
 Object token=new Object();SetterOwner owner=new SetterOwner(token);SetterOwner.Layer layer=owner.new Layer();
 if(layer.observed!=token)throw new AssertionError("capture before super callback");
 SetterOwner.Layer.Leaf leaf=layer.new Leaf();int rows=0;
 for(Object value:new Object[]{null,token}){
  SetterEffects.fail=0;SetterEffects.trace="";
  if(leaf.put(owner,value)!=value||leaf.get(owner)!=value||!SetterEffects.trace.equals("RV"))throw new AssertionError("reference result/order");rows++;
 }
 for(long n:new long[]{Long.MIN_VALUE,-1,0,Long.MAX_VALUE}){
  SetterEffects.fail=0;SetterEffects.trace="";
  if(leaf.putLong(layer,n)!=n||leaf.getLong(layer)!=n||!SetterEffects.trace.equals("LN"))throw new AssertionError("wide result/order");rows++;
 }
 for(int fail:new int[]{0,1,2}){
  SetterEffects.fail=fail;SetterEffects.trace="";
  try{leaf.put(null,token);throw new AssertionError("missing reference failure");}catch(RuntimeException e){if(fail==0?!(e instanceof NullPointerException):e!=SetterEffects.error)throw new AssertionError("reference failure identity",e);}
  if(!SetterEffects.trace.equals(fail==1?"R":"RV"))throw new AssertionError("reference failure order");rows++;
  SetterEffects.trace="";
  try{leaf.putLong(null,Long.MIN_VALUE);throw new AssertionError("missing wide failure");}catch(RuntimeException e){if(fail==0?!(e instanceof NullPointerException):e!=SetterEffects.error)throw new AssertionError("wide failure identity",e);}
  if(!SetterEffects.trace.equals(fail==1?"L":"LN"))throw new AssertionError("wide failure order");rows++;
 }
 SetterEffects.fail=0;SetterEffects.trace="";leaf.store(owner,token);leaf.storeLong(layer,Long.MAX_VALUE);
 if(leaf.get(owner)!=token||leaf.getLong(layer)!=Long.MAX_VALUE||!SetterEffects.trace.equals("RVLN"))throw new AssertionError("discarded assignment");
 if(leaf.chain(owner,token)!=token||leaf.get(owner)!=token)throw new AssertionError("nested assignment");
 for(long n:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE})if(leaf.add(layer,n)!=n+7||leaf.getLong(layer)!=n)throw new AssertionError("expression precedence");
 System.out.println(rows+":original-order:identity");
}}`

func TestNativePrivateSetterAndGetterPreserveOriginalProtocol(t *testing.T) {
	testNativePrivateSetterFixture(t, nativePrivateSetterFixture, "SetterOwner", "SetterDriver", "12:original-order:identity\n")
}

func testNativePrivateSetterFixture(t *testing.T, fixture, owner, driver, want string) {
	t.Helper()
	testNativePrivateSetterFixtureWithMutation(t, fixture, owner, driver, want, nil)
}

func testNativePrivateSetterFixtureWithMutation(t *testing.T, fixture, owner, driver, want string, mutate func(*testing.T, map[string][]byte)) {
	t.Helper()
	testNativePrivateSetterCompiledFixture(t, owner, driver, want, func(t *testing.T, debug string) map[string][]byte {
		files := nativeCompileDebugClasses(t, fixture, debug)
		if mutate != nil {
			mutate(t, files)
		}
		return files
	})
}

func testNativePrivateSetterSourceFixture(t *testing.T, sources map[string]string, owner, driver, want string) {
	t.Helper()
	testNativePrivateSetterCompiledFixture(t, owner, driver, want, func(t *testing.T, debug string) map[string][]byte {
		return nativeCompileSourceReleaseClasses(t, sources, debug, "8")
	})
}

func testNativePrivateSetterCompiledFixture(t *testing.T, owner, driver, want string, compile func(*testing.T, string) map[string][]byte, verify ...func(*testing.T, string, []byte, []byte)) {
	t.Helper()
	testNativePrivateSetterCompiledFixtureWithShape(t, owner, driver, want, compile, nil, verify...)
}

// A fixture with an intentional source representation change supplies its own
// independent ABI contract. Existing fixtures retain the exact binary/accessor
// checks; this does not globally normalize a compiler or source regression.
func testNativePrivateSetterCompiledFixtureWithShape(t *testing.T, owner, driver, want string, compile func(*testing.T, string) map[string][]byte, shape func(*testing.T, string, []byte, []byte), verify ...func(*testing.T, string, []byte, []byte)) {
	t.Helper()
	javac, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := compile(t, debug)
			original := t.TempDir()
			for n, raw := range files {
				if e := os.MkdirAll(filepath.Dir(filepath.Join(original, n)), 0700); e != nil {
					t.Fatal(e)
				}
				if e := os.WriteFile(filepath.Join(original, n), raw, 0600); e != nil {
					t.Fatal(e)
				}
			}
			oracle := t04RunJava(t, java, original, driver)
			if oracle != want {
				t.Fatalf("original JVM=%q", oracle)
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
					out := t.TempDir()
					paths := []string{}
					for n, raw := range files {
						if e := os.MkdirAll(filepath.Dir(filepath.Join(out, n)), 0700); e != nil {
							t.Fatal(e)
						}
						if !strings.HasPrefix(n, owner) {
							if e := os.WriteFile(filepath.Join(out, n), raw, 0600); e != nil {
								t.Fatal(e)
							}
							continue
						}
						source, e := z.ReadFile(n)
						if e != nil || strings.Contains(string(source), DecompileStubMarker) {
							t.Fatalf("source %s:%v\n%s", n, e, source)
						}
						path := filepath.Join(out, strings.TrimSuffix(n, ".class")+".java")
						if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
							t.Fatal(e)
						}
						if e := os.WriteFile(path, source, 0600); e != nil {
							t.Fatal(e)
						}
						paths = append(paths, path)
					}
					if data, e := exec.Command(javac, append([]string{"-proc:none", "--release", "8", "-cp", out, "-d", out}, paths...)...).CombinedOutput(); e != nil {
						t.Fatalf("rebuilt:%v\n%s", e, data)
					}
					if got := t04RunJava(t, java, out, driver); got != oracle {
						t.Fatalf("rebuilt JVM=%q want=%q", got, oracle)
					}
					for n, want := range files {
						if !strings.HasPrefix(n, owner) {
							continue
						}
						raw, e := os.ReadFile(filepath.Join(out, n))
						if e != nil {
							t.Fatal(e)
						}
						if shape != nil {
							shape(t, n, want, raw)
						} else {
							if got := nativeBinaryShape(t, raw); got != nativeBinaryShape(t, want) {
								t.Fatalf("ABI %s\n%s\n%s", n, nativeBinaryShape(t, want), got)
							}
							if got := nativeAnonymousAccessorShape(t, raw); got != nativeAnonymousAccessorShape(t, want) {
								t.Fatalf("accessor ABI %s\n%s\n%s", n, nativeAnonymousAccessorShape(t, want), got)
							}
						}
						for _, check := range verify {
							check(t, n, want, raw)
						}
					}
				})
			}
		})
	}
}

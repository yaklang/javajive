package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Source version changes private access lowering; it does not change Java's
// enclosing-instance initialization before an observable superclass call.
const sourceTargetCaptureFixture = `class SourceCaptureOwner {
 final Object token; SourceCaptureOwner(Object t){token=t;}
 public class Child extends CaptureObserver { public Child(int n){super(n);} Object enclosing(){return SourceCaptureOwner.this;} }
 Object member(int n){return new Child(n);}
 Object anonymous(int n){return new CaptureObserver(n){Object enclosing(){return SourceCaptureOwner.this;}};}
}
abstract class CaptureObserver {
 final Object observed; final int word;
 CaptureObserver(int n){CaptureEffects.trace+="P";observed=enclosing();word=n;CaptureEffects.published=this;if(CaptureEffects.fail)throw CaptureEffects.error;}
 abstract Object enclosing();
}
class CaptureEffects {static boolean fail;static Object published;static String trace;static final RuntimeException error=new RuntimeException("identity");}
class CaptureDriver {public static void main(String[]args)throws Exception{int rows=0;for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(boolean fail:new boolean[]{false,true})for(boolean anon:new boolean[]{false,true}){Object token=new Object();SourceCaptureOwner owner=new SourceCaptureOwner(token);CaptureEffects.trace="";CaptureEffects.fail=fail;CaptureEffects.published=null;try{CaptureObserver value=(CaptureObserver)(anon?owner.anonymous(n):owner.member(n));if(fail||value.enclosing()!=owner||value.observed!=owner||value.word!=n||CaptureEffects.published!=value||!CaptureEffects.trace.equals("P"))throw new AssertionError("capture/word/effect/identity");}catch(RuntimeException e){CaptureObserver value=(CaptureObserver)CaptureEffects.published;if(!fail||e!=CaptureEffects.error||value==null||value.enclosing()!=owner||value.observed!=owner||value.word!=n||!CaptureEffects.trace.equals("P"))throw new AssertionError("publication/exception/order",e);}rows++;}System.out.println(rows+":source-target:capture:observation:publication:exception");}}`

func TestAdversarialSourceTargetPreservesObservableCaptureRoundTrip(t *testing.T) {
	testSourceTargetOriginalFamilyFixture(t, sourceTargetCaptureFixture, "SourceCaptureOwner", "CaptureDriver", "20:source-target:capture:observation:publication:exception\n")
}

func TestAdversarialSourceTargetPrivateBridgePreservesObservableCaptureRoundTrip(t *testing.T) {
	fixture := strings.Replace(sourceTargetCaptureFixture, "public Child(int", "private Child(int", 1)
	testSourceTargetOriginalFamilyFixture(t, fixture, "SourceCaptureOwner", "CaptureDriver", "20:source-target:capture:observation:publication:exception\n")
}

func TestAdversarialSourceTargetEmptyMarkerPreservesNestedAnonymousRoundTrip(t *testing.T) {
	testSourceTargetOriginalFamilyFixture(t, nativeEmptyArtifactForestFixture, "ArtifactForestOwner", "ArtifactForestDriver", "125:marker:named:anonymous:overflow\n")
}

func testSourceTargetOriginalFamilyFixture(t *testing.T, fixture, originalOwner, driver, expected string) {
	testSourceTargetReleaseFamilyFixture(t, fixture, originalOwner, driver, expected, "8", []int{8, 11})
}

func testSourceTargetReleaseFamilyFixture(t *testing.T, fixture, originalOwner, driver, expected, inputRelease string, targets []int) {
	t.Helper()
	javac, java := t04Tools(t)
	for _, owner := range []string{originalOwner, "Alternate" + originalOwner} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(owner+"/"+debug, func(t *testing.T) {
				files := nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": strings.ReplaceAll(fixture, originalOwner, owner)}, debug, inputRelease)
				original := t.TempDir()
				for name, raw := range files {
					if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
						t.Fatal(err)
					}
				}
				want := t04RunJava(t, java, original, driver)
				if want != expected {
					t.Fatal(want)
				}
				for _, target := range targets {
					for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
						t.Run(strconv.Itoa(target)+"/"+policy, func(t *testing.T) {
							if policy == "no-source-rewrites" {
								t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
							}
							if policy == "no-core-cleanups" {
								t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
							}
							archive := filepath.Join(t.TempDir(), "original.jar")
							if err := os.WriteFile(archive, t23Zip(t, files), 0600); err != nil {
								t.Fatal(err)
							}
							z, err := NewJarFSFromLocalWithSourceVersion(archive, target, nil)
							if err != nil {
								t.Fatal(err)
							}
							defer z.Close()
							out := t.TempDir()
							var sources []string
							for name, raw := range files {
								if !strings.HasPrefix(name, owner) {
									if err := os.WriteFile(filepath.Join(out, name), raw, 0600); err != nil {
										t.Fatal(err)
									}
									continue
								}
								src, err := z.ReadFile(name)
								if err != nil || strings.Contains(string(src), DecompileStubMarker) {
									t.Fatalf("source %s:%v\n%s", name, err, src)
								}
								path := filepath.Join(out, strings.TrimSuffix(name, ".class")+".java")
								if err := os.WriteFile(path, src, 0600); err != nil {
									t.Fatal(err)
								}
								sources = append(sources, path)
							}
							args := append([]string{"-proc:none", "--release", strconv.Itoa(target), "-cp", out, "-d", out}, sources...)
							if log, err := exec.Command(javac, args...).CombinedOutput(); err != nil {
								t.Fatalf("compile:%v\n%s", err, log)
							}
							if got := t04RunJava(t, java, out, driver); got != want {
								t.Fatalf("got %q expected %q", got, want)
							}
							for name := range files {
								if _, err := os.Stat(filepath.Join(out, name)); err != nil {
									t.Fatalf("original class disappeared:%s:%v", name, err)
								}
							}
						})
					}
				}
			})
		}
	}
}

const sourceTargetGetterInitFixture = `class DormantAccessEffects{static String trace="";static Object init(){trace+="I";return new Object();}static Object arg(){trace+="A";return new Object();}}
class DormantAccessOwner{static final Object trigger=DormantAccessEffects.init();private Object token;static class Reader{static Object read(DormantAccessOwner value,Object ignored){return value.token;}}}
class DormantAccessDriver{public static void main(String[]args)throws Exception{Class.forName("DormantAccessOwner$Reader");if(!DormantAccessEffects.trace.equals(""))throw new AssertionError("eager init");try{DormantAccessOwner.Reader.read(null,DormantAccessEffects.arg());throw new AssertionError("no NPE");}catch(NullPointerException expected){if(!DormantAccessEffects.trace.equals("AI"))throw new AssertionError("class init before receiver failure:"+DormantAccessEffects.trace);}System.out.println("accessor:argument:init:receiver-failure");}}`

func TestAdversarialSourceTargetPreservesAccessorClassInitializationRoundTrip(t *testing.T) {
	testSourceTargetOriginalFamilyFixture(t, sourceTargetGetterInitFixture, "DormantAccessOwner", "DormantAccessDriver", "accessor:argument:init:receiver-failure\n")
}

func TestAdversarialSourceTargetPreservesWriteUpdateAndCallInitializationRoundTrip(t *testing.T) {
	for _, operation := range []string{"write", "update", "call"} {
		t.Run(operation, func(t *testing.T) {
			fixture := sourceTargetGetterInitFixture
			switch operation {
			case "write":
				fixture = strings.Replace(fixture, "return value.token;", "return value.token=ignored;", 1)
			case "update":
				fixture = strings.Replace(fixture, "private Object token;", "private int token;", 1)
				fixture = strings.Replace(fixture, "return value.token;", "return value.token++;", 1)
			case "call":
				fixture = strings.Replace(fixture, "private Object token;", `private Object token;private Object touch(Object ignored){DormantAccessEffects.trace+="B";return token;}`, 1)
				fixture = strings.Replace(fixture, "return value.token;", "return value.touch(ignored);", 1)
			}
			testSourceTargetReleaseFamilyFixture(t, fixture, "DormantAccessOwner", "DormantAccessDriver", "accessor:argument:init:receiver-failure\n", "8", []int{8, 11, 16})
		})
	}
}

func TestAdversarialSourceTargetPreservesFailedAccessorInitializationRoundTrip(t *testing.T) {
	fixture := strings.Replace(sourceTargetGetterInitFixture, `static Object init(){trace+="I";return new Object();}`, `static final RuntimeException failure=new RuntimeException("identity");static Object init(){trace+="I";throw failure;}`, 1)
	start := strings.Index(fixture, "class DormantAccessDriver")
	if start < 0 {
		t.Fatal("fixture driver missing")
	}
	fixture = fixture[:start] + `class DormantAccessDriver{public static void main(String[]args)throws Exception{Class.forName("DormantAccessOwner$Reader");if(!DormantAccessEffects.trace.equals(""))throw new AssertionError("eager init");for(int i=0;i<2;i++){try{DormantAccessOwner.Reader.read(null,DormantAccessEffects.arg());throw new AssertionError("missing initialization failure");}catch(ExceptionInInitializerError e){if(i!=0||e.getCause()!=DormantAccessEffects.failure)throw new AssertionError("initialization failure identity",e);}catch(NoClassDefFoundError e){if(i!=1)throw new AssertionError("erroneous class",e);}if(!DormantAccessEffects.trace.equals(i==0?"AI":"AIA"))throw new AssertionError("argument/init/failure order:"+DormantAccessEffects.trace);}System.out.println("accessor:argument:failed-init:identity:no-retry");}}`
	testSourceTargetOriginalFamilyFixture(t, fixture, "DormantAccessOwner", "DormantAccessDriver", "accessor:argument:failed-init:identity:no-retry\n")
}
func TestAdversarialSourceTargetPreservesGenericAccessorErasureRoundTrip(t *testing.T) {
	fixture := strings.Replace(sourceTargetGetterInitFixture, "class DormantAccessOwner{", "class DormantAccessOwner<T>{", 1)
	fixture = strings.Replace(fixture, "private Object token;", "private T token;DormantAccessOwner(T t){token=t;}", 1)
	fixture = strings.Replace(fixture, "DormantAccessOwner value,", "DormantAccessOwner<?> value,", 1)
	fixture = strings.Replace(fixture, `System.out.println("accessor:argument:init:receiver-failure");`, `Object token=new Object();DormantAccessOwner<Object> value=new DormantAccessOwner<Object>(token);if(DormantAccessOwner.Reader.read(value,null)!=token)throw new AssertionError("generic accessor identity/erasure");System.out.println("accessor:generic:erasure:identity:initialization");`, 1)
	testSourceTargetOriginalFamilyFixture(t, fixture, "DormantAccessOwner", "DormantAccessDriver", "accessor:generic:erasure:identity:initialization\n")
}

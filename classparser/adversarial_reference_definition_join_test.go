package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Three definition arms share a consumer interface. The first two also share
// an unrelated interface: choosing one representative LUB before inspecting
// the third arm can lose the common bound. An unchanged original driver owns
// every assertion and observes cache identity and branch evaluation order.
func TestAdversarialReferenceWebKeepsEveryOriginalDefinitionBound(t *testing.T) {
	fixture := `interface JoinFirst{int kind();}interface JoinSecond{int kind();}
class JoinLeft implements JoinFirst,JoinSecond{public int kind(){return 11;}}
class JoinRight implements JoinFirst,JoinSecond{public int kind(){return 23;}}
class JoinThird implements JoinSecond{public int kind(){return 37;}}
class JoinEffects{static String trace="";static boolean choose(int mode,int test){trace+="C"+test;return mode==test;}static JoinLeft left(){trace+="L";return new JoinLeft();}static JoinRight right(){trace+="R";return new JoinRight();}static JoinThird third(){trace+="T";return new JoinThird();}}
class JoinOwner{JoinSecond cached;JoinSecond resolve(int mode){JoinSecond value=cached;if(value==null){value=JoinEffects.choose(mode,0)?JoinEffects.left():JoinEffects.choose(mode,1)?JoinEffects.right():JoinEffects.third();cached=value;}return value;}}
class JoinDriver{public static void main(String[]args){int rows=0;for(int mode:new int[]{0,1,2,-1,Integer.MIN_VALUE,Integer.MAX_VALUE}){JoinOwner root=new JoinOwner();JoinEffects.trace="";JoinSecond first=root.resolve(mode),second=root.resolve(0);int expected=mode==0?11:mode==1?23:37;String trace=mode==0?"C0L":mode==1?"C0C1R":"C0C1T";if(first.kind()!=expected||first!=second||first!=root.cached||!JoinEffects.trace.equals(trace))throw new AssertionError("definition/consumer/order/cache identity");rows++;}System.out.println(rows+":all-definitions:common-bound:identity:order");}}`
	for _, shape := range []string{"conditional", "separate stores", "uninitialized join"} {
		t.Run(shape, func(t *testing.T) {
			f := fixture
			if shape != "conditional" {
				f = strings.ReplaceAll(f, "value=JoinEffects.choose(mode,0)?JoinEffects.left():JoinEffects.choose(mode,1)?JoinEffects.right():JoinEffects.third();", "if(JoinEffects.choose(mode,0)){value=JoinEffects.left();}else if(JoinEffects.choose(mode,1)){value=JoinEffects.right();}else{value=JoinEffects.third();}")
			}
			if shape == "uninitialized join" {
				f = strings.ReplaceAll(f, "JoinSecond value=cached;if(value==null){", "if(cached!=null)return cached;JoinSecond value;{")
			}
			testNativeIndependentFamilyFixture(t, f, []string{"JoinOwner"}, "JoinDriver", "6:all-definitions:common-bound:identity:order\n", nativeLexicalExactSignatures)
		})
	}
}

// The same original classes can provide declarations without belonging to the
// emitted archive. javac and the unchanged JVM oracle still see those originals.
func TestAdversarialReferenceWebUsesExternalOriginalDeclarations(t *testing.T) {
	const fixture = `interface CacheView{int value();}class CacheValue implements CacheView{final int n;CacheValue(int n){this.n=n;}public int value(){return n;}}
class CacheConfig{CacheView compile(int n){return new CacheValue(n+1);}}
class CacheOwner{CacheView cached;CacheView resolve(CacheConfig config,int n){CacheView value=cached;if(value==null){if(config==null){value=new CacheValue(n);}else{value=config.compile(n);}cached=value;}return value;}}
class CacheDriver{public static void main(String[]args){for(boolean configured:new boolean[]{false,true}){CacheOwner o=new CacheOwner();CacheView a=o.resolve(configured?new CacheConfig():null,Integer.MAX_VALUE),b=o.resolve(null,11);if(a!=b||a!=o.cached||a.value()!=(configured?Integer.MIN_VALUE:Integer.MAX_VALUE))throw new AssertionError("cache identity/definition binding/overflow");}System.out.println("2:external:cache:identity:overflow");}}`
	javac, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, fixture, debug)
			original := t.TempDir()
			for n, b := range files {
				if e := os.WriteFile(filepath.Join(original, n), b, 0600); e != nil {
					t.Fatal(e)
				}
			}
			oracle := t04RunJava(t, java, original, "CacheDriver")
			if oracle != "2:external:cache:identity:overflow\n" {
				t.Fatal(oracle)
			}
			for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
				t.Run(policy, func(t *testing.T) {
					if policy == "no-source-rewrites" {
						t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
					}
					if policy == "no-core-cleanups" {
						t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
					}
					z := nativeArchive(t, map[string][]byte{"CacheOwner.class": files["CacheOwner.class"]})
					defer z.Close()
					z.declarationResolver = func(n string) ([]byte, bool) { b, ok := files[n+".class"]; return b, ok }
					src, e := z.ReadFile("CacheOwner.class")
					if e != nil || strings.Contains(string(src), DecompileStubMarker) {
						t.Fatalf("source: %v\n%s", e, src)
					}
					out := t.TempDir()
					for n, b := range files {
						if n == "CacheOwner.class" {
							continue
						}
						if e := os.WriteFile(filepath.Join(out, n), b, 0600); e != nil {
							t.Fatal(e)
						}
					}
					p := filepath.Join(out, "CacheOwner.java")
					if e := os.WriteFile(p, src, 0600); e != nil {
						t.Fatal(e)
					}
					if b, e := exec.Command(javac, "-proc:none", "--release", "8", "-cp", out, "-d", out, p).CombinedOutput(); e != nil {
						t.Fatalf("rebuilt: %v\n%s\n%s", e, b, src)
					}
					if got := t04RunJava(t, java, out, "CacheDriver"); got != oracle {
						t.Fatalf("rebuilt %q want %q", got, oracle)
					}
					rebuilt, e := os.ReadFile(filepath.Join(out, "CacheOwner.class"))
					if e != nil {
						t.Fatal(e)
					}
					if nativeBinaryShape(t, rebuilt) != nativeBinaryShape(t, files["CacheOwner.class"]) {
						t.Fatal("owner ABI changed")
					}
					nativeLexicalExactSignatures(t, "CacheOwner.class", files["CacheOwner.class"], rebuilt)
				})
			}
		})
	}
}

func TestAdversarialReferenceWebUsesOriginalPlatformAncestry(t *testing.T) {
	const fixture = `class PlatformCacheOwner{static java.util.Random cached;static java.util.Random get(){java.util.Random value=cached;if(value==null){value=new java.security.SecureRandom();cached=value;}return value;}}
class PlatformCacheDriver{public static void main(String[]args){java.util.Random a=PlatformCacheOwner.get(),b=PlatformCacheOwner.get();if(a==null||a!=b||a!=PlatformCacheOwner.cached||a.getClass()!=java.security.SecureRandom.class)throw new AssertionError("platform original ancestry and cache identity");a.setSeed(17);for(int i=0;i<100;i++){int x=a.nextInt(7);if(x<0||x>=7)throw new AssertionError("bounded dispatch");}System.out.println("platform:original-parent:cache:identity:dispatch");}}`
	testNativeIndependentFamilyFixture(t, fixture, []string{"PlatformCacheOwner"}, "PlatformCacheDriver", "platform:original-parent:cache:identity:dispatch\n", nativeLexicalExactSignatures)
}

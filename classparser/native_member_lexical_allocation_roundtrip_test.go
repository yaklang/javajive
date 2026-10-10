package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A sibling allocation uses this constructor's original enclosing capture,
// rather than an explicit qualifier. In particular javac must not introduce a
// qualifier null check: reflective construction may supply a null enclosing
// instance even though ordinary qualified construction rejects null.
func TestNativeMemberSiblingAllocationUsesOriginalLexicalEnclosing(t *testing.T) {
	const source = `class LexicalEffects{static String trace="";static Object published;static boolean fail;static IllegalArgumentException error=new IllegalArgumentException("same");static long arg(long n){trace+="A";return n;}}
class LexicalParent{final Object observed;LexicalParent(long n){LexicalEffects.trace+="P";observed=owner();LexicalEffects.published=this;if(LexicalEffects.fail)throw LexicalEffects.error;}Object owner(){return null;}}
class LexicalOwner<K>{
 class Target<U> extends LexicalParent{final U value;final long number;Target(U value,long number){super(0L);LexicalEffects.trace+="C";this.value=value;this.number=number;}Object owner(){return LexicalOwner.this;}}
 class Actor<Z>{<U> Target<U> make(U value,long n){return new Target<U>(value,LexicalEffects.arg(n));}<Target> LexicalOwner<K>.Target<Target> explicit(Target value,long n){return LexicalOwner.this.new Target<Target>(value,LexicalEffects.arg(n));}}
 Actor<String> actor(){return new Actor<String>();}
}
class LexicalDriver{public static void main(String[]args)throws Exception{
 LexicalOwner<Integer> owner=new LexicalOwner<Integer>();Object token=new Object();int rows=0;
 java.lang.reflect.Constructor<?> ctor=LexicalOwner.Actor.class.getDeclaredConstructor(LexicalOwner.class);ctor.setAccessible(true);
 for(boolean nullOuter:new boolean[]{false,true}){
 LexicalOwner<Integer>.Actor<String> a=nullOuter?(LexicalOwner<Integer>.Actor<String>)ctor.newInstance(new Object[]{null}):owner.actor();
 for(boolean fail:new boolean[]{false,true})for(Object v:new Object[]{null,token})for(long n:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){
 LexicalEffects.fail=fail;LexicalEffects.trace="";LexicalEffects.published=null;
 try{LexicalOwner<Integer>.Target<Object> c=a.make(v,n);if(fail||c.value!=v||c.number!=n||c.owner()!=(nullOuter?null:owner)||c.observed!=(nullOuter?null:owner)||LexicalEffects.published!=c||!LexicalEffects.trace.equals("APC"))throw new AssertionError("capture/value/order");}
 catch(IllegalArgumentException e){LexicalOwner<Integer>.Target<Object> c=(LexicalOwner<Integer>.Target<Object>)LexicalEffects.published;if(!fail||e!=LexicalEffects.error||c.owner()!=(nullOuter?null:owner)||c.observed!=(nullOuter?null:owner)||c.value!=null||c.number!=0||!LexicalEffects.trace.equals("AP"))throw new AssertionError("publication/failure");}rows++;
 }
 LexicalEffects.fail=false;LexicalEffects.trace="";LexicalEffects.published=null;
 try{LexicalOwner<Integer>.Target<Object> c=a.explicit(token,Long.MAX_VALUE);if(nullOuter||c.value!=token||c.number!=Long.MAX_VALUE||c.owner()!=owner||!LexicalEffects.trace.equals("APC"))throw new AssertionError("explicit qualifier");}
 catch(NullPointerException expected){if(!nullOuter||!LexicalEffects.trace.equals("")||LexicalEffects.published!=null)throw new AssertionError("qualifier priority");}
 }System.out.println(rows+":"+LexicalEffects.trace+":"+LexicalOwner.Target.class.getDeclaredConstructor(LexicalOwner.class,Object.class,long.class).getParameterCount());
}}`
	javac, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, source, debug)
			original := t.TempDir()
			for n, b := range files {
				if err := os.WriteFile(filepath.Join(original, n), b, 0600); err != nil {
					t.Fatal(err)
				}
			}
			oracle := t04RunJava(t, java, original, "LexicalDriver")
			if oracle != "24::3\n" {
				t.Fatalf("independent oracle %q", oracle)
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
					child, err := z.ReadFile("LexicalOwner$Target.class")
					if err != nil || !strings.Contains(string(child), "original member body owned by") {
						t.Fatalf("ownership %v\n%s", err, child)
					}
					src, err := z.ReadFile("LexicalOwner.class")
					if err != nil || strings.Contains(string(src), DecompileStubMarker) {
						t.Fatalf("source %v\n%s", err, src)
					}
					output := t.TempDir()
					for n, b := range files {
						if strings.HasPrefix(n, "LexicalOwner") {
							continue
						}
						if err := os.WriteFile(filepath.Join(output, n), b, 0600); err != nil {
							t.Fatal(err)
						}
					}
					file := filepath.Join(output, "LexicalOwner.java")
					if err := os.WriteFile(file, src, 0600); err != nil {
						t.Fatal(err)
					}
					if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", output, "-d", output, file).CombinedOutput(); err != nil {
						t.Fatalf("rebuilt %v %s\n%s", err, out, src)
					}
					if got := t04RunJava(t, java, output, "LexicalDriver"); got != oracle {
						t.Fatalf("lexical capture semantics changed: %q != %q", got, oracle)
					}
					for _, n := range []string{"LexicalOwner", "LexicalOwner$Actor", "LexicalOwner$Target"} {
						raw, err := os.ReadFile(filepath.Join(output, n+".class"))
						if err != nil {
							t.Fatal(err)
						}
						if want, got := nativeBinaryShape(t, files[n+".class"]), nativeBinaryShape(t, raw); got != want {
							t.Fatalf("original ABI changed %s\n%s\n%s", n, want, got)
						}
					}
				})
			}
		})
	}
}

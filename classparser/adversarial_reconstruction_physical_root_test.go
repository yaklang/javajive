package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdversarialReconstructionPhysicalRootKeepsAccessAndAnonymousDispatch(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	for _, root := range []string{"AccessRoot", "ReboundAccessRoot"} {
		for _, visibility := range []string{"", "public "} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(root+"/"+strings.TrimSpace(visibility)+"/"+debug, func(t *testing.T) {
					source := fmt.Sprintf(`public class %[1]s {
 %[2]sstatic class Hidden {
  private final long seed;public Hidden(long seed){this.seed=seed;}
  public java.util.function.LongUnaryOperator operator(final long bias){return new java.util.function.LongUnaryOperator(){public long applyAsLong(long x){return x*31+seed+bias;}};}
 }
 public static java.util.function.LongUnaryOperator create(long seed,long bias){return new Hidden(seed).operator(bias);}
 public static Class<?> type(){return Hidden.class;}
}
class PhysicalRootDriver {public static void main(String[]args)throws Throwable {
 int rows=0;for(long seed:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(long bias:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){
 java.util.function.LongUnaryOperator op=%[1]s.create(seed,bias);
 if(!op.getClass().getName().equals("%[1]s$Hidden$1")||op.getClass().getEnclosingClass()!=%[1]s.type()||!op.getClass().getEnclosingMethod().getName().equals("operator"))throw new AssertionError("anonymous scope");
 for(long x:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){long expected=java.math.BigInteger.valueOf(x).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(seed)).add(java.math.BigInteger.valueOf(bias)).longValue();if(op.applyAsLong(x)!=expected)throw new AssertionError("captured dispatch/overflow");rows++;}}
 boolean granted=false;try{java.lang.invoke.MethodHandles.publicLookup().findVirtual(%[1]s.type(),"operator",java.lang.invoke.MethodType.methodType(java.util.function.LongUnaryOperator.class,long.class));granted=true;}catch(IllegalAccessException expected){}
 if(granted!=%[3]t)throw new AssertionError("physical public Lookup access changed");
 System.out.println(rows+":physical:access:capture:dispatch:overflow");}}
`, root, visibility, visibility != "")
					files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, source, root, debug, false)
					run := func(dir string) {
						t.Helper()
						for _, mode := range []string{"-ea", "-da"} {
							out, err := enumFreshOracleRun(t, java, mode, dir, "PhysicalRootDriver")
							if err != nil || string(out) != "125:physical:access:capture:dispatch:overflow\n" {
								t.Fatal("unchanged original caller", err, string(out))
							}
						}
					}
					run(sourceArtifactWriteFiles(t, files))
					owner := root + "$Hidden"
					owned := map[string][]byte{owner + ".class": files[owner+".class"], owner + "$1.class": files[owner+"$1.class"]}
					jar := filepath.Join(t.TempDir(), "input.jar")
					if err := os.WriteFile(jar, t23Zip(t, owned), 0600); err != nil {
						t.Fatal(err)
					}
					a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, func(name string) ([]byte, bool) { b, k := files[name+".class"]; return b, k })
					if err != nil {
						t.Fatal(err)
					}
					defer a.Close()
					rendered, err := a.ReadFile(owner + ".class")
					if err != nil || legacySourceHasFailureMarker(string(rendered)) {
						t.Fatal("complete source root", err, string(rendered))
					}
					folded, err := a.ReadFile(owner + "$1.class")
					if err != nil || !strings.HasPrefix(strings.TrimSpace(string(folded)), "// ") || !strings.Contains(string(folded), "owned by "+owner+";") {
						t.Fatal("actual owned anonymous child required", err, string(folded))
					}
					external := map[string][]byte{}
					for n, b := range files {
						if _, owned := owned[n]; !owned {
							external[n] = b
						}
					}
					out := sourceArtifactWriteFiles(t, external)
					path := filepath.Join(out, owner+".java")
					if err := os.WriteFile(path, rendered, 0600); err != nil {
						t.Fatal(err)
					}
					if log, err := exec.Command(modern, "-proc:none", "--release", "8", "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out, path).CombinedOutput(); err != nil {
						t.Fatal("fresh root without owned originals", err, string(log), string(rendered))
					}
					for _, name := range []string{owner + ".class", owner + "$1.class"} {
						path := filepath.Join(out, name)
						fresh, err := os.ReadFile(path)
						if err != nil {
							t.Fatal(err)
						}
						if name == owner+".class" && mustParseLowering(t, fresh).AccessFlags != mustParseLowering(t, files[name]).AccessFlags {
							t.Fatal("physical header changed before metadata", string(rendered))
						}
						finished, err := a.FinishClass(name, fresh)
						if err != nil {
							t.Fatal("finish complete native source", name, err)
						}
						artifactLambdaMetadataCodeUnchanged(t, fresh, finished)
						if err := os.WriteFile(path, finished, 0600); err != nil {
							t.Fatal(err)
						}
					}
					run(out)
				})
			}
		}
	}
}

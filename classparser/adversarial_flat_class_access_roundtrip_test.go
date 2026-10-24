package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdversarialFlatClassKeepsPhysicalAccessWithoutLexicalOwnerSource(t *testing.T) {
	compiler, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	for _, kind := range []string{"class", "abstract class", "interface"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(kind+"/"+debug, func(t *testing.T) {
				fixture := `public class AccessOwner {private static ` + kind + ` Hidden {
 public static long fold(long value){return value*31+7;}
}}
class AccessDriver {public static void main(String[]args)throws Throwable {
 Class<?> c=Class.forName("AccessOwner$Hidden");
 if(java.lang.reflect.Modifier.isPublic(c.getModifiers()))throw new AssertionError("lexical visibility");
 try{java.lang.invoke.MethodHandles.publicLookup().findStatic(c,"fold",java.lang.invoke.MethodType.methodType(long.class,long.class));throw new AssertionError("physical class access widened");}catch(IllegalAccessException expected){}
 java.lang.reflect.Method m=c.getDeclaredMethod("fold",long.class);m.setAccessible(true);
 int rows=0;for(long value:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){long want=java.math.BigInteger.valueOf(value).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(7)).longValue();if(((Long)m.invoke(null,value)).longValue()!=want)throw new AssertionError("overflow/binding");rows++;}
 System.out.println(rows+":physical:access:lookup:algorithm");}}
`
				files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, fixture, "AccessOwner", debug, false)
				run := func(dir string) {
					t.Helper()
					for _, mode := range []string{"-ea", "-da"} {
						log, err := enumFreshOracleRun(t, java, mode, dir, "AccessDriver")
						if err != nil || string(log) != "5:physical:access:lookup:algorithm\n" {
							t.Fatal("original-first physical access", err, string(log))
						}
					}
				}
				run(sourceArtifactWriteFiles(t, files))
				const child = "AccessOwner$Hidden.class"
				jar := filepath.Join(t.TempDir(), "flat.jar")
				if err := os.WriteFile(jar, t23Zip(t, map[string][]byte{child: files[child]}), 0600); err != nil {
					t.Fatal(err)
				}
				a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer a.Close()
				source, err := a.ReadFile(child)
				if err != nil || legacySourceHasFailureMarker(string(source)) || !strings.Contains(string(source), "AccessOwner$Hidden") {
					t.Fatal("independent flat declaration", err, string(source))
				}
				deps := map[string][]byte{}
				for n, b := range files {
					if n != child {
						deps[n] = b
					}
				}
				out := sourceArtifactWriteFiles(t, deps)
				path := filepath.Join(out, "AccessOwner$Hidden.java")
				if err := os.WriteFile(path, source, 0600); err != nil {
					t.Fatal(err)
				}
				if log, err := exec.Command(compiler, "-proc:none", "--release", "8", "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out, path).CombinedOutput(); err != nil {
					t.Fatal("flat compile without owned original", err, string(log))
				}
				fresh, err := os.ReadFile(filepath.Join(out, child))
				if err != nil {
					t.Fatal(err)
				}
				finished, err := a.FinishClass(child, fresh)
				if err != nil {
					t.Fatal("physical declaration access must precede metadata", err)
				}
				if err := os.WriteFile(filepath.Join(out, child), finished, 0600); err != nil {
					t.Fatal(err)
				}
				run(out)
			})
		}
	}
}

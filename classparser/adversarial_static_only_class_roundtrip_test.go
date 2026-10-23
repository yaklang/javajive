package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdversarialStaticOnlyClassLoweringDoesNotDependOnSwitchNames(t *testing.T) {
	compiler, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	for _, owner := range []string{"StaticPacket", "RenamedComputation"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(owner+"/"+debug, func(t *testing.T) {
				fixture := strings.ReplaceAll(`class StaticPacket {static int calls;static long fold(long input){calls++;return input*31+calls;}}
class StaticOnlyDriver {public static void main(String[]args)throws Exception {
 Class<?> c=Class.forName("StaticPacket",false,StaticOnlyDriver.class.getClassLoader());if(!c.isSynthetic()||c.getDeclaredConstructors().length!=0)throw new AssertionError("constructor/class marker");
 int rows=0;for(long input:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){rows++;long want=java.math.BigInteger.valueOf(input).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(rows)).longValue();if(StaticPacket.fold(input)!=want||StaticPacket.calls!=rows)throw new AssertionError("static effects/algorithm");}
 System.out.println(rows+":static:class:effects:algorithm");}}
`, "StaticPacket", owner)
				files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, fixture, owner, debug, false)
				// Source has no syntax for constructor absence. Construct this
				// legal authored original by changing declarations only; all
				// executable bodies and its original binary caller stay intact.
				original := mustParseLowering(t, files[owner+".class"])
				methods := []*MemberInfo{}
				for _, m := range original.Methods {
					name, _ := sourceBridgeUTF8(original, m.NameIndex)
					if name != "<init>" {
						methods = append(methods, m)
					}
				}
				original.Methods = methods
				original.AccessFlags |= 0x1000
				files[owner+".class"] = original.Bytes()
				run := func(dir string) {
					t.Helper()
					for _, mode := range []string{"-ea", "-da"} {
						log, err := enumFreshOracleRun(t, java, mode, dir, "StaticOnlyDriver")
						if err != nil || string(log) != "5:static:class:effects:algorithm\n" {
							t.Fatal("legal original-first static class oracle", err, string(log))
						}
					}
				}
				run(sourceArtifactWriteFiles(t, files))
				jar := filepath.Join(t.TempDir(), "static.jar")
				if err := os.WriteFile(jar, t23Zip(t, map[string][]byte{owner + ".class": files[owner+".class"]}), 0600); err != nil {
					t.Fatal(err)
				}
				a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer a.Close()
				source, err := a.ReadFile(owner + ".class")
				if err != nil || legacySourceHasFailureMarker(string(source)) {
					t.Fatal("arbitrary static class source", err, string(source))
				}
				out := sourceArtifactWriteFiles(t, map[string][]byte{"StaticOnlyDriver.class": files["StaticOnlyDriver.class"]})
				path := filepath.Join(out, owner+".java")
				if err := os.WriteFile(path, source, 0600); err != nil {
					t.Fatal(err)
				}
				if log, err := exec.Command(compiler, "-proc:none", "--release", "8", "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out, path).CombinedOutput(); err != nil {
					t.Fatal("fresh class without any owned original", err, string(log))
				}
				fresh, err := os.ReadFile(filepath.Join(out, owner+".class"))
				if err != nil {
					t.Fatal(err)
				}
				finished, err := a.FinishClass(owner+".class", fresh)
				if err != nil {
					t.Fatal("arbitrary static class lowering", err)
				}
				if err := os.WriteFile(filepath.Join(out, owner+".class"), finished, 0600); err != nil {
					t.Fatal(err)
				}
				run(out)
			})
		}
	}
}

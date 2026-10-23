package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Constructor absence and ACC_SYNTHETIC are JVM declarations, not a javac8
// source idiom. Use actual compiler targets rather than changing a version
// header: old verifier inputs and modern classfile attributes must both work.
func TestAdversarialStaticClassVersionKeepsConstructorAbsence(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	for _, release := range []int{5, 6, 7, 8, 11, 17, 21} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, empty := range []bool{false, true} {
				t.Run(fmt.Sprintf("release%d/%s/empty=%t", release, debug, empty), func(t *testing.T) {
					const owner = "versioned/Uninstantiable$Packet"
					body := `static int calls; static long fold(long x) { calls++; return x * 31 + calls; }`
					check := `int rows=0; for(long x:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}) { rows++; long want=java.math.BigInteger.valueOf(x).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(rows)).longValue(); if(Uninstantiable$Packet.fold(x)!=want || Uninstantiable$Packet.calls!=rows)throw new AssertionError("effects/arithmetic"); }`
					if empty {
						body, check = "", "int rows=0;"
					}
					fixture := `package versioned; class Uninstantiable$Packet {` + body + `}
public class VersionDriver { public static void main(String[] args) throws Exception {
Class<?> c=Class.forName("versioned.Uninstantiable$Packet",false,VersionDriver.class.getClassLoader());
if(!c.isSynthetic() || c.getDeclaredConstructors().length!=0) throw new AssertionError("original declarations");
try { c.getDeclaredConstructor(); throw new AssertionError("invented default constructor"); } catch(NoSuchMethodException expected) {}
` + check + ` System.out.println(rows+":version:static:constructor:absence"); }} `
					dir := t.TempDir()
					input := filepath.Join(dir, "VersionDriver.java")
					if err := os.WriteFile(input, []byte(fixture), 0600); err != nil {
						t.Fatal(err)
					}
					compiler := modern
					args := []string{"-proc:none", "--release", fmt.Sprint(release)}
					if release <= 8 {
						compiler = native
						args = []string{"-proc:none", "-source", fmt.Sprint(release), "-target", fmt.Sprint(release)}
					}
					args = append(args, "-g:"+debug, "-d", dir, input)
					if log, err := exec.Command(compiler, args...).CombinedOutput(); err != nil {
						t.Fatal("actual original target compiler", err, string(log))
					}
					read := func(name string) []byte {
						t.Helper()
						raw, err := os.ReadFile(filepath.Join(dir, name+".class"))
						if err != nil {
							t.Fatal(err)
						}
						return raw
					}
					original := mustParseLowering(t, read(owner))
					if int(original.MajorVersion) != 44+release {
						t.Fatal("compiler did not produce requested version")
					}
					// Java cannot declare constructor absence. Remove only its
					// declaration and mark the authored static class synthetic;
					// the static algorithm and original binary driver are intact.
					var methods []*MemberInfo
					for _, method := range original.Methods {
						name, _ := sourceBridgeUTF8(original, method.NameIndex)
						if name != "<init>" {
							methods = append(methods, method)
						}
					}
					original.Methods = methods
					original.AccessFlags |= 0x1000
					files := map[string][]byte{owner + ".class": original.Bytes(), "versioned/VersionDriver.class": read("versioned/VersionDriver")}
					run := func(dir string) {
						t.Helper()
						rows := 5
						if empty {
							rows = 0
						}
						for _, mode := range []string{"-ea", "-da"} {
							log, err := enumFreshOracleRun(t, java, mode, dir, "versioned.VersionDriver")
							if err != nil || string(log) != fmt.Sprintf("%d:version:static:constructor:absence\n", rows) {
								t.Fatal("original-first declaration/algorithm oracle", err, string(log))
							}
						}
					}
					run(sourceArtifactWriteFiles(t, files))
					jar := filepath.Join(t.TempDir(), "original.jar")
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
						t.Fatal("complete static declaration source", err, string(source))
					}
					out := sourceArtifactWriteFiles(t, map[string][]byte{"versioned/VersionDriver.class": files["versioned/VersionDriver.class"], owner + ".java": source})
					if log, err := exec.Command(modern, "-proc:none", "--release", "8", "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out, filepath.Join(out, owner+".java")).CombinedOutput(); err != nil {
						t.Fatal("fresh independent compiler", err, string(log), string(source))
					}
					path := filepath.Join(out, owner+".class")
					fresh, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					finished, err := a.FinishClass(owner+".class", fresh)
					if err != nil {
						t.Fatal("same original static declaration contract", err)
					}
					if err := os.WriteFile(path, finished, 0600); err != nil {
						t.Fatal(err)
					}
					run(out)
				})
			}
		}
	}
}

package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Attribute introduction and VM recognition are different boundaries. HotSpot
// recognizes MethodParameters on these valid older classfiles too. An original
// JVM reflection oracle must decide whether a final declaration is observable;
// classfile major alone cannot make the actual table disappear.
// https://github.com/openjdk/jdk21u/blob/master/src/hotspot/share/classfile/classFileParser.cpp
func TestAdversarialParameterFinalityOnOlderClassfiles(t *testing.T) {
	javac, java := t04Tools(t)
	fixture := `class ParameterVersionOwner {
 final int count; final Object token;
 ParameterVersionOwner(final int count,Object token){this.count=count;this.token=token;}
 long read(final long amount,final double ratio,Object marker){return amount+(long)ratio+(marker==null?0:1)+count;}
}
abstract class ParameterVersionAbstract {protected abstract int apply(final long amount,Object marker);}
class ParameterVersionDriver {
 static void flags(java.lang.reflect.Executable method,int... expected){
  java.lang.reflect.Parameter[] parameters=method.getParameters();
  if(parameters.length!=expected.length)throw new AssertionError("logical parameter count");
  for(int i=0;i<expected.length;i++)if(parameters[i].getModifiers()!=expected[i])throw new AssertionError("final declaration at "+i+": "+parameters[i].getModifiers());
 }
 public static void main(String[] args)throws Exception{
  Object token=new Object();ParameterVersionOwner owner=new ParameterVersionOwner(3,token);
  if(owner.token!=token||owner.read(9,2,token)!=15||owner.read(-9,-2,null)!=-8)throw new AssertionError("entry identities");
  flags(ParameterVersionOwner.class.getDeclaredConstructor(int.class,Object.class),16,0);
  flags(ParameterVersionOwner.class.getDeclaredMethod("read",long.class,double.class,Object.class),16,16,0);
  flags(ParameterVersionAbstract.class.getDeclaredMethod("apply",long.class,Object.class),16,0);
  System.out.println("parameter-finality:16,0:16,16,0:16,0:identity");
 }
}`
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			compiled := t.TempDir()
			input := filepath.Join(compiled, "Authored.java")
			if err := os.WriteFile(input, []byte(fixture), 0600); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-parameters", "-g:"+debug, "-d", compiled, input).CombinedOutput(); err != nil {
				t.Fatalf("original compile: %v\n%s", err, out)
			}
			originals := map[string][]byte{}
			for _, name := range []string{"ParameterVersionOwner", "ParameterVersionAbstract", "ParameterVersionDriver"} {
				raw, err := os.ReadFile(filepath.Join(compiled, name+".class"))
				if err != nil {
					t.Fatal(err)
				}
				originals[name+".class"] = raw
			}
			for _, major := range []uint16{45, 49, 50, 51, 52} {
				t.Run(fmt.Sprint(major), func(t *testing.T) {
					files := map[string][]byte{}
					original := t.TempDir()
					for name, raw := range originals {
						files[name] = raw
						if name != "ParameterVersionDriver.class" {
							obj, err := Parse(raw)
							if err != nil {
								t.Fatal(err)
							}
							obj.MajorVersion, obj.MinorVersion = major, 0
							files[name] = obj.Bytes()
						}
						// Keep the independently compiled caller unchanged. Its
						// reflection class literals require a newer instruction
						// contract than the actual target classes being studied.
						if err := os.WriteFile(filepath.Join(original, name), files[name], 0600); err != nil {
							t.Fatal(err)
						}
					}
					run := func(dir, stage string) string {
						t.Helper()
						out, err := exec.Command(java, "-Xverify:all", "-cp", dir, "ParameterVersionDriver").CombinedOutput()
						if err != nil || string(out) != "parameter-finality:16,0:16,16,0:16,0:identity\n" {
							t.Fatalf("%s JVM oracle: %v\n%s", stage, err, out)
						}
						t.Logf("%s JVM original declaration flags: %q", stage, out)
						return string(out)
					}
					oracle := run(original, "original")
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
							rebuilt := t.TempDir()
							var sourcePaths []string
							for _, name := range []string{"ParameterVersionOwner", "ParameterVersionAbstract"} {
								text, err := z.ReadFile(name + ".class")
								if err != nil || strings.Contains(string(text), DecompileStubMarker) || strings.Contains(string(text), "decompile dump failed") {
									t.Fatalf("source %s: %v\n%s", name, err, text)
								}
								path := filepath.Join(rebuilt, name+".java")
								if err := os.WriteFile(path, text, 0600); err != nil {
									t.Fatal(err)
								}
								sourcePaths = append(sourcePaths, path)
							}
							if err := os.WriteFile(filepath.Join(rebuilt, "ParameterVersionDriver.class"), files["ParameterVersionDriver.class"], 0600); err != nil {
								t.Fatal(err)
							}
							args := []string{"-proc:none", "--release", "8", "-parameters", "-g:" + debug, "-d", rebuilt}
							if out, err := exec.Command(javac, append(args, sourcePaths...)...).CombinedOutput(); err != nil {
								t.Fatalf("rebuilt compile: %v\n%s", err, out)
							}
							if got := run(rebuilt, "rebuilt"); got != oracle {
								t.Fatalf("entry values or declaration flags changed: %q != %q", got, oracle)
							}
							for _, name := range []string{"ParameterVersionOwner", "ParameterVersionAbstract"} {
								raw, err := os.ReadFile(filepath.Join(rebuilt, name+".class"))
								if err != nil || nativeBinaryShape(t, raw) != nativeBinaryShape(t, files[name+".class"]) {
									t.Fatalf("original declaration ABI changed %s: %v", name, err)
								}
								nativeLexicalExactSignatures(t, name, files[name+".class"], raw)
							}
						})
					}
				})
			}
		})
	}
}

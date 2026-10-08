package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdversarialAnonymousInitializerCompilerProfilesRetainOriginalLifecycle(t *testing.T) {
	testAnonymousInitializerCompilerProfiles(t, []string{"modern_release8", "modern_release21", "native8", "native7"}, false)
}

func TestAdversarialAnonymousSerializableInitializerCompilerProfiles(t *testing.T) {
	testAnonymousInitializerCompilerProfiles(t, []string{"modern_release21"}, true)
}

func testAnonymousInitializerCompilerProfiles(t *testing.T, profiles []string, serializable bool) {
	t.Helper()
	modern, java := t04Tools(t)
	legacy := os.Getenv("JAVA8_JAVAC")
	if legacy != "" {
		version, e := exec.Command(legacy, "-version").CombinedOutput()
		if e != nil || !strings.Contains(string(version), "javac 1.8.") {
			t.Fatal(e, string(version))
		}
	}
	for _, compiler := range profiles {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, static := range []bool{false, true} {
				for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
					t.Run(fmt.Sprintf("%s/%s/static=%t/%s", compiler, debug, static, policy), func(t *testing.T) {
						if strings.HasPrefix(compiler, "native") && legacy == "" {
							t.Skip("JAVA8_JAVAC required for independent native compiler oracle")
						}
						if policy == "no-source-rewrites" {
							t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
						}
						if policy == "no-core-cleanups" {
							t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
						}
						modifier := ""
						if static {
							modifier = "static "
						}
						ancestry := ""
						if serializable {
							ancestry = " implements java.io.Serializable"
						}
						sources := map[string]string{
							"InitEffects.java": `class InitEffects{static long input;static String trace="";static long arg(){trace+="A";return input;}}`,
							"InitParent.java":  `class InitParent` + ancestry + `{final long word;InitParent(long word){InitEffects.trace+="P";this.word=word;}long get(){return word;}}`,
							"InitTarget.java":  `class InitTarget{` + modifier + `final InitParent value=new InitParent(InitEffects.arg()){long get(){return word^0xCAFEBABEL;}};InitTarget(){InitEffects.trace+="C";}}`,
							"InitDriver.java":  `import java.lang.reflect.*;import java.util.*;class InitDriver{public static void main(String[]args)throws Exception{int rows=0;for(long word:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){InitEffects.input=word;InitEffects.trace="";InitTarget root=new InitTarget();InitParent v=root.value;boolean shared=Modifier.isStatic(InitTarget.class.getDeclaredField("value").getModifiers());long expected=shared?Long.MIN_VALUE:word;if(v.word!=expected||v.get()!=(expected^0xCAFEBABEL)||!InitEffects.trace.equals(shared&&rows>0?"C":"APC"))throw new AssertionError("initialization/value/order");Class<?> c=v.getClass();if(!c.isAnonymousClass()||c.getEnclosingClass()!=InitTarget.class||c.getEnclosingMethod()!=null||c.getEnclosingConstructor()!=null)throw new AssertionError("initializer ownership");if(v instanceof java.io.Serializable){java.io.ByteArrayOutputStream bytes=new java.io.ByteArrayOutputStream();try{new java.io.ObjectOutputStream(bytes).writeObject(v);if(!shared)throw new AssertionError("missing captured owner serialization failure");Object copy=new java.io.ObjectInputStream(new java.io.ByteArrayInputStream(bytes.toByteArray())).readObject();if(((InitParent)copy).get()!=v.get())throw new AssertionError("serialized dispatch/value");}catch(java.io.NotSerializableException e){if(shared||!e.getMessage().equals("InitTarget"))throw new AssertionError("serialization failure ownership",e);}System.out.println("serial:"+java.io.ObjectStreamClass.lookup(c).getSerialVersionUID());}Constructor<?> ctor=c.getDeclaredConstructors()[0];List<String> fields=new ArrayList<String>();for(Field f:c.getDeclaredFields())fields.add(f.getName()+":"+f.getType().getName()+":"+f.getModifiers()+":"+f.isSynthetic());Collections.sort(fields);System.out.println(c.getName()+":"+c.getModifiers()+":"+Arrays.toString(ctor.getParameterTypes())+":"+Arrays.toString(ctor.getExceptionTypes())+":"+fields);rows++;}System.out.println("rows:"+rows);}}`}
						original := t.TempDir()
						for name, text := range sources {
							if e := os.WriteFile(filepath.Join(original, name), []byte(text), 0600); e != nil {
								t.Fatal(e)
							}
						}
						javac := modern
						level := "8"
						if compiler == "modern_release21" {
							level = "21"
						}
						args := []string{"-proc:none", "--release", level}
						profile := ModernJavac
						if strings.HasPrefix(compiler, "native") {
							javac = legacy
							level := "8"
							if compiler == "native7" {
								level = "7"
							}
							args = []string{"-proc:none", "-source", level, "-target", level}
							profile = NativeJavac8
						}
						args = append(args, "-g:"+debug, "-d", original)
						for name := range sources {
							args = append(args, filepath.Join(original, name))
						}
						if out, e := exec.Command(javac, args...).CombinedOutput(); e != nil {
							t.Fatalf("original compile %v %s", e, out)
						}
						oracle := t04RunJava(t, java, original, "InitDriver")
						if !strings.Contains(oracle, "rows:5\n") {
							t.Fatal(oracle)
						}
						t.Logf("ORIGINAL_FIRST %s", oracle)
						files := map[string][]byte{}
						entries, e := os.ReadDir(original)
						if e != nil {
							t.Fatal(e)
						}
						for _, entry := range entries {
							if strings.HasSuffix(entry.Name(), ".class") {
								raw, e := os.ReadFile(filepath.Join(original, entry.Name()))
								if e != nil {
									t.Fatal(e)
								}
								files[entry.Name()] = raw
							}
						}
						for _, name := range []string{"InitTarget.class", "InitTarget$1.class"} {
							obj, e := Parse(files[name])
							if e != nil {
								t.Fatal(e)
							}
							t.Logf("ORIGINAL_METADATA %s major=%d flags=%04x", name, obj.MajorVersion, obj.AccessFlags)
							for _, a := range obj.Attributes {
								if table, ok := a.(*InnerClassesAttribute); ok {
									for _, r := range table.Classes {
										n, _ := sourceBridgeClassName(obj, r.InnerClassInfoIndex)
										t.Logf("SELFROW %s %s flags=%04x", name, n, r.InnerClassAccessFlags)
									}
								}
							}
						}
						jar := filepath.Join(t.TempDir(), "authored.jar")
						if e := os.WriteFile(jar, t23Zip(t, files), 0600); e != nil {
							t.Fatal(e)
						}
						archive, e := NewJarFSFromLocalWithCompilerProfile(jar, func() int {
							if compiler == "modern_release21" {
								return 21
							}
							return 8
						}(), profile, nil)
						if e != nil {
							t.Fatal(e)
						}
						defer archive.Close()
						rebuilt := t.TempDir()
						for name, raw := range files {
							if !strings.HasPrefix(name, "InitTarget") {
								if e := os.WriteFile(filepath.Join(rebuilt, name), raw, 0600); e != nil {
									t.Fatal(e)
								}
							}
						}
						paths := []string{}
						for _, name := range []string{"InitTarget", "InitTarget$1"} {
							text, e := archive.ReadFile(name + ".class")
							if e != nil {
								t.Fatal(e)
							}
							t.Logf("CANDIDATE %s %s", name, text)
							if strings.Contains(string(text), DecompileStubMarker) {
								t.Fatalf("incomplete %s %s", name, text)
							}
							path := filepath.Join(rebuilt, name+".java")
							if e := os.WriteFile(path, text, 0600); e != nil {
								t.Fatal(e)
							}
							paths = append(paths, path)
						}
						candidateArgs := []string{"-proc:none", "--release", level}
						if profile == NativeJavac8 {
							candidateArgs = []string{"-proc:none", "-source", "8", "-target", "8"}
						}
						candidateArgs = append(candidateArgs, "-g:"+debug, "-cp", rebuilt, "-d", rebuilt)
						candidateArgs = append(candidateArgs, paths...)
						if out, e := exec.Command(javac, candidateArgs...).CombinedOutput(); e != nil {
							t.Fatalf("candidate compile %v %s", e, out)
						}
						if actual := t04RunJava(t, java, rebuilt, "InitDriver"); actual != oracle {
							t.Fatalf("original:%s candidate:%s", oracle, actual)
						}
						for _, name := range []string{"InitTarget.class", "InitTarget$1.class"} {
							candidate, e := os.ReadFile(filepath.Join(rebuilt, name))
							if e != nil {
								t.Fatal(e)
							}
							if nativeBinaryShape(t, files[name]) != nativeBinaryShape(t, candidate) || nativeAnonymousAccessorShape(t, files[name]) != nativeAnonymousAccessorShape(t, candidate) {
								t.Fatalf("exact declarations differ %s", name)
							}
						}
					})
				}
			}
		}
	}
}

package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const assertionStorageFixture = `class FieldBindingScope{}
class FieldBindingEffects {
 static String trace="";
 static boolean base(int n){trace+="B";return (n&1)==0;}
 static String baseMessage(){trace+="M";return "base";}
 static boolean child(int n){trace+="C";return n>=0;}
 static Object message(Object token){trace+="Q";return token;}
}
class FieldBindingBase {
 static final boolean $assertionsDisabled=!FieldBindingBase.class.desiredAssertionStatus();
 public int fromBase(int n){if(!$assertionsDisabled&&!FieldBindingEffects.base(n))throw new AssertionError(FieldBindingEffects.baseMessage());return n*3+5;}
}
public class FieldBindingOwner extends FieldBindingBase {
 static final boolean $assertionsDisabled=!FieldBindingScope.class.desiredAssertionStatus();
 public int fromChild(int n,Object token){if(!$assertionsDisabled&&!FieldBindingEffects.child(n))throw new AssertionError(FieldBindingEffects.message(token));return n^0x5a5a5a5a;}
 public String aliasLiteral(){return "$jdec$storage$3";}
}
class FieldBindingDriver {
 public static void main(String[]args)throws Exception {
  boolean base=Boolean.parseBoolean(System.getProperty("base")),scope=Boolean.parseBoolean(System.getProperty("scope"));
  ClassLoader l=ClassLoader.getSystemClassLoader();l.setClassAssertionStatus("FieldBindingBase",base);l.setClassAssertionStatus("FieldBindingScope",scope);l.setClassAssertionStatus("FieldBindingOwner",!scope);
  FieldBindingOwner x=new FieldBindingOwner();int rows=0;if(!x.aliasLiteral().equals("$jdec$storage$3"))throw new AssertionError("shared alias/string constant was renamed");
  for(int n:new int[]{Integer.MIN_VALUE,-3,-1,0,1,2,Integer.MAX_VALUE})for(Object token:new Object[]{null,"tag",new IllegalArgumentException("cause")}){
   FieldBindingEffects.trace="";Throwable error=null;int value=0;
   try{value=x.fromBase(n);}catch(Throwable e){error=e;}
   boolean fail=base&&(n&1)!=0;if(fail!=(error instanceof AssertionError)||error!=null&&!(error instanceof AssertionError)||!FieldBindingEffects.trace.equals(base?(fail?"BM":"B"):""))throw new AssertionError("parent status/condition/message order");
   if(fail){if(!"base".equals(error.getMessage()))throw new AssertionError("parent detail");}else if(value!=java.math.BigInteger.valueOf(n).multiply(java.math.BigInteger.valueOf(3)).add(java.math.BigInteger.valueOf(5)).intValue())throw new AssertionError("parent modular arithmetic");
   FieldBindingEffects.trace="";error=null;
   try{value=x.fromChild(n,token);}catch(Throwable e){error=e;}
   fail=scope&&n<0;if(fail!=(error instanceof AssertionError)||error!=null&&!(error instanceof AssertionError)||!FieldBindingEffects.trace.equals(scope?(fail?"CQ":"C"):""))throw new AssertionError("original external assertion-status owner/condition/detail");
   if(fail){if(error.getCause()!=(token instanceof Throwable?token:null)||!String.valueOf(token).equals(error.getMessage()))throw new AssertionError("detail value/cause identity");}else if(value!=java.math.BigInteger.valueOf(n).xor(java.math.BigInteger.valueOf(0x5a5a5a5a)).intValue())throw new AssertionError("child bit arithmetic");
   rows++;
  }
  for(Class<?> c:new Class<?>[]{FieldBindingBase.class,FieldBindingOwner.class}){java.lang.reflect.Field f=c.getDeclaredField("$assertionsDisabled");if(!f.isSynthetic()||f.getModifiers()!=0x1018)throw new AssertionError("original flag declaration metadata");}
  System.out.println(rows+":assertion-storage:independent-status:effects:arithmetic:identity");
 }
}`

func testAssertionStorageRoundTrip(t *testing.T, fixture, prefix, originalCompiler, javac string, originalProfile, profile SourceCompilerProfile, debug string, named bool) {
	t.Helper()
	_, java := t04Tools(t)
	files := assertionStorageOriginalFiles(t, fixture, prefix, originalCompiler, originalProfile, debug, named)
	original := sourceArtifactWriteFiles(t, files)
	run := func(dir string) {
		t.Helper()
		for _, enabled := range []string{"-ea", "-da"} {
			for _, base := range []string{"true", "false"} {
				for _, scope := range []string{"true", "false"} {
					out, err := exec.Command(java, "-Xverify:all", enabled, "-Dbase="+base, "-Dscope="+scope, "-cp", dir, prefix+"Driver").CombinedOutput()
					if err != nil || string(out) != "21:assertion-storage:independent-status:effects:arithmetic:identity\n" {
						t.Fatalf("independent %s/%s/%s oracle: %v\n%s", enabled, base, scope, err, out)
					}
				}
			}
		}
	}
	run(original)
	jar := filepath.Join(t.TempDir(), "original.jar")
	if err := os.WriteFile(jar, t23Zip(t, files), 0600); err != nil {
		t.Fatal(err)
	}
	artifact, err := NewJarSourceArtifactFromLocal(jar, 8, profile, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer artifact.Close()
	dependencies := map[string][]byte{}
	for name, raw := range files {
		if name != prefix+"Owner.class" && name != prefix+"Base.class" {
			dependencies[name] = raw
		}
	}
	out := sourceArtifactWriteFiles(t, dependencies)
	paths := []string{}
	readOrder := []string{prefix + "Base", prefix + "Owner"}
	if named {
		readOrder[0], readOrder[1] = readOrder[1], readOrder[0]
	}
	for _, name := range readOrder {
		source, err := artifact.ReadFile(name + ".class")
		if err != nil || strings.Contains(string(source), DecompileStubMarker) || strings.Contains(string(source), "decompile dump failed") {
			t.Fatalf("source %s: %v\n%s", name, err, source)
		}
		path := filepath.Join(out, name+".java")
		if err := os.WriteFile(path, source, 0600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	args := []string{"-proc:none", "--release", "8", "-g:" + debug}
	if profile == NativeJavac8 {
		args = []string{"-proc:none", "-source", "8", "-target", "8", "-g:" + debug}
	}
	if named {
		args = append(args, "-parameters")
	}
	args = append(args, "-cp", out, "-sourcepath", out, "-d", out)
	if log, err := exec.Command(javac, append(args, paths...)...).CombinedOutput(); err != nil {
		t.Fatalf("original subjects absent from compiler: %v\n%s", err, log)
	}
	for _, name := range []string{prefix + "Base", prefix + "Owner"} {
		compiled, err := os.ReadFile(filepath.Join(out, name+".class"))
		if err != nil {
			t.Fatal(err)
		}
		restored, err := artifact.RestoreClassMetadata(name+".class", compiled)
		if err != nil {
			t.Fatal(err)
		}
		before, _ := Parse(compiled)
		after, _ := Parse(restored)
		for i, method := range before.Methods {
			for j, attr := range method.Attributes {
				if code, ok := attr.(*CodeAttribute); ok {
					other, ok := after.Methods[i].Attributes[j].(*CodeAttribute)
					if !ok || !reflect.DeepEqual(code, other) {
						t.Fatal("field identity changed fresh Code, handlers, frames or debug tables")
					}
				}
			}
		}
		nativeLexicalExactSignatures(t, name+".class", files[name+".class"], restored)
		if lambdaParameterMetadataTables(t, files[name+".class"]) != lambdaParameterMetadataTables(t, restored) {
			t.Fatal("original parameter metadata changed")
		}
		if nativeBinaryShape(t, files[name+".class"]) != nativeBinaryShape(t, restored) {
			t.Fatal("original complete ABI changed")
		}
		if err := os.WriteFile(filepath.Join(out, name+".class"), restored, 0600); err != nil {
			t.Fatal(err)
		}
	}
	run(out)
}

// Both original flags are made JVM-synthetic after compiling a legal raw
// storage program. The VM then has the same compiler-storage roles as the
// actual nested-class failure, without relying on any library or class name.
func TestAdversarialAssertionStoragePreservesInheritanceStatusAndABI(t *testing.T) {
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("JAVA8_JAVAC required")
	}
	for _, compiler := range []struct {
		name, original, rebuilt string
		from, to                SourceCompilerProfile
	}{
		{"native", native, native, NativeJavac8, NativeJavac8}, {"modern", modern, modern, ModernJavac, ModernJavac}, {"native-to-modern", native, modern, NativeJavac8, ModernJavac},
	} {
		for _, named := range []bool{false, true} {
			for _, prefix := range []string{"FieldBinding", "ReboundStorage"} {
				for _, debug := range []string{"none", "source,lines,vars"} {
					for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
						label := compiler.name + "/" + prefix + "/" + debug + "/" + policy
						if named {
							label += "/parameters"
						} else {
							label += "/unnamed"
						}
						t.Run(label, func(t *testing.T) {
							if policy == "no-source-rewrites" {
								t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
							}
							if policy == "no-core-cleanups" {
								t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
							}
							fixture := assertionStorageFixture
							if named {
								fixture = strings.NewReplacer("fromChild(int n,Object token)", "fromChild(int n,Object $jdec$storage$3)", "FieldBindingEffects.message(token)", "FieldBindingEffects.message($jdec$storage$3)").Replace(fixture)
							}
							testAssertionStorageRoundTrip(t, fixture, prefix, compiler.original, compiler.rebuilt, compiler.from, compiler.to, debug, named)
						})
					}
				}
			}
		}
	}
}
func TestAdversarialAssertionStorageAllocatesAnUnoccupiedSourceName(t *testing.T) {
	modern, _ := t04Tools(t)
	fixture := strings.Replace(assertionStorageFixture, "public class FieldBindingOwner extends FieldBindingBase {", "public class FieldBindingOwner extends FieldBindingBase { static final String $jdec$storage$3=\"ordinary storage\";", 1)
	for _, debug := range []string{"none", "source,lines,vars"} {
		for _, named := range []bool{false, true} {
			label := debug
			if named {
				label += "/parameters"
			} else {
				label += "/unnamed"
			}
			t.Run(label, func(t *testing.T) {
				testAssertionStorageRoundTrip(t, fixture, "FieldBinding", modern, modern, ModernJavac, ModernJavac, debug, named)
			})
		}
	}
}

func assertionStorageOriginalFiles(t *testing.T, fixture, prefix, originalCompiler string, originalProfile SourceCompilerProfile, debug string, named bool) map[string][]byte {
	t.Helper()
	fixture = strings.ReplaceAll(fixture, "FieldBinding", prefix)
	files := compileLambdaParameterMetadataFixture(t, originalCompiler, originalProfile, fixture, prefix+"Owner", debug, named)
	for _, name := range []string{prefix + "Base", prefix + "Owner"} {
		object, err := Parse(files[name+".class"])
		if err != nil {
			t.Fatal(err)
		}
		marked := 0
		for _, field := range object.Fields {
			if fieldName, _ := sourceBridgeUTF8(object, field.NameIndex); fieldName == nativeAssertionField {
				field.AccessFlags |= syntheticFieldFlag
				marked++
			}
		}
		if marked != 1 {
			t.Fatal("original physical assertion flag", name)
		}
		files[name+".class"] = object.Bytes()
	}
	return files
}

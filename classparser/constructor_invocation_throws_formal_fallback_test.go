package javaclassparser

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const reviewFormalFallbackSource = `
class ReviewFormalFallbackState {
 static String trace="";static final java.io.IOException left=new java.io.IOException("left"),right=new java.io.IOException("right");
}
class ReviewFormalFallbackBase {
 <E extends Exception>ReviewFormalFallbackBase(boolean fail,E value)throws E {ReviewFormalFallbackState.trace+="B";if(fail)throw value;}
}
class ReviewFormalFallbackChild extends ReviewFormalFallbackBase {
 <X extends Exception>ReviewFormalFallbackChild(boolean fail,java.io.IOException value)throws Exception {super(fail,value);ReviewFormalFallbackState.trace+="C";}
}
class ReviewFormalFallbackDriver {
 public static void main(String[] args)throws Exception{
  int rows=0;
  for(java.io.IOException value:new java.io.IOException[]{ReviewFormalFallbackState.left,ReviewFormalFallbackState.right})for(boolean fail:new boolean[]{false,true}){
   ReviewFormalFallbackState.trace="";Throwable failure=null;try{new ReviewFormalFallbackChild(fail,value);}catch(Throwable caught){failure=caught;}
   if(fail?(failure!=value||!ReviewFormalFallbackState.trace.equals("B")):(failure!=null||!ReviewFormalFallbackState.trace.equals("BC")))throw new AssertionError("same object/order",failure);rows++;
  }
  java.lang.reflect.Constructor<?> c=ReviewFormalFallbackChild.class.getDeclaredConstructors()[0];
  if(c.getExceptionTypes().length!=1||c.getExceptionTypes()[0]!=Exception.class||c.getTypeParameters().length!=1||!c.getTypeParameters()[0].getName().equals("X"))throw new AssertionError("raw/formal constructor ABI");
  java.lang.reflect.Type g=c.getGenericExceptionTypes()[0];
  if(args[0].equals("X")?(!(g instanceof java.lang.reflect.TypeVariable)||!((java.lang.reflect.TypeVariable<?>)g).getName().equals("X")):g!=Exception.class)throw new AssertionError("source throws binder");
  System.out.println(rows+":"+args[0]+":same-identity:order:rawException:formalX");
 }
}`

// The JVM ignores checked throws in execution. Only Signature changes from
// concrete Exception to formal X; erased Exceptions, executable Code and flags
// remain exact. A source-only throws proof must not fall back to raw coverage.
func TestAdversarialConstructorInvocationGenericThrowsSameErasureFormalCannotFallBack(t *testing.T) {
	evidence := os.Getenv("JDEC_REVIEW_EVIDENCE")
	if evidence == "" {
		evidence = t.TempDir()
	}
	modern, java := t04Tools(t)
	for _, compiler := range []struct {
		name, javac string
		native      bool
		profile     SourceCompilerProfile
	}{{"modern", modern, false, ModernJavac}, {"native8", os.Getenv("JAVA8_JAVAC"), true, NativeJavac8}} {
		if compiler.javac == "" {
			continue
		}
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(compiler.name+"/"+debug, func(t *testing.T) {
				dir := filepath.Join(evidence, "formal-fallback", compiler.name, debug)
				original, mutant := filepath.Join(dir, "original"), filepath.Join(dir, "signature-only")
				for _, d := range []string{original, mutant} {
					if err := os.MkdirAll(d, 0700); err != nil {
						t.Fatal(err)
					}
				}
				compile := func(source, root string, classpath string) ([]byte, error) {
					file := filepath.Join(root, "ReviewFormalFallbackChild.java")
					if err := os.WriteFile(file, []byte(source), 0600); err != nil {
						t.Fatal(err)
					}
					args := []string{"-proc:none", "--release", "8"}
					if compiler.native {
						args = []string{"-proc:none", "-source", "8", "-target", "8"}
					}
					args = append(args, "-g:"+debug, "-d", root)
					if classpath != "" {
						args = append(args, "-cp", classpath)
					}
					args = append(args, file)
					return exec.Command(compiler.javac, args...).CombinedOutput()
				}
				out, err := compile(reviewFormalFallbackSource, original, "")
				os.WriteFile(filepath.Join(dir, "original-compile.log"), out, 0600)
				if err != nil {
					t.Fatalf("authored original compile: %v\n%s", err, out)
				}
				run := func(root, mode string) {
					out, err := exec.Command(java, "-Xverify:all", "-cp", root, "ReviewFormalFallbackDriver", mode).CombinedOutput()
					os.WriteFile(filepath.Join(root, "original-jvm.log"), out, 0600)
					if err != nil {
						t.Fatalf("authored original JVM: %v\n%s", err, out)
					}
					t.Log(string(out))
				}
				run(original, "E")
				files := classMapFromDir(t, original)
				object, err := Parse(bytes.Clone(files["ReviewFormalFallbackChild"]))
				if err != nil {
					t.Fatal(err)
				}
				var method *MemberInfo
				var codeBefore []byte
				var flagsBefore uint16
				for _, m := range object.Methods {
					if n, _ := object.getUtf8(m.NameIndex); n == "<init>" {
						method = m
						flagsBefore = m.AccessFlags
						for _, a := range m.Attributes {
							if c, ok := a.(*CodeAttribute); ok {
								codeBefore = bytes.Clone(c.Code)
							}
						}
					}
				}
				if method == nil || codeBefore == nil {
					t.Fatal("original constructor missing")
				}
				declaredBefore, known := originalMethodExceptions(object, method)
				if !known || len(declaredBefore) != 1 || declaredBefore[0] != "java/lang/Exception" {
					t.Fatal("authored raw Exceptions changed", declaredBefore)
				}
				method.Attributes = append([]AttributeInfo(nil), method.Attributes...)
				kept := method.Attributes[:0]
				for _, a := range method.Attributes {
					if _, ok := a.(*SignatureAttribute); !ok {
						kept = append(kept, a)
					}
				}
				sig := "<X:Ljava/lang/Exception;>(ZLjava/io/IOException;)V^TX;"
				object.ConstantPoolManager.AddUtf8Info("Signature")
				index := object.ConstantPoolManager.AddUtf8Info(sig)
				method.Attributes = append(kept, &SignatureAttribute{Type: "Signature", AttrLen: 2, SignatureIndex: uint16(index)})
				if method.AccessFlags != flagsBefore {
					t.Fatal("Signature-only mutation changed flags")
				}
				for _, a := range method.Attributes {
					if c, ok := a.(*CodeAttribute); ok && !bytes.Equal(c.Code, codeBefore) {
						t.Fatal("Signature-only mutation changed Code")
					}
				}
				declaredAfter, known := originalMethodExceptions(object, method)
				if !known || strings.Join(declaredAfter, ",") != strings.Join(declaredBefore, ",") {
					t.Fatal("Signature-only mutation changed raw Exceptions")
				}
				files["ReviewFormalFallbackChild"] = object.Bytes()
				for n, b := range files {
					if err := os.WriteFile(filepath.Join(mutant, n+".class"), b, 0600); err != nil {
						t.Fatal(err)
					}
				}
				run(mutant, "X")
				negative := strings.Replace(reviewFormalFallbackSource, "ReviewFormalFallbackChild(boolean fail,java.io.IOException value)throws Exception", "ReviewFormalFallbackChild(boolean fail,java.io.IOException value)throws X", 1)
				negdir := filepath.Join(dir, "javac-negative")
				os.MkdirAll(negdir, 0700)
				out, err = compile(negative, negdir, "")
				os.WriteFile(filepath.Join(negdir, "compile.log"), out, 0600)
				if err == nil {
					t.Fatal("independent javac accepted IOException as arbitrary throws X")
				}
				resolve := func(n string) ([]byte, bool) { b, ok := files[n]; return bytes.Clone(b), ok }
				for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
					t.Run(policy, func(t *testing.T) {
						env := map[string]string{}
						if policy == "no-source-rewrites" {
							env["JDEC_NO_SOURCE_REWRITES"] = "1"
						}
						if policy == "no-core-cleanups" {
							env["JDEC_NO_CORE_CLEANUPS"] = "1"
						}
						result, err := DecompileWithOptions(bytes.Clone(files["ReviewFormalFallbackChild"]), DecompileOptions{Mode: Precision, TargetSourceVersion: 8, SourceCompiler: compiler.profile, Resolve: resolve, EnvSnapshot: env})
						outdir := filepath.Join(dir, policy)
						os.MkdirAll(outdir, 0700)
						os.WriteFile(filepath.Join(outdir, "decompiled.java"), []byte(result.Source), 0600)
						record := map[string]any{"decompile_error": err, "stub": strings.Contains(result.Source, DecompileStubMarker), "raw_throws": declaredAfter, "source_signature": sig, "code_unchanged": true}
						if err == nil && !strings.Contains(result.Source, DecompileStubMarker) {
							out, compileErr := compile(result.Source, outdir, mutant)
							os.WriteFile(filepath.Join(outdir, "rebuild.log"), out, 0600)
							record["rebuild_error"] = compileErr
							record["rebuild_output"] = string(out)
							t.Errorf("checked formal source coverage fell back to same erased Exceptions: rebuild=%v\n%s\n%s", compileErr, out, result.Source)
						}
						encoded, _ := json.MarshalIndent(record, "", "  ")
						os.WriteFile(filepath.Join(outdir, "observation.json"), append(encoded, '\n'), 0600)
					})
				}
			})
		}
	}
}

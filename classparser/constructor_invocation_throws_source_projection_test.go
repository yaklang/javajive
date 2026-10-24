package javaclassparser

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestAdversarialConstructorInvocationGenericThrowsSourceFormalFallbackObligations(t *testing.T) {
	const source = `
class SourceThrowState {static String trace;static final java.io.IOException io=new java.io.IOException("same");static<T>T opaque(T value){return value;}}
class SourceThrowGeneric {<E extends Exception>SourceThrowGeneric(boolean fail,E value)throws E{SourceThrowState.trace+="B";if(fail)throw value;SourceThrowState.trace+="E";}}
class SourceThrowRival {<E extends Exception>SourceThrowRival(boolean fail,E value)throws E{SourceThrowState.trace+="B";if(fail)throw value;SourceThrowState.trace+="E";}SourceThrowRival(boolean fail,Object value)throws java.sql.SQLException{SourceThrowState.trace+="R";throw new java.sql.SQLException();}}
class SourceThrowOrdinary {SourceThrowOrdinary(boolean fail,java.io.IOException value)throws java.io.IOException{SourceThrowState.trace+="B";if(fail)throw value;SourceThrowState.trace+="E";}}
class SourceThrowOpaqueChild extends SourceThrowGeneric {SourceThrowOpaqueChild(boolean fail,java.io.IOException value)throws Exception{super(fail,SourceThrowState.opaque(value));SourceThrowState.trace+="C";}}
class SourceThrowRivalChild extends SourceThrowRival {SourceThrowRivalChild(boolean fail,java.io.IOException value)throws Exception{super(fail,value);SourceThrowState.trace+="C";}}
class SourceThrowOrdinaryChild extends SourceThrowOrdinary {SourceThrowOrdinaryChild(boolean fail,java.io.IOException value)throws java.io.IOException{super(fail,value);SourceThrowState.trace+="C";}}
class SourceThrowDriver {public static void main(String[]args)throws Exception{int rows=0,formals=0;Class<?>[]targets={SourceThrowOpaqueChild.class,SourceThrowRivalChild.class,SourceThrowOrdinaryChild.class};for(int kind=0;kind<targets.length;kind++){java.lang.reflect.Constructor<?>c=targets[kind].getDeclaredConstructors()[0];Class<?>raw=kind==2?java.io.IOException.class:Exception.class;if(c.getExceptionTypes().length!=1||c.getExceptionTypes()[0]!=raw)throw new AssertionError("original Exceptions");if(c.getGenericExceptionTypes()[0] instanceof java.lang.reflect.TypeVariable){if(!((java.lang.reflect.TypeVariable<?>)c.getGenericExceptionTypes()[0]).getName().equals("X"))throw new AssertionError("formal identity");formals++;}else if(c.getGenericExceptionTypes()[0]!=raw)throw new AssertionError("original proper throws");for(boolean fail:new boolean[]{false,true}){SourceThrowState.trace="";Throwable caught=null;try{if(kind==0)new SourceThrowOpaqueChild(fail,SourceThrowState.io);else if(kind==1)new SourceThrowRivalChild(fail,SourceThrowState.io);else new SourceThrowOrdinaryChild(fail,SourceThrowState.io);}catch(Throwable ex){caught=ex;}if(fail?caught!=SourceThrowState.io||!SourceThrowState.trace.equals("B"):caught!=null||!SourceThrowState.trace.equals("BEC"))throw new AssertionError("exception identity/evaluation/order",caught);rows++;}}System.out.println(rows+":source-throws:generic"+formals+":same:order");}}
`
	modern, java := t04Tools(t)
	for _, compiler := range []struct {
		name, javac string
		native      bool
	}{{"modern", modern, false}, {"native8", os.Getenv("JAVA8_JAVAC"), true}} {
		if compiler.javac == "" {
			t.Skip("native javac8 required")
		}
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(compiler.name+"/"+debug, func(t *testing.T) {
				original := t.TempDir()
				file := filepath.Join(original, "SourceThrowOpaqueChild.java")
				if err := os.WriteFile(file, []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
				args := []string{"-proc:none", "--release", "8"}
				if compiler.native {
					args = []string{"-proc:none", "-source", "8", "-target", "8"}
				}
				args = append(args, "-g:"+debug, "-d", original, file)
				if out, err := exec.Command(compiler.javac, args...).CombinedOutput(); err != nil {
					t.Fatalf("authored original: %v\n%s", err, out)
				}
				run := func(dir string, formals int) {
					t.Helper()
					out, err := exec.Command(java, "-Xverify:all", "-cp", dir, "SourceThrowDriver").CombinedOutput()
					want := fmt.Sprintf("6:source-throws:generic%d:same:order\n", formals)
					if err != nil || string(out) != want {
						t.Fatalf("independent original JVM: %v %q", err, out)
					}
				}
				run(original, 0)
				files := classMapFromDir(t, original)
				targets := []string{"SourceThrowOpaqueChild", "SourceThrowRivalChild", "SourceThrowOrdinaryChild"}
				for index, name := range targets {
					object, err := Parse(bytes.Clone(files[name]))
					if err != nil {
						t.Fatal(err)
					}
					object.ConstantPoolManager.AddUtf8Info("Signature")
					for _, method := range object.Methods {
						n, _ := object.getUtf8(method.NameIndex)
						if n != "<init>" {
							continue
						}
						desc, _ := object.getUtf8(method.DescriptorIndex)
						declared, known := originalMethodExceptions(object, method)
						if !known {
							t.Fatal("original Exceptions missing")
						}
						flags := method.AccessFlags
						var code *CodeAttribute
						for _, a := range method.Attributes {
							if c, ok := a.(*CodeAttribute); ok {
								code = c
							}
						}
						if code == nil {
							t.Fatal("Code missing")
						}
						before := bytes.Clone(code.Code)
						bound := "java/lang/Exception"
						if index == 2 {
							bound = "java/io/IOException"
						}
						sig := "<X:L" + bound + ";>" + desc + "^TX;"
						method.Attributes = append(method.Attributes, &SignatureAttribute{Type: "Signature", AttrLen: 2, SignatureIndex: uint16(object.ConstantPoolManager.AddUtf8Info(sig))})
						after, known := originalMethodExceptions(object, method)
						if !known || !slices.Equal(declared, after) || flags != method.AccessFlags || !bytes.Equal(before, code.Code) {
							t.Fatal("Signature-only mutation changed original JVM declaration/code")
						}
					}
					files[name] = object.Bytes()
				}
				mutant := t.TempDir()
				for name, raw := range files {
					if err := os.WriteFile(filepath.Join(mutant, name+".class"), raw, 0600); err != nil {
						t.Fatal(err)
					}
				}
				run(mutant, 3)
				for index, parent := range []string{"SourceThrowGeneric", "SourceThrowRival", "SourceThrowOrdinary"} {
					name := fmt.Sprintf("SourceThrowNegative%d", index)
					bound := "Exception"
					argument := "value"
					if index == 0 {
						argument = "SourceThrowState.opaque(value)"
					}
					if index == 2 {
						bound = "java.io.IOException"
					}
					text := fmt.Sprintf("class %s extends %s {<X extends %s>%s(boolean fail,java.io.IOException value)throws X{super(fail,%s);}}", name, parent, bound, name, argument)
					negative := t.TempDir()
					path := filepath.Join(negative, name+".java")
					if err := os.WriteFile(path, []byte(text), 0600); err != nil {
						t.Fatal(err)
					}
					compileArgs := []string{"-proc:none", "--release", "8"}
					if compiler.native {
						compileArgs = []string{"-proc:none", "-source", "8", "-target", "8"}
					}
					compileArgs = append(compileArgs, "-cp", mutant, "-d", negative, path)
					if out, err := exec.Command(compiler.javac, compileArgs...).CombinedOutput(); err == nil {
						t.Fatalf("source formal must not cover a concrete checked exception: %s\n%s", name, out)
					}
				}
				for _, name := range targets {
					for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
						t.Run(name+"/"+policy, func(t *testing.T) {
							env := map[string]string{}
							if policy == "no-source-rewrites" {
								env["JDEC_NO_SOURCE_REWRITES"] = "1"
							}
							if policy == "no-core-cleanups" {
								env["JDEC_NO_CORE_CLEANUPS"] = "1"
							}
							profile := ModernJavac
							if compiler.native {
								profile = NativeJavac8
							}
							result, err := DecompileWithOptions(bytes.Clone(files[name]), DecompileOptions{Mode: Precision, TargetSourceVersion: 8, SourceCompiler: profile, EnvSnapshot: env, Resolve: func(n string) ([]byte, bool) { raw, ok := files[n]; return bytes.Clone(raw), ok }})
							if err != nil || !strings.Contains(result.Source, DecompileStubMarker) {
								t.Fatalf("Unknown/NotApplicable may use only concrete source throws: %s %v\n%s", name, err, result.Source)
							}
						})
					}
				}
			})
		}
	}
}

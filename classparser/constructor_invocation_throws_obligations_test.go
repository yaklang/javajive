package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestAdversarialConstructorInvocationGenericThrowsPrivateBridgeThrowsObligations(t *testing.T) {
	modern, java := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		javac := modern
		if profile == NativeJavac8 {
			javac = os.Getenv("JAVA8_JAVAC")
			if javac == "" {
				t.Skip("native javac8 required")
			}
		}
		for _, shape := range []string{"empty", "declared-unchecked", "proper-checked", "mixed-formal-proper", "formal-only-checked"} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(string(profile)+"/"+shape+"/"+debug, func(t *testing.T) {
					fixture := nativeMemberPrivateSuperBridgeFixture
					checked := shape == "proper-checked" || shape == "mixed-formal-proper" || shape == "formal-only-checked"
					if shape == "declared-unchecked" {
						fixture = strings.Replace(fixture, "private Parent(V value,long n){", "private Parent(V value,long n)throws IllegalArgumentException{", 1)
					}
					if checked {
						fixture = strings.ReplaceAll(fixture, "IllegalArgumentException", "java.io.IOException")
						fixture = strings.Replace(fixture, "private Parent(V value,long n){", "private Parent(V value,long n)throws java.io.IOException{", 1)
						fixture = strings.Replace(fixture, "private Child(U value,long n){", "private Child(U value,long n)throws java.io.IOException{", 1)
						fixture = strings.Replace(fixture, "Child<T> make(T value,long n){", "Child<T> make(T value,long n)throws java.io.IOException{", 1)
					}
					if shape == "mixed-formal-proper" {
						fixture = strings.Replace(fixture, "private Child(U value,long n)throws java.io.IOException{", "private <X extends Exception>Child(U value,long n)throws X,java.io.IOException{", 1)
						// Ordinary allocation inference is a separate capability. Its
						// erased throws are independently covered in this MVP, so this
						// oracle isolates the private constructor's delegation obligation.
						fixture = strings.Replace(fixture, "Child<T> make(T value,long n)throws java.io.IOException{", "Child<T> make(T value,long n)throws Exception{", 1)
					}
					var files map[string][]byte
					if profile == NativeJavac8 {
						files = nativePrivateEnumCompile(t, fixture, "SuperOwner", debug)
					} else {
						files = nativeCompileDebugClasses(t, fixture, debug)
					}
					write := func(root string) {
						t.Helper()
						for name, raw := range files {
							if err := os.WriteFile(filepath.Join(root, name), raw, 0600); err != nil {
								t.Fatal(err)
							}
						}
					}
					oracle := func(root string) {
						t.Helper()
						out, err := exec.Command(java, "-Xverify:all", "-cp", root, "SuperDriver").CombinedOutput()
						if err != nil || string(out) != "24:PC\n" {
							t.Fatalf("original/rebuilt same exception and callback/argument/publication order: %v %s", err, out)
						}
					}
					codeOf := func(method *MemberInfo) *CodeAttribute {
						t.Helper()
						for _, attribute := range method.Attributes {
							if code, ok := attribute.(*CodeAttribute); ok {
								return code
							}
						}
						t.Fatal("original Code attribute absent")
						return nil
					}
					original := t.TempDir()
					write(original)
					oracle(original) // independent JVM oracle precedes candidate analysis
					if shape == "formal-only-checked" {
						obj, err := Parse(bytes.Clone(files["SuperOwner$Child.class"]))
						if err != nil {
							t.Fatal(err)
						}
						changed := false
						for _, method := range obj.Methods {
							name, _ := obj.getUtf8(method.NameIndex)
							desc, _ := obj.getUtf8(method.DescriptorIndex)
							if name != "<init>" || method.AccessFlags&0x0002 == 0 || desc != "(LSuperOwner;Ljava/lang/Object;J)V" {
								continue
							}
							declared, known := originalMethodExceptions(obj, method)
							if !known || !slices.Equal(declared, []string{"java/io/IOException"}) {
								t.Fatal("original checked declaration witness missing")
							}
							for _, attribute := range method.Attributes {
								if signature, ok := attribute.(*SignatureAttribute); ok {
									old, _ := obj.getUtf8(signature.SignatureIndex)
									if old != "(TU;J)V" {
										t.Fatalf("unexpected original logical Signature %q", old)
									}
									before, flags := bytes.Clone(codeOf(method).Code), method.AccessFlags
									signature.SignatureIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("<X:Ljava/io/IOException;>" + old + "^TX;"))
									after, known := originalMethodExceptions(obj, method)
									if !known || !slices.Equal(declared, after) || flags != method.AccessFlags || !bytes.Equal(before, codeOf(method).Code) {
										t.Fatal("Signature-only mutation changed Code/Exceptions/flags")
									}
									changed = true
								}
							}
						}
						if !changed {
							t.Fatal("original private logical constructor not found")
						}
						files["SuperOwner$Child.class"] = obj.Bytes()
						mutant := t.TempDir()
						write(mutant)
						oracle(mutant) // valid JVM class; source formal cannot cover IO
						negative := strings.Replace(fixture, "private Child(U value,long n)throws java.io.IOException{", "private <X extends java.io.IOException>Child(U value,long n)throws X{", 1)
						dir := t.TempDir()
						path := filepath.Join(dir, "SuperOwner.java")
						if err := os.WriteFile(path, []byte(negative), 0600); err != nil {
							t.Fatal(err)
						}
						args := []string{"-proc:none", "--release", "8"}
						if profile == NativeJavac8 {
							args = []string{"-proc:none", "-source", "8", "-target", "8"}
						}
						if out, err := exec.Command(javac, append(args, "-d", dir, path)...).CombinedOutput(); err == nil {
							t.Fatalf("formal X must not grant its erased IOException: %s", out)
						}
					}
					obj, err := Parse(bytes.Clone(files["SuperOwner$Child.class"]))
					if err != nil {
						t.Fatal(err)
					}
					d := NewClassObjectDumper(obj)
					d.options.TargetSourceVersion = 8
					d.foldSiblingResolver = func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return bytes.Clone(raw), ok }
					provider := d.buildInvocationMetadata()
					var method *MemberInfo
					for _, candidate := range obj.Methods {
						name, _ := obj.getUtf8(candidate.NameIndex)
						if name == "<init>" && candidate.AccessFlags&0x0002 != 0 {
							method = candidate
						}
					}
					if method == nil {
						t.Fatal("original private ctor absent")
					}
					methodSig, _ := d.constructorThrowsOriginalSignature(method.Attributes)
					classSig, _ := d.constructorThrowsOriginalSignature(obj.Attributes)
					desc, _ := obj.getUtf8(method.DescriptorIndex)
					_, _, erased := types.EraseLexicalMethodSignatureWithThrows(classSig, methodSig)
					if erased {
						t.Fatal("fixture must expose unrelated enclosing TT outside flat erasure")
					}
					d.FuncCtx = &class_context.ClassContext{ClassName: obj.GetClassName(), ClassSig: classSig, FunctionName: "<init>", CurrentMethodDesc: desc, CurrentMethodSig: methodSig, InvocationMetadata: provider}
					code := codeOf(method)
					decoder := core.NewDecompiler(code.Code, func(index int) values.JavaValue { return GetValueFromCP(d.ConstantPool, index) })
					if err := decoder.ParseOpcode(); err != nil {
						t.Fatal(err)
					}
					var plan *constructorSourceBoundary
					for _, op := range decoder.Opcodes() {
						member := constructorMotionMember(obj, op, core.OP_INVOKESPECIAL)
						if member == nil || member.Name != "SuperOwner$Parent" {
							continue
						}
						exceptions, known := exactInvocationExceptions(provider, member.Name, member.Member, member.Description)
						wantCount := 0
						if shape != "empty" {
							wantCount = 1
						}
						if !known || len(exceptions) != wantCount {
							t.Fatalf("exact parent declaration=%v known=%v", exceptions, known)
						}
						call := &values.FunctionCallExpression{ClassName: member.Name, FunctionName: member.Member, Descriptor: member.Description, Kind: values.InvokeSpecial, IsSpecialInvoke: true, Object: constructorThrowsTestReceiver(obj.GetClassName()), OriginPC: int(op.CurrentOffset), HasOriginPC: true}
						plan = &constructorSourceBoundary{pc: call.OriginPC, delegate: call}
						if receipt, admission := d.constructorInvocationThrowsAdmission(code, method, member, plan.pc, plan); receipt != nil || admission != constructorThrowsNotApplicable {
							t.Fatalf("nongeneric private bridge must use unchanged original declaration path: %v", admission)
						}
					}
					if plan == nil {
						t.Fatal("original private parent bridge call missing")
					}
					needs, err := d.methodNeedsCheckedEscape(code, nil, method, plan)
					if shape == "formal-only-checked" {
						if err == nil || needs {
							t.Fatalf("checked formal-only source cannot use erased bound: needs=%v err=%v", needs, err)
						}
					} else if err != nil || needs {
						t.Fatalf("known empty/unchecked or independently concrete-covered throws needs=%v err=%v", needs, err)
					}
					if shape == "empty" {
						d.FuncCtx.InvocationMetadata = func(name string) (callbinding.Class, bool) {
							c, known := provider(name)
							if name == "SuperOwner$Parent" && known {
								c.Methods = slices.Clone(c.Methods)
								for i := range c.Methods {
									if c.Methods[i].Desc == plan.delegate.Descriptor {
										c.Methods[i].Exceptions = []string{"unproved/Ancestry"}
										c.Methods[i].ExceptionsKnown = true
									}
								}
							}
							return c, known
						}
						if needs, err := d.methodNeedsCheckedEscape(code, nil, method, plan); err == nil || needs {
							t.Fatalf("unknown ancestry is potentially checked: needs=%v err=%v", needs, err)
						}
					}
					for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
						t.Run(policy, func(t *testing.T) {
							if policy == "no-source-rewrites" {
								t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
							}
							if policy == "no-core-cleanups" {
								t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
							}
							z := nativeEnumOwnUnitArchive(t, files, profile)
							source, err := z.ReadFile("SuperOwner.class")
							if shape == "formal-only-checked" {
								// A refused joint transaction may publish the unrelated outer
								// class separately. Inspect the actual constructor's source unit.
								child, childErr := z.ReadFile("SuperOwner$Child.class")
								if childErr == nil && !bytes.Contains(child, []byte(DecompileStubMarker)) {
									t.Fatalf("actual child may not publish a nonstub with invalid formal coverage: %v\n%s", childErr, child)
								}
								return
							}
							if err != nil || bytes.Contains(source, []byte(DecompileStubMarker)) {
								t.Fatalf("joint private bridge no-stub: %v\n%s", err, source)
							}
							rebuilt := t.TempDir()
							for name, raw := range files {
								if !strings.HasPrefix(name, "SuperOwner") {
									if err := os.WriteFile(filepath.Join(rebuilt, name), raw, 0600); err != nil {
										t.Fatal(err)
									}
								}
							}
							path := filepath.Join(rebuilt, "SuperOwner.java")
							if err := os.WriteFile(path, source, 0600); err != nil {
								t.Fatal(err)
							}
							args := []string{"-proc:none", "--release", "8"}
							if profile == NativeJavac8 {
								args = []string{"-proc:none", "-source", "8", "-target", "8"}
							}
							args = append(args, "-g:"+debug, "-cp", rebuilt, "-d", rebuilt, path)
							if out, err := exec.Command(javac, args...).CombinedOutput(); err != nil {
								t.Fatalf("same-compiler rebuilt source: %v %s\n%s", err, out, source)
							}
							oracle(rebuilt)
							for name, raw := range files {
								if strings.HasPrefix(name, "SuperOwner") {
									after, err := os.ReadFile(filepath.Join(rebuilt, name))
									if err != nil || constructorInvocationThrowsDeclarations(t, raw) != constructorInvocationThrowsDeclarations(t, after) {
										t.Fatalf("exact class/member flags+Signature+Exceptions ABI changed: %s %v\nORIGINAL=%s\nREBUILT=%s\nSOURCE=\n%s", name, err, constructorInvocationThrowsDeclarations(t, raw), constructorInvocationThrowsDeclarations(t, after), source)
									}
								}
							}
						})
					}
				})
			}
		}
	}
}

const constructorThrowsNominalHierarchyFixture = `
class Fault extends RuntimeException {}
class CheckedError extends Exception {}
class LegacyThrowsState {static String trace;static final IllegalArgumentException illegal=new IllegalArgumentException("same-illegal");static final Fault fault=new Fault();static final CheckedError checked=new CheckedError();}
class LegacyIllegalParent {LegacyIllegalParent(boolean fail)throws IllegalArgumentException{LegacyThrowsState.trace+="P";if(fail)throw LegacyThrowsState.illegal;LegacyThrowsState.trace+="E";}}
class LegacyFaultParent {LegacyFaultParent(boolean fail)throws Fault{LegacyThrowsState.trace+="P";if(fail)throw LegacyThrowsState.fault;LegacyThrowsState.trace+="E";}}
class LegacyCheckedParent {LegacyCheckedParent(boolean fail)throws CheckedError{LegacyThrowsState.trace+="P";if(fail)throw LegacyThrowsState.checked;LegacyThrowsState.trace+="E";}}
final class LegacyThrowsIllegal extends LegacyIllegalParent {LegacyThrowsIllegal(boolean fail){super(fail);LegacyThrowsState.trace+="C";}}
final class LegacyThrowsFault extends LegacyFaultParent {LegacyThrowsFault(boolean fail){super(fail);LegacyThrowsState.trace+="C";}}
final class LegacyThrowsChecked extends LegacyCheckedParent {LegacyThrowsChecked(boolean fail)throws CheckedError{super(fail);LegacyThrowsState.trace+="C";}}
final class LegacyThrowsBroken extends LegacyCheckedParent {LegacyThrowsBroken(boolean fail)throws CheckedError{super(fail);LegacyThrowsState.trace+="C";}}
class LegacyThrowsDriver {
 public static void main(String[]args)throws Exception{
  Class<?>[]targets={LegacyThrowsIllegal.class,LegacyThrowsFault.class,LegacyThrowsChecked.class,LegacyThrowsBroken.class};int rows=0;
  for(int kind=0;kind<targets.length;kind++){
   java.lang.reflect.Constructor<?>[]ctors=targets[kind].getDeclaredConstructors();
   if(ctors.length!=1||ctors[0].getModifiers()!=0||ctors[0].getParameterTypes().length!=1||ctors[0].getParameterTypes()[0]!=boolean.class||!java.lang.reflect.Modifier.isFinal(targets[kind].getModifiers()))throw new AssertionError("class flags/constructor descriptor");
   Class<?>[]exceptions=ctors[0].getExceptionTypes();int declared=kind==2||kind==3&&args.length==0?1:0;
   if(exceptions.length!=declared||declared==1&&exceptions[0]!=CheckedError.class)throw new AssertionError("original constructor Exceptions ABI");
   for(boolean fail:new boolean[]{false,true}){
    LegacyThrowsState.trace="";Throwable caught=null;Throwable expected=kind==0?LegacyThrowsState.illegal:kind==1?LegacyThrowsState.fault:LegacyThrowsState.checked;
    try{switch(kind){case 0:new LegacyThrowsIllegal(fail);break;case 1:new LegacyThrowsFault(fail);break;case 2:new LegacyThrowsChecked(fail);break;case 3:new LegacyThrowsBroken(fail);break;}}catch(Throwable failure){caught=failure;}
    if(caught!=(fail?expected:null)||!LegacyThrowsState.trace.equals(fail?"P":"PEC"))throw new AssertionError("same exception/evaluation order",caught);rows++;
   }
  }
  System.out.println(rows+":hierarchy:same:order:flags:descriptor:abi");
 }
}`

func TestAdversarialConstructorInvocationGenericThrowsLegacyUncheckedHierarchy(t *testing.T) {
	modern, java := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		javac := modern
		if profile == NativeJavac8 {
			javac = os.Getenv("JAVA8_JAVAC")
			if javac == "" {
				t.Skip("native javac8 required")
			}
		}
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(string(profile)+"/"+debug, func(t *testing.T) {
				var files map[string][]byte
				if profile == NativeJavac8 {
					files = nativePrivateEnumCompile(t, constructorThrowsNominalHierarchyFixture, "LegacyThrowsIllegal", debug)
				} else {
					files = nativeCompileDebugClasses(t, constructorThrowsNominalHierarchyFixture, debug)
				}
				write := func(root string, inputs map[string][]byte, excluded map[string]bool) {
					t.Helper()
					for name, raw := range inputs {
						if !excluded[name] {
							if err := os.WriteFile(filepath.Join(root, name), raw, 0600); err != nil {
								t.Fatal(err)
							}
						}
					}
				}
				oracle := func(root string, mutant bool) {
					t.Helper()
					args := []string{"-Xverify:all", "-cp", root, "LegacyThrowsDriver"}
					if mutant {
						args = append(args, "absent")
					}
					out, err := exec.Command(java, args...).CombinedOutput()
					if err != nil || string(out) != "8:hierarchy:same:order:flags:descriptor:abi\n" {
						t.Fatalf("original/rebuilt JVM oracle: %v %s", err, out)
					}
				}
				original := t.TempDir()
				write(original, files, nil)
				oracle(original, false)
				resolve := func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return bytes.Clone(raw), ok }
				for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
					t.Run(policy, func(t *testing.T) {
						env := map[string]string{}
						if policy == "no-source-rewrites" {
							env["JDEC_NO_SOURCE_REWRITES"] = "1"
						}
						if policy == "no-core-cleanups" {
							env["JDEC_NO_CORE_CLEANUPS"] = "1"
						}
						targets := []string{"LegacyThrowsIllegal", "LegacyThrowsFault", "LegacyThrowsChecked"}
						excluded := map[string]bool{}
						for _, name := range targets {
							excluded[name+".class"] = true
						}
						rebuilt := t.TempDir()
						write(rebuilt, files, excluded)
						paths := []string{}
						for _, name := range targets {
							result, err := DecompileWithOptions(bytes.Clone(files[name+".class"]), DecompileOptions{Mode: Precision, TargetSourceVersion: 8, SourceCompiler: profile, EnvSnapshot: env, Resolve: resolve})
							if err != nil || strings.Contains(result.Source, DecompileStubMarker) {
								t.Fatalf("nominal hierarchy must retain correct original source throws: %s %v\n%s", name, err, result.Source)
							}
							path := filepath.Join(rebuilt, name+".java")
							if err := os.WriteFile(path, []byte(result.Source), 0600); err != nil {
								t.Fatal(err)
							}
							paths = append(paths, path)
						}
						args := []string{"-proc:none", "--release", "8"}
						if profile == NativeJavac8 {
							args = []string{"-proc:none", "-source", "8", "-target", "8"}
						}
						args = append(args, "-g:"+debug, "-cp", rebuilt, "-d", rebuilt)
						if out, err := exec.Command(javac, append(args, paths...)...).CombinedOutput(); err != nil {
							t.Fatalf("same-compiler rebuilt hierarchy: %v %s", err, out)
						}
						oracle(rebuilt, false)
						for _, name := range targets {
							after, err := os.ReadFile(filepath.Join(rebuilt, name+".class"))
							if err != nil || constructorInvocationThrowsDeclarations(t, files[name+".class"]) != constructorInvocationThrowsDeclarations(t, after) {
								t.Fatalf("nominal hierarchy changed class flags/Signature/Exceptions/descriptor: %s %v", name, err)
							}
						}
						missing := func(name string) ([]byte, bool) {
							if name == "Fault" {
								return nil, false
							}
							return resolve(name)
						}
						result, err := DecompileWithOptions(bytes.Clone(files["LegacyThrowsFault.class"]), DecompileOptions{Mode: Precision, TargetSourceVersion: 8, SourceCompiler: profile, EnvSnapshot: env, Resolve: missing})
						if err != nil || !strings.Contains(result.Source, DecompileStubMarker) {
							t.Fatalf("unknown Fault ancestry must not become unchecked or add caller throws: %v\n%s", err, result.Source)
						}
					})
				}
				object, err := Parse(bytes.Clone(files["LegacyThrowsBroken.class"]))
				if err != nil {
					t.Fatal(err)
				}
				for _, method := range object.Methods {
					name, _ := object.getUtf8(method.NameIndex)
					if name != "<init>" {
						continue
					}
					before, flags := bytes.Clone(methodCodeBytes(method)), method.AccessFlags
					attributes := []AttributeInfo{}
					removed := 0
					for _, a := range method.Attributes {
						if _, ok := a.(*ExceptionsAttribute); ok {
							removed++
						} else {
							attributes = append(attributes, a)
						}
					}
					method.Attributes = attributes
					if removed != 1 || !bytes.Equal(before, methodCodeBytes(method)) || flags != method.AccessFlags {
						t.Fatal("Exceptions-only absence mutation changed Code/flags")
					}
				}
				mutantFiles := map[string][]byte{}
				for name, raw := range files {
					mutantFiles[name] = bytes.Clone(raw)
				}
				mutantFiles["LegacyThrowsBroken.class"] = object.Bytes()
				mutant := t.TempDir()
				write(mutant, mutantFiles, nil)
				oracle(mutant, true)
				negative := strings.Replace(constructorThrowsNominalHierarchyFixture, "LegacyThrowsBroken(boolean fail)throws CheckedError{", "LegacyThrowsBroken(boolean fail){", 1)
				negativeDir := t.TempDir()
				path := filepath.Join(negativeDir, "LegacyThrowsIllegal.java")
				if err := os.WriteFile(path, []byte(negative), 0600); err != nil {
					t.Fatal(err)
				}
				args := []string{"-proc:none", "--release", "8"}
				if profile == NativeJavac8 {
					args = []string{"-proc:none", "-source", "8", "-target", "8"}
				}
				if out, err := exec.Command(javac, append(args, "-d", negativeDir, path)...).CombinedOutput(); err == nil {
					t.Fatalf("CheckedError spelling cannot erase a checked throws obligation: %s", out)
				}
				for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
					env := map[string]string{}
					if policy == "no-source-rewrites" {
						env["JDEC_NO_SOURCE_REWRITES"] = "1"
					}
					if policy == "no-core-cleanups" {
						env["JDEC_NO_CORE_CLEANUPS"] = "1"
					}
					result, err := DecompileWithOptions(bytes.Clone(mutantFiles["LegacyThrowsBroken.class"]), DecompileOptions{Mode: Precision, TargetSourceVersion: 8, SourceCompiler: profile, EnvSnapshot: env, Resolve: func(name string) ([]byte, bool) { raw, ok := mutantFiles[name+".class"]; return bytes.Clone(raw), ok }})
					if err != nil || !strings.Contains(result.Source, DecompileStubMarker) {
						t.Fatalf("checked inheritance cannot add absent original constructor Exceptions: %s %v\n%s", policy, err, result.Source)
					}
				}
			})
		}
	}
}

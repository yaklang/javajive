package javaclassparser

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

const lambdaInterfaceHandleFixture = `interface HandleOwner {
 default int bias(){return 5;}
 default java.util.function.IntUnaryOperator bound(int captured){return value->captured+value+bias();}
 static java.util.function.IntUnaryOperator free(int captured){return value->captured+value;}
 default <T,R> java.util.function.Function<T,R> mapped(java.util.function.Function<? super T,? extends R> delegate){return value->{bias();return delegate.apply(value);};}
}
class HandleDriver implements HandleOwner {
 public static void main(String[]args){HandleOwner owner=new HandleDriver();int rows=0;for(int captured:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(int value:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){if(owner.bound(captured).applyAsInt(value)!=captured+value+5||HandleOwner.free(captured).applyAsInt(value)!=captured+value)throw new AssertionError("capture/overflow/invocation");rows++;}Object token=new Object();java.util.function.Function<Object,Object> identity=owner.mapped(x->x);if(identity.apply(null)!=null||identity.apply(token)!=token)throw new AssertionError("generic return identity");final IllegalStateException failure=new IllegalStateException("same");try{owner.mapped(x->{throw failure;}).apply(token);throw new AssertionError("missing throw");}catch(IllegalStateException actual){if(actual!=failure)throw new AssertionError("exception identity");}System.out.println(rows+":interface:handles:identity");}
}`

func lambdaInterfaceImplementation(t *testing.T, object *ClassObject, factory string) (*MemberInfo, *ConstantMethodHandleInfo) {
	t.Helper()
	for _, method := range object.Methods {
		name, _ := sourceBridgeUTF8(object, method.NameIndex)
		desc, _ := sourceBridgeUTF8(object, method.DescriptorIndex)
		if !strings.HasPrefix(name, "lambda$"+factory+"$") {
			continue
		}
		for _, constant := range object.ConstantPool {
			handle, ok := constant.(*ConstantMethodHandleInfo)
			if !ok || handle == nil || handle.ReferenceIndex == 0 || int(handle.ReferenceIndex) > len(object.ConstantPool) {
				continue
			}
			ref := nativeConstantMember(object.ConstantPool[handle.ReferenceIndex-1])
			if ref == nil {
				continue
			}
			n, d := getNameAndType(object.ConstantPool, ref.NameAndTypeIndex)
			if n == name && d == desc {
				return method, handle
			}
		}
	}
	t.Fatal("authored original implementation/handle missing", factory)
	return nil, nil
}

func TestAdversarialLambdaInterfaceHandleOriginalUseProof(t *testing.T) {
	files := nativeCompileDebugClasses(t, lambdaInterfaceHandleFixture, "none")
	_, java := t04Tools(t)
	dir := t.TempDir()
	for name, raw := range files {
		if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if got := t04RunJava(t, java, dir, "HandleDriver"); got != "25:interface:handles:identity\n" {
		t.Fatal("original JVM", got)
	}
	for _, factory := range []string{"bound", "free", "mapped"} {
		for _, variant := range []string{"original", "class tag", "field tag", "interface virtual kind", "constructor kind", "static mismatch", "wrong owner role", "pre52 special/static", "bootstrap interface tag", "missing reference", "budget", "canceled"} {
			t.Run(factory+"/"+variant, func(t *testing.T) {
				object, err := Parse(files["HandleOwner.class"])
				if err != nil {
					t.Fatal(err)
				}
				method, handle := lambdaInterfaceImplementation(t, object, factory)
				original, ok := object.ConstantPool[handle.ReferenceIndex-1].(*ConstantInterfaceMethodrefInfo)
				if !ok || original == nil {
					t.Fatal("independent javac did not produce interface method reference")
				}
				d := NewClassObjectDumper(object)
				switch variant {
				case "class tag":
					object.ConstantPool[handle.ReferenceIndex-1] = &ConstantMethodrefInfo{Type: "Methodref", ConstantMemberrefInfo: original.ConstantMemberrefInfo}
				case "field tag":
					object.ConstantPool[handle.ReferenceIndex-1] = &ConstantFieldrefInfo{Type: "Fieldref", ConstantMemberrefInfo: original.ConstantMemberrefInfo}
				case "interface virtual kind":
					handle.ReferenceKind = 5
				case "constructor kind":
					handle.ReferenceKind = 8
				case "static mismatch":
					method.AccessFlags ^= StaticFlag
				case "wrong owner role":
					object.AccessFlags &^= 0x0200
				case "pre52 special/static":
					object.MajorVersion = 51
				case "bootstrap interface tag":
					for _, attribute := range object.Attributes {
						if table, ok := attribute.(*BootstrapMethodsAttribute); ok {
							for _, row := range table.BootstrapMethods {
								bootstrap := object.ConstantPool[row.BootstrapMethodRef-1].(*ConstantMethodHandleInfo)
								if ref, ok := object.ConstantPool[bootstrap.ReferenceIndex-1].(*ConstantMethodrefInfo); ok {
									object.ConstantPool[bootstrap.ReferenceIndex-1] = &ConstantInterfaceMethodrefInfo{Type: "InterfaceMethodref", ConstantMemberrefInfo: ref.ConstantMemberrefInfo}
								}
							}
						}
					}
				case "missing reference":
					handle.ReferenceIndex = 0
				case "budget":
					d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				case "canceled":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					d.Work = workbudget.New(ctx, workbudget.Limits{})
				}
				name, _ := sourceBridgeUTF8(object, method.NameIndex)
				desc, _ := sourceBridgeUTF8(object, method.DescriptorIndex)
				want := core.LambdaDeclarationUnknown
				if variant == "original" {
					want = core.LambdaDeclarationSyntheticBody
				}
				if got := d.originalLambdaDeclaration(object.GetClassName(), name, desc); got != want {
					t.Fatalf("original declaration=%v want=%v", got, want)
				}
			})
		}
	}
}

func TestAdversarialLambdaInterfaceHandleRoundTrip(t *testing.T) {
	javac, java := t04Tools(t)
	// One runtime; these two compiler targets exercise distinct original
	// invokeSpecial/invokeInterface encodings, not a JVM-version matrix.
	for _, release := range []string{"8", "21"} {
		for _, owner := range []string{"HandleOwner", "AlternateCallbackScope"} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(release+"/"+owner+"/"+debug, func(t *testing.T) {
					fixture := strings.ReplaceAll(lambdaInterfaceHandleFixture, "HandleOwner", owner)
					files := nativeCompileReleaseClasses(t, fixture, debug, release)
					original := t.TempDir()
					for name, raw := range files {
						if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
							t.Fatal(err)
						}
					}
					oracle := t04RunJava(t, java, original, "HandleDriver")
					if oracle != "25:interface:handles:identity\n" {
						t.Fatal("original JVM", oracle)
					}
					for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
						t.Run(policy, func(t *testing.T) {
							if policy == "no-source-rewrites" {
								t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
							}
							if policy == "no-core-cleanups" {
								t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
							}
							target, _ := strconv.Atoi(release)
							result, err := DecompileWithOptions(files[owner+".class"], DecompileOptions{Mode: Precision, TargetSourceVersion: target})
							if err != nil || strings.Contains(result.Source, "unsupported bootstrap") || strings.Contains(result.Source, DecompileStubMarker) {
								t.Fatalf("interface source: %v\n%s", err, result.Source)
							}
							rebuilt := t.TempDir()
							// The original Driver remains byte-for-byte unchanged. No
							// original owner class is available to the compiler/runtime.
							if err := os.WriteFile(filepath.Join(rebuilt, "HandleDriver.class"), files["HandleDriver.class"], 0600); err != nil {
								t.Fatal(err)
							}
							file := filepath.Join(rebuilt, owner+".java")
							if err := os.WriteFile(file, []byte(result.Source), 0600); err != nil {
								t.Fatal(err)
							}
							cmd := exec.Command(javac, "-proc:none", "--release", release, "-g:"+debug, "-cp", rebuilt, "-d", rebuilt, file)
							if output, err := cmd.CombinedOutput(); err != nil {
								t.Fatalf("rebuilt compile: %v\n%s\n%s", err, output, result.Source)
							}
							if got := t04RunJava(t, java, rebuilt, "HandleDriver"); got != oracle {
								t.Fatalf("rebuilt JVM=%q want=%q", got, oracle)
							}
							next, err := os.ReadFile(filepath.Join(rebuilt, owner+".class"))
							if err != nil {
								t.Fatal(err)
							}
							before, after := methodParameterNamesAllMethodDeclarations(t, files[owner+".class"]), methodParameterNamesAllMethodDeclarations(t, next)
							if !reflect.DeepEqual(before, after) {
								t.Fatalf("all original method declarations changed: %s\n%#v\n%#v", fmt.Sprint(policy), before, after)
							}
							if driver, err := os.ReadFile(filepath.Join(rebuilt, "HandleDriver.class")); err != nil || !reflect.DeepEqual(driver, files["HandleDriver.class"]) {
								t.Fatal("Driver changed", err)
							}
						})
					}
				})
			}
		}
	}
}

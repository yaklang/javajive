package javaclassparser

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

const nativePrivatePermissionFixture = `class PermissionOwner{private int value=37;private static int shared=41;private int call(){return value;}private static int staticCall(){return shared;}class Child{int read(){OPERATION}}int run(){return new Child().read();}}class PermissionDriver{public static void main(String[]a){try{new PermissionOwner().run();throw new AssertionError("missing original illegal access");}catch(IllegalAccessError expected){System.out.println("original-illegal-private-access");}}}`

// These are valid physical classes, not invalid compiler inputs. Removing the
// nest permission leaves InnerClasses intact and the original JVM must throw.
// In particular, javac recompilation must not legalize the same operation just
// because source reconstruction has put the two declarations in one scope.
func TestNativeMemberPrivatePermissionRejectsOriginalIllegalAccess(t *testing.T) {
	operations := []string{"return PermissionOwner.this.value;", "PermissionOwner.this.value=43;return 43;", "return PermissionOwner.shared;", "PermissionOwner.shared=47;return 47;", "return PermissionOwner.this.call();", "return PermissionOwner.staticCall();"}
	for operation, body := range operations {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(fmt.Sprintf("operation=%d/%s", operation, debug), func(t *testing.T) {
				fixture := strings.Replace(nativePrivatePermissionFixture, "OPERATION", body, 1)
				files := nativeCompileSourceReleaseClasses(t, map[string]string{"PermissionOwner.java": fixture}, debug, "11")
				for name, raw := range files {
					if !strings.HasPrefix(name, "PermissionOwner") {
						continue
					}
					object, err := Parse(raw)
					if err != nil {
						t.Fatal(err)
					}
					modernNestTestRemoveAttribute(object, "NestHost")
					modernNestTestRemoveAttribute(object, "NestMembers")
					object.MajorVersion = 52
					files[name] = object.Bytes()
				}
				original := t.TempDir()
				for name, raw := range files {
					if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
						t.Fatal(err)
					}
				}
				_, java := t04Tools(t)
				if got := t04RunJava(t, java, original, "PermissionDriver"); got != "original-illegal-private-access\n" {
					t.Fatal(got)
				}
				z := nativeArchive(t, files)
				defer z.Close()
				object, err := Parse(files["PermissionOwner.class"])
				if err != nil {
					t.Fatal(err)
				}
				if z.prepareNativeMemberFamilyUnpublished(object, nil) != nil {
					t.Fatal("source scope legalized original illegal private access")
				}
			})
		}
	}
}

func TestNativeMemberPrivatePermissionUsesOriginalUsedSymbols(t *testing.T) {
	fixture := strings.Replace(nativePrivatePermissionFixture, "OPERATION", "return PermissionOwner.this.value;", 1)
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"PermissionOwner.java": fixture}, "none", "11")
	for _, variant := range []string{"original modern nest", "missing nest permission", "unused private reference", "loaded private handle", "unused private handle", "wrong identity", "failed family", "duplicate code", "malformed reference", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			objects := modernNestTestObjects(t, files)
			delete(objects, "PermissionDriver")
			p := &nativeMemberFamily{owner: "PermissionOwner", lexicalObjects: objects, modernNestObjects: objects}
			child := objects["PermissionOwner$Child"]
			var read *CodeAttribute
			for _, method := range child.Methods {
				name, _ := sourceBridgeUTF8(child, method.NameIndex)
				if name == "read" {
					for _, attr := range method.Attributes {
						if code, ok := attr.(*CodeAttribute); ok {
							read = code
						}
					}
				}
			}
			if read == nil {
				t.Fatal("read method missing")
			}
			var work *workbudget.Budget
			switch variant {
			case "missing nest permission", "unused private reference":
				p.modernNestObjects = nil
				if variant == "unused private reference" {
					read.Code = []byte{3, 172}
				}
			case "loaded private handle", "unused private handle":
				p.modernNestObjects = nil
				index := uint16(0)
				for i, constant := range child.ConstantPool {
					ref, ok := constant.(*ConstantFieldrefInfo)
					if !ok || ref == nil {
						continue
					}
					owner, _ := sourceBridgeClassName(child, ref.ClassIndex)
					if owner == "PermissionOwner" {
						index = uint16(i + 1)
						break
					}
				}
				if index == 0 {
					t.Fatal("physical private reference missing")
				}
				child.ConstantPool = append(child.ConstantPool, &ConstantMethodHandleInfo{ReferenceKind: 1, ReferenceIndex: index})
				h := len(child.ConstantPool)
				read.Code = []byte{3, 172}
				if variant == "loaded private handle" {
					read.Code = append([]byte{core.OP_LDC_W, byte(h >> 8), byte(h), core.OP_POP}, read.Code...)
				}
			case "wrong identity":
				p.lexicalObjects["PermissionOwner$Child"] = objects["PermissionOwner"]
			case "failed family":
				p.failed = true
			case "duplicate code":
				child.Methods[0].Attributes = append(child.Methods[0].Attributes, child.Methods[0].Attributes[0])
			case "malformed reference":
				read.Code = []byte{178, 0, 0, 172}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			reader := NewClassObjectDumper(objects["PermissionOwner"])
			reader.options.TargetSourceVersion = 11
			reader.Work = work
			reader.foldSiblingResolver = func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return raw, ok }
			want := variant == "original modern nest" || variant == "unused private reference" || variant == "unused private handle"
			if got := nativeMemberPrivatePermissionClosed(p, reader.nativeAnnotationDeclarationResolver(), work); got != want {
				t.Fatalf("original private permission=%v want=%v", got, want)
			}
		})
	}
}

func TestNativePrivatePermissionTargetResolvesDescriptorAndFieldOrder(t *testing.T) {
	fixture := `interface Visible{int value=19;}class LookupOwner{private int value=37;private int call(){return value;}static class Alias extends LookupOwner{public long value=41;}static class ViaInterface extends LookupOwner implements Visible{}}`
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"LookupOwner.java": fixture}, "none", "11")
	objects := modernNestTestObjects(t, files)
	resolve := func(name string) (*ClassObject, bool) { o := objects[name]; return o, o != nil }
	for _, row := range []struct {
		owner, name, desc, target string
		field                     bool
	}{
		{"LookupOwner$Alias", "value", "I", "LookupOwner", true},
		{"LookupOwner$Alias", "value", "J", "LookupOwner$Alias", true},
		{"LookupOwner$ViaInterface", "value", "I", "Visible", true},
		{"LookupOwner$Alias", "call", "()I", "LookupOwner", false},
		{"LookupOwner$Alias", "<init>", "(I)V", "", false},
	} {
		t.Run(row.owner+"/"+row.name+row.desc, func(t *testing.T) {
			owner, member, known := nativePrivatePermissionTarget(row.owner, row.name, row.desc, row.field, resolve, nil)
			if !known || row.target == "" && member != nil || row.target != "" && (member == nil || owner.GetClassName() != row.target) {
				t.Fatalf("target=%v member=%v known=%v", owner, member, known)
			}
		})
	}
	for _, variant := range []string{"missing ancestor", "cycle", "duplicate target", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			objects := modernNestTestObjects(t, files)
			var work *workbudget.Budget
			switch variant {
			case "missing ancestor":
				delete(objects, "LookupOwner")
			case "cycle":
				objects["LookupOwner$Alias"].SuperClass = objects["LookupOwner$Alias"].ThisClass
			case "duplicate target":
				objects["LookupOwner"].Fields = append(objects["LookupOwner"].Fields, objects["LookupOwner"].Fields[0])
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if _, _, known := nativePrivatePermissionTarget("LookupOwner$Alias", "value", "I", true, func(name string) (*ClassObject, bool) { o := objects[name]; return o, o != nil }, work); known {
				t.Fatal("incomplete physical declaration lookup accepted")
			}
		})
	}
}

func TestNativeMemberPrivatePermissionChecksUsedBootstrapHandles(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"HandleOwner.java": `class HandleOwner{private int call(){return 37;}class Child{java.util.function.IntSupplier read(){return HandleOwner.this::call;}}}`}, "none", "11")
	for _, nest := range []bool{true, false} {
		t.Run(fmt.Sprintf("original-nest=%t", nest), func(t *testing.T) {
			objects := modernNestTestObjects(t, files)
			p := &nativeMemberFamily{owner: "HandleOwner", lexicalObjects: objects}
			if nest {
				p.modernNestObjects = objects
			}
			reader := NewClassObjectDumper(objects[p.owner])
			reader.options.TargetSourceVersion = 11
			reader.foldSiblingResolver = func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return raw, ok }
			if got := nativeMemberPrivatePermissionClosed(p, reader.nativeAnnotationDeclarationResolver(), nil); got != nest {
				t.Fatalf("original bootstrap private permission=%v want=%v", got, nest)
			}
		})
	}
}

// A constructor is selected only from its symbolic owner. The emitted private
// no-argument constructor must not turn unrelated Object/AssertionError
// constructors into unproved lexical-private accesses. The supplier also uses
// an actual REF_newInvokeSpecial bootstrap argument, not an unused pool entry.
func TestNativeMemberPrivatePermissionExternalConstructorOwnership(t *testing.T) {
	for _, release := range []string{"8", "11"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run("release="+release+"/"+debug, func(t *testing.T) {
				files := nativeCompileSourceReleaseClasses(t, map[string]string{"IsolatedPermissionOwner.java": `class IsolatedPermissionOwner{private IsolatedPermissionOwner(){}static class Child{static Object run(){return new AssertionError();}static java.util.function.Supplier<Object> maker(){return AssertionError::new;}}}class ExternalPermissionDriver{public static void main(String[]a){System.out.println(IsolatedPermissionOwner.Child.run().getClass().getName()+":"+IsolatedPermissionOwner.Child.maker().get().getClass().getName());}}`}, debug, release)
				original := t.TempDir()
				for name, raw := range files {
					if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
						t.Fatal(err)
					}
				}
				_, java := t04Tools(t)
				if got := t04RunJava(t, java, original, "ExternalPermissionDriver"); got != "java.lang.AssertionError:java.lang.AssertionError\n" {
					t.Fatal(got)
				}
				objects := modernNestTestObjects(t, files)
				delete(objects, "ExternalPermissionDriver")
				child := objects["IsolatedPermissionOwner$Child"]
				usedConstructorHandle := false
				for _, attr := range child.Attributes {
					if boot, ok := attr.(*BootstrapMethodsAttribute); ok {
						for _, site := range boot.BootstrapMethods {
							for _, index := range site.BootstrapArguments {
								if handle, ok := child.ConstantPool[index-1].(*ConstantMethodHandleInfo); ok && handle.ReferenceKind == 8 {
									ref := constructorMotionMember(child, &core.OpCode{Instr: &core.Instruction{OpCode: core.OP_INVOKESPECIAL}, Data: []byte{byte(handle.ReferenceIndex >> 8), byte(handle.ReferenceIndex)}}, core.OP_INVOKESPECIAL)
									usedConstructorHandle = usedConstructorHandle || ref != nil && ref.Name == "java/lang/AssertionError" && ref.Member == "<init>" && ref.Description == "()V"
								}
							}
						}
					}
				}
				if !usedConstructorHandle {
					t.Fatal("fixture did not retain its used REF_newInvokeSpecial bootstrap argument")
				}
				p := &nativeMemberFamily{owner: "IsolatedPermissionOwner", lexicalObjects: objects, modernNestObjects: objects}
				requested := []string{}
				resolve := func(name string) (*ClassObject, bool) { requested = append(requested, name); return nil, false }
				if !nativeMemberPrivatePermissionClosed(p, resolve, nil) {
					t.Fatalf("unrelated external constructor treated as emitted private member; lookup=%v", requested)
				}
				if len(requested) != 0 {
					t.Fatalf("constructor owner identity did not exclude unrelated external metadata: %v", requested)
				}
			})
		}
	}
}

func TestNativeMemberPrivatePermissionEmittedConstructorStillRequiresPermission(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileSourceReleaseClasses(t, map[string]string{"PrivateConstructorOwner.java": `class PrivateConstructorOwner{private PrivateConstructorOwner(){}static class Child{static Object make(){return new PrivateConstructorOwner();}}}class PrivateConstructorDriver{public static void main(String[]a){try{PrivateConstructorOwner.Child.make();throw new AssertionError("missing illegal access");}catch(IllegalAccessError expected){System.out.println("original-illegal-constructor-access");}}}`}, debug, "11")
			objects := modernNestTestObjects(t, files)
			delete(objects, "PrivateConstructorDriver")
			p := &nativeMemberFamily{owner: "PrivateConstructorOwner", lexicalObjects: objects, modernNestObjects: objects}
			resolve := func(name string) (*ClassObject, bool) { o := objects[name]; return o, o != nil }
			if !nativeMemberPrivatePermissionClosed(p, resolve, nil) {
				t.Fatal("original constructor nest permission rejected")
			}
			for name, object := range objects {
				modernNestTestRemoveAttribute(object, "NestHost")
				modernNestTestRemoveAttribute(object, "NestMembers")
				object.MajorVersion = 52
				files[name+".class"] = object.Bytes()
			}
			original := t.TempDir()
			for name, raw := range files {
				if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			_, java := t04Tools(t)
			if got := t04RunJava(t, java, original, "PrivateConstructorDriver"); got != "original-illegal-constructor-access\n" {
				t.Fatal(got)
			}
			if nativeMemberPrivatePermissionClosed(p, resolve, nil) {
				t.Fatal("emitted private constructor lost original permission check")
			}
		})
	}
}

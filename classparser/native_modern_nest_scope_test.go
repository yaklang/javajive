package javaclassparser

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

const modernNestOwnershipFixture = `class ModernNestOwner {class Child {private Child(){} Object make(){return new Object(){Object owner(){return ModernNestOwner.this;}};}} Object make(){return new Child();}}`

func modernNestTestObjects(t *testing.T, files map[string][]byte) map[string]*ClassObject {
	t.Helper()
	objects := map[string]*ClassObject{}
	for name, raw := range files {
		// Unparsed attribute payloads borrow the input buffer. Each mutation
		// control needs independent bytes; otherwise one rejected input can
		// contaminate every later control and make unrelated proofs look safe.
		object, err := Parse(append([]byte(nil), raw...))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		objects[object.GetClassName()] = object
	}
	return objects
}

func modernNestTestAttribute(object *ClassObject, name string) *UnparsedAttribute {
	for _, attr := range object.Attributes {
		if raw, ok := attr.(*UnparsedAttribute); ok && raw != nil && raw.Name == name {
			return raw
		}
	}
	return nil
}

func modernNestTestRemoveAttribute(object *ClassObject, name string) {
	for i, attr := range object.Attributes {
		if raw, ok := attr.(*UnparsedAttribute); ok && raw != nil && raw.Name == name {
			object.Attributes = append(object.Attributes[:i], object.Attributes[i+1:]...)
			return
		}
	}
}

func TestNativeModernNestFormatRequiresOriginalLegalAttributes(t *testing.T) {
	files := nativeCompileReleaseClasses(t, modernNestOwnershipFixture, "none", "11")
	for _, kind := range []string{"original", "version only", "future version", "minor", "length", "truncated", "duplicate attribute", "both attributes", "self member", "duplicate member", "foreign package", "constant dynamic", "module", "package", "typed nil", "budget", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			root := modernNestTestObjects(t, files)["ModernNestOwner"]
			raw := modernNestTestAttribute(root, "NestMembers")
			if raw == nil || len(raw.Info) != 6 {
				t.Fatal("expected original two-member nest")
			}
			var work *workbudget.Budget
			switch kind {
			case "version only":
				modernNestTestRemoveAttribute(root, "NestMembers")
			case "future version":
				root.MajorVersion = 56
			case "minor":
				root.MinorVersion = 1
			case "length":
				raw.Length++
			case "truncated":
				raw.Info = raw.Info[:len(raw.Info)-1]
				raw.Length = uint32(len(raw.Info))
			case "duplicate attribute":
				root.Attributes = append(root.Attributes, raw)
			case "both attributes":
				root.Attributes = append(root.Attributes, &UnparsedAttribute{Name: "NestHost", Length: 2, Info: raw.Info[2:4]})
			case "self member":
				binary.BigEndian.PutUint16(raw.Info[2:], root.ThisClass)
			case "duplicate member":
				copy(raw.Info[4:], raw.Info[2:4])
			case "foreign package":
				cp := NewConstantPoolWithConstant(&root.ConstantPool)
				binary.BigEndian.PutUint16(raw.Info[2:], uint16(cp.AddNewClassInfo("elsewhere/Member")))
			case "constant dynamic":
				root.ConstantPool = append(root.ConstantPool, &ConstantDynamicInfo{})
			case "module":
				root.ConstantPool = append(root.ConstantPool, &ConstantModuleInfo{})
			case "package":
				root.ConstantPool = append(root.ConstantPool, &ConstantPackageInfo{})
			case "typed nil":
				root.ConstantPool = append(root.ConstantPool, (*ConstantClassInfo)(nil))
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeAccessorVersion(root, work); got != (kind == "original") {
				t.Fatalf("legal original format=%v", got)
			}
		})
	}
}

func TestNativeModernNestScopeRequiresReciprocalOriginalLexicalOwnership(t *testing.T) {
	files := nativeCompileReleaseClasses(t, modernNestOwnershipFixture, "none", "11")
	for _, kind := range []string{"original", "source8", "missing resolver", "missing member", "wrong identity", "missing reciprocal", "wrong host", "member also host", "missing lexical owner", "lexical cycle", "budget", "memory", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			objects := modernNestTestObjects(t, files)
			root, child := objects["ModernNestOwner"], objects["ModernNestOwner$Child"]
			d := NewClassObjectDumper(root)
			d.options.TargetSourceVersion = 11
			d.foldSiblingResolver = func(name string) ([]byte, bool) {
				object := objects[name]
				if object == nil {
					return nil, false
				}
				return object.Bytes(), true
			}
			switch kind {
			case "source8":
				d.options.TargetSourceVersion = 8
			case "missing resolver":
				d.foldSiblingResolver = nil
			case "missing member":
				delete(objects, "ModernNestOwner$Child$1")
			case "wrong identity":
				objects["ModernNestOwner$Child"] = root
			case "missing reciprocal":
				modernNestTestRemoveAttribute(child, "NestHost")
			case "wrong host":
				cp := NewConstantPoolWithConstant(&child.ConstantPool)
				binary.BigEndian.PutUint16(modernNestTestAttribute(child, "NestHost").Info, uint16(cp.AddNewClassInfo("AnotherOwner")))
			case "member also host":
				child.Attributes = append(child.Attributes, &UnparsedAttribute{Name: "NestMembers", Length: 2, Info: []byte{0, 0}})
			case "missing lexical owner":
				for i, a := range child.Attributes {
					if _, ok := a.(*InnerClassesAttribute); ok {
						child.Attributes = append(child.Attributes[:i], child.Attributes[i+1:]...)
						break
					}
				}
			case "lexical cycle":
				for _, a := range child.Attributes {
					if rows, ok := a.(*InnerClassesAttribute); ok {
						for _, row := range rows.Classes {
							if row.InnerClassInfoIndex == child.ThisClass {
								row.OuterClassInfoIndex = child.ThisClass
							}
						}
					}
				}
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			packet, known := d.nativeModernNestOriginalScope()
			if known != (kind == "original") {
				t.Fatalf("original complete ownership=%v", known)
			}
			if known && len(packet) != 3 {
				t.Fatalf("incomplete nest: %d", len(packet))
			}
		})
	}
}

func TestNativeModernNestSourceCommitAndPrivateAccessRequireWholeOwnedPacket(t *testing.T) {
	files := nativeCompileReleaseClasses(t, modernNestOwnershipFixture, "none", "11")
	for _, kind := range []string{"original", "missing source member", "source wrong reciprocal", "unlisted source member", "copied caller", "copied target", "foreign root", "failed plan", "memory", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			original := modernNestTestObjects(t, files)
			source := modernNestTestObjects(t, files)
			// The root pointer is the original transaction identity; members are
			// independently parsed source objects, as in the production planner.
			source["ModernNestOwner"] = original["ModernNestOwner"]
			p := &nativeMemberFamily{owner: "ModernNestOwner", modernNestObjects: original, lexicalObjects: source}
			caller, target := source["ModernNestOwner$Child$1"], source["ModernNestOwner$Child"]
			var work *workbudget.Budget
			switch kind {
			case "missing source member":
				delete(source, "ModernNestOwner$Child$1")
			case "source wrong reciprocal":
				binary.BigEndian.PutUint16(modernNestTestAttribute(target, "NestHost").Info, target.ThisClass)
			case "unlisted source member":
				extra := modernNestTestObjects(t, files)["ModernNestOwner$Child$1"]
				cp := NewConstantPoolWithConstant(&extra.ConstantPool)
				extra.ThisClass = uint16(cp.AddNewClassInfo("Unlisted"))
				source["Unlisted"] = extra
			case "copied caller":
				caller = modernNestTestObjects(t, files)[caller.GetClassName()]
			case "copied target":
				target = modernNestTestObjects(t, files)[target.GetClassName()]
			case "foreign root":
				p.lexicalObjects["ModernNestOwner"] = modernNestTestObjects(t, files)["ModernNestOwner"]
			case "failed plan":
				p.failed = true
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeModernNestPrivateConstructorAccess(p, caller, target, work); got != (kind == "original") {
				t.Fatalf("private access in committed original nest=%v", got)
			}
		})
	}
}

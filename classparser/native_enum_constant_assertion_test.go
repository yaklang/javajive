package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeEnumConstantAssertionsRequireCompleteOriginalProtocol(t *testing.T) {
	fixture, _ := enumConstantAssertionFixture(false)
	originals := nativeCompileClasses(t, fixture)
	for _, variant := range []string{"original", "ordinary flag", "mutable flag", "wrong flag descriptor", "duplicate flag", "additional field", "missing initializer", "duplicate initializer", "wrong status owner", "wrong status kind", "wrong negation", "initializer effect", "initializer stack", "initializer locals", "initializer handler", "flag write in method", "missing assertion method", "field handle", "wrong constructor operand", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			objects := map[string]*ClassObject{}
			for _, raw := range originals {
				obj, e := Parse(append([]byte(nil), raw...))
				if e != nil {
					t.Fatal(e)
				}
				objects[obj.GetClassName()] = obj
			}
			parent := objects["EnumAssertionOwner$Mode"]
			obj := objects["EnumAssertionOwner$Mode$2"]
			resolve := func(n string) (*ClassObject, bool) { o, k := objects[n]; return o, k }
			reader := NewClassObjectDumper(parent)
			plans, e := reader.nativeEnumConstantInitializationsWithDeclarations(resolve)
			if e != nil {
				t.Fatal(e)
			}
			plan := plans["XOR"]
			bridges := reader.nativeConstructorAccessBridges()
			var flag *MemberInfo
			var initializer, method, ctor *CodeAttribute
			var initMethod *MemberInfo
			for _, f := range obj.Fields {
				n, _ := sourceBridgeUTF8(obj, f.NameIndex)
				if n == nativeAssertionField {
					flag = f
				}
			}
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				for _, a := range m.Attributes {
					if code, ok := a.(*CodeAttribute); ok {
						switch n {
						case "<clinit>":
							initializer = code
							initMethod = m
						case "apply":
							method = code
						case "<init>":
							ctor = code
						}
					}
				}
			}
			if flag == nil || initializer == nil || method == nil || ctor == nil {
				t.Fatal("original constant-body assertion")
			}
			var work *workbudget.Budget
			switch variant {
			case "ordinary flag":
				flag.AccessFlags &^= 0x1000
			case "mutable flag":
				flag.AccessFlags &^= 16
			case "wrong flag descriptor":
				flag.DescriptorIndex = initMethod.DescriptorIndex
			case "duplicate flag":
				obj.Fields = append(obj.Fields, flag)
			case "additional field":
				copy := *flag
				copy.NameIndex = obj.Methods[0].NameIndex
				obj.Fields = append(obj.Fields, &copy)
			case "missing initializer":
				for i, m := range obj.Methods {
					if m == initMethod {
						obj.Methods = append(obj.Methods[:i], obj.Methods[i+1:]...)
						break
					}
				}
			case "duplicate initializer":
				obj.Methods = append(obj.Methods, initMethod)
			case "wrong status owner":
				initializer.Code[1] = byte(obj.ThisClass)
			case "wrong status kind":
				initializer.Code[2] = core.OP_INVOKESTATIC
			case "wrong negation":
				initializer.Code[8] = core.OP_ICONST_0
			case "initializer effect":
				initializer.Code = append([]byte{core.OP_ACONST_NULL, core.OP_POP}, initializer.Code...)
			case "initializer stack":
				initializer.MaxStack++
			case "initializer locals":
				initializer.MaxLocals++
			case "initializer handler":
				initializer.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 2, HandlerPc: 16}}
			case "flag write in method":
				method.Code[0] = core.OP_PUTSTATIC
			case "missing assertion method":
				for i, m := range obj.Methods {
					n, _ := sourceBridgeUTF8(obj, m.NameIndex)
					if n == "apply" {
						obj.Methods = append(obj.Methods[:i], obj.Methods[i+1:]...)
						break
					}
				}
			case "field handle":
				index := uint16(method.Code[1])<<8 | uint16(method.Code[2])
				obj.ConstantPool = append(obj.ConstantPool, &ConstantMethodHandleInfo{ReferenceKind: 2, ReferenceIndex: index})
			case "wrong constructor operand":
				ctor.Code[0] = core.OP_ALOAD_1
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			body := nativeEnumConstantBodyProof(parent, plan, 2, bridges, resolve, work)
			if (body != nil) != (variant == "original") {
				t.Fatalf("body admitted=%v", body != nil)
			}
			if body != nil {
				if body.assertions == nil || !body.assertions.pureInitializer || body.assertions.statusOwner != "EnumAssertionOwner" {
					t.Fatal("missing original body assertion certificate")
				}
				p := &nativeMemberFamily{enumConstants: map[string]*nativeEnumConstantBody{obj.GetClassName(): body}}
				if !nativeEnumConstantAssertionFieldOwned(p, obj.GetClassName(), nativeAssertionField, "Z", nil) {
					t.Fatal("certified own field refused")
				}
				for _, bad := range []struct{ owner, name, descriptor string }{{"Foreign", nativeAssertionField, "Z"}, {obj.GetClassName(), "custom", "Z"}, {obj.GetClassName(), nativeAssertionField, "I"}} {
					if nativeEnumConstantAssertionFieldOwned(p, bad.owner, bad.name, bad.descriptor, nil) {
						t.Fatal("unproved field granted ownership")
					}
				}
			}
		})
	}
}

func TestNativeEnumConstantAssertionFieldDoesNotGrantForeignArchiveOwnership(t *testing.T) {
	fixture, _ := enumConstantAssertionFixture(false)
	originals := nativeCompileClasses(t, fixture+`class ForeignConstantReader{static Object read(){return null;}}`)
	for _, variant := range []string{"original", "foreign field reference", "foreign executable read", "foreign field handle"} {
		t.Run(variant, func(t *testing.T) {
			files := map[string][]byte{}
			for n, raw := range originals {
				files[n] = append([]byte(nil), raw...)
			}
			if variant != "original" {
				obj, e := Parse(files["ForeignConstantReader.class"])
				if e != nil {
					t.Fatal(e)
				}
				add := func(c ConstantInfo) uint16 {
					obj.ConstantPool = append(obj.ConstantPool, c)
					return uint16(len(obj.ConstantPool))
				}
				className := add(&ConstantUtf8Info{Value: "EnumAssertionOwner$Mode$2"})
				owner := add(&ConstantClassInfo{NameIndex: className})
				name := add(&ConstantUtf8Info{Value: nativeAssertionField})
				descriptor := add(&ConstantUtf8Info{Value: "Z"})
				nt := add(&ConstantNameAndTypeInfo{NameIndex: name, DescriptorIndex: descriptor})
				field := add(&ConstantFieldrefInfo{ConstantMemberrefInfo: ConstantMemberrefInfo{ClassIndex: owner, NameAndTypeIndex: nt}})
				if variant == "foreign field handle" {
					add(&ConstantMethodHandleInfo{ReferenceKind: 2, ReferenceIndex: field})
				}
				if variant == "foreign executable read" {
					for _, m := range obj.Methods {
						n, _ := sourceBridgeUTF8(obj, m.NameIndex)
						if n != "read" {
							continue
						}
						for _, a := range m.Attributes {
							if code, ok := a.(*CodeAttribute); ok {
								code.Code = []byte{core.OP_GETSTATIC, byte(field >> 8), byte(field), core.OP_POP, core.OP_ACONST_NULL, core.OP_ARETURN}
								code.MaxStack = 1
							}
						}
					}
				}
				files["ForeignConstantReader.class"] = obj.Bytes()
			}
			a := nativeArchive(t, files)
			defer a.Close()
			root, e := Parse(append([]byte(nil), files["EnumAssertionOwner.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			d := a.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || len(p.enumConstants) != 2 {
				t.Fatal("original independently proved enum constant bodies")
			}
			if got := a.nativeEnumConstantsArchiveClosed(p, a.originalMemberIndex(), nil); got != (variant == "original") {
				t.Fatalf("archive closure=%v", got)
			}
		})
	}
}

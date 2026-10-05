package javaclassparser

import (
	"context"
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestNativeProtectedStaticFieldRequiresCompleteOriginalResolution(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		files := nativeCompileSourceReleaseClasses(t, nativeProtectedStaticFieldSources("ProtectedFieldOwner"), debug, "8")
		for _, variant := range []string{"original", "unknown parent", "wrong parent identity", "parent cycle", "interface search unknown", "own shadow", "intermediate shadow", "duplicate field", "private field", "public field", "instance field", "synthetic field", "constant field", "descriptor mismatch", "missing field", "same package", "wrong opcode", "wrong return", "stack", "extra effect", "handler", "nonsynthetic packet", "budget", "memory", "canceled"} {
			t.Run(debug+"/"+variant, func(t *testing.T) {
				root, _ := Parse(files["probe/access/ProtectedFieldOwner.class"])
				middle, _ := Parse(files["probe/base/FieldMiddle.class"])
				parent, _ := Parse(files["probe/base/FieldParent.class"])
				var bridge, field *MemberInfo
				var code *CodeAttribute
				for _, m := range root.Methods {
					n, _ := sourceBridgeUTF8(root, m.NameIndex)
					if n == "access$000" {
						bridge = m
						for _, a := range m.Attributes {
							if c, ok := a.(*CodeAttribute); ok {
								code = c
							}
						}
					}
				}
				for _, f := range parent.Fields {
					n, _ := sourceBridgeUTF8(parent, f.NameIndex)
					if n == "value" {
						field = f
					}
				}
				if bridge == nil || field == nil || code == nil {
					t.Fatal("original inherited field packet")
				}
				resolve := func(n string) (*ClassObject, bool) {
					if n == parent.GetClassName() {
						return parent, true
					}
					if n == middle.GetClassName() {
						return middle, true
					}
					return nil, false
				}
				var work *workbudget.Budget
				switch variant {
				case "unknown parent":
					resolve = func(string) (*ClassObject, bool) { return nil, false }
				case "wrong parent identity":
					resolve = func(string) (*ClassObject, bool) { return parent, true }
				case "parent cycle":
					middle.SuperClass = middle.ThisClass
				case "interface search unknown":
					middle.Interfaces = []uint16{middle.ThisClass}
				case "own shadow", "intermediate shadow":
					o := root
					if variant == "intermediate shadow" {
						o = middle
					}
					copy := *field
					copy.NameIndex = sourceBridgePoolString(t, o, "value")
					copy.DescriptorIndex = sourceBridgePoolString(t, o, "Ljava/lang/String;")
					o.Fields = append(o.Fields, &copy)
				case "duplicate field":
					parent.Fields = append(parent.Fields, field)
				case "private field":
					field.AccessFlags = (field.AccessFlags &^ 7) | 2
				case "public field":
					field.AccessFlags = (field.AccessFlags &^ 7) | 1
				case "instance field":
					field.AccessFlags &^= 8
				case "synthetic field":
					field.AccessFlags |= 0x1000
				case "constant field":
					field.Attributes = append(field.Attributes, &ConstantValueAttribute{})
				case "descriptor mismatch":
					field.DescriptorIndex = sourceBridgePoolString(t, parent, "Ljava/lang/String;")
				case "missing field":
					field.NameIndex = sourceBridgePoolString(t, parent, "otherValue")
				case "same package":
					cp := NewConstantPoolWithConstant(&root.ConstantPool)
					root.ThisClass = uint16(cp.AddNewClassInfo("probe/base/FieldOwner"))
					ref := root.ConstantPool[(int(code.Code[1])<<8|int(code.Code[2]))-1].(*ConstantFieldrefInfo)
					ref.ClassIndex = root.ThisClass
				case "wrong opcode":
					code.Code[0] = core.OP_GETFIELD
				case "wrong return":
					code.Code[3] = core.OP_IRETURN
				case "stack":
					code.MaxStack++
				case "extra effect":
					code.Code = append([]byte{core.OP_ICONST_0, core.OP_POP}, code.Code...)
				case "handler":
					code.ExceptionTable = []*ExceptionTableEntry{{}}
				case "nonsynthetic packet":
					bridge.AccessFlags &^= 0x1000
				case "budget":
					work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				case "memory":
					work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
				case "canceled":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					work = workbudget.New(ctx, workbudget.Limits{})
				}
				g := nativeMemberProtectedStaticFieldProof(root, bridge, resolve, work)
				if (g != nil) != (variant == "original") {
					t.Fatalf("inherited static read admitted=%v", g != nil)
				}
				if g != nil && (!g.inheritedField || !g.staticField || g.setter || g.call != nil) {
					t.Fatal("original operation identity")
				}
			})
		}
	}
}

func TestNativeProtectedStaticFieldClonesEachOriginalSymbol(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, nativeProtectedStaticFieldSources("ProtectedFieldOwner"), "none", "8")
	root, _ := Parse(files["probe/access/ProtectedFieldOwner.class"])
	middle, _ := Parse(files["probe/base/FieldMiddle.class"])
	parent, _ := Parse(files["probe/base/FieldParent.class"])
	resolve := func(n string) (*ClassObject, bool) {
		if n == parent.GetClassName() {
			return parent, true
		}
		if n == middle.GetClassName() {
			return middle, true
		}
		return nil, false
	}
	p := &nativeMemberFamily{owner: root.GetClassName(), lexicalObjects: map[string]*ClassObject{root.GetClassName(): root}}
	if !nativeMemberCollectPrivateGettersResolved(p, resolve, nil) || len(p.getters) != 4 {
		t.Fatal("distinct original field bridge identities")
	}
	a := "/*jdec-owned-getter:0:probe/access/ProtectedFieldOwner:value*/"
	b := "/*jdec-owned-getter:100:probe/access/ProtectedFieldOwner:value*/"
	c := "/*jdec-owned-getter:200:probe/access/ProtectedFieldOwner:number*/"
	d := "/*jdec-owned-getter:300:probe/access/ProtectedFieldOwner:bits*/"
	for _, r := range []struct {
		source string
		want   bool
	}{{a + b + c + d, true}, {a + c + d, false}, {a + a + b + c + d, false}, {b + a + c + d, false}, {strings.Join([]string{a, b, c, d}, "\""), false}} {
		if got := nativeMemberPrivateGetterSourceClosed(p, r.source, nil); got != r.want {
			t.Fatalf("registration %v want %v", got, r.want)
		}
	}
}

func TestNativeProtectedStaticFieldNeedsClosedInterfaceNameGraph(t *testing.T) {
	sources := nativeProtectedStaticFieldSources("ProtectedFieldOwner")
	sources["probe/base/FieldMarker.java"] = `package probe.base;public interface FieldMarker{}`
	files := nativeCompileSourceReleaseClasses(t, sources, "none", "8")
	for _, variant := range []string{"empty", "transitive empty", "unknown", "wrong identity", "not interface", "cycle", "same name", "same name other descriptor", "unrelated field", "deep", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			root, _ := Parse(files["probe/access/ProtectedFieldOwner.class"])
			middle, _ := Parse(files["probe/base/FieldMiddle.class"])
			parent, _ := Parse(files["probe/base/FieldParent.class"])
			marker, _ := Parse(files["probe/base/FieldMarker.class"])
			cp := NewConstantPoolWithConstant(&middle.ConstantPool)
			middle.Interfaces = []uint16{uint16(cp.AddNewClassInfo(marker.GetClassName()))}
			objects := map[string]*ClassObject{middle.GetClassName(): middle, parent.GetClassName(): parent, marker.GetClassName(): marker}
			var work *workbudget.Budget
			switch variant {
			case "transitive empty":
				second, _ := Parse(files["probe/base/FieldMarker.class"])
				c := NewConstantPoolWithConstant(&second.ConstantPool)
				second.ThisClass = uint16(c.AddNewClassInfo("probe/base/SecondMarker"))
				objects[second.GetClassName()] = second
				c = NewConstantPoolWithConstant(&marker.ConstantPool)
				marker.Interfaces = []uint16{uint16(c.AddNewClassInfo(second.GetClassName()))}
			case "unknown":
				delete(objects, marker.GetClassName())
			case "wrong identity":
				objects[marker.GetClassName()] = parent
			case "not interface":
				marker.AccessFlags &^= 0x0200
			case "cycle":
				marker.Interfaces = []uint16{marker.ThisClass}
			case "same name", "same name other descriptor", "unrelated field":
				name, desc := "value", "Ljava/lang/Object;"
				if variant == "same name other descriptor" {
					desc = "I"
				}
				if variant == "unrelated field" {
					name = "differentValue"
				}
				marker.Fields = []*MemberInfo{{NameIndex: sourceBridgePoolString(t, marker, name), DescriptorIndex: sourceBridgePoolString(t, marker, desc), AccessFlags: 0x19}}
			case "deep":
				current := marker
				for i := 0; i < 70; i++ {
					next, _ := Parse(files["probe/base/FieldMarker.class"])
					c := NewConstantPoolWithConstant(&next.ConstantPool)
					next.ThisClass = uint16(c.AddNewClassInfo("probe/base/Marker" + fmt.Sprint(i)))
					objects[next.GetClassName()] = next
					c = NewConstantPoolWithConstant(&current.ConstantPool)
					current.Interfaces = []uint16{uint16(c.AddNewClassInfo(next.GetClassName()))}
					current = next
				}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			resolve := func(n string) (*ClassObject, bool) { o, known := objects[n]; return o, known }
			owner, f := nativeMemberInheritedFieldTarget(root, "value", "Ljava/lang/Object;", resolve, work)
			want := variant == "empty" || variant == "transitive empty" || variant == "unrelated field"
			if (f != nil) != want || f != nil && owner != parent {
				t.Fatalf("resolved=%v owner=%v", f != nil, owner)
			}
		})
	}
}

func TestNativeProtectedStaticFieldSignatureKeepsOriginalErasure(t *testing.T) {
	files := nativeCompileClasses(t, nativeConcreteGenericCallFixture)
	for _, row := range []struct {
		name, sig string
		want      bool
	}{
		{"parameterized", "Ljava/util/List<Ljava/lang/String;>;", true},
		{"wildcard", "Ljava/util/List<+Ljava/lang/Number;>;", true},
		{"class formal", "Ljava/util/List<TT;>;", false},
		{"wrong erasure", "Ljava/util/ArrayList<Ljava/lang/String;>;", false},
		{"primitive argument", "Ljava/util/List<I>;", false},
		{"method grammar", "()Ljava/util/List<Ljava/lang/String;>;", false},
		{"malformed", "Ljava/util/List<Ljava/lang/String;>;x", false},
		{"nil signature", "", false}, {"duplicate signature", "Ljava/util/List<Ljava/lang/String;>;", false},
	} {
		t.Run(row.name, func(t *testing.T) {
			owner, _ := Parse(files["ConcreteOwner.class"])
			field := &MemberInfo{Attributes: []AttributeInfo{&SignatureAttribute{SignatureIndex: sourceBridgePoolString(t, owner, row.sig)}}}
			if row.name == "nil signature" {
				field.Attributes = []AttributeInfo{(*SignatureAttribute)(nil)}
			}
			if row.name == "duplicate signature" {
				field.Attributes = append(field.Attributes, field.Attributes[0])
			}
			generic, known := nativeMemberInheritedFieldSignature(owner, field, "Ljava/util/List;", nil)
			if known != row.want || known && !generic {
				t.Fatalf("generic=%v known=%v", generic, known)
			}
		})
	}
}

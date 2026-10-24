package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestNativeSetterPacketsRequireOriginalStorageAndTypedWrite(t *testing.T) {
	for _, storage := range []string{"own static", "inherited static", "inherited instance"} {
		for _, typ := range []string{"Object", "long"} {
			static := "static "
			if storage == "inherited instance" {
				static = ""
			}
			field := "private " + static + typ + " value;"
			extends := ""
			if strings.HasPrefix(storage, "inherited") {
				field = ""
				extends = " extends p.Middle"
			}
			lhs := "Root.value"
			args := typ + " value"
			if static == "" {
				lhs = "owner.value"
				args = "Root owner," + args
			}
			sources := map[string]string{"p/Base.java": "package p;public class Base{protected " + static + typ + " value;}", "p/Middle.java": "package p;public class Middle extends Base{}", "q/Root.java": "package q;public class Root" + extends + "{" + field + "public static class Writer{public " + typ + " put(" + args + "){return " + lhs + "=value;}}}"}
			for _, debug := range []string{"none", "source,lines,vars"} {
				files := nativeCompileSourceReleaseClasses(t, sources, debug, "8")
				for _, variant := range []string{"original", "missing parent", "wrong parent identity", "parent cycle", "unknown interface", "own hiding", "intermediate hiding", "duplicate field", "private inherited field", "mixed visibility", "enum field", "unknown field flags", "public field", "storage mismatch", "final field", "synthetic field", "constant field", "wrong descriptor", "wrong field name", "non-synthetic", "wrong duplicate", "wrong return", "wrong opcode", "nil code attribute", "nil field attribute", "wrong stack", "wrong locals", "extra effect", "handler", "budget", "memory", "canceled"} {
					if storage == "own static" && (variant == "missing parent" || variant == "wrong parent identity" || variant == "parent cycle" || variant == "unknown interface" || variant == "intermediate hiding" || variant == "private inherited field") {
						continue
					}
					t.Run(storage+"/"+typ+"/"+debug+"/"+variant, func(t *testing.T) {
						parse := func(n string) *ClassObject {
							t.Helper()
							o, e := Parse(files[n+".class"])
							if e != nil {
								t.Fatal(e)
							}
							return o
						}
						root, middle, parent := parse("q/Root"), parse("p/Middle"), parse("p/Base")
						target := parent
						if storage == "own static" {
							target = root
						}
						var method, field *MemberInfo
						var code *CodeAttribute
						for _, m := range root.Methods {
							n, _ := sourceBridgeUTF8(root, m.NameIndex)
							if n == "access$002" {
								method = m
								for _, a := range m.Attributes {
									if c, ok := a.(*CodeAttribute); ok {
										code = c
									}
								}
							}
						}
						for _, f := range target.Fields {
							n, _ := sourceBridgeUTF8(target, f.NameIndex)
							if n == "value" {
								field = f
							}
						}
						if method == nil || field == nil || code == nil {
							t.Fatal("independent compiler produced no original setter packet")
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
						fieldAt := 2
						if static == "" {
							fieldAt = 3
						}
						switch variant {
						case "missing parent":
							resolve = func(string) (*ClassObject, bool) { return nil, false }
						case "wrong parent identity":
							resolve = func(string) (*ClassObject, bool) { return parent, true }
						case "parent cycle":
							middle.SuperClass = middle.ThisClass
						case "unknown interface":
							middle.Interfaces = []uint16{middle.ThisClass}
						case "own hiding", "intermediate hiding":
							o := root
							if variant == "intermediate hiding" {
								o = middle
							}
							f := *field
							f.NameIndex = sourceBridgePoolString(t, o, "value")
							f.DescriptorIndex = sourceBridgePoolString(t, o, "Ljava/lang/String;")
							o.Fields = append(o.Fields, &f)
						case "duplicate field":
							target.Fields = append(target.Fields, field)
						case "private inherited field":
							field.AccessFlags = field.AccessFlags&^7 | 2
						case "mixed visibility":
							field.AccessFlags |= 1 | 2
						case "enum field":
							field.AccessFlags |= 0x4000
						case "unknown field flags":
							field.AccessFlags |= 0x20
						case "public field":
							field.AccessFlags = field.AccessFlags&^7 | 1
						case "storage mismatch":
							field.AccessFlags ^= 8
						case "final field":
							field.AccessFlags |= 16
						case "synthetic field":
							field.AccessFlags |= 0x1000
						case "constant field":
							field.Attributes = append(field.Attributes, &ConstantValueAttribute{})
						case "wrong descriptor":
							field.DescriptorIndex = sourceBridgePoolString(t, target, "I")
						case "wrong field name":
							field.NameIndex = sourceBridgePoolString(t, target, "other")
						case "non-synthetic":
							method.AccessFlags &^= 0x1000
						case "wrong duplicate":
							if typ == "long" {
								code.Code[fieldAt-1] = core.OP_DUP
							} else {
								code.Code[fieldAt-1] = core.OP_DUP2
							}
						case "wrong return":
							code.Code[len(code.Code)-1] = core.OP_RETURN
						case "wrong opcode":
							code.Code[fieldAt] = core.OP_GETSTATIC
						case "nil code attribute":
							method.Attributes = append([]AttributeInfo{(*CodeAttribute)(nil)}, method.Attributes...)
						case "nil field attribute":
							field.Attributes = append(field.Attributes, (*SignatureAttribute)(nil))
						case "wrong stack":
							code.MaxStack++
						case "wrong locals":
							code.MaxLocals++
						case "extra effect":
							code.Code = append([]byte{core.OP_ICONST_0, core.OP_POP}, code.Code...)
						case "handler":
							code.ExceptionTable = []*ExceptionTableEntry{{}}
						case "budget":
							work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
						case "memory":
							work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
						case "canceled":
							ctx, cancel := context.WithCancel(context.Background())
							cancel()
							work = workbudget.New(ctx, workbudget.Limits{})
						}
						packet := nativeMemberSetterPacketProof(root, method, resolve, work)
						if (packet != nil) != (variant == "original") {
							t.Fatalf("setter admitted=%v", packet != nil)
						}
						if packet != nil && (!packet.setter || packet.staticField != (static != "") || packet.inheritedField != (target == parent)) {
							t.Fatal("packet lost original write/storage/declaration identity")
						}
					})
				}
			}
		}
	}
}

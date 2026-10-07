package javaclassparser

import (
	"bytes"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeLegacyEnumConstantMetadataRequiresOriginalExecutablePacket(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"ConstantPacketOwner.java": nativeEnumConstantPacketFixture}, "none", "8")
	for _, variant := range []string{"original", "old final row", "old static row", "old static final row", "current missing parameters", "pre enum version", "unknown version", "minor version", "opaque attribute", "parameters on old version", "wrong flags", "wrong operand", "handler", "effect", "frame", "work"} {
		t.Run(variant, func(t *testing.T) {
			objects := map[string]*ClassObject{}
			for n, raw := range files {
				obj, e := Parse(bytes.Clone(raw))
				if e != nil {
					t.Fatal(e)
				}
				objects[n[:len(n)-6]] = obj
			}
			parent := objects["ConstantPacketOwner$Mode"]
			obj := objects["ConstantPacketOwner$Mode$1"]
			resolve := func(n string) (*ClassObject, bool) { v, ok := objects[n]; return v, ok }
			reader := NewClassObjectDumper(parent)
			plans, e := reader.nativeEnumConstantInitializationsWithDeclarations(resolve)
			if e != nil {
				t.Fatal(e)
			}
			plan := plans["FIRST"]
			bridges := reader.nativeConstructorAccessBridges()
			obj.MajorVersion = 51
			var ctor *MemberInfo
			var code *CodeAttribute
			var parameters *UnparsedAttribute
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n != "<init>" {
					continue
				}
				ctor = m
				attrs := []AttributeInfo{}
				for _, a := range m.Attributes {
					if p, ok := a.(*UnparsedAttribute); ok && p.Name == "MethodParameters" {
						parameters = p
						continue
					}
					if c, ok := a.(*CodeAttribute); ok {
						code = c
					}
					attrs = append(attrs, a)
				}
				m.Attributes = attrs
			}
			var self *InnerClassInfo
			for _, a := range obj.Attributes {
				if table, ok := a.(*InnerClassesAttribute); ok {
					for _, row := range table.Classes {
						n, _ := sourceBridgeClassName(obj, row.InnerClassInfoIndex)
						if n == obj.GetClassName() {
							self = row
							self.InnerClassAccessFlags = 0x4000
						}
					}
				}
			}
			if ctor == nil || code == nil || parameters == nil || self == nil {
				t.Fatal("original packet missing")
			}
			var work *workbudget.Budget
			switch variant {
			case "old final row":
				self.InnerClassAccessFlags = 0x4010
			case "old static row":
				self.InnerClassAccessFlags = 0x4008
			case "old static final row":
				self.InnerClassAccessFlags = 0x4018
			case "current missing parameters":
				obj.MajorVersion = 52
			case "pre enum version":
				obj.MajorVersion = 48
			case "unknown version":
				obj.MajorVersion = 53
			case "minor version":
				obj.MinorVersion = 1
			case "opaque attribute":
				ctor.Attributes = append(ctor.Attributes, &UnparsedAttribute{Name: "OpaqueOriginalMetadata"})
			case "parameters on old version":
				ctor.Attributes = append(ctor.Attributes, parameters)
			case "wrong flags":
				self.InnerClassAccessFlags = 0x4001
			case "wrong operand":
				code.Code[0] = byte(core.OP_ALOAD_1)
			case "handler":
				code.ExceptionTable = append(code.ExceptionTable, &ExceptionTableEntry{StartPc: 0, EndPc: 1, HandlerPc: 1})
			case "effect":
				code.Code = append([]byte{byte(core.OP_ICONST_0), byte(core.OP_POP)}, code.Code...)
			case "frame":
				code.MaxLocals--
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				work.Charge(workbudget.CounterGraphScans, 1)
			}
			got := nativeEnumConstantBodyProof(parent, plan, 1, bridges, resolve, work)
			want := variant == "original" || variant == "current missing parameters" || variant == "old final row" || variant == "old static row" || variant == "old static final row"
			if (got != nil) != want {
				t.Fatalf("accepted=%v", got != nil)
			}
			if got != nil && !got.legacyConstructorMetadata {
				t.Fatal("reflection representation delta lost")
			}
		})
	}
}

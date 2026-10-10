package javaclassparser

import (
	"context"
	"encoding/binary"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeEnumSwitchUserClosureRejectsEscapesAndChangedOriginalPackets(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, nativeEnumSwitchFamilySources("SwitchFamilyOwner"), "none", "8")
	for _, variant := range []string{"original", "invalid index", "foreign reader", "handle", "array escape", "array write", "changed ordinal", "changed enum parameter descriptor", "changed array opcode", "unmapped switch key", "alternate entry", "source constructor marker", "extra own declaration", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["SwitchFamilyOwner.class"])
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || !d.planNativeMemberAnonymousScopes(p) {
				t.Fatal("original plan")
			}
			originalIndex := z.originalMemberIndex()
			index := *originalIndex
			child := p.anonymousUnits["SwitchFamilyOwner$1"].children["SwitchFamilyOwner$1"].object
			var method *MemberInfo
			var code *CodeAttribute
			for _, m := range child.Methods {
				n, _ := sourceBridgeUTF8(child, m.NameIndex)
				if n == "run" {
					method = m
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			if code == nil {
				t.Fatal("original method")
			}
			var work *workbudget.Budget
			switch variant {
			case "invalid index":
				index.valid = false
			case "foreign reader":
				index.typeUsers = map[string]map[string]bool{"SwitchFamilyOwner$2": {"Foreign": true}}
			case "handle":
				index.handles = map[string]bool{"SwitchFamilyOwner$2": true}
			case "array escape":
				code.Code = append([]byte{core.OP_GETSTATIC, code.Code[1], code.Code[2], core.OP_ARRAYLENGTH, core.OP_IRETURN}, code.Code[8:]...)
			case "array write":
				code.Code[0] = core.OP_PUTSTATIC
			case "changed ordinal":
				ref := child.ConstantPool[(int(code.Code[5])<<8|int(code.Code[6]))-1].(*ConstantMethodrefInfo)
				nt := child.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				nt.NameIndex = sourceBridgePoolString(t, child, "hashCode")
			case "changed enum parameter descriptor":
				method.DescriptorIndex = sourceBridgePoolString(t, child, "(Ljava/lang/Object;)I")
			case "changed array opcode":
				code.Code[7] = core.OP_AALOAD
			case "unmapped switch key":
				if code.Code[8] != core.OP_LOOKUPSWITCH {
					t.Fatal("fixture is not lookup")
				}
				binary.BigEndian.PutUint32(code.Code[20:24], 7)
			case "alternate entry":
				code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 8, HandlerPc: 3, CatchType: child.SuperClass}}
			case "source constructor marker":
				child.Fields = append(child.Fields, &MemberInfo{NameIndex: sourceBridgePoolString(t, child, "marker"), DescriptorIndex: sourceBridgePoolString(t, child, "LSwitchFamilyOwner$2;")})
			case "extra own declaration":
				child.Fields = append(child.Fields, nil)
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := z.nativeEnumSwitchUsersClosed(p, root, &index, work); got != (variant == "original") {
				t.Fatalf("admission=%v", got)
			}
		})
	}
}

func TestNativeEnumSwitchArtifactOwnershipRequiresOriginalAttributesAndNamespace(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, nativeEnumSwitchFamilySources("SwitchFamilyOwner"), "source,lines,vars", "8")
	for _, variant := range []string{"original", "foreign owner", "missing self row", "named row", "member row", "wrong row flags", "duplicate inner", "missing enclosing", "duplicate enclosing", "executable enclosing method", "unknown attribute", "duplicate source", "source length", "wrong ordinal", "private constructor marker", "work", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj, _ := Parse(files["SwitchFamilyOwner$2.class"])
			var work *workbudget.Budget
			owner := "SwitchFamilyOwner"
			for _, a := range obj.Attributes {
				switch a := a.(type) {
				case *InnerClassesAttribute:
					row := a.Classes[0]
					switch variant {
					case "missing self row":
						a.Classes = nil
					case "named row":
						row.InnerNameIndex = sourceBridgePoolString(t, obj, "Helper")
					case "member row":
						row.OuterClassInfoIndex = obj.SuperClass
					case "wrong row flags":
						row.InnerClassAccessFlags = 0x1000
					case "duplicate inner":
						obj.Attributes = append(obj.Attributes, a)
					}
				case *UnparsedAttribute:
					if a.Name == "EnclosingMethod" {
						switch variant {
						case "missing enclosing":
							a.Name = "Unknown"
						case "duplicate enclosing":
							obj.Attributes = append(obj.Attributes, a)
						case "executable enclosing method":
							a.Info[3] = 1
						}
					}
				case *SourceFileAttribute:
					switch variant {
					case "duplicate source":
						obj.Attributes = append(obj.Attributes, a)
					case "source length":
						a.AttrLen++
					}
				}
			}
			if variant == "foreign owner" {
				owner = "Foreign"
			}
			if variant == "unknown attribute" {
				obj.Attributes = append(obj.Attributes, &UnparsedAttribute{Name: "Unknown"})
			}
			if variant == "work" {
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			if variant == "canceled" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if variant == "wrong ordinal" || variant == "private constructor marker" {
				table := nativeEnumSwitchTableProof(obj, nil)
				if table == nil {
					t.Fatal("original packet")
				}
				p := &nativeMemberFamily{owner: "SwitchFamilyOwner", enumSwitchTables: map[string]*nativeEnumSwitchTable{obj.GetClassName(): table}, anonymous: &nativeAnonymousFamily{children: map[string]*nativeAnonymousClass{"SwitchFamilyOwner$1": {}}}}
				if variant == "wrong ordinal" {
					p.anonymous = nil
				} else {
					p.emptyMarkers = map[string]*ClassObject{"SwitchFamilyOwner$1": {}}
				}
				if nativeEnumSwitchOrdinalClosed(p, nil) {
					t.Fatal("unproved compiler namespace")
				}
				return
			}
			if got := nativeEnumSwitchArtifactMetadata(obj, owner, work); got != (variant == "original") {
				t.Fatalf("metadata admission=%v", got)
			}
		})
	}
}

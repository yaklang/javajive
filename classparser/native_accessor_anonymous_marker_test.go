package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeAccessorAnonymousMarkerRequiresExactBridgeSymbol(t *testing.T) {
	files := nativeCompileClasses(t, nativeAccessorAnonymousConstructorMarkerFixture())
	for _, variant := range []string{"original", "missing members", "foreign root", "missing group", "wrong ordinal", "foreign child identity", "missing unit", "ordinary field", "ordinary constructor", "changed bridge code", "foreign constructor owner", "constructor handle", "dynamic name type", "dead name type", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, e := Parse(files["AccessScopeOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || !d.planNativeMemberAnonymousScopes(p) || p.anonymousForest == nil {
				t.Fatal("missing original mixed forest")
			}
			forest := p.anonymousForest
			child := p.children["AccessScopeOwner$Reader"]
			if child == nil || len(child.accessBridges) != 1 {
				t.Fatal("original private constructor bridge")
			}
			var bridge *nativeConstructorAccessBridge
			for _, b := range child.accessBridges {
				bridge = b
			}
			var reference *ConstantMethodrefInfo
			var referenceIndex uint16
			for i, k := range root.ConstantPool {
				if m, ok := k.(*ConstantMethodrefInfo); ok && m != nil {
					owner, known := sourceBridgeClassName(root, m.ClassIndex)
					if known && owner == child.object.GetClassName() {
						nt, ok := root.ConstantPool[m.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
						if ok {
							name, _ := sourceBridgeUTF8(root, nt.NameIndex)
							desc, _ := sourceBridgeUTF8(root, nt.DescriptorIndex)
							if name == "<init>" && desc == bridge.descriptor {
								reference = m
								referenceIndex = uint16(i + 1)
							}
						}
					}
				}
			}
			if reference == nil {
				t.Fatal("original constructor symbol")
			}
			var work *workbudget.Budget
			switch variant {
			case "missing members":
				forest.members = nil
			case "foreign root":
				forest.root = "Foreign"
			case "missing group":
				delete(forest.groups, forest.root)
			case "wrong ordinal":
				forest.units[bridge.marker].ordinal = 2
			case "foreign child identity":
				marker := forest.units[bridge.marker].object
				cp := NewConstantPoolWithConstant(&marker.ConstantPool)
				marker.ThisClass = uint16(cp.AddNewClassInfo("Foreign"))
			case "missing unit":
				delete(forest.units, bridge.marker)
			case "ordinary field":
				cp := NewConstantPoolWithConstant(&root.ConstantPool)
				root.Fields[0].DescriptorIndex = uint16(cp.AddUtf8Info("L" + bridge.marker + ";"))
			case "ordinary constructor":
				bridge.method.AccessFlags = 0
			case "changed bridge code":
				for _, a := range bridge.method.Attributes {
					if code, ok := a.(*CodeAttribute); ok {
						code.Code[0] = 0x01
					}
				}
			case "foreign constructor owner":
				reference.ClassIndex = root.ThisClass
			case "constructor handle":
				root.ConstantPool = append(root.ConstantPool, &ConstantMethodHandleInfo{ReferenceKind: 8, ReferenceIndex: referenceIndex})
			case "dynamic name type":
				root.ConstantPool = append(root.ConstantPool, &ConstantInvokeDynamicInfo{NameAndTypeIndex: reference.NameAndTypeIndex})
			case "dead name type":
				nt := *root.ConstantPool[reference.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				root.ConstantPool = append(root.ConstantPool, &nt)
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if known := nativeAnonymousForestBridgeMarker(forest, bridge.marker) && nativeAnonymousForestSymbolClosure(forest, work); known != (variant == "original") {
				t.Fatalf("symbol closure %v", known)
			}
		})
	}
}

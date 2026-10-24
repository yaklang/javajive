package javaclassparser

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialExtraVisibilityBridgeRequiresExactFreshForwarding(t *testing.T) {
	original, raw, artifact := constructorSuffixFamilyRoundTrip(t, inheritedVisibilityDomainFixture, "none", []string{"SuffixOwner$Visible"}, "72:inherited:visibility:dispatch:failure:words\n")
	base, owner := artifact.source.jar.sourceConstructorLowering, "SuffixOwner$Visible"
	origin, active := base.bridgeActive[owner]
	if !active {
		t.Fatal("missing returned source receipt")
	}
	for _, variant := range []string{"control", "idempotent", "ordinary method", "wrong receiver", "wrong wide slot", "virtual call", "foreign target", "wrong return", "extra ordinary method", "unreturned", "pending", "stale original", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			a := &sourceConstructorLoweringAuthority{jar: base.jar, bridgeActive: map[string]constructorNativeSourceOrigin{owner: origin}}
			obj := mustParseLowering(t, bytes.Clone(raw))
			var bridge *MemberInfo
			for _, m := range obj.Methods {
				if m.AccessFlags == 0x1041 {
					if bridge != nil {
						t.Fatal("ambiguous fresh fixture bridge")
					}
					bridge = m
				}
			}
			packet := inheritedBridgeForwardPacket(obj, bridge, core.OP_INVOKESPECIAL, nil)
			if packet == nil {
				t.Fatal("fresh compiler did not produce visibility forwarder")
			}
			code := constructorLoweringCode(bridge)
			old := bytes.Clone(original)
			var work *workbudget.Budget
			switch variant {
			case "ordinary method":
				bridge.AccessFlags = 1
			case "wrong receiver":
				code.Code[0] = core.OP_ALOAD_1
			case "wrong wide slot":
				if code.Code[1] != core.OP_LLOAD_1 {
					t.Fatal("unexpected original wide port")
				}
				code.Code[1] = core.OP_LLOAD_2
			case "virtual call":
				code.Code[packet.invokePC] = core.OP_INVOKEVIRTUAL
			case "foreign target":
				r := nativeConstantMember(obj.ConstantPool[packet.invokeIndex-1])
				r.ClassIndex = uint16(obj.ConstantPoolManager.AddNewClassInfo("UnrelatedParent"))
			case "wrong return":
				code.Code[len(code.Code)-1] = core.OP_IRETURN
			case "extra ordinary method":
				copy := *bridge
				copy.NameIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("additional"))
				copy.AccessFlags = 1
				obj.Methods = append(obj.Methods, &copy)
			case "unreturned":
				a.bridgeActive = nil
			case "pending":
				a.bridgePending, a.bridgeActive = a.bridgeActive, nil
			case "stale original":
				o := mustParseLowering(t, old)
				o.ConstantPoolManager.AddUtf8Info("changed-original")
				old = o.Bytes()
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			input := obj.Bytes()
			if _, err := Parse(input); err != nil {
				t.Fatal("negative fixture must remain structurally parseable", err)
			}
			if variant == "idempotent" {
				var err error
				input, err = a.lowerInheritedBridges(owner, old, input, nil)
				if err != nil {
					t.Fatal(err)
				}
			}
			saved := bytes.Clone(input)
			out, err := a.lowerInheritedBridges(owner, old, input, work)
			if !bytes.Equal(input, saved) {
				t.Fatal("mutated caller bytes")
			}
			if variant == "budget" || variant == "canceled" {
				if err == nil || out != nil {
					t.Fatal("work boundary ignored")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if variant != "control" && variant != "idempotent" {
				if !bytes.Equal(out, input) {
					t.Fatal("unproved fresh declaration removed")
				}
				return
			}
			before, after := mustParseLowering(t, input), mustParseLowering(t, out)
			if !reflect.DeepEqual(before.ConstantPool, after.ConstantPool) || len(after.Methods) != len(mustParseLowering(t, original).Methods) {
				t.Fatal("constant or original method universe changed")
			}
			for _, m := range after.Methods {
				var matched *MemberInfo
				for _, n := range before.Methods {
					if m.NameIndex == n.NameIndex && m.DescriptorIndex == n.DescriptorIndex {
						matched = n
					}
				}
				if matched == nil || !reflect.DeepEqual(m, matched) {
					t.Fatal("retained fresh method changed")
				}
			}
			if again, e := a.lowerInheritedBridges(owner, old, out, nil); e != nil || !bytes.Equal(again, out) {
				t.Fatal("not idempotent", e)
			}
		})
	}
}

func TestAdversarialExtraVisibilityBridgeRequiresOriginalDeclarationDomain(t *testing.T) {
	original, raw, artifact := constructorSuffixFamilyRoundTrip(t, inheritedVisibilityDomainFixture, "none", []string{"SuffixOwner$Visible"}, "72:inherited:visibility:dispatch:failure:words\n")
	base := artifact.source.jar.sourceConstructorLowering
	root := NewClassObjectDumper(mustParseLowering(t, original))
	root.foldSiblingResolver = base.jar.enumSiblingResolver()
	root.declarationResolver = base.jar.declarationResolver
	parent := root.inheritedVisibilityDomainParent()
	if parent == nil {
		t.Fatal("fixture needs its original non-public parent")
	}
	parentBytes := parent.Bytes()
	for _, variant := range []string{"control", "non-public child", "public parent", "interface parent", "missing parent", "foreign parent", "preview child", "old child", "new child", "existing declaration", "missing target", "duplicate target", "private target", "static target", "final target", "synchronized target", "native target", "abstract target", "changed throws", "nil original", "nil target", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			want, fresh, parent := mustParseLowering(t, original), mustParseLowering(t, raw), mustParseLowering(t, parentBytes)
			d := NewClassObjectDumper(want)
			var bridge, target *MemberInfo
			for _, m := range fresh.Methods {
				if m.AccessFlags == 0x1041 {
					bridge = m
				}
			}
			p := inheritedBridgeForwardPacket(fresh, bridge, core.OP_INVOKESPECIAL, nil)
			if p == nil {
				t.Fatal("missing fresh forwarding packet")
			}
			for _, m := range parent.Methods {
				n, _ := sourceBridgeUTF8(parent, m.NameIndex)
				desc, _ := sourceBridgeUTF8(parent, m.DescriptorIndex)
				if n == p.name && desc == p.descriptor {
					target = m
				}
			}
			if target == nil {
				t.Fatal("missing original parent target")
			}
			switch variant {
			case "non-public child":
				want.AccessFlags &^= 1
			case "public parent":
				parent.AccessFlags |= 1
			case "interface parent":
				parent.AccessFlags |= 0x200
			case "foreign parent":
				parent.ThisClass = uint16(parent.ConstantPoolManager.AddNewClassInfo("DifferentAncestor"))
			case "preview child":
				want.MinorVersion = 65535
			case "old child":
				want.MajorVersion = 48
			case "new child":
				want.MajorVersion = 65
			case "existing declaration":
				copy := *bridge
				copy.NameIndex = uint16(want.ConstantPoolManager.AddUtf8Info(p.name))
				copy.DescriptorIndex = uint16(want.ConstantPoolManager.AddUtf8Info(p.descriptor))
				want.Methods = append(want.Methods, &copy)
			case "missing target":
				target.NameIndex = uint16(parent.ConstantPoolManager.AddUtf8Info("anotherMethod"))
			case "duplicate target":
				parent.Methods = append(parent.Methods, target)
			case "private target":
				target.AccessFlags = 2
			case "static target":
				target.AccessFlags |= 8
			case "final target":
				target.AccessFlags |= 0x10
			case "synchronized target":
				target.AccessFlags |= 0x20
			case "native target":
				target.AccessFlags |= 0x100
			case "abstract target":
				target.AccessFlags |= 0x400
			case "changed throws":
				for _, a := range target.Attributes {
					if e, ok := a.(*ExceptionsAttribute); ok {
						e.ExceptionIndexTable = nil
					}
				}
			case "nil original":
				want.Methods = append(want.Methods, nil)
			case "nil target":
				parent.Methods = append(parent.Methods, nil)
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if variant == "nil original" || variant == "nil target" {
				if d.extraInheritedVisibilityBridge(fresh, bridge, parent) {
					t.Fatal("incomplete declaration inventory accepted")
				}
				return
			}
			d.foldSiblingResolver = func(name string) ([]byte, bool) {
				if variant == "missing parent" {
					return nil, false
				}
				return parent.Bytes(), true
			}
			resolved := d.inheritedVisibilityDomainParent()
			accepted := resolved != nil && d.extraInheritedVisibilityBridge(fresh, bridge, resolved)
			if accepted != (variant == "control") {
				t.Fatalf("original declaration proof accepted=%v", accepted)
			}
		})
	}
}

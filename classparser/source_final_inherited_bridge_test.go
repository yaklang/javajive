package javaclassparser

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func installAuthoredFinalBridge(t *testing.T, files map[string][]byte, owner string) {
	t.Helper()
	obj := mustParseLowering(t, bytes.Clone(files[owner+".class"]))
	found := 0
	for _, m := range obj.Methods {
		n, _ := sourceBridgeUTF8(obj, m.NameIndex)
		if n == "forwardForOriginal" {
			m.NameIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("value"))
			m.AccessFlags = 0x1041
			found++
		}
	}
	if found != 1 {
		t.Fatal("authored super-forwarder")
	}
	files[owner+".class"] = obj.Bytes()
}

func TestAdversarialFinalBridgeOriginalRequiresClosedFinalTarget(t *testing.T) {
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native compiler required")
	}
	files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, finalInheritedBridgeFixture, "FinalBridgeOwner", "none", false)
	installAuthoredFinalBridge(t, files, "FinalBridgeOwner")
	for _, variant := range []string{"control", "not bridge", "not synthetic", "virtual call", "wrong receiver", "wrong return", "nonfinal target", "static target", "private target", "missing target", "foreign target", "unbound signature", "missing interface", "duplicate declaration", "work", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj := mustParseLowering(t, bytes.Clone(files["FinalBridgeOwner.class"]))
			parent := mustParseLowering(t, bytes.Clone(files["FinalBase.class"]))
			var bridge, target *MemberInfo
			for _, m := range obj.Methods {
				if m.AccessFlags == 0x1041 {
					bridge = m
				}
			}
			for _, m := range parent.Methods {
				n, _ := sourceBridgeUTF8(parent, m.NameIndex)
				if n == "value" && m.AccessFlags == 0x11 {
					target = m
				}
			}
			if bridge == nil || target == nil {
				t.Fatal("fixture protocol")
			}
			code := constructorLoweringCode(bridge)
			var work *workbudget.Budget
			switch variant {
			case "not bridge":
				bridge.AccessFlags &^= 0x40
			case "not synthetic":
				bridge.AccessFlags &^= 0x1000
			case "virtual call":
				code.Code[1] = core.OP_INVOKEVIRTUAL
			case "wrong receiver":
				code.Code[0] = core.OP_ALOAD_1
			case "wrong return":
				code.Code[len(code.Code)-1] = core.OP_IRETURN
			case "nonfinal target":
				target.AccessFlags &^= 0x10
			case "static target":
				target.AccessFlags |= 8
			case "private target":
				target.AccessFlags = 0x12
			case "missing target":
				target.NameIndex = uint16(parent.ConstantPoolManager.AddUtf8Info("other"))
			case "foreign target":
				parent.ThisClass = uint16(parent.ConstantPoolManager.AddNewClassInfo("AnotherBase"))
			case "unbound signature":
				for _, attr := range target.Attributes {
					if sig, ok := attr.(*SignatureAttribute); ok {
						sig.SignatureIndex = uint16(parent.ConstantPoolManager.AddUtf8Info("()TUnbound;"))
					}
				}
			case "duplicate declaration":
				obj.Methods = append(obj.Methods, bridge)
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			d := NewClassObjectDumper(obj)
			d.Work = work
			d.foldSiblingResolver = func(name string) ([]byte, bool) {
				if name == "FinalBase" {
					return parent.Bytes(), true
				}
				if variant == "missing interface" && name == "FinalTyped" {
					return nil, false
				}
				b, k := files[name+".class"]
				return b, k
			}
			if got := d.finalInheritedBridgeOriginal(bridge); (got != nil) != (variant == "control") {
				t.Fatal("unproved original final forwarding", variant, got)
			}
		})
	}
}

func TestAdversarialFinalBridgeCompletionKeepsFreshCodeAndRejectsConsumers(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native compiler required")
	}
	testIndependentInheritedBridgeOwner(t, modern, java, native, finalInheritedBridgeFixture, "FinalBridgeOwner", "FinalBridgeOwner", "source,lines,vars", false, "6:final:inherited:bridge:identity:effects\n", func(files map[string][]byte) { installAuthoredFinalBridge(t, files, "FinalBridgeOwner") }, func(t *testing.T, owner string, original, fresh []byte, a *JarReconstructionArtifact) {
		base := a.source.jar.sourceConstructorLowering
		for _, variant := range []string{"control", "idempotent", "unpublished", "stale original", "wrong receiver", "virtual call", "wrong target", "helper visibility", "helper name", "helper descriptor", "method consumer", "handle consumer", "missing helper", "competing helper", "work", "canceled"} {
			t.Run(variant, func(t *testing.T) {
				authority := &sourceConstructorLoweringAuthority{jar: base.jar, bridgeActive: map[string]constructorNativeSourceOrigin{owner: base.bridgeActive[owner]}}
				obj := mustParseLowering(t, bytes.Clone(fresh))
				var helper, ctor *MemberInfo
				for _, m := range obj.Methods {
					n, _ := sourceBridgeUTF8(obj, m.NameIndex)
					if strings.HasPrefix(n, "jdec$super$") {
						helper = m
					}
					if n == "<init>" {
						ctor = m
					}
				}
				if helper == nil || ctor == nil {
					t.Fatal("fresh compiler helper absent")
				}
				name, _ := sourceBridgeUTF8(obj, helper.NameIndex)
				desc, _ := sourceBridgeUTF8(obj, helper.DescriptorIndex)
				code := constructorLoweringCode(helper)
				var work *workbudget.Budget
				switch variant {
				case "unpublished":
					authority.bridgeActive = nil
				case "stale original":
					r := authority.bridgeActive[owner]
					r.originalHash[0] ^= 1
					authority.bridgeActive[owner] = r
				case "wrong receiver":
					code.Code[0] = core.OP_ALOAD_1
				case "virtual call":
					code.Code[1] = core.OP_INVOKEVIRTUAL
				case "wrong target":
					ref := uint16(obj.ConstantPoolManager.AddNewMethodInfo("FinalBase", "other", "()LFinalBase;"))
					code.Code[2], code.Code[3] = byte(ref>>8), byte(ref)
				case "helper visibility":
					helper.AccessFlags = 1
				case "helper name":
					helper.NameIndex = uint16(obj.ConstantPoolManager.AddUtf8Info(name + "unknown"))
				case "helper descriptor":
					helper.DescriptorIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("()Ljava/lang/String;"))
				case "method consumer", "handle consumer":
					ref := uint16(obj.ConstantPoolManager.AddNewMethodInfo(owner, name, desc))
					if variant == "handle consumer" {
						obj.ConstantPool = append(obj.ConstantPool, &ConstantMethodHandleInfo{ReferenceKind: 7, ReferenceIndex: ref})
					} else {
						c := constructorLoweringCode(ctor)
						c.Code = append(c.Code[:len(c.Code)-1], core.OP_ALOAD_0, core.OP_INVOKESPECIAL, byte(ref>>8), byte(ref), core.OP_POP, core.OP_RETURN)
						c.MaxStack = 1
					}
				case "missing helper":
					var kept []*MemberInfo
					for _, m := range obj.Methods {
						if m != helper {
							kept = append(kept, m)
						}
					}
					obj.Methods = kept
				case "competing helper":
					copy := *helper
					copy.NameIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("value"))
					copy.AccessFlags = 0x1041
					obj.Methods = append(obj.Methods, &copy)
				case "work":
					work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
				case "canceled":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					work = workbudget.New(ctx, workbudget.Limits{})
				}
				input := obj.Bytes()
				if variant == "idempotent" {
					var e error
					input, e = authority.lowerFinalInheritedBridges(owner, original, input, nil)
					if e != nil {
						t.Fatal(e)
					}
				}
				saved := bytes.Clone(input)
				result, err := authority.lowerFinalInheritedBridges(owner, original, input, work)
				if !bytes.Equal(saved, input) {
					t.Fatal("input changed")
				}
				switch variant {
				case "control":
					if err != nil {
						t.Fatal(err)
					}
					after := mustParseLowering(t, result)
					found := 0
					for _, m := range after.Methods {
						n, _ := sourceBridgeUTF8(after, m.NameIndex)
						if n == "value" {
							found++
							if m.AccessFlags != 0x1041 || !bytes.Equal(constructorLoweringCode(m).Code, code.Code) {
								t.Fatal("fresh code or original flags changed")
							}
						}
					}
					if found != 1 {
						t.Fatal("original declaration not restored")
					}
				case "idempotent", "unpublished", "stale original":
					if err != nil || !bytes.Equal(result, input) {
						t.Fatal("unpublished or completed bytes changed", err)
					}
				default:
					if err == nil || result != nil {
						t.Fatal("unproved helper accepted", variant, err)
					}
				}
			})
		}
	})
}

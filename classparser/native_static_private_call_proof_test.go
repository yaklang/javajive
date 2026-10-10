package javaclassparser

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeStaticPrivateCallRequiresOriginalPacket(t *testing.T) {
	for _, fixture := range []struct{ source, owner, target string }{{nativeStaticPrivateCallFixture, "StaticCallOwner", "prepare"}, {nativeStaticPrivateFactoryFixture, "StaticFactoryOwner", "failure"}} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			files := nativeCompileDebugClasses(t, fixture.source, debug)
			for _, variant := range []string{"original", "not synthetic", "not static", "public target", "instance target", "native target", "abstract target", "wrong invocation kind", "wrong return", "wrong locals", "wrong stack", "extra effect", "handler", "generic target", "missing target", "target descriptor", "ordinal", "opaque bridge", "opaque code", "duplicate code", "checked contract", "budget", "canceled"} {
				t.Run(fixture.owner+"/"+debug+"/"+variant, func(t *testing.T) {
					obj, e := Parse(files[fixture.owner+".class"])
					if e != nil {
						t.Fatal(e)
					}
					var bridge, target *MemberInfo
					var code *CodeAttribute
					for _, m := range obj.Methods {
						n, _ := sourceBridgeUTF8(obj, m.NameIndex)
						d, _ := sourceBridgeUTF8(obj, m.DescriptorIndex)
						if strings.HasPrefix(n, "access$") {
							bridge = m
							for _, a := range m.Attributes {
								if c, ok := a.(*CodeAttribute); ok {
									code = c
								}
							}
						}
						if n == fixture.target && !strings.Contains(d, "Ljava/lang/String;") {
							target = m
						}
					}
					if bridge == nil || target == nil || code == nil {
						t.Fatal("original private static packet")
					}
					var work *workbudget.Budget
					switch variant {
					case "not synthetic":
						bridge.AccessFlags &^= 0x1000
					case "not static":
						bridge.AccessFlags &^= 8
					case "public target":
						target.AccessFlags = (target.AccessFlags &^ 2) | 1
					case "instance target":
						target.AccessFlags &^= 8
					case "native target":
						target.AccessFlags |= 0x100
					case "abstract target":
						target.AccessFlags |= 0x400
					case "wrong invocation kind":
						code.Code[len(code.Code)-4] = core.OP_INVOKEVIRTUAL
					case "wrong return":
						code.Code[len(code.Code)-1] = core.OP_IRETURN
					case "wrong locals":
						code.MaxLocals++
					case "wrong stack":
						code.MaxStack++
					case "extra effect":
						code.Code = append([]byte{core.OP_ACONST_NULL, core.OP_POP}, code.Code...)
					case "handler":
						code.ExceptionTable = append(code.ExceptionTable, &ExceptionTableEntry{})
					case "generic target":
						target.Attributes = append(target.Attributes, &SignatureAttribute{})
					case "missing target":
						target.NameIndex = sourceBridgePoolString(t, obj, "unrelatedFactory")
					case "target descriptor":
						target.DescriptorIndex = sourceBridgePoolString(t, obj, "()V")
					case "ordinal":
						bridge.NameIndex = sourceBridgePoolString(t, obj, "access$001")
					case "opaque bridge":
						bridge.Attributes = append(bridge.Attributes, &UnparsedAttribute{Name: "Opaque"})
					case "opaque code":
						code.Attributes = append(code.Attributes, &UnparsedAttribute{Name: "Opaque"})
					case "duplicate code":
						bridge.Attributes = append(bridge.Attributes, code)
					case "checked contract":
						found := false
						for _, a := range target.Attributes {
							if ex, ok := a.(*ExceptionsAttribute); ok {
								ex.ExceptionIndexTable = []uint16{obj.ThisClass}
								found = true
							}
						}
						if !found {
							target.Attributes = append(target.Attributes, &ExceptionsAttribute{ExceptionIndexTable: []uint16{obj.ThisClass}})
						}
					case "budget":
						work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
					case "canceled":
						ctx, cancel := context.WithCancel(context.Background())
						cancel()
						work = workbudget.New(ctx, workbudget.Limits{})
					}
					got := nativeMemberPrivateCallProof(obj, bridge, work)
					if (got != nil) != (variant == "original") {
						t.Fatalf("packet admitted=%v", got != nil)
					}
					if got != nil && (got.call == nil || !got.call.static || got.staticField || got.setter) {
						t.Fatal("wrong operation classification")
					}
				})
			}
		}
	}
}

func TestNativeStaticPrivateCallRefusesHiddenClassQualifier(t *testing.T) {
	ctx := &class_context.ClassContext{}
	ctx.TypeParams = []string{"Owner"}
	getter := &nativeMemberPrivateGetter{owner: "Owner", field: "factory", fieldDescriptor: "()Ljava/lang/Object;", call: &nativeMemberPrivateCall{static: true}}
	if source, ok := nativeMemberPrivateCallSource(getter, nil, ctx); ok || source != "" {
		t.Fatalf("unproved static method lexical binding: %s", source)
	}
}

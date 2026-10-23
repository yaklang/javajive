package javaclassparser

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialConstructorMarkerRequiresEquivalentTypedForwarding(t *testing.T) {
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	old := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, constructorMarkerFixture, "MarkerHost", "none", false)
	freshFiles := compileLambdaParameterMetadataFixture(t, modern, ModernJavac, constructorMarkerFixture, "MarkerHost", "none", false)
	const owner = "MarkerHost$Target.class"
	for _, variant := range []string{"control", "arbitrary marker name", "MR9", "MR10", "no-op padding", "receiver", "wide argument slot", "marker read", "extra effect", "different target", "handler", "stack", "locals", "nonprivate target", "different throws", "extra method", "missing constructor", "constructor access", "initializer marker", "nest target", "fresh nest", "unknown method attribute", "work", "inner work", "output", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			o := mustParseLowering(t, bytes.Clone(old[owner]))
			f := mustParseLowering(t, bytes.Clone(freshFiles[owner]))
			var bridge, target *MemberInfo
			for _, m := range f.Methods {
				n, _ := sourceBridgeUTF8(f, m.NameIndex)
				if n == "<init>" {
					if m.AccessFlags == 0x1000 {
						bridge = m
						m.AccessFlags = 0
					} else {
						target = m
					}
				}
			}
			if bridge == nil || target == nil {
				t.Fatal("independent compiler bridge required")
			}
			code := constructorLoweringCode(bridge)
			var work *workbudget.Budget
			switch variant {
			case "arbitrary marker name":
				for _, obj := range []*ClassObject{o, f} {
					for _, c := range obj.ConstantPool {
						if u, ok := c.(*ConstantUtf8Info); ok {
							u.Value = strings.ReplaceAll(u.Value, "MarkerHost$1", "UnrelatedPortType")
						}
					}
				}
			case "MR9":
				f.MajorVersion = 53
			case "MR10":
				f.MajorVersion = 54
			case "no-op padding":
				code.Code = append([]byte{byte(core.OP_NOP)}, code.Code...)
				code.AttrLen++
			case "receiver":
				code.Code[0] = byte(core.OP_ALOAD_1)
			case "wide argument slot":
				code.Code[1] = byte(core.OP_LLOAD_2)
			case "marker read":
				code.Code = append([]byte{byte(core.OP_ALOAD), 7, byte(core.OP_POP)}, code.Code...)
				code.AttrLen += 3
			case "extra effect":
				code.Code = append([]byte{byte(core.OP_ICONST_0), byte(core.OP_POP)}, code.Code...)
				code.AttrLen += 2
			case "different target":
				for _, c := range f.ConstantPool {
					if r, ok := c.(*ConstantMethodrefInfo); ok {
						nt, ok := f.ConstantPool[r.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
						if !ok {
							continue
						}
						n, _ := sourceBridgeUTF8(f, nt.NameIndex)
						if n == "<init>" && r.ClassIndex == f.ThisClass {
							r.ClassIndex = f.SuperClass
						}
					}
				}
			case "handler":
				code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 1, HandlerPc: 0}}
				code.AttrLen += 8
			case "stack":
				code.MaxStack = 0
			case "locals":
				code.MaxLocals = 1
			case "nonprivate target":
				target.AccessFlags = 0
				for _, m := range o.Methods {
					if m.AccessFlags == 2 {
						n, _ := sourceBridgeUTF8(o, m.NameIndex)
						if n == "<init>" {
							m.AccessFlags = 0
						}
					}
				}
			case "different throws":
				index := uint16(f.ConstantPoolManager.AddNewClassInfo("java/lang/RuntimeException"))
				for _, m := range []*MemberInfo{bridge, target} {
					for _, a := range m.Attributes {
						if a, ok := a.(*ExceptionsAttribute); ok {
							a.ExceptionIndexTable = []uint16{index}
						}
					}
				}
			case "extra method":
				f.Methods = append(f.Methods, target)
			case "missing constructor":
				var keep []*MemberInfo
				for _, m := range f.Methods {
					if m != target {
						keep = append(keep, m)
					}
				}
				f.Methods = keep
			case "constructor access":
				bridge.AccessFlags = 1
			case "initializer marker":
				for _, obj := range []*ClassObject{o, f} {
					for _, m := range obj.Methods {
						if m == bridge || obj == o && m.AccessFlags == 0x1000 {
							m.NameIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("<clinit>"))
						}
					}
				}
			case "nest target":
				o.MajorVersion, f.MajorVersion = 55, 55
			case "fresh nest":
				f.ConstantPoolManager.AddUtf8Info("NestMembers")
				f.Attributes = append(f.Attributes, &UnparsedAttribute{Name: "NestMembers", Length: 2, Info: []byte{0, 0}})
			case "unknown method attribute":
				f.ConstantPoolManager.AddUtf8Info("Opaque")
				bridge.Attributes = append(bridge.Attributes, &UnparsedAttribute{Name: "Opaque", Length: 1, Info: []byte{42}})
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "inner work":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: int64(len(old[owner]) + len(freshFiles[owner]) + 100)})
			case "output":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			original, compiled := o.Bytes(), f.Bytes()
			savedOriginal, savedCompiled := bytes.Clone(original), bytes.Clone(compiled)
			got, err := restoreReconstructionSyntheticFlags(original, compiled, work)
			positive := variant == "control" || variant == "arbitrary marker name" || variant == "MR9" || variant == "MR10" || variant == "no-op padding"
			if positive {
				if err != nil || bytes.Equal(got, compiled) {
					t.Fatal("independent forwarding packet refused", err)
				}
				if len(got) != len(compiled) {
					t.Fatal("flag edit changed layout")
				}
				diff := 0
				for i := range got {
					if got[i] != compiled[i] {
						diff++
						if got[i] != 0x10 || compiled[i] != 0 {
							t.Fatal("non-marker byte edited")
						}
					}
				}
				if diff != 1 {
					t.Fatal("expected one flag bit change", diff)
				}
				artifactLambdaMetadataCodeUnchanged(t, compiled, got)
				again, err := restoreReconstructionSyntheticFlags(original, got, nil)
				if err != nil || !bytes.Equal(again, got) {
					t.Fatal("idempotence", err)
				}
			} else if work != nil {
				var failure *workbudget.Error
				if got != nil || !errors.As(err, &failure) {
					t.Fatal("resource/cancellation returned a partial class", err)
				}
			} else if err != nil || !bytes.Equal(got, compiled) {
				t.Fatal("unproved packet was edited", err)
			}
			if !bytes.Equal(original, savedOriginal) || !bytes.Equal(compiled, savedCompiled) {
				t.Fatal("input mutation")
			}
		})
	}
}

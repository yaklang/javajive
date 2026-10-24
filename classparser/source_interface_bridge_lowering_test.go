package javaclassparser

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestLegacyInterfaceBridgeRequiresOriginalHierarchyAndPureFreshPacket(t *testing.T) {
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	const source = `interface GenericPort<T>{T value(long n,T value);} public interface ProtocolOwner extends GenericPort<String>{String value(long n,String value);}`
	originals := compileLambdaParameterMetadataFixtureWithSourcesAtRelease(t, native, NativeJavac8, source, "ProtocolOwner", "none", nil, 5, false)
	for _, release := range []int{8, 16} {
		compiled := compileLambdaParameterMetadataFixtureWithSourcesAtRelease(t, modern, ModernJavac, source, "ProtocolOwner", "none", nil, release, false)["ProtocolOwner.class"]
		for _, variant := range []string{"control", "unreturned", "stale original", "unknown ancestor", "ancestor default", "ancestor cycle", "receiver", "wide argument", "cast", "invoke kind", "invoke count", "return", "extra effect", "handler", "stack", "locals", "bridge flags", "unknown attribute", "extra ordinary method", "missing original method", "changed original flags", "abstract target code", "fresh preview", "fresh future", "method handle", "budget", "output", "canceled"} {
			t.Run(fmt.Sprintf("release%d/%s", release, variant), func(t *testing.T) {
				dependency := bytes.Clone(originals["GenericPort.class"])
				resolve := func(n string) ([]byte, bool) { return dependency, n == "GenericPort" && dependency != nil }
				path := filepath.Join(t.TempDir(), "original.jar")
				if err := os.WriteFile(path, t23Zip(t, map[string][]byte{"ProtocolOwner.class": originals["ProtocolOwner.class"]}), 0600); err != nil {
					t.Fatal(err)
				}
				artifact, err := NewJarReconstructionArtifactFromLocal(path, release, ModernJavac, resolve)
				if err != nil {
					t.Fatal(err)
				}
				defer artifact.Close()
				if variant != "unreturned" {
					if _, err := artifact.ReadFile("ProtocolOwner.class"); err != nil {
						t.Fatal(err)
					}
				}
				authority := artifact.source.jar.sourceConstructorLowering
				want := bytes.Clone(originals["ProtocolOwner.class"])
				object := mustParseLowering(t, bytes.Clone(compiled))
				var bridge, target *MemberInfo
				for _, m := range object.Methods {
					if m.AccessFlags == 0x1041 {
						bridge = m
					} else if m.AccessFlags == 0x0401 {
						target = m
					}
				}
				if bridge == nil || target == nil {
					t.Fatal("independently compiled bridge required")
				}
				code := constructorLoweringCode(bridge)
				size := len(code.Code)
				decoder := core.NewDecompiler(code.Code, nil)
				if err := decoder.ParseOpcode(); err != nil {
					t.Fatal(err)
				}
				ops := constructorMotionOps(decoder)
				cast, invoke := -1, -1
				for _, op := range ops {
					switch op.Instr.OpCode {
					case core.OP_CHECKCAST:
						cast = int(op.CurrentOffset)
					case core.OP_INVOKEINTERFACE:
						invoke = int(op.CurrentOffset)
					}
				}
				if cast < 0 || invoke < 0 {
					t.Fatal("typed cast/dispatch packet required")
				}
				var work *workbudget.Budget
				switch variant {
				case "stale original":
					want = append(want, 0)
				case "unknown ancestor":
					dependency = nil
				case "ancestor default", "ancestor cycle":
					parent := mustParseLowering(t, dependency)
					if variant == "ancestor default" {
						parent.MajorVersion = 52
						parent.Methods[0].AccessFlags = 1
					} else {
						parent.Interfaces = []uint16{parent.ThisClass}
					}
					dependency = parent.Bytes()
				case "receiver":
					code.Code[0] = core.OP_ALOAD_3
				case "wide argument":
					code.Code[int(ops[1].CurrentOffset)] = core.OP_LLOAD_2
				case "cast":
					code.Code[cast+1], code.Code[cast+2] = byte(object.ThisClass>>8), byte(object.ThisClass)
				case "invoke kind":
					code.Code[invoke] = core.OP_INVOKEVIRTUAL
				case "invoke count":
					code.Code[invoke+3]++
				case "return":
					code.Code[len(code.Code)-1] = core.OP_IRETURN
				case "extra effect":
					code.Code = append([]byte{core.OP_ACONST_NULL, core.OP_POP}, code.Code...)
				case "handler":
					code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 1, HandlerPc: 0}}
					code.AttrLen += 8
				case "stack":
					code.MaxStack = 1
				case "locals":
					code.MaxLocals = 1
				case "bridge flags":
					bridge.AccessFlags = 1
				case "unknown attribute":
					object.ConstantPoolManager.AddUtf8Info("OpaqueBridge")
					bridge.Attributes = append(bridge.Attributes, &UnparsedAttribute{Name: "OpaqueBridge"})
				case "extra ordinary method":
					copy := *bridge
					copy.NameIndex = uint16(object.ConstantPoolManager.AddUtf8Info("unrelated"))
					copy.AccessFlags = 1
					object.Methods = append(object.Methods, &copy)
				case "missing original method":
					object.Methods = []*MemberInfo{bridge}
				case "changed original flags":
					target.AccessFlags |= 8
				case "abstract target code":
					target.Attributes = append(target.Attributes, code)
				case "fresh preview":
					object.MinorVersion = 65535
				case "fresh future":
					object.MajorVersion = 66
				case "method handle":
					object.ConstantPool = append(object.ConstantPool, &ConstantNameAndTypeInfo{NameIndex: bridge.NameIndex, DescriptorIndex: bridge.DescriptorIndex})
					nt := uint16(len(object.ConstantPool))
					object.ConstantPool = append(object.ConstantPool, &ConstantInterfaceMethodrefInfo{ConstantMemberrefInfo: ConstantMemberrefInfo{ClassIndex: object.ThisClass, NameAndTypeIndex: nt}})
					object.ConstantPool = append(object.ConstantPool, &ConstantMethodHandleInfo{ReferenceKind: 7, ReferenceIndex: uint16(len(object.ConstantPool))})
				case "budget":
					work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
				case "output":
					work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
				case "canceled":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					work = workbudget.New(ctx, workbudget.Limits{})
				}
				code.AttrLen = uint32(int(code.AttrLen) + len(code.Code) - size)
				input := object.Bytes()
				saved := bytes.Clone(input)
				old := bytes.Clone(want)
				mustParseLowering(t, input)
				got, err := authority.lowerLegacyInterfaceBridges("ProtocolOwner", want, input, work)
				if variant == "control" {
					if err != nil || bytes.Equal(got, input) {
						t.Fatal("proved added bridge retained", err)
					}
					after := mustParseLowering(t, got)
					if len(after.Methods) != 1 || !reflect.DeepEqual(after.Methods[0], target) || !reflect.DeepEqual(after.ConstantPool, object.ConstantPool) || !reflect.DeepEqual(after.Fields, object.Fields) || !reflect.DeepEqual(after.Attributes, object.Attributes) {
						t.Fatal("surviving fresh bytes changed")
					}
					again, e := authority.lowerLegacyInterfaceBridges("ProtocolOwner", want, got, nil)
					if e != nil || !bytes.Equal(got, again) {
						t.Fatal("idempotence", e)
					}
				} else if work != nil {
					if err == nil || got != nil {
						t.Fatal("resource failure returned a candidate", err)
					}
				} else if err != nil && got != nil || err == nil && !bytes.Equal(got, input) {
					t.Fatal("unproved bridge removed", err)
				}
				if !bytes.Equal(input, saved) || !bytes.Equal(want, old) {
					t.Fatal("input mutation")
				}
			})
		}
	}
}

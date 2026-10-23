package javaclassparser

import (
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func enumArrayLoweringInputs(t *testing.T) ([]byte, []byte, *JarReconstructionArtifact) {
	t.Helper()
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, enumFreshHelperFixture, "PacketValues", "none", false)
	jar := filepath.Join(t.TempDir(), "enum.jar")
	if err := os.WriteFile(jar, t23Zip(t, map[string][]byte{"PacketValues.class": files["PacketValues.class"]}), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	source, err := a.ReadFile("PacketValues.class")
	if err != nil || legacySourceHasFailureMarker(string(source)) {
		t.Fatal("published enum source", err, string(source))
	}
	fresh := compileLambdaParameterMetadataFixture(t, modern, ModernJavac, string(source), "PacketValues", "none", false)
	return files["PacketValues.class"], fresh["PacketValues.class"], a
}

func TestAdversarialEnumArrayLoweringKeepsCompletePacketAndConsumerProof(t *testing.T) {
	original, compiled, artifact := enumArrayLoweringInputs(t)
	for _, variant := range []string{"control", "renamed helper", "expanded integer", "lost synthetic metadata", "new synthetic metadata", "lost synthetic plus synchronized", "unreturned source", "pending namespace", "stale original", "pre-enum version", "unqualified newer version", "preview version", "field universe", "extra method", "visibility", "helper flags", "array order", "array opcode", "extra instruction", "other consumer", "wrong call kind", "method handle", "handler", "frame", "unknown PC table", "max stack", "work", "output", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			object := mustParseLowering(t, bytes.Clone(compiled))
			var initializer, helper, fold *MemberInfo
			for _, method := range object.Methods {
				name, _ := sourceBridgeUTF8(object, method.NameIndex)
				if name == "<clinit>" {
					initializer = method
				} else if method.AccessFlags == 0x100a {
					helper = method
				} else if name == "fold" {
					fold = method
				}
			}
			if initializer == nil || helper == nil || fold == nil {
				t.Fatal("independent compiler helper control")
			}
			init, array := constructorLoweringCode(initializer), constructorLoweringCode(helper)
			ops, known := nativeEnumMethodOps(object, helper, nil)
			initOps, initKnown := nativeEnumMethodOps(object, initializer, nil)
			if !known || !initKnown || len(initOps) != 21 || len(ops) != 15 {
				t.Fatal("authored three-constant compiler packet")
			}
			call := int(initOps[18].CurrentOffset)
			ref, rk := nativeEnumCPIndex(initOps[18])
			if !rk || !nativeEnumOpcode(initOps[18], core.OP_INVOKESTATIC) {
				t.Fatal("independent unique array call")
			}
			var work *workbudget.Budget
			authority := artifact.source.jar.sourceConstructorLowering
			want := bytes.Clone(original)
			switch variant {
			case "lost synthetic metadata", "lost synthetic plus synchronized":
				old := mustParseLowering(t, want)
				for _, m := range old.Methods {
					name, _ := sourceBridgeUTF8(old, m.NameIndex)
					if name == "fold" {
						m.AccessFlags |= syntheticMethodFlag
					}
				}
				want = old.Bytes()
				origin := authority.enumActive["PacketValues"]
				origin.originalHash = sha256.Sum256(want)
				authority = &sourceConstructorLoweringAuthority{jar: authority.jar, enumActive: map[string]constructorNativeSourceOrigin{"PacketValues": origin}}
				if variant == "lost synthetic plus synchronized" {
					fold.AccessFlags |= 0x20
				}
			case "new synthetic metadata":
				fold.AccessFlags |= syntheticMethodFlag
			case "renamed helper":
				index := uint16(object.ConstantPoolManager.AddUtf8Info("compiler$array$packet"))
				helper.NameIndex = index
				member := nativeConstantMember(object.ConstantPool[ref-1])
				if member == nil {
					t.Fatal("authored helper reference")
				}
				object.ConstantPool[member.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo).NameIndex = index
			case "expanded integer":
				if array.Code[0] != 0x06 {
					t.Fatal("authored compact integer")
				}
				array.Code = append([]byte{0x10, 3}, array.Code[1:]...)
			case "unreturned source":
				authority = &sourceConstructorLoweringAuthority{}
			case "pending namespace":
				authority.mu.Lock()
				origin := authority.enumActive["PacketValues"]
				authority.mu.Unlock()
				authority = &sourceConstructorLoweringAuthority{jar: authority.jar, parent: authority, enumPending: map[string]constructorNativeSourceOrigin{"PacketValues": origin}}
			case "stale original":
				old := mustParseLowering(t, want)
				old.ConstantPoolManager.AddUtf8Info("unused but independently owned physical namespace")
				want = old.Bytes()
			case "pre-enum version", "unqualified newer version", "preview version":
				old := mustParseLowering(t, want)
				switch variant {
				case "pre-enum version":
					old.MajorVersion = 48
				case "unqualified newer version":
					old.MajorVersion = 53
				case "preview version":
					old.MinorVersion = 65535
				}
				want = old.Bytes()
				// Reach the version proof, not merely stale publication refusal.
				origin := authority.enumActive["PacketValues"]
				origin.originalHash = sha256.Sum256(want)
				authority = &sourceConstructorLoweringAuthority{jar: authority.jar, enumActive: map[string]constructorNativeSourceOrigin{"PacketValues": origin}}
			case "field universe":
				copy := *object.Fields[0]
				copy.NameIndex = uint16(object.ConstantPoolManager.AddUtf8Info("additional"))
				object.Fields = append(object.Fields, &copy)
			case "extra method":
				copy := *fold
				copy.NameIndex = uint16(object.ConstantPoolManager.AddUtf8Info("additional"))
				object.Methods = append(object.Methods, &copy)
			case "visibility":
				fold.AccessFlags = 0x000a
			case "helper flags":
				helper.AccessFlags = 0x000a
			case "array order":
				first, last := int(ops[4].CurrentOffset), int(ops[12].CurrentOffset)
				temp := bytes.Clone(array.Code[first : first+3])
				copy(array.Code[first:first+3], array.Code[last:last+3])
				copy(array.Code[last:last+3], temp)
			case "array opcode":
				array.Code[int(ops[4].CurrentOffset)] = 0xb4 // getfield is not getstatic
			case "extra instruction":
				array.Code = append(array.Code[:len(array.Code)-1], 0, 0xb0)
			case "other consumer":
				body := constructorLoweringCode(fold)
				body.Code = []byte{0xb8, byte(ref >> 8), byte(ref), 0x57, 0x1e, 0xad}
				body.MaxStack, body.MaxLocals, body.Attributes = 2, 2, nil
				body.AttrLen = uint32(12 + len(body.Code))
			case "wrong call kind":
				init.Code[call] = 0xb6
			case "method handle":
				object.ConstantPool = append(object.ConstantPool, &ConstantMethodHandleInfo{ReferenceKind: 6, ReferenceIndex: ref})
			case "handler":
				array.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 4, HandlerPc: 4}}
			case "frame":
				array.Attributes = append(array.Attributes, &UnparsedAttribute{Name: "StackMapTable", Length: 3, Info: []byte{0, 1, 0}})
			case "unknown PC table":
				array.Attributes = append(array.Attributes, &UnparsedAttribute{Name: "RuntimeVisibleTypeAnnotations", Length: 2, Info: []byte{0, 0}})
			case "max stack":
				array.MaxStack = 0
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "output":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			// Keep negative classes structurally parseable. A changed handler,
			// instruction or PC table must reach its actual proof obligation.
			for _, code := range []*CodeAttribute{init, array} {
				code.AttrLen = uint32(12 + len(code.Code) + 8*len(code.ExceptionTable))
				for _, attribute := range code.Attributes {
					switch x := attribute.(type) {
					case *UnparsedAttribute:
						object.ConstantPoolManager.AddUtf8Info(x.Name)
						code.AttrLen += 6 + x.Length
					case *LineNumberTableAttribute:
						code.AttrLen += 6 + x.AttrLen
					default:
						t.Fatal("unexpected authored Code attribute")
					}
				}
			}
			input := object.Bytes()
			if _, err := Parse(input); err != nil {
				t.Fatal("negative packet did not reach semantic validation", err)
			}
			inputBefore, wantBefore := bytes.Clone(input), bytes.Clone(want)
			got, err := authority.lowerEnumArrayHelper("PacketValues", want, input, work)
			if variant == "lost synthetic metadata" {
				if err != nil || bytes.Equal(got, input) {
					t.Fatal("array proof must precede independent synthetic metadata", err)
				}
				if sourceMethodMetadataClosed(mustParseLowering(t, want), got, nil) == nil {
					t.Fatal("array completion waived the remaining synthetic obligation")
				}
				finished, e := restoreReconstructionSyntheticFlags(want, got, nil)
				if e != nil {
					t.Fatal("synthetic metadata", e)
				}
				finished, e = restoreMethodParameterMetadata(want, finished, nil, nil)
				if e != nil {
					t.Fatal("remaining parameter metadata", e)
				}
				if e = sourceMethodMetadataClosed(mustParseLowering(t, want), finished, nil); e != nil {
					t.Fatal("composed metadata did not close the original method universe", e)
				}
			} else if variant == "control" || variant == "renamed helper" || variant == "expanded integer" {
				if err != nil || bytes.Equal(got, input) {
					t.Fatal("positive fresh packet edit", err)
				}
				if finished, err := artifact.FinishClass("PacketValues.class", input); err != nil || finished == nil {
					t.Fatal("positive complete original universe and remaining parameter/metadata obligations", err)
				}
				if variant == "expanded integer" {
					result := mustParseLowering(t, got)
					found := false
					for _, method := range result.Methods {
						name, _ := sourceBridgeUTF8(result, method.NameIndex)
						if name == "<clinit>" {
							body := constructorLoweringCode(method).Code
							found = body[call] == 0x10 && body[call+1] == 3
						}
					}
					if !found {
						t.Fatal("fresh integer encoding replaced by original bytes")
					}
				}
			} else if err == nil {
				if !bytes.Equal(got, input) || sourceMethodMetadataClosed(mustParseLowering(t, original), got, nil) == nil {
					t.Fatal("unproved helper packet was partially repaired or its strict gate waived")
				}
			} else if got != nil {
				t.Fatal("failure exposed a partially rewritten class", err)
			}
			if variant == "work" || variant == "output" || variant == "canceled" {
				if err == nil || got != nil {
					t.Fatal("resource/cancellation boundary returned a result", err)
				}
			}
			if !bytes.Equal(input, inputBefore) || !bytes.Equal(want, wantBefore) {
				t.Fatal("input mutated on completion/refusal")
			}
		})
	}
}

package javaclassparser

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialEnumEntryHandlerCompletionRequiresClosedRegions(t *testing.T) {
	for _, final := range []bool{false, true} {
		name := "direct stores"
		if final {
			name = "joined final stores"
		}
		t.Run(name, func(t *testing.T) { enumEntryHandlerCompletionPremises(t, final) })
	}
}

func enumEntryHandlerCompletionPremises(t *testing.T, final bool) {
	t.Helper()
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	files := compileLambdaParameterMetadataFixtureWithSourcesAtRelease(t, native, NativeJavac8, enumEntryHandlersFixture, "PreludeChoice", "none", nil, 5, false)
	original := enumEntryHandlersOriginal(t, files["PreludeChoice.class"])
	if final {
		obj := mustParseLowering(t, original)
		for _, f := range obj.Fields {
			n, _ := sourceBridgeUTF8(obj, f.NameIndex)
			if n == "mode" {
				f.AccessFlags |= 0x10
			}
		}
		original = obj.Bytes()
	}
	jar := filepath.Join(t.TempDir(), "original.jar")
	if err := os.WriteFile(jar, t23Zip(t, map[string][]byte{"PreludeChoice.class": original}), 0600); err != nil {
		t.Fatal(err)
	}
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			a, err := NewJarReconstructionArtifactFromLocal(jar, 8, profile, func(name string) ([]byte, bool) { b, k := files[name+".class"]; return b, k })
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			source, err := a.ReadFile("PreludeChoice.class")
			if err != nil {
				t.Fatal(err)
			}
			compiler := modern
			if profile == NativeJavac8 {
				compiler = native
			}
			probe := enumEntryHandlersFixture[:strings.Index(enumEntryHandlersFixture, "public enum")]
			fresh := compileLambdaParameterMetadataFixture(t, compiler, profile, probe+string(source), "PreludeChoice", "none", false)["PreludeChoice.class"]
			for _, variant := range []string{"control", "catch type", "catch order", "handler protects allocation", "handler targets allocation", "handler value read", "backedge", "call changed", "handler result changed", "local capacity", "missing frame", "frame stack changed", "opaque attribute", "work", "canceled"} {
				t.Run(variant, func(t *testing.T) {
					obj := mustParseLowering(t, bytes.Clone(fresh))
					var init *MemberInfo
					for _, m := range obj.Methods {
						n, _ := sourceBridgeUTF8(obj, m.NameIndex)
						if n == "<clinit>" {
							init = m
						}
					}
					code := constructorLoweringCode(init)
					ops, known := enumEntryInitializerOps(obj, init, nil)
					if !known || len(code.ExceptionTable) != 2 {
						t.Fatal("fresh exception region")
					}
					var work *workbudget.Budget
					switch variant {
					case "catch type":
						code.ExceptionTable[0].CatchType = uint16(obj.ConstantPoolManager.AddNewClassInfo("java/lang/RuntimeException"))
					case "catch order":
						code.ExceptionTable[0], code.ExceptionTable[1] = code.ExceptionTable[1], code.ExceptionTable[0]
					case "handler protects allocation":
						code.ExceptionTable[0].StartPc = 0
					case "handler targets allocation":
						code.ExceptionTable[0].HandlerPc = 0
					case "handler value read":
						code.Code[code.ExceptionTable[0].HandlerPc] = core.OP_ALOAD_0
					case "backedge":
						for _, op := range ops {
							if op.Instr.OpCode == core.OP_GOTO {
								binary.BigEndian.PutUint16(code.Code[int(op.CurrentOffset)+1:], uint16(int16(-int(op.CurrentOffset))))
								break
							}
						}
					case "handler result changed":
						found := false
						for _, op := range ops {
							if op.Instr.OpCode == core.OP_BIPUSH && len(op.Data) == 1 && op.Data[0] == 11 {
								code.Code[int(op.CurrentOffset)+1] = 12
								found = true
							}
						}
						if !found {
							t.Fatal("missing handler scalar")
						}
					case "local capacity":
						code.MaxLocals = 0
					case "call changed":
						for _, op := range ops {
							member := constructorMotionMember(obj, op, core.OP_INVOKESTATIC)
							if member != nil && member.Name == "PreludeProbe" && member.Member == "choose" {
								index := obj.ConstantPoolManager.AddNewMethodInfo("PreludeChoice", "read", "()I")
								binary.BigEndian.PutUint16(code.Code[int(op.CurrentOffset)+1:], uint16(index))
							}
						}
					case "missing frame", "frame stack changed":
						found := false
						for _, attr := range code.Attributes {
							if table, ok := attr.(*UnparsedAttribute); ok && table.Name == "StackMapTable" {
								found = true
								if variant == "missing frame" {
									code.AttrLen -= table.Length - 2
									table.Info = []byte{0, 0}
									table.Length = 2
								} else {
									enumHandlerAlterFirstStackType(t, obj, table.Info)
								}
							}
						}
						if !found {
							t.Fatal("missing fresh verifier frames")
						}
					case "opaque attribute":
						obj.ConstantPoolManager.AddUtf8Info("OpaquePCMap")
						code.Attributes = append(code.Attributes, &UnparsedAttribute{Name: "OpaquePCMap", Length: 2, Info: []byte{0, 0}})
						code.AttrLen += 8
					case "work":
						work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
					case "canceled":
						ctx, cancel := context.WithCancel(context.Background())
						cancel()
						work = workbudget.New(ctx, workbudget.Limits{})
					}
					input := obj.Bytes()
					saved := bytes.Clone(input)
					out, err := a.source.jar.sourceConstructorLowering.lowerEnumArrayHelper("PreludeChoice", original, input, work)
					if !bytes.Equal(input, saved) {
						t.Fatal("input mutated")
					}
					if variant != "control" {
						if err == nil || out != nil {
							t.Fatal("unproved handler motion", variant, err)
						}
						return
					}
					if err != nil || bytes.Equal(out, input) {
						t.Fatal("closed region not completed", err)
					}
					again, err := a.source.jar.sourceConstructorLowering.lowerEnumArrayHelper("PreludeChoice", original, out, nil)
					if err != nil || !bytes.Equal(again, out) {
						t.Fatal("region motion is not idempotent", err)
					}
					done, err := a.FinishClass("PreludeChoice.class", out)
					if err != nil {
						t.Fatal("complete metadata", err)
					}
					again, err = a.FinishClass("PreludeChoice.class", done)
					if err != nil || !bytes.Equal(again, done) {
						t.Fatal("complete artifact is not stable", err)
					}
				})
			}
		})
	}
}

// Mutate one actual stack verification type, independent of javac choosing a
// compact or full frame for the branch-local temporary. Keep every frame PC,
// local row, frame count and attribute length unchanged.
func enumHandlerAlterFirstStackType(t *testing.T, obj *ClassObject, raw []byte) {
	t.Helper()
	if len(raw) < 3 {
		t.Fatal("missing frames")
	}
	at := 2
	u2 := func() int {
		if at+2 > len(raw) {
			t.Fatal("truncated frame")
		}
		v := int(binary.BigEndian.Uint16(raw[at:]))
		at += 2
		return v
	}
	verification := func() int {
		if at >= len(raw) {
			t.Fatal("missing verification type")
		}
		start := at
		tag := raw[at]
		at++
		if tag == 7 || tag == 8 {
			_ = u2()
		}
		if tag > 8 {
			t.Fatal("invalid verification tag")
		}
		return start
	}
	for n := 0; n < int(binary.BigEndian.Uint16(raw)); n++ {
		if at >= len(raw) {
			t.Fatal("missing frame")
		}
		tag := int(raw[at])
		at++
		stack := 0
		switch {
		case tag <= 63:
		case tag <= 127:
			stack = 1
		case tag == 247:
			_ = u2()
			stack = 1
		case tag >= 248 && tag <= 251:
			_ = u2()
		case tag >= 252 && tag <= 254:
			_ = u2()
			for i := 0; i < tag-251; i++ {
				verification()
			}
		case tag == 255:
			_ = u2()
			locals := u2()
			for i := 0; i < locals; i++ {
				verification()
			}
			stack = u2()
		default:
			t.Fatal("invalid frame")
		}
		for i := 0; i < stack; i++ {
			start := verification()
			if raw[start] == 7 {
				index := obj.ConstantPoolManager.AddNewClassInfo("java/lang/String")
				binary.BigEndian.PutUint16(raw[start+1:], uint16(index))
				return
			}
		}
	}
	t.Fatal("missing reference stack entry")
}

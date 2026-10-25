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

func TestAdversarialEnumEntryEffectCompletionRequiresExactFreshEffectAndCuts(t *testing.T) {
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("original native compiler required")
	}
	files := compileLambdaParameterMetadataFixtureWithSourcesAtRelease(t, native, NativeJavac8, enumEntryEffectFixture, "PreludeChoice", "none", nil, 5, false)
	original := enumEntryEffectOriginal(t, files["PreludeChoice.class"])
	jar := filepath.Join(t.TempDir(), "input.jar")
	if err := os.WriteFile(jar, t23Zip(t, map[string][]byte{"PreludeChoice.class": original}), 0600); err != nil {
		t.Fatal(err)
	}
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			a, err := NewJarReconstructionArtifactFromLocal(jar, 8, profile, func(name string) ([]byte, bool) { b, ok := files[name+".class"]; return b, ok })
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
			probe := enumEntryEffectFixture[:strings.Index(enumEntryEffectFixture, "public enum")]
			fresh := compileLambdaParameterMetadataFixture(t, compiler, profile, probe+string(source), "PreludeChoice", "none", false)["PreludeChoice.class"]
			for _, variant := range []string{"control", "different call", "different call descriptor", "changed effect result", "missing call", "live locals", "handler", "frame in moved region", "unknown PC attribute", "work budget", "canceled"} {
				t.Run(variant, func(t *testing.T) {
					obj := mustParseLowering(t, bytes.Clone(fresh))
					var method *MemberInfo
					for _, m := range obj.Methods {
						name, _ := sourceBridgeUTF8(obj, m.NameIndex)
						if name == "<clinit>" {
							method = m
						}
					}
					code := constructorLoweringCode(method)
					ops, known := nativeEnumMethodOps(obj, method, nil)
					if !known || code == nil {
						t.Fatal("fresh initializer")
					}
					call := -1
					for _, op := range ops {
						if member := constructorMotionMember(obj, op, core.OP_INVOKESTATIC); member != nil && member.Name == "PreludeProbe" && member.Member == "choose" {
							call = int(op.CurrentOffset)
						}
					}
					if call < 0 {
						t.Fatal("fresh effect invocation")
					}
					var work *workbudget.Budget
					switch variant {
					case "different call":
						index := obj.ConstantPoolManager.AddNewMethodInfo("PreludeChoice", "read", "()I")
						binary.BigEndian.PutUint16(code.Code[call+1:], uint16(index))
					case "different call descriptor":
						index := obj.ConstantPoolManager.AddNewMethodInfo("PreludeProbe", "choose", "()J")
						binary.BigEndian.PutUint16(code.Code[call+1:], uint16(index))
					case "changed effect result":
						code.Code = append(append(bytes.Clone(code.Code[:call+3]), byte(core.OP_INEG)), code.Code[call+3:]...)
						code.AttrLen++
					case "missing call":
						copy(code.Code[call:call+3], []byte{core.OP_ICONST_0, core.OP_NOP, core.OP_NOP})
					case "live locals":
						code.MaxLocals = 1
					case "handler":
						code.ExceptionTable = []*ExceptionTableEntry{{StartPc: uint16(call), EndPc: uint16(call + 3), HandlerPc: uint16(call)}}
						code.AttrLen += 8
					case "frame in moved region":
						obj.ConstantPoolManager.AddUtf8Info("StackMapTable")
						code.Attributes = append(code.Attributes, &UnparsedAttribute{Name: "StackMapTable", Length: 3, Info: []byte{0, 1, 0}})
						code.AttrLen += 9
					case "unknown PC attribute":
						obj.ConstantPoolManager.AddUtf8Info("UnknownPCMap")
						code.Attributes = append(code.Attributes, &UnparsedAttribute{Name: "UnknownPCMap", Length: 2, Info: []byte{0, 0}})
						code.AttrLen += 8
					case "work budget":
						work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
					case "canceled":
						ctx, cancel := context.WithCancel(context.Background())
						cancel()
						work = workbudget.New(ctx, workbudget.Limits{})
					}
					input := obj.Bytes()
					if _, err := Parse(input); err != nil {
						t.Fatal("negative must reach semantic proof", err)
					}
					saved := bytes.Clone(input)
					out, err := a.source.jar.sourceConstructorLowering.lowerEnumArrayHelper("PreludeChoice", original, input, work)
					if !bytes.Equal(input, saved) {
						t.Fatal("input mutated")
					}
					if variant != "control" {
						if err == nil || out != nil {
							t.Fatal("unproved entry order accepted", err)
						}
						return
					}
					if err != nil || bytes.Equal(input, out) {
						t.Fatal("proved ordering was not completed", err)
					}
					again, err := a.source.jar.sourceConstructorLowering.lowerEnumArrayHelper("PreludeChoice", original, out, nil)
					if err != nil || !bytes.Equal(again, out) {
						t.Fatal("ordering lowering is not idempotent", err)
					}
					completed, err := a.FinishClass("PreludeChoice.class", out)
					if err != nil {
						t.Fatal("metadata completion", err)
					}
					again, err = a.FinishClass("PreludeChoice.class", completed)
					if err != nil || !bytes.Equal(again, completed) {
						t.Fatal("complete artifact is not idempotent", err)
					}
				})
			}
		})
	}
}

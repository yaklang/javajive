package javaclassparser

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialEnumEntryPathRejectsCyclesAndUnvisitedEffects(t *testing.T) {
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, enumFreshHelperFixture, "PacketValues", "none", false)
	original := enumEntryPathOriginal(t, files["PacketValues.class"], false, 3)
	for _, variant := range []string{"control", "entry cycle", "multi-edge cycle", "operand target", "unvisited tail", "skip initializations", "conditional edge", "protected body", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj := mustParseLowering(t, bytes.Clone(original))
			var method *MemberInfo
			for _, m := range obj.Methods {
				name, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if name == "<clinit>" {
					method = m
				}
			}
			code := constructorLoweringCode(method)
			if code == nil {
				t.Fatal("original initializer")
			}
			tail := len(code.Code) - 9
			var work *workbudget.Budget
			setTarget := func(pc, target int) { binary.BigEndian.PutUint16(code.Code[pc+1:], uint16(int16(target-pc))) }
			switch variant {
			case "entry cycle":
				setTarget(0, 0)
			case "multi-edge cycle":
				setTarget(tail+6, 0)
			case "operand target":
				setTarget(0, 1)
			case "unvisited tail":
				setTarget(0, 3)
			case "skip initializations":
				setTarget(tail+6, tail-1)
			case "conditional edge":
				code.Code[0] = 0x99
			case "protected body":
				code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 3, EndPc: uint16(tail - 1), HandlerPc: uint16(tail - 1)}}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			decoder := core.NewDecompiler(code.Code, nil)
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal("negative instruction encoding", err)
			}
			ops := constructorMotionOps(decoder)
			saved := bytes.Clone(code.Code)
			path, ok := enumArrayOriginalEntryPath(code, ops, work)
			if ok != (variant == "control") {
				t.Fatal("unproved path accepted", variant, ok)
			}
			if ok && (len(path) != len(ops)-4 || int(path[0].CurrentOffset) != 3 || int(path[len(path)-1].CurrentOffset) != tail-1) {
				t.Fatal("lost original instruction identity")
			}
			if !bytes.Equal(saved, code.Code) {
				t.Fatal("original executable bytes changed")
			}
		})
	}
}

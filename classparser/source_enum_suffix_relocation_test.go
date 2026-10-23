package javaclassparser

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialEnumSuffixFramesKeepTypedAllocationOffsets(t *testing.T) {
	original, _, _ := enumArrayLoweringInputs(t)
	object := mustParseLowering(t, original)
	class := object.ThisClass
	objectType := []byte{7, byte(class >> 8), byte(class)}
	for _, row := range []struct {
		name            string
		input, expected []byte
	}{
		{"same compact", []byte{0, 1, 63}, []byte{0, 1, 251, 0, 67}},
		{"same extended", []byte{0, 1, 251, 0, 63}, []byte{0, 1, 251, 0, 67}},
		{"stack compact", []byte{0, 1, 127, 1}, []byte{0, 1, 247, 0, 67, 1}},
		{"stack extended", []byte{0, 1, 247, 0, 63, 1}, []byte{0, 1, 247, 0, 67, 1}},
		{"append", append([]byte{0, 1, 252, 0, 63}, objectType...), append([]byte{0, 1, 252, 0, 67}, objectType...)},
		{"chop", []byte{0, 2, 252, 0, 63, 1, 250, 0, 8}, []byte{0, 2, 252, 0, 67, 1, 250, 0, 8}},
		{"full allocation", []byte{0, 1, 255, 0, 63, 0, 1, 4, 0, 2, 8, 0, 40, 1}, []byte{0, 1, 255, 0, 67, 0, 1, 4, 0, 2, 8, 0, 44, 1}},
		{"later delta unchanged", []byte{0, 2, 63, 8}, []byte{0, 2, 251, 0, 67, 251, 0, 8}},
		{"unknown tag", []byte{0, 1, 200}, nil},
		{"missing offset", []byte{0, 1, 251, 0}, nil},
		{"prefix frame", []byte{0, 1, 0}, nil},
		{"nonboundary frame", []byte{0, 1, 62}, nil},
		{"not NEW", []byte{0, 1, 127, 8, 0, 63}, nil},
		{"prefix NEW", []byte{0, 1, 127, 8, 0, 5}, nil},
		{"missing object", []byte{0, 1, 127, 7, 0, 0}, nil},
		{"static uninitialized this", []byte{0, 1, 127, 6}, nil},
		{"unknown verification", []byte{0, 1, 127, 9}, nil},
		{"extra bytes", []byte{0, 0, 0}, nil},
	} {
		t.Run(row.name, func(t *testing.T) {
			positions := map[int]int{0: core.OP_NEW, 5: core.OP_NEW, 40: core.OP_NEW, 63: core.OP_GOTO, 72: core.OP_RETURN}
			saved := bytes.Clone(row.input)
			got, ok := enumArrayRelocateFrames(object, row.input, positions, 10, 4, nil)
			if ok != (row.expected != nil) || ok && !bytes.Equal(got, row.expected) || !bytes.Equal(saved, row.input) {
				t.Fatal("typed PC map", ok, got, row.expected)
			}
		})
	}
	// Frame offsets are byte offsets, not four-byte-aligned addresses. An
	// ordinary branch suffix can move by three while its relative edges remain
	// exact. Only switch operand alignment requires a multiple of four.
	if got, ok := enumArrayRelocateFrames(object, []byte{0, 1, 63}, map[int]int{63: core.OP_GOTO}, 10, 3, nil); !ok || !bytes.Equal(got, []byte{0, 1, 251, 0, 66}) {
		t.Fatal("ordinary suffix frame acquired a switch-only alignment restriction", got, ok)
	}
	for _, kind := range []string{"canceled", "budget", "unaligned", "overflow"} {
		t.Run(kind, func(t *testing.T) {
			var work *workbudget.Budget
			delta := 4
			raw := []byte{0, 1, 251, 0, 63}
			positions := map[int]int{63: core.OP_RETURN}
			switch kind {
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "unaligned":
				delta = 3
				positions[63] = core.OP_LOOKUPSWITCH
			case "overflow":
				raw = []byte{0, 1, 251, 255, 254}
				positions = map[int]int{65534: core.OP_RETURN}
			}
			if _, ok := enumArrayRelocateFrames(object, raw, positions, 10, delta, work); ok {
				t.Fatal("resource/encoding boundary admitted")
			}
		})
	}
}

func TestAdversarialEnumSuffixRejectsEdgesIntoRegeneratedPrefix(t *testing.T) {
	compiler, _ := t04Tools(t)
	files := compileLambdaParameterMetadataFixture(t, compiler, ModernJavac, enumInitializerTailFixture, "TailEnum", "none", false)
	for _, kind := range []string{"control", "backedge prefix", "nonboundary", "switch default prefix", "switch case prefix", "handler prefix"} {
		t.Run(kind, func(t *testing.T) {
			o := mustParseLowering(t, bytes.Clone(files["TailEnum.class"]))
			var init *MemberInfo
			for _, m := range o.Methods {
				n, _ := sourceBridgeUTF8(o, m.NameIndex)
				if n == "<clinit>" {
					init = m
				}
			}
			ops, ok := nativeEnumMethodOps(o, init, nil)
			if !ok {
				t.Fatal("actual compiler initializer")
			}
			code := constructorLoweringCode(init)
			start := 0
			changed := false
			for i, op := range ops {
				if m := constructorMotionMember(o, op, core.OP_INVOKESTATIC); m != nil && m.Name == "TailEnum" && m.Description == "()[LTailEnum;" {
					start = int(ops[i+2].CurrentOffset)
					break
				}
			}
			if start == 0 {
				t.Fatal("actual helper boundary")
			}
			for _, op := range ops {
				pc := int(op.CurrentOffset)
				if kind == "backedge prefix" || kind == "nonboundary" {
					if !constructorMotionPacketBranch(op.Instr.OpCode) || len(op.Data) != 2 {
						continue
					}
					target := 0
					if kind == "nonboundary" {
						target = pc + 1
					}
					binary.BigEndian.PutUint16(op.Data, uint16(int16(target-pc)))
					changed = true
					break
				}
				if kind == "switch default prefix" || kind == "switch case prefix" {
					if op.Instr.OpCode != core.OP_LOOKUPSWITCH {
						continue
					}
					at := pc + 1 + (4-(pc+1)%4)%4
					if kind == "switch case prefix" {
						at += 12
					}
					binary.BigEndian.PutUint32(code.Code[at:], uint32(int32(-pc)))
					changed = true
					break
				}
			}
			if kind == "handler prefix" {
				code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 3, HandlerPc: uint16(start)}}
				changed = true
			}
			if kind != "control" && !changed {
				t.Fatal("mutation did not reach target")
			}
			if enumArraySuffixClosed(code, ops, start) != (kind == "control") {
				t.Fatal("control flow isolation", kind)
			}
		})
	}
}

package frametransfer

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

// Each row is a JVMS 6.5 stack diagram, expressed from bottom to top.
// The oracle substitutes value identities into that diagram; it never pops
// the implementation's expanded slots or calls its duplication helpers.
// https://docs.oracle.com/javase/specs/jvms/se21/html/jvms-6.html
type boundedStackForm struct {
	widths []int
	order  []int
}

var boundedStackForms = map[int][]boundedStackForm{
	core.OP_DUP:     {{[]int{1}, []int{0, 0}}},
	core.OP_DUP_X1:  {{[]int{1, 1}, []int{1, 0, 1}}},
	core.OP_DUP_X2:  {{[]int{1, 1, 1}, []int{2, 0, 1, 2}}, {[]int{2, 1}, []int{1, 0, 1}}},
	core.OP_DUP2:    {{[]int{1, 1}, []int{0, 1, 0, 1}}, {[]int{2}, []int{0, 0}}},
	core.OP_DUP2_X1: {{[]int{1, 1, 1}, []int{1, 2, 0, 1, 2}}, {[]int{1, 2}, []int{1, 0, 1}}},
	core.OP_DUP2_X2: {
		{[]int{1, 1, 1, 1}, []int{2, 3, 0, 1, 2, 3}},
		{[]int{1, 1, 2}, []int{2, 0, 1, 2}},
		{[]int{2, 1, 1}, []int{1, 2, 0, 1, 2}},
		{[]int{2, 2}, []int{1, 0, 1}},
	},
	core.OP_SWAP: {{[]int{1, 1}, []int{1, 0}}},
	core.OP_POP:  {{[]int{1}, nil}},
	core.OP_POP2: {{[]int{1, 1}, nil}, {[]int{2}, nil}},
}

var boundedStackOps = []int{
	core.OP_DUP, core.OP_DUP_X1, core.OP_DUP_X2, core.OP_DUP2,
	core.OP_DUP2_X1, core.OP_DUP2_X2, core.OP_SWAP, core.OP_POP, core.OP_POP2,
}

func boundedStackSlots(logical []Type) []Type {
	var slots []Type
	for _, value := range logical {
		slots = append(slots, value)
		switch value.Kind {
		case Long:
			slots = append(slots, Type{Kind: LongTail})
		case Double:
			slots = append(slots, Type{Kind: DoubleTail})
		}
	}
	return slots
}

func boundedStackDiagram(logical []Type, op int) ([]Type, bool) {
	for _, form := range boundedStackForms[op] {
		start := len(logical) - len(form.widths)
		if start < 0 {
			continue
		}
		matches := true
		for i, width := range form.widths {
			actual := 1
			if logical[start+i].Kind == Long || logical[start+i].Kind == Double {
				actual = 2
			}
			matches = matches && actual == width
		}
		if matches {
			out := append([]Type(nil), logical[:start]...)
			for _, index := range form.order {
				out = append(out, logical[start+index])
			}
			return out, true
		}
	}
	return nil, false
}

func boundedStackValue(symbol, position int) Type {
	switch symbol {
	case 0:
		return Type{Kind: Int, HasInt: true, Int: -2147483648 + int32(position)}
	case 1:
		// Distinguish signed zero from NaN payloads without float equality.
		bits := uint64(0x80000000)
		if position%2 != 0 {
			bits = 0x7fc00001 + uint64(position)
		}
		return Type{Kind: Float, HasBits: true, Bits: bits}
	case 2:
		return Type{Kind: Long, HasLong: true, Long: -9223372036854775808 + int64(position)}
	case 3:
		bits := uint64(0x7ff8000000000001) + uint64(position)
		if position%2 != 0 {
			bits = 0x8000000000000000
		}
		return Type{Kind: Double, HasBits: true, Bits: bits}
	case 4:
		return Type{Kind: Ref, Class: fmt.Sprintf("model/Value%d", position)}
	case 5:
		return Type{Kind: Null}
	case 6:
		return Type{Kind: UninitThis}
	case 7:
		return Type{Kind: UninitNew, NewPC: uint16(position)}
	default:
		return Type{Kind: UninitNew, NewPC: uint16(65535 - position)}
	}
}

func boundedStackLocals() []Type {
	return []Type{
		{Kind: Int, HasInt: true, Int: 42},
		{Kind: Ref, Class: "model/Owner"},
		{Kind: UninitNew, NewPC: 0},
		{Kind: UninitNew, NewPC: 65535},
		{Kind: Long, HasLong: true, Long: -9223372036854775808},
		{Kind: LongTail}, {Kind: UninitThis}, {Kind: Top},
	}
}

// Compare every typed value bit and frame flag without reflecting hundreds of
// thousands of frames. Empty slice allocation is not part of the stack model.
func boundedFrameExact(a, b Frame) bool {
	if len(a.Locals) != len(b.Locals) || len(a.Stack) != len(b.Stack) || a.maxStack != b.maxStack || a.hasLimits != b.hasLimits || a.ThisUninitialized != b.ThisUninitialized || a.ThisClass != b.ThisClass || a.DirectSuperClass != b.DirectSuperClass {
		return false
	}
	for i := range a.Locals {
		if a.Locals[i] != b.Locals[i] {
			return false
		}
	}
	for i := range a.Stack {
		if a.Stack[i] != b.Stack[i] {
			return false
		}
	}
	return true
}

func boundedFrameUnpublished(out Frame) bool {
	return out.Locals == nil && out.Stack == nil && boundedFrameExact(out, Frame{})
}

// This exhausts the declared finite grammar, not all JVM programs: 0..4
// logical values over nine typed identity/bit representatives, all nine stack
// moves, every capacity 0..10 and the u2 maximum. Capacity includes invalid
// incoming frames and limits immediately around the expected output size.
func TestBoundedStackMoveDiagrams(t *testing.T) {
	digest := sha256.New()
	var encoded [20]byte
	count, accepted, rejected := 0, 0, 0
	for n, words := 0, 1; n <= 4; n, words = n+1, words*9 {
		for word := 0; word < words; word++ {
			logical := make([]Type, n)
			remaining := word
			for i := range logical {
				logical[i] = boundedStackValue(remaining%9, i)
				remaining /= 9
			}
			input := boundedStackSlots(logical)
			for _, op := range boundedStackOps {
				wantLogical, legal := boundedStackDiagram(logical, op)
				want := boundedStackSlots(wantLogical)
				for limitIndex := 0; limitIndex <= 11; limitIndex++ {
					limit := limitIndex
					if limitIndex == 11 {
						limit = 65535
					}
					f, err := NewFrameWithLimits(8, limit)
					if err != nil {
						t.Fatal(err)
					}
					f.Locals = boundedStackLocals()
					f.Stack = append([]Type(nil), input...)
					f.ThisUninitialized, f.ThisClass, f.DirectSuperClass = true, "model/Owner", "model/Base"
					before := f.Clone()
					out, exception, err := Transfer(f, Instr{Op: op, Local: -1})
					valid := legal && len(input) <= limit && len(want) <= limit
					if !boundedFrameExact(f, before) || exception != nil {
						t.Fatalf("input mutation/exception n=%d word=%d op=%#x limit=%d", n, word, op, limit)
					}
					if valid {
						accepted++
						expected := before.Clone()
						expected.Stack = want
						if err != nil || !boundedFrameExact(out, expected) {
							t.Fatalf("diagram n=%d word=%d op=%#x limit=%d got=%+v want=%+v err=%v", n, word, op, limit, out, expected, err)
						}
					} else {
						rejected++
						if err == nil || !IsInvalid(err) || !boundedFrameUnpublished(out) {
							t.Fatalf("refusal n=%d word=%d op=%#x limit=%d out=%+v err=%v", n, word, op, limit, out, err)
						}
					}
					for i, v := range []int{n, word, op, limit, len(want)} {
						binary.LittleEndian.PutUint32(encoded[i*4:], uint32(v))
					}
					digest.Write(encoded[:])
					count++
				}
			}
		}
	}
	if count != 797148 || accepted == 0 || rejected == 0 {
		t.Fatalf("incomplete grammar: count=%d accepted=%d rejected=%d", count, accepted, rejected)
	}
	t.Logf("JVMS bounded stack diagrams: %d cases (%d legal, %d refused), sha256=%x", count, accepted, rejected, digest.Sum(nil))
}

// Independently recognize head/tail layouts, including crossed/dangling pairs
// and unknown kinds. These are invalid input contract cases, not legal bytecode
// behavioral witnesses. Every rejection must leave the caller's slices intact.
func TestBoundedMalformedStackSlots(t *testing.T) {
	alphabet := []Type{{Kind: Top}, {Kind: Int}, {Kind: Long}, {Kind: LongTail}, {Kind: Double}, {Kind: DoubleTail}, {Kind: Kind(255)}}
	rejected := 0
	for n, words := 0, 1; n <= 4; n, words = n+1, words*len(alphabet) {
		for word := 0; word < words; word++ {
			slots := make([]Type, n)
			remaining := word
			for i := range slots {
				slots[i] = alphabet[remaining%len(alphabet)]
				remaining /= len(alphabet)
			}
			wellFormed := true
			for i := 0; i < len(slots); i++ {
				switch slots[i].Kind {
				case Int:
				case Long, Double:
					want := LongTail
					if slots[i].Kind == Double {
						want = DoubleTail
					}
					if i+1 >= len(slots) || slots[i+1].Kind != want {
						wellFormed = false
					} else {
						i++
					}
				default:
					wellFormed = false
				}
			}
			if wellFormed {
				continue // Legal layouts are covered by the diagram oracle.
			}
			for _, op := range boundedStackOps {
				f, _ := NewFrameWithLimits(8, n+2)
				f.Locals = boundedStackLocals()
				f.Stack = slots
				before := f.Clone()
				out, exception, err := Transfer(f, Instr{Op: op, Local: -1})
				if err == nil || !IsInvalid(err) || exception != nil || !boundedFrameUnpublished(out) || !boundedFrameExact(f, before) {
					t.Fatalf("malformed slots n=%d word=%d op=%#x out=%+v err=%v", n, word, op, out, err)
				}
				rejected++
			}
		}
	}
	if rejected != 25020 {
		t.Fatalf("malformed grammar incomplete: %d", rejected)
	}
	t.Logf("malformed expanded layouts rejected transactionally: %d", rejected)
}

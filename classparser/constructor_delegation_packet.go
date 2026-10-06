package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

type constructorMotionPacket struct {
	arguments   []string
	origins     []int
	allocations map[string]string
	arrays      map[int]string
}

// A forward join preserves only facts true on every incoming edge. Reference
// types currently require the same original erased descriptor; an unproved
// hierarchy LUB must not turn a foreign operand into an enclosing identity.
// Different narrow integer words can join as computational I, but a canonical
// boolean witness survives only when both physical producer paths prove it.
func constructorMotionPacketJoin(prior, next *constructorMotionPacket, pc int, booleans map[int]bool) (*constructorMotionPacket, bool) {
	if next == nil || len(next.arguments) != len(next.origins) {
		return nil, false
	}
	out := &constructorMotionPacket{arguments: append([]string(nil), next.arguments...), origins: append([]int(nil), next.origins...), allocations: map[string]string{}, arrays: map[int]string{}}
	for token, owner := range next.allocations {
		out.allocations[token] = owner
	}
	for origin, descriptor := range next.arrays {
		out.arrays[origin] = descriptor
	}
	if prior == nil {
		return out, true
	}
	if len(prior.arguments) != len(next.arguments) || len(prior.origins) != len(next.origins) || len(prior.allocations) != len(next.allocations) {
		return nil, false
	}
	for token, owner := range prior.allocations {
		if next.allocations[token] != owner {
			return nil, false
		}
	}
	for origin, descriptor := range out.arrays {
		if prior.arrays[origin] != descriptor {
			delete(out.arrays, origin)
		}
	}
	for i, actual := range next.arguments {
		old := prior.arguments[i]
		if actual != old {
			integerWord := func(d string) bool { return d == "I" || d == "B" || d == "S" || d == "C" }
			if !integerWord(actual) || !integerWord(old) {
				return nil, false
			}
			out.arguments[i] = "I"
		}
		if prior.origins[i] != next.origins[i] {
			out.origins[i] = -1
			if out.arguments[i] == "I" && booleans[prior.origins[i]] && booleans[next.origins[i]] {
				// Every packet copy consumes the shared 512-unit budget. These
				// join tags are disjoint from physical literals, local parameters,
				// lexical reads and fresh-array allocation origins.
				origin := -262144 - pc*512 - i
				booleans[origin] = true
				out.origins[i] = origin
			}
		}
	}
	return out, true
}

func constructorMotionPacketBranch(opcode int) bool {
	return opcode == core.OP_GOTO || opcode == core.OP_GOTO_W || opcode >= core.OP_IFEQ && opcode <= core.OP_IF_ACMPNE || opcode == core.OP_IFNULL || opcode == core.OP_IFNONNULL
}

func constructorMotionPacketDestination(ops []*core.OpCode, index int) (int, bool) {
	op := ops[index]
	delta := 0
	if len(op.Data) == 2 {
		delta = int(int16(binary.BigEndian.Uint16(op.Data)))
	} else if op.Instr.OpCode == core.OP_GOTO_W && len(op.Data) == 4 {
		delta = int(int32(binary.BigEndian.Uint32(op.Data)))
	} else {
		return 0, false
	}
	pc := int(op.CurrentOffset) + delta
	for i := index + 1; i < len(ops) && i-index <= 512; i++ {
		if ops[i] != nil && int(ops[i].CurrentOffset) == pc {
			return i, true
		}
	}
	return 0, false
}

// Branch operands belong to one unique original constructor Code, including
// its instruction boundaries, stack limit and complete parameter slots. A
// mutable decoded opcode list cannot invent an edge or omit an unsafe arm.
func constructorMotionPacketOriginalCode(obj *ClassObject, params []string, ops []*core.OpCode) *CodeAttribute {
	if obj == nil {
		return nil
	}
	descriptor := "(" + strings.Join(params, "") + ")V"
	var code *CodeAttribute
	matches := 0
	for _, method := range obj.Methods {
		if method == nil {
			return nil
		}
		name, nk := sourceBridgeUTF8(obj, method.NameIndex)
		desc, dk := sourceBridgeUTF8(obj, method.DescriptorIndex)
		if !nk || !dk {
			return nil
		}
		if name != "<init>" || desc != descriptor {
			continue
		}
		matches++
		for _, attribute := range method.Attributes {
			if original, ok := attribute.(*CodeAttribute); ok {
				if code != nil {
					return nil
				}
				code = original
			}
		}
	}
	if matches != 1 || code == nil || len(code.Code) == 0 || len(code.Code) > 65535 || len(code.ExceptionTable) != 0 || int(code.MaxLocals) < nativeMemberParameterWidth(params)+1 {
		return nil
	}
	decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
	if decoder.ParseOpcode() != nil {
		return nil
	}
	original := constructorMotionOps(decoder)
	if len(original) != len(ops) {
		return nil
	}
	for i, op := range ops {
		physical := original[i]
		if op == nil || op.Instr == nil || physical == nil || physical.Instr == nil || op.CurrentOffset != physical.CurrentOffset || op.IsWide != physical.IsWide || op.Instr.OpCode != physical.Instr.OpCode || !bytes.Equal(op.Data, physical.Data) {
			return nil
		}
	}
	return code
}

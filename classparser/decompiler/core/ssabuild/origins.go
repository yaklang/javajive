package ssabuild

import (
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
)

// InstructionValues records computational operands in stack order (receiver
// first for calls). Loads/stores and stack permutations preserve aliases;
// arithmetic and checked results have stable (PC, result index) identities.
type InstructionValues struct {
	PC                uint16
	Before            frametransfer.Frame
	BeforeOrigins     []Origin
	Uses, Results     []Origin
	UseIDs, ResultIDs []ValueID
}
type originValue struct {
	origin Origin
	width  int
}

func decodeOrigins(types []frametransfer.Type, origins []Origin) ([]originValue, error) {
	if len(types) != len(origins) {
		return nil, fmt.Errorf("invalid_input: origin/frame length mismatch")
	}
	var values []originValue
	for i := 0; i < len(types); i++ {
		t := types[i]
		if !t.Computational() || origins[i].Kind == OriginTop {
			return nil, fmt.Errorf("invalid_input: unavailable stack origin at %d", i)
		}
		v := originValue{origins[i], t.Width()}
		if v.width == 2 {
			if i+1 >= len(types) || types[i+1].Kind != frametransfer.TailOf(t).Kind || origins[i+1] != origins[i] {
				return nil, fmt.Errorf("invalid_input: mismatched wide origin at %d", i)
			}
			i++
		}
		values = append(values, v)
	}
	return values, nil
}

// originGroup consumes exact JVM slots, never a half category-2 value.
func originGroup(v []originValue, slots int) (int, error) {
	i := len(v)
	for slots > 0 && i > 0 {
		i--
		slots -= v[i].width
	}
	if slots != 0 {
		return 0, fmt.Errorf("invalid_input: origin stack category boundary/underflow")
	}
	return i, nil
}

func transferOrigins(before []Origin, beforeF, afterF frametransfer.Frame, ins methodir.Instr) ([]Origin, InstructionValues, error) {
	record := InstructionValues{PC: ins.PC}
	fail := func(err error) ([]Origin, InstructionValues, error) { return nil, record, err }
	if len(before) != len(beforeF.Locals)+len(beforeF.Stack) {
		return fail(fmt.Errorf("invalid_input: missing origins at pc %d", ins.PC))
	}
	stack, err := decodeOrigins(beforeF.Stack, before[len(beforeF.Locals):])
	if err != nil {
		return fail(err)
	}
	out := originsFromFrame(afterF, Origin{Kind: OriginTop})
	// Only explicit local writes change identity. Constructor initialization changes
	// verifier types of aliases, not their identities. TOP kills invalidate origins.
	for i, t := range afterF.Locals {
		if t.Kind != frametransfer.Top && i < len(beforeF.Locals) {
			out[i] = before[i]
		}
	}
	op := ins.Opcode
	acc := core.LocalAccessOf(op)
	loc := effectiveLocal(ins)
	addUses := func(v []originValue) {
		for _, x := range v {
			record.Uses = append(record.Uses, x.origin)
		}
	}
	def := Origin{Kind: OriginInstr, PC: ins.PC}
	switch {
	case op == core.OP_IINC:
		if loc < 0 || loc >= len(beforeF.Locals) || loc >= len(afterF.Locals) {
			return fail(fmt.Errorf("invalid_input: iinc origin local"))
		}
		record.Uses = []Origin{before[loc]}
		def.Slot = loc
		record.Results = []Origin{def}
		out[loc] = def
	case acc.Read && !acc.Write:
		if loc < 0 || loc >= len(beforeF.Locals) || before[loc].Kind == OriginTop {
			return fail(fmt.Errorf("invalid_input: load origin local"))
		}
		v := originValue{before[loc], beforeF.Locals[loc].Width()}
		record.Uses = []Origin{v.origin}
		stack = append(stack, v)
	case acc.Write:
		if loc < 0 || loc >= len(afterF.Locals) || len(stack) == 0 {
			return fail(fmt.Errorf("invalid_input: store origin local/stack"))
		}
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		record.Uses = []Origin{v.origin}
		out[loc] = v.origin
		if v.width == 2 {
			if loc+1 >= len(out) {
				return fail(fmt.Errorf("invalid_input: wide store origin"))
			}
			out[loc+1] = v.origin
		}
	case op >= core.OP_POP && op <= core.OP_SWAP:
		top, under := 1, 0
		switch op {
		case core.OP_POP2, core.OP_DUP2:
			top = 2
		case core.OP_DUP_X1, core.OP_SWAP:
			under = 1
		case core.OP_DUP_X2:
			under = 2
		case core.OP_DUP2_X1:
			top, under = 2, 1
		case core.OP_DUP2_X2:
			top, under = 2, 2
		}
		a, e := originGroup(stack, top)
		if e != nil {
			return fail(e)
		}
		b, e := originGroup(stack[:a], under)
		if e != nil {
			return fail(e)
		}
		addUses(stack[b:])
		hi := append([]originValue(nil), stack[a:]...)
		lo := append([]originValue(nil), stack[b:a]...)
		stack = append([]originValue(nil), stack[:b]...)
		if op != core.OP_POP && op != core.OP_POP2 {
			stack = append(stack, hi...)
			stack = append(stack, lo...)
			if op != core.OP_SWAP {
				stack = append(stack, hi...)
			}
		}
	case op == core.OP_ATHROW:
		if len(stack) == 0 {
			return fail(fmt.Errorf("invalid_input: athrow origin stack"))
		}
		v := stack[len(stack)-1]
		record.Uses = []Origin{v.origin}
		stack = []originValue{v}
	default:
		pop, push, e := originEffect(ins)
		if e != nil {
			return fail(e)
		}
		if pop > len(stack) {
			return fail(fmt.Errorf("invalid_input: origin operand underflow at pc %d", ins.PC))
		}
		base := len(stack) - pop
		addUses(stack[base:])
		stack = stack[:base]
		if push {
			prefixSlots := 0
			for _, v := range stack {
				prefixSlots += v.width
			}
			if prefixSlots >= len(afterF.Stack) {
				return fail(fmt.Errorf("invalid_input: missing result at pc %d", ins.PC))
			}
			stack = append(stack, originValue{def, afterF.Stack[prefixSlots].Width()})
			record.Results = []Origin{def}
		}
	}
	// Encode aliases back to the expanded frame; both halves share one identity.
	encoded := []Origin{}
	for _, v := range stack {
		for j := 0; j < v.width; j++ {
			encoded = append(encoded, v.origin)
		}
	}
	if len(encoded) != len(afterF.Stack) {
		return fail(fmt.Errorf("invalid_input: origin stack effect mismatch at pc %d", ins.PC))
	}
	if _, err := decodeOrigins(afterF.Stack, encoded); err != nil {
		return fail(err)
	}
	copy(out[len(afterF.Locals):], encoded)
	return out, record, nil
}

// Explicit computational-value effects for every non-alias opcode accepted by
// frametransfer.Transfer. Type equality is never evidence of value identity.
func originEffect(ins methodir.Instr) (pop int, push bool, err error) {
	op := ins.Opcode
	switch {
	case op == core.OP_NOP || op == core.OP_GOTO || op == core.OP_GOTO_W || op == core.OP_RETURN:
		return 0, false, nil
	case op >= core.OP_ACONST_NULL && op <= core.OP_LDC2_W:
		return 0, true, nil
	case op >= core.OP_IALOAD && op <= core.OP_SALOAD:
		return 2, true, nil
	case op >= core.OP_IASTORE && op <= core.OP_SASTORE:
		return 3, false, nil
	case op >= core.OP_IADD && op <= core.OP_DREM:
		return 2, true, nil
	case op >= core.OP_INEG && op <= core.OP_DNEG:
		return 1, true, nil
	case op >= core.OP_ISHL && op <= core.OP_LXOR:
		return 2, true, nil
	case op >= core.OP_I2L && op <= core.OP_I2S:
		return 1, true, nil
	case op >= core.OP_LCMP && op <= core.OP_DCMPG:
		return 2, true, nil
	case op >= core.OP_IFEQ && op <= core.OP_IFLE:
		return 1, false, nil
	case op >= core.OP_IF_ICMPEQ && op <= core.OP_IF_ACMPNE:
		return 2, false, nil
	case op == core.OP_IFNULL || op == core.OP_IFNONNULL || op == core.OP_TABLESWITCH || op == core.OP_LOOKUPSWITCH:
		return 1, false, nil
	case op >= core.OP_IRETURN && op <= core.OP_ARETURN:
		return 1, false, nil
	case op == core.OP_GETSTATIC || op == core.OP_NEW:
		return 0, true, nil
	case op == core.OP_PUTSTATIC:
		return 1, false, nil
	case op == core.OP_GETFIELD || op == core.OP_NEWARRAY || op == core.OP_ANEWARRAY || op == core.OP_ARRAYLENGTH || op == core.OP_CHECKCAST || op == core.OP_INSTANCEOF:
		return 1, true, nil
	case op == core.OP_PUTFIELD:
		return 2, false, nil
	case op == core.OP_MONITORENTER || op == core.OP_MONITOREXIT:
		return 1, false, nil
	case op >= core.OP_INVOKEVIRTUAL && op <= core.OP_INVOKEDYNAMIC:
		if ins.Desc == "" {
			return 0, false, fmt.Errorf("unsupported_feature: invocation without descriptor at pc %d", ins.PC)
		}
		args, _, ret, e := frametransfer.ParseDescriptor(ins.Desc)
		if e != nil {
			return 0, false, e
		}
		n := len(args)
		if op != core.OP_INVOKESTATIC && op != core.OP_INVOKEDYNAMIC {
			n++
		}
		return n, ret, nil
	case op == core.OP_MULTIANEWARRAY:
		dims := frametransfer.FromIR(ins).Dims
		if dims <= 0 {
			return 0, false, fmt.Errorf("invalid_input: missing array dimensions")
		}
		return dims, true, nil
	default:
		return 0, false, fmt.Errorf("unsupported_feature: origin opcode %#x at pc %d", op, ins.PC)
	}
}

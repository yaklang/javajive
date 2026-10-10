package javaclassparser

import (
	"encoding/binary"
	"strconv"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

type constructorEffectPathMemo struct {
	result, stable bool
	epoch          uint64
}

// A linear handler suffix can forget a local's VALUE only if it never reads
// that incoming value before overwriting it. Its wide shape remains significant:
// overwriting either half of a category-2 local invalidates the other half.
// This is a memo-key projection; the interpreter still receives the original
// input frame. Branches, covered throws and missing exits retain the full frame.
func constructorLinearHandlerLiveness(ops []*core.OpCode, start, slots int, handlers map[int]int, remaining *int, work *workbudget.Budget) []bool {
	if start < 0 || start >= len(ops) || slots < 1 || remaining == nil {
		return nil
	}
	end := -1
	for i := start; i < len(ops); i++ {
		*remaining--
		if *remaining < 0 || !nativeProofWork(work, 1) || ops[i] == nil || ops[i].Instr == nil {
			return nil
		}
		opcode := ops[i].Instr.OpCode
		if _, covered := handlers[i]; covered {
			return nil
		}
		if opcode >= core.OP_IFEQ && opcode <= core.OP_LOOKUPSWITCH || opcode == core.OP_IFNULL || opcode == core.OP_IFNONNULL || opcode == core.OP_GOTO_W || opcode == core.OP_JSR_W {
			return nil
		}
		if opcode == core.OP_ATHROW || opcode >= core.OP_IRETURN && opcode <= core.OP_RETURN {
			end = i
			break
		}
	}
	if end < 0 || work != nil && work.CheckAlloc(int64(slots)) != nil {
		return nil
	}
	live := make([]bool, slots)
	for i := end; i >= start; i-- {
		*remaining--
		if *remaining < 0 || !nativeProofWork(work, 1) {
			return nil
		}
		op := ops[i]
		access := core.LocalAccessOf(op.Instr.OpCode)
		if !access.Read && !access.Write {
			continue
		}
		slot := core.GetStoreIdx(op)
		if access.Read {
			slot = core.GetRetrieveIdx(op)
		}
		if slot < 0 || slot+access.Width > slots {
			return nil
		}
		if access.Write {
			for j := slot; j < slot+access.Width; j++ {
				live[j] = false
			}
		}
		if access.Read {
			for j := slot; j < slot+access.Width; j++ {
				live[j] = true
			}
		}
	}
	return live
}

func constructorEffectFrameKey(start int, locals, stack []constructorEffectValue, initialized bool, live []bool, failedAllocation int) string {
	state := []byte(strconv.Itoa(start) + ":")
	if initialized {
		state = append(state, 1)
	} else {
		state = append(state, 0)
	}
	for part, values := range [][]constructorEffectValue{locals, stack} {
		for slot, v := range values {
			if part == 0 && failedAllocation != 0 && v.allocation == failedAllocation {
				v = constructorEffectValue{}
			}
			if part == 0 && len(live) == len(locals) && !live[slot] {
				if v.width() == 2 {
					v = constructorEffectValue{kind: 'J'}
				} else {
					v = constructorEffectValue{}
				}
			}
			state = append(state, v.kind)
			if v.knownInt {
				state = append(state, 1)
				state = binary.BigEndian.AppendUint32(state, uint32(v.intWord))
			} else {
				state = append(state, 0)
			}
			state = binary.BigEndian.AppendUint32(state, uint32(v.allocation))
			if v.receiver {
				state = append(state, 1)
			} else {
				state = append(state, 0)
			}
		}
		state = append(state, 255)
	}
	return string(state)
}

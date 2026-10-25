package javaclassparser

import (
	"encoding/binary"
	"sort"
	"strconv"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

type enumEntrySymbol struct {
	value string
	width int
}

// A compiler may factor branch-local own-field writes through a temporary.
// Compare bounded path effects rather than instruction counts. Locals are
// private storage, but calls, resolution-bearing constants and field reads /
// writes remain ordered events. Every potentially throwing event also records
// each ordered catch continuation and its escape path. No callee is assumed
// pure, and only an exactly declared own static field can avoid resolution
// failure while its owning <clinit> is already active.
func enumEntryPathEffects(obj *ClassObject, code *CodeAttribute, ops []*core.OpCode, work *workbudget.Budget) ([]string, bool) {
	if obj == nil || code == nil || len(ops) == 0 || len(ops) > 256 || code.MaxLocals > 32 || len(code.ExceptionTable) > 64 || work.CheckAlloc(1<<20) != nil {
		return nil, false
	}
	pos := map[int]int{}
	start := int(ops[0].CurrentOffset)
	last := ops[len(ops)-1]
	end := int(last.CurrentOffset) + 1 + len(last.Data)
	for i, op := range ops {
		if op == nil || op.Instr == nil || op.IsWide {
			return nil, false
		}
		pos[int(op.CurrentOffset)] = i
	}
	pos[end] = len(ops)
	for _, h := range code.ExceptionTable {
		if h == nil {
			return nil, false
		}
		_, ak := pos[int(h.StartPc)]
		_, bk := pos[int(h.EndPc)]
		_, ck := pos[int(h.HandlerPc)]
		if !ak || !bk || !ck || int(h.StartPc) < start || h.StartPc >= h.EndPc || int(h.EndPc) > end || int(h.HandlerPc) >= end {
			return nil, false
		}
	}
	// Scratch locals cannot communicate with the allocation/body continuation.
	decoder := core.NewDecompiler(code.Code, nil)
	decoder.Work = work
	if decoder.ParseOpcode() != nil {
		return nil, false
	}
	for _, op := range constructorMotionOps(decoder) {
		pc := int(op.CurrentOffset)
		if pc >= start && pc < end {
			continue
		}
		access := core.LocalAccessOf(op.Instr.OpCode)
		if access.Read || access.Write || op.Instr.OpCode == core.OP_IINC {
			return nil, false
		}
	}
	ownField := func(op *core.OpCode) bool {
		index, known := nativeEnumCPIndex(op)
		if !known || index == 0 || int(index) > len(obj.ConstantPool) {
			return false
		}
		ref, known := obj.ConstantPool[index-1].(*ConstantFieldrefInfo)
		if !known || ref == nil || ref.NameAndTypeIndex == 0 || int(ref.NameAndTypeIndex) > len(obj.ConstantPool) {
			return false
		}
		pair, known := obj.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
		if !known || pair == nil {
			return false
		}
		owner, okOwner := sourceParameterClassKey(obj, ref.ClassIndex)
		self, okSelf := sourceParameterClassKey(obj, obj.ThisClass)
		name, okName := sourceParameterUTF8Key(obj, pair.NameIndex)
		desc, okDesc := sourceParameterUTF8Key(obj, pair.DescriptorIndex)
		if !okOwner || !okSelf || !okName || !okDesc || owner != self {
			return false
		}
		count := 0
		for _, f := range obj.Fields {
			if f == nil {
				return false
			}
			n, nk := sourceParameterUTF8Key(obj, f.NameIndex)
			d, dk := sourceParameterUTF8Key(obj, f.DescriptorIndex)
			if nk && dk && n == name && d == desc {
				if f.AccessFlags&8 == 0 {
					return false
				}
				count++
			}
		}
		return count == 1
	}
	width := func(desc string) int {
		if desc == "J" || desc == "D" {
			return 2
		}
		return 1
	}
	var paths []string
	steps, total := 0, 0
	valid := true
	var walk func(int, []enumEntrySymbol, []enumEntrySymbol, []string)
	walk = func(at int, stack, locals []enumEntrySymbol, trace []string) {
		stack = append([]enumEntrySymbol(nil), stack...)
		locals = append([]enumEntrySymbol(nil), locals...)
		trace = append([]string(nil), trace...)
		pop := func() (enumEntrySymbol, bool) {
			if len(stack) == 0 {
				return enumEntrySymbol{}, false
			}
			v := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			return v, true
		}
		terminal := func(tail string) {
			line := strings.Join(append(append([]string(nil), trace...), tail), "\n")
			total += len(line)
			if total > 1<<20 || len(paths) >= 1024 {
				valid = false
				return
			}
			paths = append(paths, line)
		}
		for valid && at < len(ops) {
			steps++
			if steps > 16384 || !nativeProofWork(work, 1) {
				valid = false
				return
			}
			op := ops[at]
			pc, x := int(op.CurrentOffset), op.Instr.OpCode
			if x == core.OP_GOTO || x == core.OP_GOTO_W {
				delta := 0
				if x == core.OP_GOTO && len(op.Data) == 2 {
					delta = int(int16(binary.BigEndian.Uint16(op.Data)))
				} else if x == core.OP_GOTO_W && len(op.Data) == 4 {
					delta = int(int32(binary.BigEndian.Uint32(op.Data)))
				} else {
					valid = false
					return
				}
				next, known := pos[pc+delta]
				if !known || next <= at {
					valid = false
					return
				}
				at = next
				continue
			}
			local, load, localKnown := enumEntryScratchLocal(op)
			if localKnown {
				if local < 0 || local >= len(locals) {
					valid = false
					return
				}
				if load {
					if locals[local].width != 1 {
						valid = false
						return
					}
					stack = append(stack, locals[local])
				} else {
					v, ok := pop()
					if !ok || v.width != 1 {
						valid = false
						return
					}
					locals[local] = v
				}
				at++
				continue
			}
			if x == core.OP_POP || x == core.OP_POP2 {
				v, ok := pop()
				if !ok {
					valid = false
					return
				}
				need := 1
				if x == core.OP_POP2 {
					need = 2
				}
				need -= v.width
				if need == 1 {
					v, ok = pop()
					if !ok || v.width != 1 {
						valid = false
						return
					}
					need--
				}
				if need != 0 {
					valid = false
					return
				}
				at++
				continue
			}
			one, known := enumEntryEffectProgram(obj, []*core.OpCode{op}, work)
			if !known || len(one) != 1 {
				valid = false
				return
			}
			key := strconv.Itoa(one[0].opcode) + ":" + strconv.Quote(one[0].operand)
			if one[0].opcode == core.OP_ICONST_0 {
				stack = append(stack, enumEntrySymbol{key, 1})
				at++
				continue
			}
			if x == core.OP_ACONST_NULL {
				stack = append(stack, enumEntrySymbol{"null", 1})
				at++
				continue
			}
			result, mayThrow := 1, true
			var args []enumEntrySymbol
			switch x {
			case core.OP_LDC, core.OP_LDC_W, core.OP_LDC2_W:
				if strings.HasPrefix(one[0].operand, "long\x00") || strings.HasPrefix(one[0].operand, "double\x00") {
					result = 2
				}
				if !strings.HasPrefix(one[0].operand, "string\x00") && !strings.HasPrefix(one[0].operand, "class\x00") {
					stack = append(stack, enumEntrySymbol{key, result})
					at++
					continue
				}
			case core.OP_GETSTATIC, core.OP_PUTSTATIC:
				m := constructorMotionMember(obj, op, x)
				if m == nil {
					valid = false
					return
				}
				_, typ, e := callbinding.Descriptor("()" + m.Description)
				if e != nil || typ == "V" {
					valid = false
					return
				}
				result = width(typ)
				mayThrow = !ownField(op)
				if x == core.OP_PUTSTATIC {
					v, ok := pop()
					if !ok || v.width != result {
						valid = false
						return
					}
					args = []enumEntrySymbol{v}
					result = 0
				}
			case core.OP_INVOKESTATIC:
				m := constructorMotionMember(obj, op, x)
				if m == nil {
					valid = false
					return
				}
				params, ret, e := callbinding.Descriptor(m.Description)
				if e != nil {
					valid = false
					return
				}
				args = make([]enumEntrySymbol, len(params))
				for i := len(params) - 1; i >= 0; i-- {
					v, ok := pop()
					if !ok || v.width != width(params[i]) {
						valid = false
						return
					}
					args[i] = v
				}
				result = width(ret)
				if ret == "V" {
					result = 0
				}
			default:
				valid = false
				return
			}
			for _, arg := range args {
				key += "|" + strconv.Quote(arg.value)
			}
			if len(key) > 65535 || !nativeProofWork(work, int64(len(key))+1) {
				valid = false
				return
			}
			trace = append(trace, key)
			value := "v" + strconv.Itoa(len(trace))
			if mayThrow {
				terminal("escape")
				route := 0
				for _, h := range code.ExceptionTable {
					if pc < int(h.StartPc) || pc >= int(h.EndPc) {
						continue
					}
					typ := "*"
					if h.CatchType != 0 {
						var known bool
						typ, known = sourceParameterClassKey(obj, h.CatchType)
						if !known {
							valid = false
							return
						}
					}
					next := pos[int(h.HandlerPc)]
					if next <= at {
						valid = false
						return
					}
					branch := append(append([]string(nil), trace...), "catch "+strconv.Itoa(route)+":"+strconv.Quote(typ))
					route++
					walk(next, []enumEntrySymbol{{value + ":exception", 1}}, locals, branch)
				}
			}
			if result > 0 {
				stack = append(stack, enumEntrySymbol{value, result})
			}
			at++
		}
		if !valid {
			return
		}
		if at != len(ops) || len(stack) != 0 {
			valid = false
			return
		}
		terminal("complete")
	}
	walk(0, nil, make([]enumEntrySymbol, int(code.MaxLocals)), nil)
	if !valid || len(paths) == 0 {
		return nil, false
	}
	sort.Strings(paths)
	return paths, true
}

func enumEntryScratchLocal(op *core.OpCode) (int, bool, bool) {
	x := op.Instr.OpCode
	for _, family := range []struct {
		generic, first int
		load           bool
	}{{core.OP_ILOAD, core.OP_ILOAD_0, true}, {core.OP_ALOAD, core.OP_ALOAD_0, true}, {core.OP_ISTORE, core.OP_ISTORE_0, false}, {core.OP_ASTORE, core.OP_ASTORE_0, false}} {
		if x == family.generic && len(op.Data) == 1 {
			return int(op.Data[0]), family.load, true
		}
		if x >= family.first && x <= family.first+3 && len(op.Data) == 0 {
			return x - family.first, family.load, true
		}
	}
	return 0, false, false
}

package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/internal/workbudget"
)

func nativeAnonymousPrivateArgumentConsumers(f *nativeAnonymousForest, obj *ClassObject, method *MemberInfo, code *CodeAttribute, ops []*core.OpCode, read *nativeMemberLexicalRead, target string, metadata callbinding.Provider, work *workbudget.Budget) []*nativeAnonymousLexicalConsumer {
	if f == nil || f.members == nil || obj == nil || method == nil || code == nil || read == nil || method.AccessFlags&StaticFlag != 0 {
		return nil
	}
	// Avoid a frame analysis for every ordinary enclosing-field read. Only
	// methods with a physical parameterized private bridge need this query.
	eligible := false
	for _, op := range ops {
		if !nativeProofWork(work, 1) {
			return nil
		}
		if op == nil || int(op.CurrentOffset) <= read.pc {
			continue
		}
		symbol := constructorMotionMember(obj, op, core.OP_INVOKESTATIC)
		if symbol != nil && symbol.Name == target {
			g := f.members.getters[nativeMemberGetterKey(target, symbol.Member, symbol.Description)]
			if nativeAnonymousAccessorArgumentCount(g) > 1 {
				eligible = true
				break
			}
		}
	}
	if !eligible {
		return nil
	}
	reader := NewClassObjectDumper(obj)
	reader.Work = work
	ir, fn, known := reader.nativeOriginalMethodSnapshot(method, code)
	if !known {
		return nil
	}
	records, known := nativeAnonymousArgumentRecords(fn, work)
	if !known {
		return nil
	}
	consumers := []*nativeAnonymousLexicalConsumer{}
	for _, op := range ops {
		if !nativeProofWork(work, 1) {
			return nil
		}
		if op == nil || int(op.CurrentOffset) <= read.pc {
			continue
		}
		symbol := constructorMotionMember(obj, op, core.OP_INVOKESTATIC)
		if symbol == nil || symbol.Name != target {
			continue
		}
		g := f.members.getters[nativeMemberGetterKey(target, symbol.Member, symbol.Description)]
		if nativeAnonymousAccessorArgumentCount(g) <= 1 {
			continue
		}
		arguments, known := nativeAnonymousArgumentReceiver(ir, records, int(op.CurrentOffset), read, work)
		if !known || len(arguments)+1 != nativeAnonymousAccessorArgumentCount(g) || !nativeAnonymousArgumentFrameTypes(obj, method, ir, fn, records[int(op.CurrentOffset)], symbol.Description, metadata, work) {
			continue
		}
		consumer := nativeAnonymousLexicalConsumerProof(f, obj, read, target, op, f.resolve, work)
		if consumer != nil {
			consumer.argumentOrigins = arguments
			consumers = append(consumers, consumer)
		}
	}
	return consumers
}

// Original frames keep the lexical receiver below the argument producers.
// They establish bytecode identity only; source operands need their separate
// original-producer DAG proof before this certificate can render a call.
func nativeAnonymousArgumentReceiver(ir *methodir.MethodIR, records map[int]*ssabuild.InstructionValues, pc int, path *nativeMemberLexicalRead, work *workbudget.Budget) ([]ssabuild.Origin, bool) {
	if ir == nil || path == nil || pc < 0 || pc > 65535 || !nativeProofWork(work, 1) {
		return nil, false
	}
	invoke, known := ir.InstrByID(methodir.InstrID(pc))
	call := records[pc]
	if !known || invoke.Opcode != core.OP_INVOKESTATIC || call == nil || len(call.Uses) == 0 {
		return nil, false
	}
	origin := call.Uses[0]
	active := map[*nativeMemberLexicalRead]bool{}
	for r := path; r != nil; r = r.prior {
		if active[r] || len(active) >= 64 || !nativeProofWork(work, 1) || r.pc < 0 || r.pc >= pc || origin != (ssabuild.Origin{Kind: ssabuild.OriginInstr, PC: uint16(r.pc)}) {
			return nil, false
		}
		active[r] = true
		ins, known := ir.InstrByID(methodir.InstrID(r.pc))
		record := records[r.pc]
		if !known || ins.Opcode != core.OP_GETFIELD || ins.Class != r.owner || ins.Member != r.field || ins.Desc != r.descriptor || record == nil || len(record.Uses) != 1 {
			return nil, false
		}
		origin = record.Uses[0]
	}
	if origin != (ssabuild.Origin{Kind: ssabuild.OriginParam, Slot: 0}) {
		return nil, false
	}
	return append([]ssabuild.Origin(nil), call.Uses[1:]...), true
}

func nativeAnonymousArgumentRecords(fn *ssabuild.Function, work *workbudget.Budget) (map[int]*ssabuild.InstructionValues, bool) {
	if fn == nil || work != nil && work.CheckAlloc(int64(len(fn.Instructions)+1)*64) != nil {
		return nil, false
	}
	records := map[int]*ssabuild.InstructionValues{}
	for i := range fn.Instructions {
		if !nativeProofWork(work, 1) || records[int(fn.Instructions[i].PC)] != nil {
			return nil, false
		}
		records[int(fn.Instructions[i].PC)] = &fn.Instructions[i]
	}
	return records, true
}

func nativeAnonymousArgumentFrameTypes(obj *ClassObject, method *MemberInfo, ir *methodir.MethodIR, fn *ssabuild.Function, record *ssabuild.InstructionValues, descriptor string, metadata callbinding.Provider, work *workbudget.Budget) bool {
	parameters, _, err := callbinding.Descriptor(descriptor)
	if err != nil || record == nil || len(parameters) != len(record.Uses) || len(record.Before.Stack) < nativeMemberParameterWidth(parameters) {
		return false
	}
	words := newNativeMethodFrameWords(obj, method, ir, fn, work)
	if words == nil {
		return false
	}
	widening := newConstructorWideningQuery(metadata)
	slot := len(record.Before.Stack) - nativeMemberParameterWidth(parameters)
	for i, parameter := range parameters {
		if !nativeProofWork(work, 1) || slot >= len(record.Before.Stack) {
			return false
		}
		value := record.Before.Stack[slot]
		if callbinding.Reference(parameter) {
			if !words.reference(record.Uses[i], parameter, widening, fn) {
				return false
			}
		} else {
			switch parameter {
			case "Z", "B", "C", "S":
				if !words.argument(value, record.Uses[i], parameter) {
					return false
				}
			case "I":
				if value.Kind != frametransfer.Int {
					return false
				}
			case "F":
				if value.Kind != frametransfer.Float {
					return false
				}
			case "J", "D":
				expected := frametransfer.Long
				if parameter == "D" {
					expected = frametransfer.Double
				}
				if value.Kind != expected || slot+1 >= len(record.Before.Stack) || record.Before.Stack[slot+1].Kind != frametransfer.TailOf(value).Kind {
					return false
				}
			default:
				return false
			}
		}
		if parameter == "J" || parameter == "D" {
			slot += 2
		} else {
			slot++
		}
	}
	return slot == len(record.Before.Stack)
}

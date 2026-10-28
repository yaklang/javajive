package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"reflect"
)

// BALOAD is shared by byte[] and boolean[]. Only the original reference
// producers and complete frozen reaching definitions can distinguish them;
// an inferred JavaArrayMember type or a mutable local's Val cannot do so.
type byteArrayProvenance struct {
	d         *Decompiler
	decoded   map[*OpCode]bool
	ambiguous map[values.JavaValue]bool
	remaining int
	original  map[*OpCode]bool
}

func (d *Decompiler) byteArrayProvenance() *byteArrayProvenance {
	if d == nil || d.semanticCFG == nil || d.semanticCFG.Err != nil || len(d.semanticCFG.Nodes) > 8192 || len(d.opCodes) > 8192 {
		return nil
	}
	n := len(d.opCodes) + len(d.semanticCFG.Nodes)
	if d.Work != nil && (d.Work.Charge(workbudget.CounterGraphScans, int64(n)) != nil || d.Work.CheckAlloc(int64(n)*128) != nil) {
		return nil
	}
	p := &byteArrayProvenance{d: d, decoded: map[*OpCode]bool{}, ambiguous: map[values.JavaValue]bool{}, remaining: 65536, original: map[*OpCode]bool{}}
	for _, op := range d.semanticCFG.Nodes {
		if op == nil || op.Instr == nil || p.original[op] {
			return nil
		}
		p.original[op] = true
	}
	owners := map[values.JavaValue]*OpCode{}
	for _, op := range d.opCodes {
		if op == nil || op.Instr == nil {
			return nil
		}
		p.decoded[op] = true
		if len(op.stackProduced) == 1 {
			cell := op.stackProduced[0]
			p.ambiguous[cell] = p.ambiguous[cell] || owners[cell] != nil
			owners[cell] = op
		}
	}
	return p
}

func (p *byteArrayProvenance) charge(n int) bool {
	p.remaining -= n
	return p.remaining >= 0 && (p.d.Work == nil || p.d.Work.Charge(workbudget.CounterGraphScans, int64(n)) == nil)
}

func (p *byteArrayProvenance) known(op *OpCode) bool {
	if p == nil || op == nil || op.Instr == nil || op.IsCustom || !p.decoded[op] || !p.original[op] {
		return false
	}
	_, ok := p.d.semanticCFG.order[op]
	return ok
}

func originalReferenceLocalPacket(op *OpCode, load bool) bool {
	if op == nil || op.Instr == nil {
		return false
	}
	code := op.Instr.OpCode
	generic := OP_ASTORE
	valid := isReferenceStoreOpcode(code)
	if load {
		generic = OP_ALOAD
		valid = isReferenceLoadOpcode(code)
	}
	if !valid {
		return false
	}
	if code != generic {
		return !op.IsWide && len(op.Data) == 0
	}
	width := 1
	if op.IsWide {
		width = 2
	}
	return len(op.Data) == width
}

func (p *byteArrayProvenance) byteParameter(slot int) bool {
	ctx := p.d.FunctionContext
	if ctx == nil || !p.charge(len(ctx.CurrentMethodDesc)+1) {
		return false
	}
	args, _, err := callbinding.Descriptor(ctx.CurrentMethodDesc)
	if err != nil {
		return false
	}
	index := 0
	if !ctx.IsStatic {
		index = 1
	}
	for _, arg := range args {
		if index == slot {
			return arg == "[B"
		}
		index++
		if arg == "J" || arg == "D" {
			index++
		}
	}
	return false
}

// Traverse each reference definition at most once per proof. A closed copy
// cycle needs a genuine descriptor/allocation seed, and every incoming arm
// must resolve to byte[]. Unknown, null-only, boolean[] and mixed roots reject.
func (p *byteArrayProvenance) proves(initial values.JavaValue) bool {
	if p == nil {
		return false
	}
	pending := []values.JavaValue{initial}
	seen := map[*OpCode]bool{}
	seed := false
	for len(pending) > 0 {
		if !p.charge(1) {
			return false
		}
		cell := values.OriginalStackLifetimeUse(pending[len(pending)-1])
		pending = pending[:len(pending)-1]
		if cell == nil || p.ambiguous[cell] || reflect.ValueOf(cell).Kind() == reflect.Ptr && reflect.ValueOf(cell).IsNil() {
			return false
		}
		op := p.d.stackValueProducers[cell]
		if !p.known(op) || len(op.stackProduced) != 1 || op.stackProduced[0] != cell {
			return false
		}
		if seen[op] {
			continue
		}
		seen[op] = true
		if originalReferenceLocalPacket(op, true) {
			slot := GetRetrieveIdx(op)
			defs, entry := p.d.semanticCFG.ReachingDefinitions(op, slot)
			if p.d.semanticCFG.Err != nil || !p.charge(len(defs)+1) || !entry && len(defs) == 0 {
				return false
			}
			if entry {
				if !p.byteParameter(slot) {
					return false
				}
				seed = true
			}
			for _, def := range defs {
				if !p.known(def) || !originalReferenceLocalPacket(def, false) || GetStoreIdx(def) != slot || len(def.stackConsumed) != 1 {
					return false
				}
				pending = append(pending, def.stackConsumed[0])
			}
			continue
		}
		if op.IsWide {
			return false
		}
		switch op.Instr.OpCode {
		case OP_GETFIELD, OP_GETSTATIC:
			if len(op.Data) != 2 || p.d.constantPoolGetter == nil {
				return false
			}
			member, ok := p.d.constantPoolGetter(int(Convert2bytesToInt(op.Data))).(*values.JavaClassMember)
			if !ok || member == nil || member.Description != "[B" || !p.charge(len(member.Name)+len(member.Member)+3) {
				return false
			}
			valid := false
			switch field := safeSeedOperand(cell).(type) {
			case *values.RefMember:
				valid = field != nil && op.Instr.OpCode == OP_GETFIELD && len(op.stackConsumed) == 1 && field.OriginalInstanceFieldRead(int(op.CurrentOffset), member.Name, member.Member, member.Description)
			case *values.JavaClassMember:
				valid = field != nil && op.Instr.OpCode == OP_GETSTATIC && len(op.stackConsumed) == 0 && field.OriginalStaticFieldRead(int(op.CurrentOffset), member.Name, member.Member, member.Description)
			}
			if !valid {
				return false
			}
		case OP_INVOKESTATIC, OP_INVOKEVIRTUAL, OP_INVOKESPECIAL, OP_INVOKEINTERFACE:
			width := 2
			if op.Instr.OpCode == OP_INVOKEINTERFACE {
				width = 4
			}
			if len(op.Data) != width || width == 4 && (op.Data[2] == 0 || op.Data[3] != 0) || p.d.constantPoolGetter == nil {
				return false
			}
			member, ok := p.d.constantPoolGetter(int(Convert2bytesToInt(op.Data[:2]))).(*values.JavaClassMember)
			if !ok || member == nil {
				return false
			}
			call := p.d.invokeFuncCall[op]
			if call == nil || member.Description != call.Descriptor || safeSeedOperand(cell) != call || !call.HasOriginPC || call.OriginPC != int(op.CurrentOffset) || !p.charge(len(call.Descriptor)+1) {
				return false
			}
			_, ret, err := callbinding.Descriptor(call.Descriptor)
			if err != nil || ret != "[B" {
				return false
			}
		case OP_NEWARRAY:
			if len(op.Data) != 1 || op.Data[0] != 8 || len(op.stackConsumed) != 1 {
				return false
			}
			array, ok := safeSeedOperand(cell).(*values.NewExpression)
			if !ok || array == nil || !array.HasOriginPC || array.OriginPC != int(op.CurrentOffset) {
				return false
			}
		default:
			return false
		}
		seed = true
	}
	return seed
}

func (p *byteArrayProvenance) numericRead(op *OpCode) *values.JavaArrayMember {
	if !p.known(op) || op.Instr.OpCode != OP_BALOAD || op.IsWide || len(op.Data) != 0 || len(op.stackProduced) != 1 || len(op.stackConsumed) != 2 {
		return nil
	}
	x, ok := safeSeedOperand(op.stackProduced[0]).(*values.JavaArrayMember)
	if !ok || x == nil || !x.HasOriginPC || x.OriginPC != int(op.CurrentOffset) {
		return nil
	}
	array := values.OriginalStackLifetimeUse(op.stackConsumed[1])
	index := values.OriginalStackLifetimeUse(op.stackConsumed[0])
	if array == nil || index == nil || values.OriginalStackLifetimeUse(x.Object) != array || values.OriginalStackLifetimeUse(x.Index) != index || !p.proves(array) {
		return nil
	}
	return x
}

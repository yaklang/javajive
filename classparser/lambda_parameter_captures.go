package javaclassparser

import (
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// LambdaToMethod prefixes a static implementation's SAM parameters with its
// captured source variables. Descriptor words and source binders are separate
// domains: category-2 captures consume two slots, but one MethodParameters row.
// Admit unchanged original outer formals, not a same-named local, field read,
// expression, implicit receiver, or guessed alias. The source callback and the
// completed enclosing render must separately consume these exact identities.
func (c *ClassObjectDumper) native8LambdaParameterCaptures(p *lambdaParameterMetadata, parentDescriptor string, parentNames []string, work *workbudget.Budget) bool {
	if c == nil || c.obj == nil || p == nil || p.object != c.obj || p.factory == nil || !p.factory.closed() || len(p.names) != len(p.parameters) || len(p.flags) != len(p.parameters) || len(p.captureTypes) > 64 || len(p.captureTypes) > len(p.parameters) || !slices.Equal(p.captureTypes, p.parameters[:len(p.captureTypes)]) {
		return false
	}
	_, result, err := callbinding.Descriptor(p.descriptor)
	if err != nil {
		return false
	}
	p.samDescriptor = "(" + strings.Join(p.parameters[len(p.captureTypes):], "") + ")" + result
	if len(p.captureTypes) == 0 {
		return true
	}
	parentTypes, _, err := callbinding.Descriptor(parentDescriptor)
	if err != nil || len(parentNames) != len(parentTypes) || work != nil && work.CheckAlloc(int64(len(p.factory.code.Code))*48+int64(len(parentTypes))*128) != nil {
		return false
	}
	typesBySlot, namesBySlot := map[int]string{}, map[int]string{}
	slot := 0
	if p.factory.method.AccessFlags&8 == 0 {
		slot = 1 // THIS is not an ordinary captured source formal.
	}
	for i, descriptor := range parentTypes {
		typesBySlot[slot], namesBySlot[slot] = descriptor, parentNames[i]
		slot++
		if descriptor == "J" || descriptor == "D" {
			slot++
		}
	}
	decoder := core.NewDecompiler(p.factory.code.Code, func(i int) values.JavaValue { return GetValueFromCP(c.obj.ConstantPool, i) })
	decoder.Work = work
	if decoder.ParseOpcode() != nil {
		return false
	}
	ops := constructorMotionOps(decoder)
	entries, known := nativeMemberLexicalControlEntries(decoder, p.factory.code, work)
	if !known {
		return false
	}
	end := -1
	for i, op := range ops {
		if !nativeProofWork(work, 1) {
			return false
		}
		if int(op.CurrentOffset) == p.factory.pc {
			end = i
		}
	}
	start := end - len(p.captureTypes)
	if start < 0 || end < 0 || ops[end].Instr.OpCode != core.OP_INVOKEDYNAMIC {
		return false
	}
	used := map[int]bool{}
	for i, descriptor := range p.captureTypes {
		op := ops[start+i]
		slot := core.GetRetrieveIdx(op)
		if !nativeProofWork(work, 1) || !constructorMotionLoad(op, descriptor) || typesBySlot[slot] != descriptor || namesBySlot[slot] != p.names[i] || p.flags[i] != 0x1010 || used[slot] {
			return false
		}
		used[slot] = true
		if descriptor == "J" || descriptor == "D" {
			used[slot+1] = true
		}
		p.captures = append(p.captures, &nativeEnumSelectorProducer{opcode: op.Instr.OpCode, pc: int(op.CurrentOffset), slot: slot, result: descriptor})
		// An enclosing SAM formal has a real source binder in this scope.
		// Forwarded physical capture-prefix words still need a separate
		// substitution/provenance proof; equal names do not supply that proof.
		if p.parent != nil && slot < p.parent.samSlot() {
			return false
		}
	}
	entry := sort.SearchInts(entries, int(ops[start].CurrentOffset)+1)
	return (entry == len(entries) || entries[entry] > p.factory.pc) && nativeEnumSelectorParametersUnchanged(ops, used)
}

// Called while the real factory's context is still current. Capture values are
// supplied by that invokedynamic callback, never recovered from source text.
// Retain its local-name map: ordinary header binding finishes later, before
// that factory is rendered. No source renderer runs here. Pending evaluation
// evidence cannot replace the final inline/source binding proof below.
func (c *ClassObjectDumper) lambdaParameterCaptureContext(p *lambdaParameterMetadata, request methodSourceRequest) (*class_context.ClassContext, []values.JavaValue, bool) {
	if p == nil || !p.factory.same(request.factory) || request.captureCount != len(p.captures) || len(c.nativeLambdaBodyCapturedValues) != len(p.captures) {
		return nil, nil, false
	}
	if len(p.captures) == 0 {
		return nil, nil, true
	}
	if c.FuncCtx == nil || c.CurrentMethod != p.factory.method || !nativeProofWork(c.Work, int64(len(p.captures))+1) || c.Work != nil && c.Work.CheckAlloc(int64(len(p.captures))*32+256) != nil {
		return nil, nil, false
	}
	context := *c.FuncCtx
	captures := slices.Clone(c.nativeLambdaBodyCapturedValues)
	for i, value := range captures {
		v, known := nativeMemberEnclosingUnpack(value, c.Work)
		ref, typed := v.(*values.JavaRef)
		if !known || !typed || ref == nil || ref.Id == nil || ref.CustomValue != nil || ref.StackVar != nil || ref.IsThis {
			return nil, nil, false
		}
		pc, index, original := ref.OriginalPendingDynamicOperand(ref.Val)
		seed, known := nativeMemberEnclosingUnpack(ref.Val, c.Work)
		parameter, typed := seed.(*values.JavaRef)
		if !original || pc != p.factory.pc || index != i || !known || !typed || parameter == nil {
			return nil, nil, false
		}
		slot, original := parameter.OriginalParameterSlot()
		if !original || slot != p.captures[i].slot {
			return nil, nil, false
		}
	}
	return &context, captures, true
}

func (p *lambdaParameterMetadata) samSlot() int {
	slot := 0
	for _, descriptor := range p.captureTypes {
		slot++
		if descriptor == "J" || descriptor == "D" {
			slot++
		}
	}
	return slot
}

// javac8 discovers captures by first lexical use in the body. Reordering those
// uses could change a physical implementation's ordered metadata even when an
// expression happens to commute. Check the sealed capture placeholders before
// substitution; this grants no operand effects or renderer equivalence.
func lambdaParameterCaptureOrder(code string, count int, work *workbudget.Budget) bool {
	if count < 0 || count > 64 || len(code) > 1<<20 || !nativeProofWork(work, int64(len(code))+1) || work != nil && work.CheckAlloc(64) != nil {
		return false
	}
	seen, next := [64]bool{}, 0
	for cursor := 0; cursor < len(code); {
		start := strings.IndexByte(code[cursor:], 0)
		if start < 0 {
			break
		}
		start += cursor + 1
		end := strings.IndexByte(code[start:], 0)
		if end < 0 {
			return false
		}
		end += start
		ordinal, prefix := strings.CutPrefix(code[start:end], "LCAP")
		index, err := strconv.Atoi(ordinal)
		if !prefix || err != nil || index < 0 || index >= count || ordinal != strconv.Itoa(index) {
			return false
		}
		if !seen[index] {
			if index != next {
				return false
			}
			seen[index], next = true, next+1
		}
		cursor = end + 1
	}
	return next == count
}

// The final enclosing source must use the original named parameter identities.
// A body receipt may exist earlier, but cannot publish source on its own.
func (b *lambdaParameterBinding) capturedNamesClosed(work *workbudget.Budget) bool {
	if b == nil || b.metadata == nil || len(b.captured) != len(b.metadata.captures) || !lambdaParameterCaptureOrder(b.arrow, len(b.captured), work) {
		return false
	}
	if len(b.captured) == 0 {
		return true
	}
	if b.captureContext == nil {
		return false
	}
	for i, value := range b.captured {
		v, known := nativeMemberEnclosingUnpack(value, work)
		ref, typed := v.(*values.JavaRef)
		if !known || !typed || ref == nil || ref.Id == nil || !nativeProofWork(work, 1) {
			return false
		}
		pc, index, original := ref.OriginalInlinedDynamicOperandWitness(ref.Val)
		if !original || pc != b.metadata.factory.pc || index != i || !nativeLambdaLocalCaptureValue(b.metadata.captures[i], value, pc, i, b.captureContext, work) || ref.String(b.captureContext) != b.metadata.names[i] {
			return false
		}
	}
	return true
}

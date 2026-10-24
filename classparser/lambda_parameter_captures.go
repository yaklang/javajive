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

// LambdaToMethod prefixes an implementation's SAM parameters with its
// captured source variables. Descriptor words and source binders are separate
// domains: category-2 captures consume two slots, but one MethodParameters row.
// Admit unchanged outer formals or typed effectively-final local definitions.
// A local's original reaching STORE and final source binder are separate proof
// obligations. An implicit receiver has its own slot-zero identity, outside
// these physical binders. The callback and final render must consume these IDs.
func (c *ClassObjectDumper) native8LambdaParameterCaptures(p *lambdaParameterMetadata, parentDescriptor string, parentNames []string, work *workbudget.Budget) bool {
	if c == nil || c.obj == nil || p == nil || p.object != c.obj || p.factory == nil || !p.factory.closed() || len(p.names) != len(p.parameters) || len(p.flags) != len(p.parameters) || len(p.captureTypes) > 64 || len(p.captureTypes) > len(p.parameters) || !slices.Equal(p.captureTypes, p.parameters[:len(p.captureTypes)]) {
		return false
	}
	_, result, err := callbinding.Descriptor(p.descriptor)
	if err != nil {
		return false
	}
	p.samDescriptor = "(" + strings.Join(p.parameters[len(p.captureTypes):], "") + ")" + result
	if len(p.captureTypes) == 0 && p.receiverWords == 0 {
		return true
	}
	parentTypes, _, err := callbinding.Descriptor(parentDescriptor)
	if err != nil || len(parentNames) != len(parentTypes) || work != nil && work.CheckAlloc(int64(len(p.factory.code.Code))*48+int64(len(parentTypes))*128) != nil {
		return false
	}
	typesBySlot, namesBySlot := map[int]string{}, map[int]string{}
	slot := 0
	if p.factory.method.AccessFlags&8 == 0 {
		typesBySlot[0] = "L" + c.obj.GetClassName() + ";"
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
	if end < 0 || ops[end].Instr.OpCode != core.OP_INVOKEDYNAMIC {
		return false
	}
	// A logical capture is one descriptor port, not necessarily one opcode.
	// Hidden anonymous captures are the already-proved THIS/GETFIELD packet.
	loads := make([]int, len(p.captureTypes))
	fields := make([]*nativeEnumSelectorProducer, len(p.captureTypes))
	start := end
	for i := len(p.captureTypes) - 1; i >= 0; i-- {
		start--
		if start < 0 {
			return false
		}
		loads[i] = start
		if view := c.nativeAnonymousLambdaCurrent; view != nil && p.factory.method.AccessFlags&8 == 0 {
			fields[i] = nativeAnonymousLambdaCaptureField(c.obj, view.lambdaContext.anonymousCaptures, ops, start, p.captureTypes[i], work)
		}
		if fields[i] != nil {
			if !c.lambdaParameterAnonymousFieldClosed(p, fields[i], p.names[i], work) {
				return false
			}
			start--
		} else if !constructorMotionLoad(ops[start], p.captureTypes[i]) {
			return false
		}
	}
	start -= p.receiverWords
	if start < 0 {
		return false
	}
	var localReads map[int]*nativeEnumLocalRead
	var localFlow *nativeEnumParameterFlow
	localDomain := nativeLocalReferenceDomain{descriptors: map[int]string{}}
	used, parameterWords := map[int]bool{}, map[int]bool{}
	if p.receiverWords != 0 {
		op := ops[start]
		if p.receiverWords != 1 || p.factory.method.AccessFlags&8 != 0 || !constructorMotionLoad(op, "L"+c.obj.GetClassName()+";") || core.GetRetrieveIdx(op) != 0 {
			return false
		}
		p.receiver = &nativeEnumSelectorProducer{opcode: op.Instr.OpCode, pc: int(op.CurrentOffset), slot: 0, result: "L" + c.obj.GetClassName() + ";"}
		if !lambdaParameterReceiverInitialized(p, work) {
			return false
		}
		used[0], parameterWords[0] = true, true
	}
	for i, descriptor := range p.captureTypes {
		if fields[i] == nil && callbinding.Reference(descriptor) {
			localDomain.descriptors[int(ops[loads[i]].CurrentOffset)] = descriptor
		}
	}
	for i, descriptor := range p.captureTypes {
		if p.flags[i] != 0x1010 || !nativeProofWork(work, 1) {
			return false
		}
		if fields[i] != nil {
			p.captures = append(p.captures, fields[i])
			used[0], parameterWords[0] = true, true
			continue
		}
		op := ops[loads[i]]
		readSlot := core.GetRetrieveIdx(op)
		if !constructorMotionLoad(op, descriptor) || used[readSlot] || (descriptor == "J" || descriptor == "D") && used[readSlot+1] {
			return false
		}
		operand := &nativeEnumSelectorProducer{opcode: op.Instr.OpCode, pc: int(op.CurrentOffset), slot: readSlot, result: descriptor}
		if typesBySlot[readSlot] == descriptor && namesBySlot[readSlot] == p.names[i] {
			parameterWords[readSlot] = true
			if descriptor == "J" || descriptor == "D" {
				parameterWords[readSlot+1] = true
			}
		} else {
			// An entry word (including a category-2 tail) cannot impersonate
			// a local. This domain also excludes lexical name collisions.
			if readSlot < slot || slices.Contains(parentNames, p.names[i]) {
				return false
			}
			if localReads == nil {
				localFlow = nativeEnumParameterOriginalFlow(decoder, p.factory.code, work)
				if localFlow == nil {
					return false
				}
				reader := *c
				reader.Work = work
				localDomain.metadata = reader.buildInvocationMetadata()
				var known bool
				localReads, known = reader.nativeTypedLocalReads(p.factory.method, p.factory.code, localFlow, typesBySlot, true, localDomain)
				if !known {
					return false
				}
			}
			read := localReads[int(op.CurrentOffset)]
			if read == nil || read.slot != readSlot || read.descriptor != descriptor || !nativeLambdaLocalCaptureAtFactory(ops, localFlow, read, p.factory.pc, work) {
				return false
			}
			operand.local = read
		}
		used[readSlot] = true
		if descriptor == "J" || descriptor == "D" {
			used[readSlot+1] = true
		}
		p.captures = append(p.captures, operand)
		// An enclosing SAM formal has a real source binder. A physical
		// capture-prefix word instead needs its exact parent capture port;
		// its actual source binding is reopened at completed emission below.
		if p.parent != nil && readSlot < p.parent.samSlot() {
			if _, known := lambdaForwardedCaptureIndex(p, i); !known {
				return false
			}
		}
	}
	entry := sort.SearchInts(entries, int(ops[start].CurrentOffset)+1)
	return (entry == len(entries) || entries[entry] > p.factory.pc) && nativeEnumSelectorParametersUnchanged(ops, parameterWords)
}

// Called while the real factory's context is still current. Capture values are
// supplied by that invokedynamic callback, never recovered from source text.
// Retain its local-name map: ordinary header binding finishes later, before
// that factory is rendered. No source renderer runs here. Pending evaluation
// evidence cannot replace the final inline/source binding proof below.
func (c *ClassObjectDumper) lambdaParameterCaptureContext(p *lambdaParameterMetadata, request methodSourceRequest) (*class_context.ClassContext, []values.JavaValue, bool) {
	if p == nil || !p.factory.same(request.factory) || request.captureCount != p.factoryCaptureCount() || len(c.nativeLambdaBodyCapturedValues) != p.factoryCaptureCount() {
		return nil, nil, false
	}
	if p.factoryCaptureCount() == 0 {
		return nil, nil, true
	}
	if c.FuncCtx == nil || c.CurrentMethod != p.factory.method || !nativeProofWork(c.Work, int64(len(p.captures))+1) || c.Work != nil && c.Work.CheckAlloc(int64(len(p.captures))*32+256) != nil {
		return nil, nil, false
	}
	context := *c.FuncCtx
	if p.receiverWords != 0 && !lambdaParameterReceiverPendingClosed(p, c.nativeLambdaBodyCapturedValues[0], &context, c.Work) {
		return nil, nil, false
	}
	captures := slices.Clone(c.nativeLambdaBodyCapturedValues[p.receiverWords:])
	for i, value := range captures {
		v, known := nativeMemberEnclosingUnpack(value, c.Work)
		ref, typed := v.(*values.JavaRef)
		if !known || !typed || ref == nil || ref.Id == nil || ref.CustomValue != nil || ref.StackVar != nil || ref.IsThis {
			return nil, nil, false
		}
		pc, index, original := ref.OriginalPendingDynamicOperand(ref.Val)
		seed, known := nativeMemberEnclosingUnpack(ref.Val, c.Work)
		if !original || pc != p.factory.pc || index != i+p.receiverWords || !known {
			return nil, nil, false
		}
		if lambdaParameterFieldCapture(p.captures[i]) {
			if !c.lambdaParameterAnonymousFieldClosed(p, p.captures[i], p.names[i], c.Work) || !nativeAnonymousLambdaCaptureSource(p.captures[i], seed, pc, index, &context, c.Work) {
				return nil, nil, false
			}
			continue
		}
		parameter, typed := seed.(*values.JavaRef)
		if !typed || parameter == nil {
			return nil, nil, false
		}
		if p.captures[i].local == nil {
			slot, original := parameter.OriginalParameterSlot()
			if !original || slot != p.captures[i].slot {
				return nil, nil, false
			}
		} else if parameter.Id == nil || parameter.IsParam || parameter.IsThis || parameter.CustomValue != nil || parameter.StackVar != nil {
			return nil, nil, false
		}
	}
	if !c.recordLambdaLocalParameterNameDemands(p, captures) {
		return nil, nil, false
	}
	return &context, captures, true
}

func (p *lambdaParameterMetadata) samSlot() int {
	slot := p.receiverWords
	for _, descriptor := range p.captureTypes {
		slot++
		if descriptor == "J" || descriptor == "D" {
			slot++
		}
	}
	return slot
}

func (p *lambdaParameterMetadata) factoryCaptureCount() int {
	return p.receiverWords + len(p.captures)
}

func (p *lambdaParameterMetadata) captureOrderClosed(code string, work *workbudget.Budget) bool {
	return p.factoryCaptureCount() <= 64 && lambdaSourceCaptureOrderFrom(code, len(p.captures), p.receiverWords, work)
}

// javac8 discovers captures by first lexical use in the body. Reordering those
// uses could change a physical implementation's ordered metadata even when an
// expression happens to commute. Check the sealed capture placeholders before
// substitution; this grants no operand effects or renderer equivalence.
func lambdaParameterCaptureOrder(code string, count int, work *workbudget.Budget) bool {
	if count > 64 || work != nil && work.CheckAlloc(64) != nil {
		return false
	}
	return lambdaSourceCaptureOrder(code, count, work)
}

// The static-body contract also applies when implementation parameter tables
// are absent. Its physical arity is already bounded by the JVM descriptor;
// this does not expand the narrower native metadata/capture proof domain.
func lambdaSourceCaptureOrder(code string, count int, work *workbudget.Budget) bool {
	return lambdaSourceCaptureOrderFrom(code, count, 0, work)
}

func lambdaSourceCaptureOrderFrom(code string, count, offset int, work *workbudget.Budget) bool {
	if offset < 0 || offset > 1 || count < 0 || count > 255-offset || len(code) > 1<<20 || !nativeProofWork(work, int64(len(code))+1) || work != nil && work.CheckAlloc(int64(count)) != nil {
		return false
	}
	seen, next := make([]bool, count), 0
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
		if !prefix || err != nil || index < 0 || index >= count+offset || ordinal != strconv.Itoa(index) {
			return false
		}
		if index < offset {
			cursor = end + 1 // implicit receiver is not a discovered capture
			continue
		}
		index -= offset
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
	return b.capturedNamesClosedInScope(work, map[*lambdaParameterBinding]bool{})
}

func (b *lambdaParameterBinding) capturedNamesClosedInScope(work *workbudget.Budget, active map[*lambdaParameterBinding]bool) bool {
	if b == nil || b.metadata == nil {
		return false
	}
	if active[b] || len(active) >= methodSourceLambdaDepthLimit || !nativeProofWork(work, 1) {
		return false
	}
	active[b] = true
	defer delete(active, b)
	if b.hasForwardedCaptures() && (!b.forwardingEmissionClosed(work) || !b.forwardParent.capturedNamesClosedInScope(work, active)) {
		return false
	}
	for i, operand := range b.metadata.captures {
		_, forwarded := lambdaForwardedCaptureIndex(b.metadata, i)
		if operand != nil && operand.local != nil || lambdaParameterFieldCapture(operand) || forwarded {
			if b.metadata.factory == nil || !b.captureEmission.closed() || b.captureEmission.body == nil || b.captureEmission.body.member != b.metadata.factory.method || len(b.captureDeclarationIDs) != len(b.captured) || len(b.captureSourceIDs) != len(b.captured) {
				return false
			}
			for i, capture := range b.captured {
				v, known := nativeMemberEnclosingUnpack(capture, work)
				ref, typed := v.(*values.JavaRef)
				if !known || !typed || ref == nil || ref.Id == nil || ref.Id != b.captureDeclarationIDs[i] {
					return false
				}
				if lambdaParameterFieldCapture(b.metadata.captures[i]) {
					if b.captureSourceIDs[i] != nil {
						return false
					}
					continue
				}
				source, known := lambdaCaptureSourceReference(ref, b.metadata.factory.pc, i+b.metadata.receiverWords, work)
				if !known || source.Id != b.captureSourceIDs[i] {
					return false
				}
			}
			break
		}
	}
	return b.capturedNamesInContextClosed(b.captureContext, work)
}

func (b *lambdaParameterBinding) capturedNamesInContextClosed(context *class_context.ClassContext, work *workbudget.Budget) bool {
	if b == nil || b.metadata == nil || len(b.captured) != len(b.metadata.captures) || !b.metadata.captureOrderClosed(b.arrow, work) {
		return false
	}
	if b.metadata.receiverWords != 0 && !lambdaParameterReceiverSourceClosed(b.metadata, b.receiver, context, work) {
		return false
	}
	if len(b.captured) == 0 {
		return true
	}
	if context == nil {
		return false
	}
	for i, value := range b.captured {
		v, known := nativeMemberEnclosingUnpack(value, work)
		ref, typed := v.(*values.JavaRef)
		if !known || !typed || ref == nil || ref.Id == nil || !nativeProofWork(work, 1) {
			return false
		}
		pc, index, original := ref.OriginalInlinedDynamicOperandWitness(ref.Val)
		if !original && b.metadata.captures[i].local != nil {
			pc, index, original = ref.OriginalDynamicOperandWitness(ref.Val)
		}
		if !original || pc != b.metadata.factory.pc || index != i+b.metadata.receiverWords {
			return false
		}
		if !nativeLambdaLocalCaptureValue(b.metadata.captures[i], value, pc, index, context, work) {
			if b.metadata.captures[i].local == nil || !b.retainedCaptureSourceClosed(work) || !nativeLambdaReusedLocalSource(b.metadata.captures[i], b.captureBody, index, work) {
				return false
			}
		}
		if lambdaParameterFieldCapture(b.metadata.captures[i]) && !nativeAnonymousLambdaCaptureSource(b.metadata.captures[i], value, pc, index, context, work) {
			return false
		}
		if _, forwarded := lambdaForwardedCaptureIndex(b.metadata, i); forwarded {
			if !b.forwardedCaptureEntryClosed(i, ref, context, work) {
				return false
			}
		} else if ref.String(context) != b.metadata.names[i] {
			return false
		}
		if b.metadata.captures[i].local != nil {
			source, known := lambdaCaptureSourceReference(ref, pc, index, work)
			if !known || context.LocalNames[source.Id] != b.metadata.names[i] {
				return false
			}
		}
	}
	return true
}

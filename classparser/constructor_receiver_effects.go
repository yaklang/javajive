package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"github.com/yaklang/javajive/internal/workbudget"
	"sort"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// Values are logical JVM values, including category-2 width. Only the newly
// allocated receiver can carry receiver=true: publishing it into any other
// object, static field or opaque call is rejected. Therefore a receiver-free
// value loaded from a parameter/field/call cannot acquire an alias to it.
type constructorEffectValue struct {
	kind       byte
	knownInt   bool
	intWord    int32
	receiver   bool
	allocation int // Original NEW PC + 1, until its exact <init> completes.
}

// Stores of THIS into THIS's own nonvolatile storage do not publish it. Until
// a field-sensitive heap proof exists, reject their combination with any
// reference-valued read from THIS anywhere in the constructor chain. Facts are
// monotone across all branches and recursive delegations: a different branch
// or parent cannot hide a potentially receiver-valued alias behind a plain L.
type constructorSelfStorageProof struct {
	selfStored    bool
	referenceRead bool
}

func (p *constructorSelfStorageProof) closed() bool {
	return p != nil && !(p.selfStored && p.referenceRead)
}

// A source release is not a runtime pin. Where original platform code is
// supplied by the catalog, every catalogued runtime capable of running this
// source must admit the same movement. Actual caller-supplied bytes retain
// precedence. All attempts share the original bounded proof budget.
func (c *ClassObjectDumper) constructorCaptureChainDoesNotObserve(owner, descriptor string, writes map[string]bool, arguments ...constructorEffectValue) bool {
	if c == nil || c.obj == nil {
		return false
	}
	target := c.options.TargetSourceVersion
	if target == 0 {
		target = core.ClassMajorToSourceVersion(c.obj.MajorVersion)
	}
	remaining := 512
	var previous *constructorProfileEvidence
	prove := func(release int) (bool, bool) {
		d := *c
		d.options.TargetSourceVersion = release
		if !nativeProofWork(c.Work, 1) || c.Work != nil && c.Work.CheckAlloc(256) != nil {
			return false, false
		}
		evidence := &constructorProfileEvidence{originals: map[string][32]byte{}, eligible: true, rootName: c.obj.GetClassName(), rootSuper: c.obj.GetSupperClassName(), rootFlags: c.obj.AccessFlags}
		d.constructorProfileEvidence = evidence
		platformUsed := false
		d.foldSiblingResolver = func(name string) ([]byte, bool) {
			evidence.providerObserved()
			if c.foldSiblingResolver != nil {
				if raw, ok := c.foldSiblingResolver(name); ok {
					evidence.original(&d, name, raw)
					return raw, true
				}
			}
			if c.declarationResolver != nil {
				if raw, ok := c.declarationResolver(name); ok {
					evidence.original(&d, name, raw)
					return raw, true
				}
			}
			raw, ok := jdkConstructorClassBytes(name, release)
			platformUsed = platformUsed || ok
			if ok && name != "java/lang/Object" {
				evidence.eligible = false
			}
			return raw, ok
		}
		d.declarationResolver = nil // already consulted before the platform evidence
		d.FuncCtx = &class_context.ClassContext{}
		d.FuncCtx.InvocationMetadata = evidence.metadata(d.buildInvocationMetadata())
		d.constructorReceiverFinalizerSilent = d.constructorReceiverCannotObserveFinalization(&remaining)
		evidence.finalizerSilent = d.constructorReceiverFinalizerSilent
		if evidence.revalidates(&d, previous, &remaining) {
			previous = evidence
			return true, platformUsed
		}
		if evidence.inconsistent || remaining < 0 || d.checkWork() != nil {
			return false, platformUsed
		}
		aliases := &constructorSelfStorageProof{}
		evidence.transcript = nil
		evidence.inBody = true
		safe := d.constructorChainEffectsWithArguments(owner, descriptor, writes, map[string]bool{}, &remaining, 0, aliases, arguments) && aliases.closed() && !evidence.inconsistent
		evidence.inBody = false
		if safe {
			previous = evidence
		}
		return safe, platformUsed
	}
	if safe, platformUsed := prove(target); !safe {
		return false
	} else if !platformUsed {
		return true
	}
	// Metadata initialization above also initializes the supported release set.
	releases := []int{}
	for release := range jdkInvocationCatalog.profiles {
		if release > target {
			releases = append(releases, release)
		}
	}
	sort.Ints(releases)
	for _, release := range releases {
		if safe, _ := prove(release); !safe {
			return false
		}
	}
	return true
}

func constructorEffectType(desc string) constructorEffectValue {
	if len(desc) == 0 {
		return constructorEffectValue{}
	}
	kind := desc[0]
	if kind == '[' || kind == 'L' {
		kind = 'L'
	} else if kind == 'Z' || kind == 'B' || kind == 'C' || kind == 'S' {
		kind = 'I'
	}
	return constructorEffectValue{kind: kind}
}

func (v constructorEffectValue) width() int {
	if v.kind == 'J' || v.kind == 'D' {
		return 2
	}
	if v.kind == 'I' || v.kind == 'F' || v.kind == 'L' {
		return 1
	}
	return 0
}

func constructorEffectOriginalOperandFree(code *CodeAttribute, op *core.OpCode) bool {
	if code == nil || op == nil || op.Instr == nil || op.IsWide || len(op.Data) != 0 {
		return false
	}
	pc := int(op.CurrentOffset)
	return pc >= 0 && pc < len(code.Code) && int(code.Code[pc]) == op.Instr.OpCode
}

func (c *ClassObjectDumper) constructorMotionClass(owner string) (*ClassObject, bool) {
	if c.obj != nil && owner == c.obj.GetClassName() {
		// The caller's parsed object is not an authoritative resolver-byte
		// dependency. Its own constructor body must be reanalyzed per profile.
		if c.constructorProfileEvidence != nil && c.constructorProfileEvidence.inBody {
			c.constructorProfileEvidence.eligible = false
		}
		return c.obj, true
	}
	var raw []byte
	var ok bool
	if c.foldSiblingResolver != nil {
		raw, ok = c.foldSiblingResolver(owner)
	}
	if !ok && c.declarationResolver != nil {
		raw, ok = c.declarationResolver(owner)
	}
	if !ok {
		target := c.options.TargetSourceVersion
		if target == 0 {
			target = core.ClassMajorToSourceVersion(c.obj.MajorVersion)
		}
		raw, ok = jdkConstructorClassBytes(owner, target)
	}
	if !ok {
		return nil, false
	}
	e := c.constructorProfileEvidence
	certified := false
	if e != nil && e.inBody && e.eligible && !e.inconsistent {
		_, certified = e.originals[owner]
	}
	var retained, itemsBefore int64
	if certified {
		// Own the provider snapshot. A later callback cannot mutate this parsed
		// body's byte slices through a buffer returned by an earlier lookup.
		var err error
		retained, err = workbudget.CheckedProduct(int64(len(raw)), 4)
		max := int64(^uint64(0) >> 1)
		if err != nil || e.parsedRetention > max-e.bodyRetention || retained > max-e.parsedRetention-e.bodyRetention {
			return nil, false
		}
		if !nativeProofWork(c.Work, 1) || c.Work != nil && (c.Work.Charge(workbudget.CounterReadBytes, int64(len(raw))) != nil || c.Work.CheckAlloc(e.parsedRetention+e.bodyRetention+retained) != nil) {
			return nil, false
		}
		raw = bytes.Clone(raw)
		itemsBefore = c.Work.Used(workbudget.CounterParseItems)
	}
	obj, err := c.parseResolved(raw)
	if err != nil || obj == nil || obj.GetClassName() != owner {
		return nil, false
	}
	if certified {
		// Parser items are already a logical work bound (not a general heap quota).
		// Include their retained metadata and the owned bytes cumulatively in the
		// existing intermediate-allocation guard, rather than one class at a time.
		metadata, err := workbudget.CheckedProduct(c.Work.Used(workbudget.CounterParseItems)-itemsBefore, 128)
		if err != nil || retained > int64(^uint64(0)>>1)-metadata-32 {
			return nil, false
		}
		retained += metadata + 32
		if e.parsedRetention > int64(^uint64(0)>>1)-retained-e.bodyRetention || c.Work != nil && c.Work.CheckAlloc(e.parsedRetention+retained+e.bodyRetention) != nil {
			return nil, false
		}
		if e.parsedOriginals == nil {
			e.parsedOriginals = map[*ClassObject]bool{}
		}
		e.parsedOriginals[obj] = true
		e.parsedRetention += retained
	}

	return obj, true
}

// A Fieldref may name a subclass while resolving to an ancestor's field. Bind
// the symbolic owner first, then search its original declaration hierarchy.
// Parent and child fields named this$0 must remain different storage locations.
func (c *ClassObjectDumper) constructorEffectField(obj *ClassObject, member *values.JavaClassMember, put bool, remaining *int) (string, bool) {
	if member == nil {
		return "", false
	}
	caller := obj.GetClassName()
	pkg := func(name string) string {
		if slash := strings.LastIndexByte(name, '/'); slash >= 0 {
			return name[:slash]
		}
		return ""
	}
	symbolic := false
	seen := map[string]bool{}
	for depth := 0; obj != nil && depth <= 16; depth++ {
		*remaining--
		name := obj.GetClassName()
		if *remaining < 0 || seen[name] {
			return "", false
		}
		seen[name] = true
		symbolic = symbolic || name == member.Name
		if symbolic {
			matches := 0
			var flags uint16
			for _, field := range obj.Fields {
				n, _ := obj.getUtf8(field.NameIndex)
				d, _ := obj.getUtf8(field.DescriptorIndex)
				if n == member.Member && d == member.Description {
					matches++
					flags = field.AccessFlags
				}
			}
			if matches != 0 {
				// Resolution must not introduce an IllegalAccessError after
				// initialization. Protected access is through this descendant;
				// private ancestor access needs a separate nestmate proof.
				accessible := name == caller || flags&0x0001 != 0 || flags&0x0004 != 0 || flags&0x0002 == 0 && pkg(name) == pkg(caller)
				writable := !put || flags&0x0010 == 0 || name == caller
				return name + "\x00" + member.Member + "\x00" + member.Description, matches == 1 && flags&(0x0008|0x0040) == 0 && accessible && writable
			}
		}
		var ok bool
		obj, ok = c.constructorMotionClass(obj.GetSupperClassName())
		if !ok {
			return "", false
		}
	}
	return "", false
}

// This effect domain admits receiver-independent computation, local aliases
// and original field/array effects through conditional control flow. Every
// normal exit must finish initialization; every reachable path must remain
// receiver-silent, including abrupt exits and nonterminating cyclic paths.
// Paths are memoized by PC and exact type/receiver state. Cyclic paths require
// a closed type/receiver invariant; differing integer facts widen to unknown. A normal
// Java exception after Object initialization can expose the receiver to a
// finalizer, even without an explicit publication. Opaque operations after
// initialization therefore require the separate closed-finalizer proof; all
// receiver publication and moved-storage observation remain forbidden.
func (c *ClassObjectDumper) constructorReceiverEffects(obj *ClassObject, code *CodeAttribute, ops []*core.OpCode, descriptor string, writes, active map[string]bool, remaining *int, depth int) bool {
	aliases := &constructorSelfStorageProof{}
	return c.constructorReceiverEffectsWithStorage(obj, code, ops, descriptor, writes, active, remaining, depth, aliases) && aliases.closed()
}

func (c *ClassObjectDumper) constructorReceiverEffectsWithStorage(obj *ClassObject, code *CodeAttribute, ops []*core.OpCode, descriptor string, writes, active map[string]bool, remaining *int, depth int, aliases *constructorSelfStorageProof, arguments ...constructorEffectValue) bool {
	return c.constructorReceiverBodyEffectsWithStorage(obj, code, ops, descriptor, writes, active, remaining, depth, aliases, false, arguments...)
}

// Constructors enter with an uninitialized receiver; a proved exact instance
// callee enters with the same initialized receiver. Both phases use the same
// field identities, monotone alias facts, loop invariants and shared budget.
// Callee returns are checked for publication, but deliberately carry no new
// scalar constants back to the caller. This prevents branch pruning from
// concealing effects on another return path or at a narrow JVM return boundary.
func (c *ClassObjectDumper) constructorReceiverBodyEffectsWithStorage(obj *ClassObject, code *CodeAttribute, ops []*core.OpCode, descriptor string, writes, active map[string]bool, remaining *int, depth int, aliases *constructorSelfStorageProof, initializedAtEntry bool, arguments ...constructorEffectValue) bool {
	params, ret, err := callbinding.Descriptor(descriptor)
	if err != nil || !initializedAtEntry && ret != "V" || code.MaxLocals == 0 {
		return false
	}
	locals := make([]constructorEffectValue, int(code.MaxLocals))
	locals[0] = constructorEffectValue{kind: 'L', receiver: true}
	slot := 1
	if len(arguments) != 0 && len(arguments) != len(params) {
		return false
	}
	for i, param := range params {
		value := constructorEffectType(param)
		if len(arguments) > 0 {
			if arguments[i].kind != value.kind || arguments[i].receiver || arguments[i].allocation != 0 {
				return false
			}
			value = arguments[i]
		}
		if value.width() == 0 || slot+value.width() > len(locals) {
			return false
		}
		locals[slot] = value
		slot += value.width()
	}
	offsets := map[int]int{}
	for index, op := range ops {
		if op != nil {
			offsets[int(op.CurrentOffset)] = index
		}
	}
	handlers, closedHandlers := constructorReceiverCatchAllHandlerTargets(code, ops, remaining, c.Work)
	if !closedHandlers || len(code.ExceptionTable) > 0 && (!initializedAtEntry || !c.constructorReceiverFinalizerSilent) {
		return false
	}
	target := func(op *core.OpCode) (int, bool) {
		delta := 0
		if len(op.Data) == 2 {
			delta = int(int16(binary.BigEndian.Uint16(op.Data)))
		} else if len(op.Data) == 4 && op.Instr.OpCode == core.OP_GOTO_W {
			delta = int(int32(binary.BigEndian.Uint32(op.Data)))
		} else {
			return 0, false
		}
		pc := int(op.CurrentOffset) + delta
		index, ok := offsets[pc]
		return index, ok
	}
	allocations := map[int]string{}
	memo := map[string]constructorEffectPathMemo{}
	// Only linear exceptional suffixes acquire a liveness projection. General
	// branches/cycles keep their original exact-frame memo and loop invariant.
	linearMasks := map[int][]bool{}
	linearMemo := map[string]uint64{}
	if evidence := c.constructorProfileEvidence; evidence != nil && evidence.inBody && evidence.eligible {
		for _, start := range handlers {
			if _, seen := linearMasks[start]; !seen {
				if c.Work != nil && c.Work.CheckAlloc(int64(len(linearMasks)+1)*32) != nil {
					return false
				}
				linearMasks[start] = constructorLinearHandlerLiveness(ops, start, len(locals), handlers, remaining, c.Work)
			}
		}
	}
	cyclicTargets := map[int]bool{}
	for i, op := range ops {
		if op == nil || op.Instr == nil {
			return false
		}
		opcode := op.Instr.OpCode
		if opcode == core.OP_GOTO || opcode == core.OP_GOTO_W || opcode >= core.OP_IFEQ && opcode <= core.OP_IF_ACMPNE || opcode == core.OP_IFNULL || opcode == core.OP_IFNONNULL {
			if !constructorEffectOriginalBranch(code, op) {
				return false
			}
			branch, ok := target(op)
			if !ok {
				return false
			}
			if branch <= i {
				cyclicTargets[branch] = true
			}
		}
	}
	for from, to := range handlers {
		if to <= from {
			cyclicTargets[to] = true
		}
	}
	activeFrames := map[int]*constructorEffectLoopFrame{}
	var walk func(int, []constructorEffectValue, []constructorEffectValue, bool) bool
	walk = func(start int, locals, stack []constructorEffectValue, initialized bool) (result bool) {
		if start < 0 || start >= len(ops) {
			return false
		}
		prior := activeFrames[start]
		if cyclicTargets[start] {
			// Retain frames only at original cyclic destinations. Their copies
			// and joins share the existing 512-unit chain budget, independent
			// of runtime iteration count; a large frame cannot bypass it.
			cost := len(locals) + len(stack) + 1
			*remaining -= cost
			if *remaining < 0 || !nativeProofWork(c.Work, int64(cost)) || c.Work != nil && c.Work.CheckAlloc(int64(cost)*32) != nil {
				return false
			}
		}
		if prior != nil {
			// A reached cycle is checked against an inductive receiver/type
			// invariant, never accepted merely because its PC was seen. Drop
			// changed scalar constants and recheck the whole body with both
			// branch arms; publication/read/failure checks remain unchanged.
			joined, changed, closed := constructorEffectLoopJoin(prior, locals, stack, initialized)
			if !closed {
				return false
			}
			if !changed {
				return true
			}
			locals, stack = joined.locals, joined.stack
		}
		if cyclicTargets[start] {
			activeFrames[start] = &constructorEffectLoopFrame{locals: append([]constructorEffectValue(nil), locals...), stack: append([]constructorEffectValue(nil), stack...), initialized: initialized}
			defer func() {
				if prior == nil {
					delete(activeFrames, start)
				} else {
					activeFrames[start] = prior
				}
			}()
		}
		key := constructorEffectFrameKey(start, locals, stack, initialized, nil, 0)
		evidence := c.constructorProfileEvidence
		var epoch uint64
		if evidence != nil {
			epoch = evidence.observationEpoch
		}
		if prior, known := memo[key]; known && (!prior.result || prior.stable && prior.epoch == epoch) {
			return prior.result
		}
		defer func() {
			// An exact-frame shortcut must not hide provider observations
			// either. A body that queried metadata is repeated; a proof that
			// made no such query is reusable only at the same epoch.
			stable := evidence == nil || evidence.observationEpoch == epoch && !evidence.inconsistent
			memo[key] = constructorEffectPathMemo{result: result, stable: stable, epoch: epoch}
		}()
		pop := func(kind byte) (constructorEffectValue, bool) {
			if len(stack) == 0 {
				return constructorEffectValue{}, false
			}
			v := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			return v, kind == 0 || v.kind == kind
		}
		for index := start; index < len(ops); index++ {
			op := ops[index]
			*remaining--
			if *remaining < 0 || !nativeProofWork(c.Work, 1) {
				return false
			}
			// Check before each instruction, including local-load fast paths and
			// RETURN. A malformed declared stack limit cannot certify movement.
			words := 0
			for _, v := range stack {
				words += v.width()
			}
			if words > int(code.MaxStack) {
				return false
			}
			if op == nil || op.Instr == nil {
				return false
			}
			opcode := op.Instr.OpCode
			if handler, covered := handlers[index]; covered {
				failedAllocation := 0
				if opcode == core.OP_INVOKESPECIAL {
					member := constructorMotionMember(obj, op, opcode)
					if member == nil {
						return false
					}
					if member.Member == "<init>" {
						args, _, err := callbinding.Descriptor(member.Description)
						receiverIndex := len(stack) - len(args) - 1
						if err != nil || receiverIndex < 0 {
							return false
						}
						failedAllocation = stack[receiverIndex].allocation
					}
				}
				evidence := c.constructorProfileEvidence
				projected := ""
				var epoch uint64
				if live := linearMasks[handler]; live != nil && evidence != nil && evidence.inBody && evidence.eligible && !evidence.inconsistent {
					cost := len(locals) + 2
					if !nativeProofWork(c.Work, int64(cost)) || c.Work != nil && c.Work.CheckAlloc(int64(cost)*24) != nil {
						return false
					}
					projected = constructorEffectFrameKey(handler, locals, []constructorEffectValue{{kind: 'L'}}, initialized, live, failedAllocation)
					epoch = evidence.observationEpoch
				}
				previous, known := linearMemo[projected]
				if projected != "" && known && previous == epoch {
					// Reuse the already checked exception frame without copying
					// locals or walking the same cleanup again. Wide shapes,
					// initialized state and the Throwable operand remain exact.
					*remaining--
					if *remaining < 0 || !nativeProofWork(c.Work, 1) {
						return false
					}
				} else {
					cost := len(locals) + 2
					*remaining -= cost
					if *remaining < 0 || !nativeProofWork(c.Work, int64(cost)) || c.Work != nil && c.Work.CheckAlloc(int64(cost)*32) != nil {
						return false
					}
					incoming := constructorEffectExceptionLocals(locals, failedAllocation)
					if !walk(handler, incoming, []constructorEffectValue{{kind: 'L'}}, initialized) {
						return false
					}
					// The first proof performs every original operation. A provider
					// attempt, metadata call or root absence query within it (or
					// since it) forbids reuse. Storage aliases stay monotone and
					// shared with that first proof; no simulated locals change.
					if projected != "" && evidence.eligible && !evidence.inconsistent && evidence.observationEpoch == epoch {
						*remaining--
						if *remaining < 0 || !nativeProofWork(c.Work, 1) || c.Work != nil && c.Work.CheckAlloc(int64(len(linearMemo)+1)*64+int64(len(projected))) != nil {
							return false
						}
						linearMemo[projected] = epoch
					}
				}
			}
			access := core.LocalAccessOf(opcode)
			if access.Read || access.Write {
				kind := byte(0)
				if opcode == core.OP_IINC {
					slot := core.GetRetrieveIdx(op)
					if slot < 0 || slot >= len(locals) || locals[slot].kind != 'I' {
						return false
					}
					if !op.IsWide && len(op.Data) != 2 || op.IsWide && len(op.Data) != 4 {
						return false
					}
					delta := int32(int8(op.Data[len(op.Data)-1]))
					if len(op.Data) == 4 {
						delta = int32(int16(core.Convert2bytesToInt(op.Data[2:])))
					}
					if locals[slot].knownInt {
						locals[slot].intWord += delta
					}
					continue
				}
				for category, k := range []byte{'I', 'J', 'F', 'D', 'L'} {
					if opcode == core.OP_ILOAD+category || opcode == core.OP_ISTORE+category || opcode >= core.OP_ILOAD_0+category*4 && opcode < core.OP_ILOAD_0+category*4+4 || opcode >= core.OP_ISTORE_0+category*4 && opcode < core.OP_ISTORE_0+category*4+4 {
						kind = k
					}
				}
				if kind == 0 {
					return false
				}
				if access.Read {
					slot := core.GetRetrieveIdx(op)
					if slot < 0 || slot+access.Width > len(locals) || locals[slot].kind != kind {
						return false
					}
					stack = append(stack, locals[slot])
				} else {
					v, ok := pop(kind)
					slot := core.GetStoreIdx(op)
					if !ok || slot < 0 || slot+v.width() > len(locals) {
						return false
					}
					// Overwriting either half invalidates the previous wide value.
					if slot > 0 && locals[slot-1].width() == 2 {
						locals[slot-1] = constructorEffectValue{}
					}
					if locals[slot].width() == 2 {
						locals[slot+1] = constructorEffectValue{}
					}
					locals[slot] = v
					if v.width() == 2 {
						locals[slot+1] = constructorEffectValue{}
					}
				}
				continue
			}
			switch {
			case opcode == core.OP_NOP:
				continue
			case opcode == core.OP_GETSTATIC || opcode == core.OP_PUTSTATIC:
				// Class initialization/linkage may fail, but no published THIS
				// can reside in a static field. Preserve the original operation;
				// after initialization, failure requires a closed finalizer.
				if initialized && !c.constructorReceiverFinalizerSilent {
					return false
				}
				member := constructorMotionMember(obj, op, opcode)
				if member == nil {
					return false
				}
				fields, _, err := callbinding.Descriptor("(" + member.Description + ")V")
				if err != nil || len(fields) != 1 {
					return false
				}
				if opcode == core.OP_GETSTATIC {
					stack = append(stack, constructorEffectType(fields[0]))
				} else {
					// An independent value may be published in the same original
					// position. Publishing THIS, including an alias, is forbidden.
					// The field/linkage failure boundary is unchanged and covered
					// by the same closed-finalizer requirement as GETSTATIC.
					pc := int(op.CurrentOffset)
					if op.IsWide || pc < 0 || pc+3 > len(code.Code) || code.Code[pc] != core.OP_PUTSTATIC || core.Convert2bytesToInt(code.Code[pc+1:pc+3]) != core.Convert2bytesToInt(op.Data) {
						return false
					}
					v, ok := pop(constructorEffectType(fields[0]).kind)
					if !ok || v.receiver || v.allocation != 0 {
						return false
					}
				}
			case opcode == core.OP_CHECKCAST || opcode == core.OP_INSTANCEOF:
				if len(op.Data) != 2 || initialized && !c.constructorReceiverFinalizerSilent {
					return false
				}
				if _, known := sourceBridgeClassName(obj, core.Convert2bytesToInt(op.Data)); !known {
					return false
				}
				v, ok := pop('L')
				if !ok || v.receiver || v.allocation != 0 {
					return false
				}
				if opcode == core.OP_INSTANCEOF {
					v = constructorEffectValue{kind: 'I'}
				}
				stack = append(stack, v)
			case opcode == core.OP_NEW:
				// A distinct allocation is not THIS. Preserve its identity through
				// DUP/local aliases until its original constructor initializes all
				// those aliases. Failure still needs the caller's finalizer proof.
				if len(op.Data) != 2 || initialized && !c.constructorReceiverFinalizerSilent {
					return false
				}
				owner, known := sourceBridgeClassName(obj, core.Convert2bytesToInt(op.Data))
				if !known || strings.HasPrefix(owner, "[") || owner == "" {
					return false
				}
				id := int(op.CurrentOffset) + 1
				allocations[id] = owner
				stack = append(stack, constructorEffectValue{kind: 'L', allocation: id})
			case opcode == core.OP_NEWARRAY || opcode == core.OP_ANEWARRAY:
				// Allocation and array-component linkage can fail, including
				// OOME. This is the same failure boundary as a receiver-free call;
				// the allocation never receives THIS or changes effect order.
				if initialized && !c.constructorReceiverFinalizerSilent {
					return false
				}
				if opcode == core.OP_NEWARRAY {
					if len(op.Data) != 1 || op.Data[0] < 4 || op.Data[0] > 11 {
						return false
					}
				} else {
					if len(op.Data) != 2 {
						return false
					}
					if _, known := sourceBridgeClassName(obj, core.Convert2bytesToInt(op.Data)); !known {
						return false
					}
				}
				if _, ok := pop('I'); !ok {
					return false
				}
				stack = append(stack, constructorEffectValue{kind: 'L'})
			case opcode == core.OP_ARRAYLENGTH || opcode >= core.OP_IALOAD && opcode <= core.OP_SALOAD || opcode >= core.OP_IASTORE && opcode <= core.OP_SASTORE:
				// The original independent array operation keeps its own bounds,
				// null, component-store and linkage failures. No moved capture can
				// be observed through it: THIS was not published into any array,
				// and neither the array operand nor a stored value may be THIS.
				// Only a closed finalizer licenses a failure after initialization.
				if !constructorEffectOriginalOperandFree(code, op) || initialized && !c.constructorReceiverFinalizerSilent {
					return false
				}
				store := opcode >= core.OP_IASTORE && opcode <= core.OP_SASTORE
				kind := byte('I')
				if opcode != core.OP_ARRAYLENGTH {
					base := core.OP_IALOAD
					if store {
						base = core.OP_IASTORE
					}
					kind = []byte{'I', 'J', 'F', 'D', 'L', 'I', 'I', 'I'}[opcode-base]
					if store {
						v, ok := pop(kind)
						if !ok || v.receiver || v.allocation != 0 {
							return false
						}
					}
					if _, ok := pop('I'); !ok {
						return false
					}
				}
				array, ok := pop('L')
				if !ok || array.receiver || array.allocation != 0 {
					return false
				}
				if !store {
					stack = append(stack, constructorEffectValue{kind: kind})
				}
			case opcode == core.OP_ACONST_NULL:
				stack = append(stack, constructorEffectValue{kind: 'L'})
			case opcode >= core.OP_ICONST_M1 && opcode <= core.OP_ICONST_5 || opcode == core.OP_BIPUSH || opcode == core.OP_SIPUSH:
				v, known := constructorOriginalIntLiteral(obj, op)
				if !known {
					return false
				}
				stack = append(stack, v)
			case opcode >= core.OP_LCONST_0 && opcode <= core.OP_DCONST_1:
				kind := byte('F')
				if opcode <= core.OP_LCONST_1 {
					kind = 'J'
				} else if opcode >= core.OP_DCONST_0 {
					kind = 'D'
				}
				stack = append(stack, constructorEffectValue{kind: kind})
			case opcode == core.OP_LDC || opcode == core.OP_LDC_W || opcode == core.OP_LDC2_W:
				cp := 0
				if len(op.Data) == 1 {
					cp = int(op.Data[0])
				} else if len(op.Data) == 2 {
					cp = int(core.Convert2bytesToInt(op.Data))
				}
				if cp <= 0 || cp > len(obj.ConstantPool) {
					return false
				}
				kind := byte(0)
				switch obj.ConstantPool[cp-1].(type) {
				case *ConstantIntegerInfo:
					kind = 'I'
				case *ConstantFloatInfo:
					kind = 'F'
				case *ConstantLongInfo:
					kind = 'J'
				case *ConstantDoubleInfo:
					kind = 'D'
				case *ConstantStringInfo, *ConstantClassInfo:
					kind = 'L'
				}
				if _, class := obj.ConstantPool[cp-1].(*ConstantClassInfo); class && initialized {
					// A missing class can fail linkage after Object initialization.
					// Do not move captures past that potentially finalizable failure.
					return false
				}
				if kind == 0 || (opcode == core.OP_LDC2_W) != (kind == 'J' || kind == 'D') {
					return false
				}
				v := constructorEffectValue{kind: kind}
				if kind == 'I' {
					literal, known := constructorOriginalIntLiteral(obj, op)
					if !known {
						return false
					}
					v = literal
				}
				stack = append(stack, v)
			case opcode >= core.OP_DUP && opcode <= core.OP_SWAP:
				if len(op.Data) != 0 {
					return false
				}
				var known bool
				stack, known = constructorEffectStackPermutation(stack, opcode)
				if !known {
					return false
				}
			case opcode == core.OP_POP || opcode == core.OP_POP2:
				v, ok := pop(0)
				if !ok {
					return false
				}
				if opcode == core.OP_POP && v.width() != 1 {
					return false
				}
				if opcode == core.OP_POP2 && v.width() == 1 {
					w, ok := pop(0)
					if !ok || w.width() != 1 {
						return false
					}
				}
			case opcode == core.OP_GETFIELD || opcode == core.OP_PUTFIELD:
				member := constructorMotionMember(obj, op, opcode)
				if member == nil {
					return false
				}
				typeOf := constructorEffectType(member.Description)
				params, _, err := callbinding.Descriptor("(" + member.Description + ")V")
				if err != nil || len(params) != 1 || typeOf.width() == 0 {
					return false
				}
				var stored constructorEffectValue
				if opcode == core.OP_PUTFIELD {
					value, ok := pop(typeOf.kind)
					if !ok {
						return false
					}
					if value.allocation != 0 || value.receiver && !initialized {
						// Uninitialized THIS is not a legal stored reference value.
						return false
					}
					stored = value
				}
				receiver, ok := pop('L')
				if !ok || receiver.allocation != 0 {
					return false
				}
				if !receiver.receiver {
					// No earlier operation has published THIS. An independent
					// input or initialized allocation cannot hide an alias to it.
					// Keep the original field operation and failure timing; after
					// Object initialization, failure needs the closed finalizer.
					if stored.receiver || initialized && !c.constructorReceiverFinalizerSilent {
						return false
					}
					if opcode == core.OP_GETFIELD {
						stack = append(stack, typeOf)
					}
					continue
				}
				aliases.selfStored = aliases.selfStored || stored.receiver
				field, known := c.constructorEffectField(obj, member, opcode == core.OP_PUTFIELD, remaining)
				if !known || writes[field] {
					return false
				}
				if opcode == core.OP_GETFIELD {
					if !initialized {
						return false
					}
					aliases.referenceRead = aliases.referenceRead || typeOf.kind == 'L'
					stack = append(stack, typeOf)
				}
			case opcode == core.OP_INVOKESPECIAL || opcode == core.OP_INVOKESTATIC || opcode == core.OP_INVOKEVIRTUAL || opcode == core.OP_INVOKEINTERFACE:
				member := constructorMotionMember(obj, op, opcode)
				if member == nil || member.Member == "<clinit>" {
					return false
				}
				args, result, err := callbinding.Descriptor(member.Description)
				if err != nil {
					return false
				}
				words := 1
				for _, arg := range args {
					words += constructorEffectType(arg).width()
				}
				if opcode == core.OP_INVOKEINTERFACE && (words > 255 || int(op.Data[2]) != words) {
					return false
				}
				actuals := make([]constructorEffectValue, len(args))
				for i := len(args) - 1; i >= 0; i-- {
					v, ok := pop(constructorEffectType(args[i]).kind)
					if !ok || v.receiver || v.allocation != 0 {
						return false
					}
					actuals[i] = v
				}
				var receiver constructorEffectValue
				if opcode != core.OP_INVOKESTATIC {
					var ok bool
					receiver, ok = pop('L')
					if !ok {
						return false
					}
				}
				if receiver.receiver && member.Member != "<init>" {
					if !initialized || !c.constructorReceiverFinalizerSilent {
						return false
					}
					value, known := c.constructorReceiverReadOnlyMethodWithStorage(obj, member, opcode, writes, remaining, aliases, actuals...)
					if !known {
						value, known = c.constructorReceiverClosedMethod(obj, member, opcode, writes, active, remaining, depth+1, aliases, actuals...)
					}
					if !known {
						return false
					}
					if result != "V" {
						stack = append(stack, value)
					}
					continue
				}
				if initialized && !c.constructorReceiverFinalizerSilent {
					return false
				}
				if member.Member == "<init>" {
					if receiver.allocation != 0 {
						if opcode != core.OP_INVOKESPECIAL || result != "V" || member.Name != allocations[receiver.allocation] {
							return false
						}
						// JVM initialization changes every alias of THIS allocation,
						// without changing the enclosing receiver's initialized state.
						for i := range locals {
							if locals[i].allocation == receiver.allocation {
								locals[i].allocation = 0
							}
						}
						for i := range stack {
							if stack[i].allocation == receiver.allocation {
								stack[i].allocation = 0
							}
						}
					} else {
						if initialized || opcode != core.OP_INVOKESPECIAL || result != "V" || !receiver.receiver || (member.Name != obj.GetClassName() && member.Name != obj.GetSupperClassName()) || !c.constructorChainEffectsWithArguments(member.Name, member.Description, writes, active, remaining, depth+1, aliases, actuals) {
							return false
						}
						initialized = true
					}
				} else {
					// Receiver-free calls keep external effects in the same order.
					// Before Object initialization, a throw cannot expose this via
					// finalization. Afterward, require the separate closed-receiver
					// proof that its original finalizer is unobservable.
					// THIS arguments/receiver are rejected; results cannot alias
					// THIS because no earlier operation has published it.
					if receiver.receiver || receiver.allocation != 0 {
						return false
					}
					if result != "V" {
						stack = append(stack, constructorEffectType(result))
					}
				}
			case opcode >= core.OP_IADD && opcode <= core.OP_DREM:
				kind := []byte{'I', 'J', 'F', 'D'}[(opcode-core.OP_IADD)%4]
				if kind == 'I' || kind == 'J' {
					if opcode >= core.OP_IDIV {
						// Division/remainder preserve their original zero-divisor
						// failure and operand evaluation, as do independent arrays.
						if !constructorEffectOriginalOperandFree(code, op) || initialized && !c.constructorReceiverFinalizerSilent {
							return false
						}
					}
				}
				_, ok := pop(kind)
				_, ok2 := pop(kind)
				if !ok || !ok2 {
					return false
				}
				stack = append(stack, constructorEffectValue{kind: kind})
			case opcode >= core.OP_INEG && opcode <= core.OP_DNEG:
				kind := []byte{'I', 'J', 'F', 'D'}[opcode-core.OP_INEG]
				_, ok := pop(kind)
				if !ok {
					return false
				}
				stack = append(stack, constructorEffectValue{kind: kind})
			case opcode >= core.OP_ISHL && opcode <= core.OP_LXOR:
				kind := byte('I')
				if (opcode-core.OP_ISHL)%2 == 1 {
					kind = 'J'
				}
				right := kind
				if opcode <= core.OP_LUSHR {
					right = 'I'
				}
				_, ok := pop(right)
				_, ok2 := pop(kind)
				if !ok || !ok2 {
					return false
				}
				stack = append(stack, constructorEffectValue{kind: kind})
			case opcode >= core.OP_I2L && opcode <= core.OP_I2S:
				conversions := [][2]byte{{'I', 'J'}, {'I', 'F'}, {'I', 'D'}, {'J', 'I'}, {'J', 'F'}, {'J', 'D'}, {'F', 'I'}, {'F', 'J'}, {'F', 'D'}, {'D', 'I'}, {'D', 'J'}, {'D', 'F'}, {'I', 'I'}, {'I', 'I'}, {'I', 'I'}}
				pair := conversions[opcode-core.OP_I2L]
				_, ok := pop(pair[0])
				if !ok {
					return false
				}
				stack = append(stack, constructorEffectValue{kind: pair[1]})
			case opcode >= core.OP_IFEQ && opcode <= core.OP_IF_ACMPNE || opcode == core.OP_IFNULL || opcode == core.OP_IFNONNULL:
				kind, count := byte('I'), 1
				if opcode >= core.OP_IF_ICMPEQ && opcode <= core.OP_IF_ACMPNE {
					count = 2
				}
				if opcode >= core.OP_IF_ACMPEQ && opcode <= core.OP_IF_ACMPNE || opcode == core.OP_IFNULL || opcode == core.OP_IFNONNULL {
					kind = 'L'
				}
				conditions := make([]constructorEffectValue, count)
				for i := count - 1; i >= 0; i-- {
					v, ok := pop(kind)
					if !ok || v.allocation != 0 {
						return false
					}
					conditions[i] = v
				}
				branch, ok := target(op)
				if !ok {
					return false
				}
				if taken, known := constructorKnownIntBranch(opcode, conditions); known {
					if taken {
						return walk(branch, locals, stack, initialized)
					}
					return walk(index+1, locals, stack, initialized)
				}
				// Each arm owns its aliases and operand stack. A publication or exceptional
				// operation in even one arm rejects the entire movement proof.
				return walk(branch, append([]constructorEffectValue(nil), locals...), append([]constructorEffectValue(nil), stack...), initialized) && walk(index+1, locals, stack, initialized)
			case opcode == core.OP_GOTO || opcode == core.OP_GOTO_W:
				branch, ok := target(op)
				if !ok {
					return false
				}
				return walk(branch, locals, stack, initialized)
			case opcode >= core.OP_LCMP && opcode <= core.OP_DCMPG:
				kind := byte('D')
				if opcode == core.OP_LCMP {
					kind = 'J'
				} else if opcode <= core.OP_FCMPG {
					kind = 'F'
				}
				_, ok := pop(kind)
				_, ok2 := pop(kind)
				if !ok || !ok2 {
					return false
				}
				stack = append(stack, constructorEffectValue{kind: 'I'})
			case opcode == core.OP_ATHROW:
				// Capture motion preserves this original throw and its operand;
				// it is an abrupt exit, not a missing normal RETURN. With no
				// publication/read of THIS, only finalization could expose the
				// moved store after a failed initialized constructor. Its closed
				// receiver proof is mandatory then. Null keeps the original NPE;
				// a distinct initialized throwable keeps its original identity.
				if !constructorEffectOriginalOperandFree(code, op) || initialized && !c.constructorReceiverFinalizerSilent {
					return false
				}
				v, ok := pop('L')
				return ok && !v.receiver && v.allocation == 0 && len(stack) == 0
			case opcode >= core.OP_IRETURN && opcode <= core.OP_ARETURN:
				kind := []byte{'I', 'J', 'F', 'D', 'L'}[opcode-core.OP_IRETURN]
				if !initializedAtEntry || !initialized || ret == "V" || constructorEffectType(ret).kind != kind || !constructorEffectOriginalOperandFree(code, op) {
					return false
				}
				value, ok := pop(kind)
				return ok && !value.receiver && value.allocation == 0 && len(stack) == 0
			case opcode == core.OP_RETURN:
				return initialized && ret == "V" && len(stack) == 0 && constructorEffectOriginalOperandFree(code, op)
			default:
				return false
			}
		}
		return false
	}
	return walk(0, locals, nil, initializedAtEntry)
}

// JVM dup forms copy an exact one/two-word top packet below an exact
// zero/one/two-word packet. Packet boundaries may never split a category-2
// value. Copy whole abstract values so receiver and NEW identities survive.
func constructorEffectStackPermutation(stack []constructorEffectValue, opcode int) ([]constructorEffectValue, bool) {
	packet := func(end, words int) (int, bool) {
		for words > 0 {
			if end <= 0 {
				return 0, false
			}
			end--
			width := stack[end].width()
			if width == 0 {
				return 0, false
			}
			words -= width
		}
		return end, words == 0
	}
	if opcode == core.OP_SWAP {
		top, ok := packet(len(stack), 1)
		if !ok {
			return nil, false
		}
		under, ok := packet(top, 1)
		if !ok {
			return nil, false
		}
		result := append([]constructorEffectValue(nil), stack[:under]...)
		result = append(result, stack[top:]...)
		result = append(result, stack[under:top]...)
		return result, true
	}
	if opcode < core.OP_DUP || opcode > core.OP_DUP2_X2 {
		return nil, false
	}
	copyWords, insertWords := 1, opcode-core.OP_DUP
	if opcode >= core.OP_DUP2 {
		copyWords = 2
		insertWords = opcode - core.OP_DUP2
	}
	top, ok := packet(len(stack), copyWords)
	if !ok {
		return nil, false
	}
	under, ok := packet(top, insertWords)
	if !ok {
		return nil, false
	}
	result := make([]constructorEffectValue, 0, len(stack)+len(stack)-top)
	result = append(result, stack[:under]...)
	result = append(result, stack[top:]...)
	result = append(result, stack[under:]...)
	return result, true
}

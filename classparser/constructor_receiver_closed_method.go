package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// A closed instance call is not necessarily read-only. It can modify storage
// distinct from the captures being moved, provided its entire original body
// satisfies the constructor's receiver-effect invariant. Prove closed dispatch
// to an exact own declaration, then analyze it with initialized THIS.
// Open dispatch, monitors, native bodies, typed handlers and recursive call
// cycles require separate proofs. Catch-all edges use original input locals. No member name or library identity grants admission.
func (c *ClassObjectDumper) constructorReceiverClosedMethod(obj *ClassObject, member *values.JavaClassMember, opcode int, writes, active map[string]bool, remaining *int, depth int, aliases *constructorSelfStorageProof, arguments ...constructorEffectValue) (constructorEffectValue, bool) {
	return c.constructorReceiverClosedMethodWithBinding(obj, member, opcode, writes, active, remaining, depth, aliases, nil, arguments...)
}

func (c *ClassObjectDumper) constructorReceiverClosedMethodWithBinding(obj *ClassObject, member *values.JavaClassMember, opcode int, writes, active map[string]bool, remaining *int, depth int, aliases *constructorSelfStorageProof, binding *constructorReceiverMethodBinding, arguments ...constructorEffectValue) (constructorEffectValue, bool) {
	// Even a rejected nested attempt makes the outer body context-dependent.
	// Successful leaf bodies never consult the recursion/depth domain inside
	// their body; the ordinary entry cycle/depth guards still run on every hit.
	if c != nil && c.constructorProfileEvidence != nil {
		e := c.constructorProfileEvidence
		e.closedCallEpoch++
		if e.closedCallEpoch == 0 {
			e.eligible, e.inconsistent = false, true
		}
	}

	if c == nil || !c.constructorReceiverFinalizerSilent || obj == nil || member == nil || aliases == nil || active == nil || remaining == nil || depth > 16 || *remaining <= 0 || obj.AccessFlags&0x0200 != 0 || member.Name != obj.GetClassName() || member.Member == "" || member.Member == "<init>" || member.Member == "<clinit>" || opcode != core.OP_INVOKEVIRTUAL && opcode != core.OP_INVOKESPECIAL {
		return constructorEffectValue{}, false
	}
	key := member.Name + "\x00" + member.Member + "\x00" + member.Description
	if active[key] {
		return constructorEffectValue{}, false
	}
	active[key] = true
	defer delete(active, key)
	if binding == nil {
		var found bool
		binding, found = c.constructorReceiverBindMethod(obj, member, opcode, remaining, arguments...)
		if !found {
			return constructorEffectValue{}, false
		}
	}
	result, target, code := binding.result, binding.target, binding.code
	evidence := c.constructorProfileEvidence

	leafKey, memoEligible := c.constructorClosedBodyKey(obj, target, code, arguments, writes, nil, remaining, 0)
	leafKey.leaf = true
	memoKey := leafKey
	var epoch, entries uint64
	if memoEligible {
		epoch, entries = evidence.observationEpoch, evidence.closedCallEpoch
		prior, known := evidence.closedBodies[leafKey]
		if !known || prior.providerEpoch != evidence.providerEpoch {
			memoKey, memoEligible = c.constructorClosedBodyContext(leafKey, active, remaining, depth)
			prior, known = evidence.closedBodies[memoKey]
		}
		if memoEligible && known && prior.providerEpoch == evidence.providerEpoch {
			aliases.selfStored = aliases.selfStored || prior.selfStored
			aliases.referenceRead = aliases.referenceRead || prior.referenceRead
			if !aliases.closed() {
				return constructorEffectValue{}, false
			}
			return constructorEffectType(result), true
		}
	}

	if len(code.Code) > *remaining || !nativeProofWork(c.Work, int64(len(code.Code))) || c.Work != nil && c.Work.CheckAlloc((int64(code.MaxLocals)+int64(code.MaxStack))*32) != nil {
		return constructorEffectValue{}, false
	}
	decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
	decoder.Work = c.Work
	if decoder.ParseOpcode() != nil {
		return constructorEffectValue{}, false
	}
	ops := constructorMotionOps(decoder)
	if len(ops) > *remaining || !c.constructorReceiverBodyEffectsWithStorage(obj, code, ops, member.Description, writes, active, remaining, depth, aliases, true, arguments...) || !aliases.closed() {
		return constructorEffectValue{}, false
	}
	if memoEligible && evidence.eligible && !evidence.inconsistent && evidence.observationEpoch == epoch {
		// In an admitted initialized body, active/depth are consulted only by
		// nested closed-instance calls. An initialized THIS constructor is refused
		// before chain analysis. Zero nested attempts therefore proves this body's
		// independence from those two analysis inputs; all actual values/storage
		// and provider observations retain their existing exact binding.
		if evidence.closedCallEpoch == entries {
			memoKey = leafKey
		}

		retained := int64(128 + len(memoKey.operands) + len(memoKey.context))
		if _, existing := evidence.closedBodies[memoKey]; existing {
			retained = 0
		}
		*remaining--
		if *remaining < 0 || !nativeProofWork(c.Work, 1) || !c.constructorOwnMethodRetentionAllowed(retained) {
			return constructorEffectValue{}, false
		}
		if evidence.closedBodies == nil {
			evidence.closedBodies = map[constructorClosedBodyKey]constructorClosedBodyMemo{}
		}
		evidence.bodyRetention += retained
		evidence.closedBodies[memoKey] = constructorClosedBodyMemo{providerEpoch: evidence.providerEpoch, selfStored: aliases.selfStored, referenceRead: aliases.referenceRead}
	}
	return constructorEffectType(result), true
}

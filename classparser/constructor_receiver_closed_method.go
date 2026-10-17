package javaclassparser

import (
	"bytes"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
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
	leafKey.freshReceiver = aliases.freshReceiver
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

// A static call can borrow an initialized fresh receiver without publishing
// it. Bind an exact original own static declaration and interpret all paths;
// transferring the value grants neither THIS storage nor declaring-class
// dispatch. Results carry no receiver identity or scalar branch facts.
func (c *ClassObjectDumper) constructorReceiverBorrowedStaticMethod(member *values.JavaClassMember, opcode int, writes, active map[string]bool, remaining *int, depth int, aliases *constructorSelfStorageProof, arguments ...constructorEffectValue) (constructorEffectValue, bool) {
	if c != nil && c.constructorProfileEvidence != nil {
		e := c.constructorProfileEvidence
		e.closedCallEpoch++
		if e.closedCallEpoch == 0 {
			e.eligible, e.inconsistent = false, true
		}
	}
	if c == nil || c.obj == nil || member == nil || remaining == nil || *remaining <= 0 || depth > 16 || active == nil || aliases == nil || !aliases.freshReceiver || !c.constructorReceiverFinalizerSilent || opcode != core.OP_INVOKESTATIC || member.Member == "" || member.Member == "<init>" || member.Member == "<clinit>" {
		return constructorEffectValue{}, false
	}
	key := "static\x00" + member.Name + "\x00" + member.Member + "\x00" + member.Description
	if active[key] {
		return constructorEffectValue{}, false
	}
	active[key] = true
	defer delete(active, key)
	params, result, err := callbinding.Descriptor(member.Description)
	if err != nil || len(params) != len(arguments) || len(params) > 128 || nativeMemberParameterWidth(params) > 255 {
		return constructorEffectValue{}, false
	}
	provider := c.buildInvocationMetadata()
	if evidence := c.constructorProfileEvidence; evidence != nil {
		provider = evidence.metadata(provider)
	}
	widening := newConstructorWideningQuery(func(name string) (callbinding.Class, bool) {
		*remaining--
		if *remaining < 0 || !nativeProofWork(c.Work, 1) {
			return callbinding.Class{}, false
		}
		declaration, known := provider(name)
		if !known || declaration.Name != name || !declaration.ParentsComplete || len(declaration.Parents) > 63 || !nativeProofWork(c.Work, int64(len(declaration.Parents))+1) || c.Work != nil && c.Work.CheckAlloc(int64(len(declaration.Parents)+1)*64) != nil {
			return callbinding.Class{}, false
		}
		return callbinding.Class{Name: name, Parents: declaration.Parents, ParentsComplete: true}, true
	})
	borrowed := false
	for i, param := range params {
		v := arguments[i]
		if !constructorValidNullFact(v) || v.kind != constructorEffectType(param).kind || v.allocation != 0 || v.retainsReceiver || v.receiver && (v.kind != 'L' || v.knownInt || v.knownNull) {
			return constructorEffectValue{}, false
		}
		if v.receiver {
			actual := "L" + c.obj.GetClassName() + ";"
			// A coarse L word does not make a class receiver an array or an
			// unrelated formal type. Close the original declared port as well.
			if actual != param && param != "Ljava/lang/Object;" {
				if _, known := widening.class(callbinding.Name(param)); !known {
					return constructorEffectValue{}, false
				}
			}
			if !widening.assignable(actual, param) {
				return constructorEffectValue{}, false
			}
		}
		borrowed = borrowed || v.receiver
	}
	if !borrowed {
		return constructorEffectValue{}, false
	}
	object, known := c.constructorMotionClassWithSnapshot(member.Name, true)
	if !known || object == nil || object.GetClassName() != member.Name || object.AccessFlags&(0x0200|0x2000|0x8000) != 0 {
		return constructorEffectValue{}, false
	}
	target, known := c.constructorReceiverOwnMethod(object, member.Member, member.Description, remaining)
	if !known || target == nil || target.AccessFlags&0x0008 == 0 || target.AccessFlags&(0x0020|0x0100|0x0400) != 0 {
		return constructorEffectValue{}, false
	}
	var code *CodeAttribute
	for _, a := range target.Attributes {
		if !nativeProofWork(c.Work, 1) {
			return constructorEffectValue{}, false
		}
		if body, ok := a.(*CodeAttribute); ok {
			if body == nil || code != nil {
				return constructorEffectValue{}, false
			}
			code = body
		}
	}
	if code == nil || int(code.MaxLocals) < nativeMemberParameterWidth(params) || len(code.Code) == 0 || len(code.Code) > *remaining || !nativeProofWork(c.Work, int64(len(code.Code))) || c.Work != nil && c.Work.CheckAlloc((int64(code.MaxLocals)+int64(code.MaxStack))*32) != nil {
		return constructorEffectValue{}, false
	}
	before, known := constructorConfinedRootSnapshot(object)
	if !known || len(before) > 2<<20 || !nativeProofWork(c.Work, int64(len(before))+1) || c.Work != nil && c.Work.CheckAlloc(int64(len(before))*4) != nil {
		return constructorEffectValue{}, false
	}
	decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(object.ConstantPool, i) })
	decoder.Work = c.Work
	if decoder.ParseOpcode() != nil {
		return constructorEffectValue{}, false
	}
	ops := constructorMotionOps(decoder)
	if len(ops) > *remaining || !c.constructorReceiverTypeBodyOriginal(object, code, ops, remaining) || !c.constructorReceiverBodyEffectsWithEntry(object, code, ops, member.Description, writes, active, remaining, depth, aliases, true, true, arguments...) || !aliases.closed() {
		return constructorEffectValue{}, false
	}
	after, known := constructorConfinedRootSnapshot(object)
	if !known || !nativeProofWork(c.Work, int64(len(after))+1) || !bytes.Equal(before, after) {
		return constructorEffectValue{}, false
	}
	aliases.borrowedReceiver = true
	return constructorEffectType(result), true
}

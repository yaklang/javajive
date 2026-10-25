package javaclassparser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

type constructorLoweringCapture struct {
	field sourceFieldIdentity
	slot  int
}

// A leaf is an exact typed parameter slot, one symbolic GETSTATIC, or an
// original literal. Null has a bottom reference type, not an Object upcast.
// Keep producer PCs outside the plan: equivalent freshly compiled packets can
// use different instruction widths and constant-pool indices.
type constructorLoweringArgument struct {
	null                    bool
	nullCasts               []string
	slot                    int
	owner, name, descriptor string
	invocation              *constructorLoweringInvocation
	literal                 *constructorLoweringLiteral
}

// The movable prefix contains distinct own final synthetic captures from typed
// parameter slots and a direct SUPER of typed ordered operand trees. An isolated suffix
// starts with an empty stack and stays at exactly the same bytecode offsets.
// No original executable body is retained.
type sourceConstructorLoweringPlan struct {
	owner, descriptor, super, superDescriptor string
	flags                                     uint16
	captures                                  []constructorLoweringCapture
	prelude                                   []constructorLoweringArgument
	arguments                                 []constructorLoweringArgument
	body                                      bool
	qualified                                 *constructorLoweringQualifiedSuper
	checkedSuperExceptions                    string
}

type sourceConstructorLoweringPublication struct {
	originalHash, sourceHash [32]byte
	plans                    map[string]*sourceConstructorLoweringPlan
}

type sourceConstructorLoweringAuthority struct {
	jar                               *JarFS
	parent                            *sourceConstructorLoweringAuthority
	mu                                sync.Mutex
	pending, active                   map[string]*sourceConstructorLoweringPublication
	nativePending, nativeActive       map[string]constructorNativeSourceOrigin
	enumPending, enumActive           map[string]constructorNativeSourceOrigin
	staticPending, staticActive       map[string]constructorNativeSourceOrigin
	bridgePending, bridgeActive       map[string]constructorNativeSourceOrigin
	nestPending, nestActive           map[string]constructorNativeSourceOrigin
	nestNativeActive                  map[string]constructorNativeSourceOrigin
	bindingPending, bindingActive     map[string]constructorNativeSourceOrigin
	genericPending, genericActive     map[string]constructorNativeSourceOrigin
	referencePending, referenceActive map[string]constructorNativeSourceOrigin
	namedRowsPending, namedRowsActive map[string]constructorNativeSourceOrigin
	escapePending, escapeActive       map[string]checkedEscapeSourceOrigin
}

type constructorNativeSourceOrigin struct{ originalHash, sourceHash [32]byte }

// An independently flattened declaration is a physical reconstruction unit.
// JVM class access uses ClassFile flags; InnerClasses flags describe lexical
// source visibility. Promoting the physical class can grant public Lookup
// access that its unchanged original binary callers never had. Native member,
// local and enum-body declarations retain their lexical source protocols.
func (c *ClassObjectDumper) reconstructionPhysicalHeader() bool {
	if c == nil || c.obj == nil || c.nativeMemberCurrent != nil || c.nativeMethodLocalCurrent != nil || c.nativeEnumConstantCurrent != nil {
		return false
	}
	return c.options.sourceConstructorLowering != nil || c.options.reconstructionPhysicalRoot == c.obj
}

// Native member/anonymous source ownership has its own capture regeneration
// proof. Flat lowering must neither enroll its speculative readers nor replace
// that protocol. Record the actual selected native source only at JarFS's final
// ownership dispatch, then activate it on the exact public ReadFile return.
func (z *JarFS) noteNativeConstructorSource(object *ClassObject, source []byte) {
	a := z.sourceConstructorLowering
	if a == nil || object == nil || legacySourceHasFailureMarker(string(source)) {
		return
	}
	reader := z.nativeMemberReader(nil)
	raw, err := z.readJarEntry(object.GetClassName() + ".class")
	if err != nil || !nativeProofWork(reader.Work, int64(len(raw)+len(source))+1) || !bytes.Equal(raw, object.Bytes()) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.nativePending == nil {
		a.nativePending = map[string]constructorNativeSourceOrigin{}
	}
	a.nativePending[object.GetClassName()] = constructorNativeSourceOrigin{sha256.Sum256(raw), sha256.Sum256(source)}
}

type constructorLoweringPacket struct {
	plan                *sourceConstructorLoweringPlan
	code                *CodeAttribute
	firstEnd, secondEnd int
	ops                 []*core.OpCode
	argumentOrigins     []constructorLoweringOperandOrigin
	preludeOrigins      []constructorLoweringOperandOrigin
	qualifierGuards     []constructorLoweringQualifierGuard
}

func constructorLoweringCode(method *MemberInfo) *CodeAttribute {
	if method == nil {
		return nil
	}
	var found *CodeAttribute
	for _, attr := range method.Attributes {
		if code, ok := attr.(*CodeAttribute); ok {
			if code == nil || found != nil {
				return nil
			}
			found = code
		}
	}
	return found
}

func constructorLoweringPacketFor(object *ClassObject, method *MemberInfo, before, original bool, work *workbudget.Budget, captureCount ...int) *constructorLoweringPacket {
	return constructorLoweringTypedPacketFor(object, method, before, original, work, nil, captureCount...)
}

func constructorLoweringTypedPacketFor(object *ClassObject, method *MemberInfo, before, original bool, work *workbudget.Budget, provider callbinding.Provider, captureCount ...int) *constructorLoweringPacket {
	return constructorLoweringTypedPacketWithQualifier(object, method, before, original, false, work, provider, captureCount...)
}

func constructorLoweringTypedPacketWithQualifier(object *ClassObject, method *MemberInfo, before, original, qualified bool, work *workbudget.Budget, provider callbinding.Provider, captureCount ...int) *constructorLoweringPacket {
	limit := -1
	if len(captureCount) == 1 {
		limit = captureCount[0]
	}
	return constructorLoweringReadPacket(object, method, before, original, qualified, work, provider, limit, -1)
}

func constructorLoweringReadPacket(object *ClassObject, method *MemberInfo, before, original, qualified bool, work *workbudget.Budget, provider callbinding.Provider, captureLimit, preludeLimit int) *constructorLoweringPacket {
	// The pre-nestmate capture packet has the same typed operands and receiver
	// initialization cut in Java4 through Java8. Its whole instruction domain
	// is checked below (including rejection of JSR/RET), independently of the
	// original verifier's frame format. An implicit MR rebuild can use a later
	// supported target; never change a version header to make a packet fit.
	if object == nil || method == nil || object.MinorVersion != 0 || original && (object.MajorVersion < 48 || object.MajorVersion > 52) || !original && (object.MajorVersion < 52 || object.MajorVersion > 65) || object.AccessFlags&0x6200 != 0 || method.AccessFlags&^uint16(0x0087) != 0 || !nativeProofWork(work, 1) {
		return nil
	}
	name, nk := sourceBridgeUTF8(object, method.NameIndex)
	desc, dk := sourceBridgeUTF8(object, method.DescriptorIndex)
	params, result, err := callbinding.Descriptor(desc)
	code := constructorLoweringCode(method)
	if !nk || name != "<init>" || !dk || err != nil || result != "V" || code == nil || len(code.Code) > 4096 || len(code.Code) == 0 || !nativeProofWork(work, int64(len(code.Code))+int64(len(object.Fields))*64) {
		return nil
	}
	if work.CheckAlloc(int64(len(code.Code))*128+int64(len(object.Fields))*128) != nil {
		return nil
	}
	slots := constructorParameterSlots(params)
	words := 1
	for _, p := range params {
		words++
		if p == "J" || p == "D" {
			words++
		}
	}
	if words > 255 || int(code.MaxLocals) < words {
		return nil
	}
	d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(object.ConstantPool, i) })
	d.Work = work
	if d.ParseOpcode() != nil {
		return nil
	}
	ops := []*core.OpCode{}
	for _, op := range d.Opcodes() {
		if op == nil || op.Instr == nil {
			return nil
		}
		if op.Instr.OpCode != core.OP_START {
			ops = append(ops, op)
		}
	}
	p := &sourceConstructorLoweringPlan{owner: object.GetClassName(), descriptor: desc, super: object.GetSupperClassName(), flags: method.AccessFlags}
	if !nativeSourceBinaryName(p.owner) || !nativeSourceBinaryName(p.super) || len(ops) == 0 || ops[0].CurrentOffset != 0 {
		return nil
	}
	i, peak := 0, 1
	var argumentOrigins []constructorLoweringOperandOrigin
	var preludeOrigins []constructorLoweringOperandOrigin
	var qualifierGuards []constructorLoweringQualifierGuard
	widening := newConstructorWideningQuery(provider)
	load := func(op *core.OpCode, slot int, typ string) bool {
		return core.GetRetrieveIdx(op) == slot && constructorMotionLoad(op, typ)
	}
	captures := func() bool {
		seen := map[sourceFieldIdentity]bool{}
		for i+2 < len(ops) {
			if captureLimit >= 0 && len(p.captures) == captureLimit {
				break
			}
			member := constructorMotionMember(object, ops[i+2], core.OP_PUTFIELD)
			if member == nil {
				break
			}
			slot := core.GetRetrieveIdx(ops[i+1])
			param, known := slots[slot]
			if !known || !load(ops[i], 0, "Ljava/lang/Object;") || !load(ops[i+1], slot, params[param]) || member.Name != p.owner || member.Description != params[param] {
				return false
			}
			key := sourceFieldIdentity{member.Member, member.Description}
			if seen[key] || len(seen) >= 32 {
				return false
			}
			seen[key] = true
			count := 0
			for _, field := range object.Fields {
				if field == nil {
					return false
				}
				n, nk := sourceBridgeUTF8(object, field.NameIndex)
				typ, tk := sourceBridgeUTF8(object, field.DescriptorIndex)
				if !nk || !tk {
					return false
				}
				if n == key.name && typ == key.descriptor {
					// Java 1.4 communicates synthetic capture ownership through
					// its Synthetic attribute, not the Java 5 ACC_SYNTHETIC bit.
					// Do not qualify a later class by changing only its header.
					if original && object.MajorVersion < 49 && field.AccessFlags&syntheticFieldFlag != 0 {
						return false
					}
					flags, _, ok := nativeMemberEffectiveFieldFlags(field, work)
					if !ok {
						return false
					}
					if flags&(0x0008|0x0040) != 0 || flags&0x0010 == 0 || original && flags&syntheticFieldFlag == 0 {
						return len(p.captures) > 0
					}
					count++
				}
			}
			if count != 1 {
				return false
			}
			width := 2
			if params[param] == "J" || params[param] == "D" {
				width++
			}
			if width > peak {
				peak = width
			}
			p.captures = append(p.captures, constructorLoweringCapture{key, slot})
			i += 3
		}
		return len(p.captures) > 0
	}
	prefix := func() bool {
		start := i
		if captures() {
			return true
		}
		if i != start || len(p.captures) != 0 || preludeLimit == 0 {
			return false
		}
		var known bool
		p.prelude, preludeOrigins, known = constructorLoweringPrelude(object, code, ops, &i, slots, params, words, preludeLimit, &peak, work)
		return known
	}
	delegate := func() bool {
		var ok bool
		p.arguments, argumentOrigins, p.superDescriptor, ok = constructorLoweringDelegateOperands(object, ops, &i, slots, params, p.super, widening, &peak, work, qualified, &qualifierGuards)
		return ok
	}
	first, second := prefix, delegate
	if !before {
		first, second = delegate, prefix
	}
	if !first() || i >= len(ops) {
		return nil
	}
	boundary := int(ops[i].CurrentOffset)
	if !second() || i >= len(ops) || int(code.MaxStack) < peak || !enumArrayInstructionSpans(code.Code, ops) || !constructorLoweringSuffixClosed(object, code, ops, int(ops[i].CurrentOffset), work) {
		return nil
	}
	p.body = i+1 != len(ops) || ops[i].Instr.OpCode != core.OP_RETURN
	if qualified && (original || len(qualifierGuards) != 1 || before && !qualifierGuards[0].noop) {
		return nil
	}
	return &constructorLoweringPacket{plan: p, code: code, firstEnd: boundary, secondEnd: int(ops[i].CurrentOffset), ops: ops, argumentOrigins: argumentOrigins, preludeOrigins: preludeOrigins, qualifierGuards: qualifierGuards}
}

func (c *ClassObjectDumper) planArtifactConstructorLowering(boundary *constructorSourceBoundary, code *CodeAttribute, method *MemberInfo, decoder *core.Decompiler) bool {
	if c == nil || c.FuncCtx == nil || c.options.sourceConstructorLowering == nil || c.options.TargetSourceVersion < 8 || c.options.TargetSourceVersion > 21 || c.nativeCaptureFields != nil || c.nativeMemberCurrent != nil || c.nativeMethodLocalCurrent != nil || c.nativeEnumConstantCurrent != nil || boundary == nil || boundary.delegate == nil {
		return false
	}
	// The original input's Java4-8 capture convention and the selected
	// source target are independent. All supported fresh targets retain the
	// same initialization cut; lower() re-proves its exact fresh packet,
	// verifier/debug offsets and typed SUPER operands before moving anything.
	p := constructorLoweringTypedPacketFor(c.obj, method, true, true, c.Work, c.FuncCtx.InvocationMetadata)
	if p == nil || p.code != code || len(p.plan.captures)+len(p.plan.prelude) != len(boundary.prefix) || boundary.delegate.OriginPC != boundary.pc || boundary.delegate.Kind != values.InvokeSpecial || boundary.delegate.Descriptor != p.plan.superDescriptor || strings.ReplaceAll(boundary.delegate.ClassName, ".", "/") != p.plan.super || p.secondEnd != boundary.pc+3 {
		return false
	}
	if len(p.plan.prelude) != 0 && !c.constructorLoweringPreludeSource(boundary, p, decoder, method) {
		return false
	}
	params, _, _ := callbinding.Descriptor(p.plan.descriptor)
	slots := constructorParameterSlots(params)
	for i, capture := range p.plan.captures {
		assign, ok := boundary.prefix[i].(*statements.AssignStatement)
		if !ok || assign == nil || assign.IsDeclare || assign.ArrayMember != nil {
			return false
		}
		field, ok := values.UnpackSoltValue(assign.LeftValue).(*values.RefMember)
		if !ok || field == nil || field.Member != capture.field.name {
			return false
		}
		receiver, ok := values.UnpackSoltValue(field.Object).(*values.JavaRef)
		index, known := slots[capture.slot]
		if !ok || receiver == nil || !receiver.IsThis || !known || index >= len(boundary.params) {
			return false
		}
		param, ok := boundary.params[index].(*values.JavaRef)
		if !ok || param == nil || !param.IsParam || param.IsThis || param.CustomValue != nil || param.StackVar != nil || values.UnpackSoltValue(assign.JavaValue) != param {
			return false
		}
	}
	// Bind the source SUPER operands as well as the raw packet. The postcompile
	// check will independently repeat this binding against fresh instructions.
	if len(boundary.delegate.Arguments) != len(p.plan.arguments) {
		return false
	}
	for i, argument := range p.plan.arguments {
		if !constructorLoweringSourceOperand(argument, p.argumentOrigins[i], boundary.delegate.Arguments[i], slots, boundary.params, 0, c.FuncCtx) {
			return false
		}
	}
	raw, err := c.options.sourceConstructorLowering.jar.readJarEntry(p.plan.owner + ".class")
	if err != nil || !nativeProofWork(c.Work, int64(len(raw))*2) || !bytes.Equal(raw, c.obj.Bytes()) {
		return false
	}
	if len(c.constructorLowerings) >= 64 && c.constructorLowerings[p.plan.descriptor] == nil {
		return false
	}
	p.plan.qualified = c.constructorLoweringQualifiedSuperView(p.plan)
	p.plan.checkedSuperExceptions, _ = constructorLoweringCheckedSuperExceptions(c.obj, method, p.plan, c.FuncCtx.InvocationMetadata, c.Work)
	boundary.lowering = p.plan
	return true
}

func (c *ClassObjectDumper) publishConstructorLowering(source string, methods []*dumpedMethods) bool {
	if !c.publishNamedMemberRows(source) {
		return false
	}
	if !c.publishMemberReferenceSignatures(source) {
		return false
	}
	if !c.publishMemberGenericScope(source) {
		return false
	}
	if !c.publishCheckedEscapeLowering(source, methods) {
		return false
	}
	if !c.publishInheritedBridgeLowering(source, methods) {
		return false
	}
	if !c.publishStaticClassLowering(source) {
		return false
	}
	if !c.publishEnumArrayLowering(source) {
		return false
	}
	if !c.publishConstructorBindingSignatures(source, methods) {
		return false
	}
	if len(c.constructorLowerings) == 0 {
		return true
	}
	a := c.options.sourceConstructorLowering
	if a == nil || c.sourceDegraded || legacySourceHasFailureMarker(source) || !nativeProofWork(c.Work, int64(len(source))+1) {
		return false
	}
	raw, err := a.jar.readJarEntry(c.obj.GetClassName() + ".class")
	if err != nil || !bytes.Equal(raw, c.obj.Bytes()) {
		return false
	}
	p := &sourceConstructorLoweringPublication{originalHash: sha256.Sum256(raw), sourceHash: sha256.Sum256([]byte(source)), plans: map[string]*sourceConstructorLoweringPlan{}}
	for _, method := range methods {
		if method == nil || method.constructorLowering == nil {
			continue
		}
		plan := method.constructorLowering
		if method.isStub || method.descriptor != plan.descriptor || method.methodName != "<init>" || method.code != method.constructorLoweringCode || strings.Count(source, method.code) != 1 || c.constructorLowerings[plan.descriptor] != plan || p.plans[plan.descriptor] != nil {
			return false
		}
		p.plans[plan.descriptor] = plan
	}
	if len(p.plans) != len(c.constructorLowerings) {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.pending == nil {
		a.pending = map[string]*sourceConstructorLoweringPublication{}
	}
	a.pending[c.obj.GetClassName()] = p
	return true
}

func (a *sourceConstructorLoweringAuthority) sourceReturned(owner string, source []byte) error {
	// A child's comment is not its owning compilation unit. Native readers
	// carry only source-origin records, never flat executable-lowering power.
	// Import their enum origins only when this exact complete owning source
	// is actually returned through the reconstruction API.
	var enums []sourceEnumArrayOrigin
	var nests []sourcePrivateNestOrigin
	if a.jar != nil && a.jar.sourceLambdaMetadata != nil {
		if publication := a.jar.sourceLambdaMetadata.publication(owner, string(source)); publication != nil {
			enums = publication.enums
			nests = publication.nests
		}
		if len(enums)+len(nests) != 0 {
			if err := a.jar.nativeMemberReader(nil).Work.CheckAlloc(int64(len(enums)+len(nests)+1) * 192); err != nil {
				return err
			}
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	hash := sha256.Sum256(source)
	namedRowsOrigin, namedRowsFound := a.namedRowsPending[owner]
	namedRowsFound = namedRowsFound && namedRowsOrigin.sourceHash == hash
	if previous, active := a.namedRowsActive[owner]; active && (!namedRowsFound || previous != namedRowsOrigin) {
		return fmt.Errorf("named member rows: changed returned source")
	}
	referenceOrigin, referenceFound := a.referencePending[owner]
	referenceFound = referenceFound && referenceOrigin.sourceHash == hash
	if previous, active := a.referenceActive[owner]; active && (!referenceFound || previous != referenceOrigin) {
		return fmt.Errorf("member reference signature: changed returned source")
	}
	genericOrigin, genericFound := a.genericPending[owner]
	genericFound = genericFound && genericOrigin.sourceHash == hash
	if previous, active := a.genericActive[owner]; active && (!genericFound || previous != genericOrigin) {
		return fmt.Errorf("member generic scope: changed returned source")
	}
	escapeOrigin, escapeFound := a.escapePending[owner]
	escapeFound = escapeFound && escapeOrigin.sourceHash == hash
	if previous, active := a.escapeActive[owner]; active && (!escapeFound || previous != escapeOrigin) {
		return fmt.Errorf("checked escape lowering: changed returned source")
	}
	bindingOrigin, bindingFound := a.bindingPending[owner]
	bindingFound = bindingFound && bindingOrigin.sourceHash == hash
	if previous, active := a.bindingActive[owner]; active && (!bindingFound || previous != bindingOrigin) {
		return fmt.Errorf("constructor binding: changed returned source")
	}
	staticOrigin, staticFound := a.staticPending[owner]
	staticFound = staticFound && staticOrigin.sourceHash == hash
	if previous, active := a.staticActive[owner]; active && (!staticFound || previous != staticOrigin) {
		return fmt.Errorf("static class lowering: changed returned source")
	}
	nestOrigin, nestFound := a.nestPending[owner]
	nestFound = nestFound && nestOrigin.sourceHash == hash
	if previous, active := a.nestActive[owner]; active && (!nestFound || previous != nestOrigin) {
		return fmt.Errorf("private nest completion: changed returned source")
	}
	nativeNests := map[string]constructorNativeSourceOrigin{}
	for _, origin := range nests {
		candidate := constructorNativeSourceOrigin{origin.originalHash, hash}
		if _, duplicate := nativeNests[origin.owner]; duplicate {
			return fmt.Errorf("private nest completion: duplicate owning source")
		}
		if prior, active := a.nestNativeActive[origin.owner]; active && prior != candidate {
			return fmt.Errorf("private nest completion: changed owning source")
		}
		if prior, active := a.nestActive[origin.owner]; active && prior != candidate {
			return fmt.Errorf("private nest completion: conflicting physical source")
		}
		nativeNests[origin.owner] = candidate
	}
	bridgeOrigin, bridgeFound := a.bridgePending[owner]
	bridgeFound = bridgeFound && bridgeOrigin.sourceHash == hash
	if previous, active := a.bridgeActive[owner]; active && (!bridgeFound || previous != bridgeOrigin) {
		return fmt.Errorf("inherited bridge lowering: changed returned source")
	}
	toActivate := map[string]constructorNativeSourceOrigin{}
	for _, origin := range enums {
		candidate := constructorNativeSourceOrigin{origin.originalHash, hash}
		if _, duplicate := toActivate[origin.owner]; duplicate {
			return fmt.Errorf("enum lowering: duplicate source origin")
		}
		toActivate[origin.owner] = candidate
	}
	if origin, found := a.enumPending[owner]; found {
		if origin.sourceHash != hash {
			if _, active := a.enumActive[owner]; active {
				return fmt.Errorf("enum lowering: changed returned source")
			}
		} else {
			if prior, known := toActivate[owner]; known && prior != origin {
				return fmt.Errorf("enum lowering: conflicting source publication")
			}
			toActivate[owner] = origin
		}
	}
	for binary, origin := range toActivate {
		if prior, active := a.enumActive[binary]; active && prior != origin {
			return fmt.Errorf("enum lowering: conflicting completed owning sources")
		}
	}
	activateOrigins := func() {
		if len(nativeNests) != 0 && a.nestNativeActive == nil {
			a.nestNativeActive = map[string]constructorNativeSourceOrigin{}
		}
		for name, origin := range nativeNests {
			a.nestNativeActive[name] = origin
		}
		if nestFound {
			if a.nestActive == nil {
				a.nestActive = map[string]constructorNativeSourceOrigin{}
			}
			a.nestActive[owner] = nestOrigin
		}
		if namedRowsFound {
			if a.namedRowsActive == nil {
				a.namedRowsActive = map[string]constructorNativeSourceOrigin{}
			}
			a.namedRowsActive[owner] = namedRowsOrigin
		}
		if referenceFound {
			if a.referenceActive == nil {
				a.referenceActive = map[string]constructorNativeSourceOrigin{}
			}
			a.referenceActive[owner] = referenceOrigin
		}
		if genericFound {
			if a.genericActive == nil {
				a.genericActive = map[string]constructorNativeSourceOrigin{}
			}
			a.genericActive[owner] = genericOrigin
		}
		if escapeFound {
			if a.escapeActive == nil {
				a.escapeActive = map[string]checkedEscapeSourceOrigin{}
			}
			a.escapeActive[owner] = escapeOrigin
		}
		if bindingFound {
			if a.bindingActive == nil {
				a.bindingActive = map[string]constructorNativeSourceOrigin{}
			}
			a.bindingActive[owner] = bindingOrigin
		}
		if bridgeFound {
			if a.bridgeActive == nil {
				a.bridgeActive = map[string]constructorNativeSourceOrigin{}
			}
			a.bridgeActive[owner] = bridgeOrigin
		}
		// Commit only at a successful return, while holding the lock. A
		// later native/constructor conflict cannot leave partial authority.
		if len(toActivate) != 0 {
			if a.enumActive == nil {
				a.enumActive = map[string]constructorNativeSourceOrigin{}
			}
			for binary, origin := range toActivate {
				a.enumActive[binary] = origin
			}
		}
		if staticFound {
			if a.staticActive == nil {
				a.staticActive = map[string]constructorNativeSourceOrigin{}
			}
			a.staticActive[owner] = staticOrigin
		}
	}
	if origin, found := a.nativePending[owner]; found && origin.sourceHash == hash {
		if a.active[owner] != nil {
			return fmt.Errorf("constructor lowering: conflicting native and flat source publication")
		}
		if prior, ok := a.nativeActive[owner]; ok && prior != origin {
			return fmt.Errorf("constructor lowering: changed native source publication")
		}
		if a.nativeActive == nil {
			a.nativeActive = map[string]constructorNativeSourceOrigin{}
		}
		a.nativeActive[owner] = origin
		activateOrigins()
		return nil
	}
	p := a.pending[owner]
	if p == nil || p.sourceHash != hash {
		if a.active[owner] != nil {
			return fmt.Errorf("constructor lowering: changed source requires a fresh artifact")
		}
		activateOrigins()
		return nil
	}
	if prior := a.active[owner]; prior != nil && !reflect.DeepEqual(prior, p) {
		return fmt.Errorf("constructor lowering: conflicting source publication")
	}
	if a.active == nil {
		a.active = map[string]*sourceConstructorLoweringPublication{}
	}
	a.active[owner] = p
	activateOrigins()
	return nil
}

func constructorLoweringFailure(work *workbudget.Budget, reason string) error {
	if err := work.Check(); err != nil {
		return err
	}
	return fmt.Errorf("constructor lowering: %s", reason)
}

// Both movable prefix packets end at an empty stack and have no branch or
// protected instruction. A closed suffix may contain control flow and handlers;
// it stays after both packets, with every byte and absolute PC unchanged.
// putfield of this class's own fields on uninitializedThis is legal (JVMS
// 4.10.1.9). Reversing the freshly compiled packets therefore restores the
// original receiver observations, including callbacks, publication and abrupt
// SUPER exit, without needing any assumption about the superclass's effects.
func (a *sourceConstructorLoweringAuthority) lower(original, compiled []byte, work *workbudget.Budget) ([]byte, error) {
	if a == nil || !nativeProofWork(work, int64(len(original)+len(compiled))+1) || len(original) > 32<<20 || len(compiled) > 32<<20 {
		return nil, constructorLoweringFailure(work, "unavailable authority or input budget")
	}
	want, err := ParseWithBudget(original, work)
	if err != nil {
		return nil, err
	}
	reader, err := NewClassReaderWithBudget(compiled, work)
	if err != nil {
		return nil, err
	}
	layout := &compiledMetadataLayout{methods: map[*MemberInfo]compiledMetadataSpan{}}
	got, err := parseWithMetadataLayout(reader, nil, layout)
	if err != nil {
		return nil, err
	}
	if want.GetClassName() != got.GetClassName() || want.GetSupperClassName() != got.GetSupperClassName() {
		return nil, constructorLoweringFailure(work, "class or SUPER identity changed")
	}
	var provider callbinding.Provider
	if a.jar != nil {
		reader := a.jar.nativeMemberReader(want)
		reader.Work = work
		provider = reader.buildInvocationMetadata()
	}
	if a.jar != nil && a.jar.sourceCompiler.effective() == NativeJavac8 && got.MajorVersion != 52 {
		return nil, constructorLoweringFailure(work, "fresh class target disagrees with selected native compiler")
	}
	a.mu.Lock()
	publication := a.active[want.GetClassName()]
	native, nativeClosed := a.nativeActive[want.GetClassName()]
	pending := a.pending[want.GetClassName()]
	a.mu.Unlock()
	// javac can implicitly compile an unchanged base source in an MR output
	// tree. Reuse its actual returned-source obligation only for byte-identical
	// originals and the identical immutable compiler/source tuple. This does not
	// fabricate a publication in the new namespace. The complete fresh packet,
	// local field bindings and SUPER signature are still independently checked
	// below; no parent effect or global use-closure assumption is transported.
	if parent := a.parent; publication == nil && pending == nil && parent != nil && a.jar != nil && parent.jar != nil && a.jar.sourceCompiler.effective() == parent.jar.sourceCompiler.effective() && a.jar.targetSourceVersion == parent.jar.targetSourceVersion && nativeProofWork(work, 1) {
		hash := sha256.Sum256(original)
		parent.mu.Lock()
		if p := parent.active[want.GetClassName()]; p != nil && p.originalHash == hash && p.sourceHash != ([32]byte{}) {
			publication = p
		}
		if origin, known := parent.nativeActive[want.GetClassName()]; known && origin.originalHash == hash && origin.sourceHash != ([32]byte{}) {
			native, nativeClosed = origin, true
		}
		parent.mu.Unlock()
	}
	// A missing source receipt must not silently publish an intermediate
	// constructor with late captures (including source-cache reuse in another
	// physical release namespace). Native lexical reconstruction can already
	// reproduce the early packet; validate that instead of editing it. Object's
	// exact inert initialization remains the ordinary source path's own proof.
	for _, method := range want.Methods {
		if nativeClosed && native.originalHash == sha256.Sum256(original) {
			break
		}
		packet := constructorLoweringTypedPacketFor(want, method, true, true, work, provider)
		if packet == nil || packet.plan.super == "java/lang/Object" || publication != nil && publication.plans[packet.plan.descriptor] != nil {
			continue
		}
		var fresh *constructorLoweringPacket
		matches := 0
		for _, candidate := range got.Methods {
			n, _ := sourceBridgeUTF8(got, candidate.NameIndex)
			d, _ := sourceBridgeUTF8(got, candidate.DescriptorIndex)
			if n == "<init>" && d == packet.plan.descriptor {
				matches++
				fresh = constructorLoweringReadPacket(got, candidate, true, false, false, work, provider, len(packet.plan.captures), len(packet.plan.prelude))
			}
		}
		if matches != 1 || fresh == nil || !reflect.DeepEqual(packet.plan, fresh.plan) {
			return nil, constructorLoweringFailure(work, "constructor schedule requires its own returned source publication")
		}
	}
	if publication == nil {
		// No source-lowering plan means no permission to edit executable Code.
		if err := work.Check(); err != nil {
			return nil, err
		}
		return bytes.Clone(compiled), nil
	}
	if publication.originalHash != sha256.Sum256(original) || publication.sourceHash == ([32]byte{}) || len(publication.plans) == 0 || len(publication.plans) > 64 {
		return nil, constructorLoweringFailure(work, "stale or invalid publication")
	}
	expected := map[string]*MemberInfo{}
	for _, method := range want.Methods {
		name, _ := sourceBridgeUTF8(want, method.NameIndex)
		if name == "<init>" {
			desc, _ := sourceBridgeUTF8(want, method.DescriptorIndex)
			if expected[desc] != nil {
				return nil, constructorLoweringFailure(work, "duplicate original constructor")
			}
			expected[desc] = method
		}
	}
	seen := map[string]bool{}
	var edits []compiledMetadataEdit
	for _, method := range got.Methods {
		name, _ := sourceBridgeUTF8(got, method.NameIndex)
		desc, _ := sourceBridgeUTF8(got, method.DescriptorIndex)
		plan := publication.plans[desc]
		if name != "<init>" || plan == nil {
			continue
		}
		if seen[desc] {
			return nil, constructorLoweringFailure(work, "duplicate compiled constructor")
		}
		seen[desc] = true
		originalPacket := constructorLoweringTypedPacketFor(want, expected[desc], true, true, work, provider)
		if originalPacket == nil || !constructorLoweringExecutablePlanEqual(plan, originalPacket.plan) || !a.constructorLoweringQualifiedSuperUnchanged(want, plan, work) {
			return nil, constructorLoweringFailure(work, "original packet changed")
		}
		if exceptions, known := constructorLoweringCheckedSuperExceptions(want, expected[desc], plan, provider, work); known != (plan.checkedSuperExceptions != "") || known && exceptions != plan.checkedSuperExceptions {
			return nil, constructorLoweringFailure(work, "original checked SUPER declaration changed")
		}
		permute := true
		packet := constructorLoweringReadPacket(got, method, false, false, plan.qualified != nil, work, provider, len(plan.captures), len(plan.prelude))
		if packet == nil || !constructorLoweringFreshPlanEqual(plan, packet.plan) {
			// A separately verified already-lowered packet makes completion
			// idempotent. An arbitrary constructor body is never a substitute.
			packet = constructorLoweringReadPacket(got, method, true, false, plan.qualified != nil, work, provider, len(plan.captures), len(plan.prelude))
			if packet == nil || !constructorLoweringFreshPlanEqual(plan, packet.plan) {
				return nil, constructorLoweringFailure(work, "fresh constructor does not match original slots, fields and delegation")
			}
			permute = false
		}
		edit, err := constructorLoweringMethodEdit(compiled, got, method, layout, packet, permute, work)
		if err != nil {
			return nil, err
		}
		edit, err = constructorLoweringCheckedSuperMethodEdit(edit, got, method, plan.checkedSuperExceptions, permute, work)
		if err != nil {
			return nil, err
		}
		// The fresh-plan proof permits only loss of the original varargs
		// marker; visibility and all executable operands were checked above.
		binary.BigEndian.PutUint16(edit.data, plan.flags)
		edits = append(edits, edit)
	}
	if len(seen) != len(publication.plans) {
		return nil, constructorLoweringFailure(work, "missing published constructor")
	}
	return applyCompiledMetadataEdits(compiled, edits, work)
}

func constructorLoweringMethodEdit(compiled []byte, object *ClassObject, method *MemberInfo, layout *compiledMetadataLayout, packet *constructorLoweringPacket, permute bool, work *workbudget.Budget) (compiledMetadataEdit, error) {
	fail := func(reason string) (compiledMetadataEdit, error) {
		return compiledMetadataEdit{}, constructorLoweringFailure(work, reason)
	}
	span, recorded := layout.methods[method]
	if !recorded || span.start < layout.poolEnd || span.end > len(compiled) || span.end-span.start < 8 || work.CheckAlloc(int64(span.end-span.start)*2) != nil {
		return fail("invalid fresh method span")
	}
	raw := bytes.Clone(compiled[span.start:span.end])
	codeFound := false
	for pos := 8; pos < len(raw); {
		if len(raw)-pos < 6 {
			return fail("truncated fresh method attribute")
		}
		size := int64(binary.BigEndian.Uint32(raw[pos+2:]))
		if size > int64(len(raw)-pos-6) || !nativeProofWork(work, size+1) {
			return fail("invalid fresh method attribute size")
		}
		end := pos + 6 + int(size)
		name, ok := sourceBridgeUTF8(object, binary.BigEndian.Uint16(raw[pos:]))
		if !ok {
			return fail("invalid fresh method attribute name")
		}
		if name == "Code" {
			if codeFound {
				return fail("duplicate fresh Code")
			}
			codeFound = true
			if err := constructorLoweringCodeBytes(raw[pos+6:end], object, packet, permute, work); err != nil {
				return compiledMetadataEdit{}, err
			}
		}
		pos = end
	}
	if !codeFound {
		return fail("missing fresh Code")
	}
	return compiledMetadataEdit{span, raw}, nil
}

// Debug PCs are part of the transformation proof. Complete parameter scopes
// retain their ranges; line entries follow their original fresh instructions.
// Frames, handlers and interior local scopes must belong wholly to the fixed
// suffix. Unknown Code attributes remain unsupported. Equal packet length alone
// cannot discharge uninitializedThis frames or type-annotation offsets.
func constructorLoweringCodeBytes(raw []byte, object *ClassObject, packet *constructorLoweringPacket, permute bool, work *workbudget.Budget) error {
	fail := func(reason string) error { return constructorLoweringFailure(work, reason) }
	if packet == nil || len(raw) < 12 {
		return fail("truncated Code")
	}
	n := int64(binary.BigEndian.Uint32(raw[4:]))
	if n != int64(len(packet.code.Code)) || n > int64(len(raw)-12) || !bytes.Equal(raw[8:8+int(n)], packet.code.Code) {
		return fail("fresh Code byte span changed")
	}
	codeEnd := 8 + int(n)
	handlers := int(binary.BigEndian.Uint16(raw[codeEnd:]))
	if handlers != len(packet.code.ExceptionTable) || codeEnd+4+8*handlers > len(raw) {
		return fail("constructor handler table changed")
	}
	count := int(binary.BigEndian.Uint16(raw[codeEnd+2+8*handlers:]))
	pos := codeEnd + 4 + 8*handlers
	pcMap := map[int]int{int(n): int(n)}
	opcodes := map[int]int{}
	for _, op := range packet.ops {
		pc := int(op.CurrentOffset)
		next := pc
		if permute && pc < packet.firstEnd {
			next += packet.secondEnd - packet.firstEnd
		} else if permute && pc < packet.secondEnd {
			next -= packet.firstEnd
		}
		pcMap[pc] = next
		opcodes[pc] = op.Instr.OpCode
	}
	params, _, _ := callbinding.Descriptor(packet.plan.descriptor)
	slots := constructorParameterSlots(params)
	seen := map[string]bool{}
	for i := 0; i < count; i++ {
		if len(raw)-pos < 6 {
			return fail("truncated Code attribute")
		}
		size := int64(binary.BigEndian.Uint32(raw[pos+2:]))
		if size > int64(len(raw)-pos-6) || !nativeProofWork(work, size+1) {
			return fail("invalid Code attribute size")
		}
		name, ok := sourceBridgeUTF8(object, binary.BigEndian.Uint16(raw[pos:]))
		if !ok || seen[name] {
			return fail("invalid or duplicate Code attribute")
		}
		seen[name] = true
		body := raw[pos+6 : pos+6+int(size)]
		if len(body) < 2 {
			return fail("truncated Code subtable")
		}
		rows := int(binary.BigEndian.Uint16(body))
		switch name {
		case "StackMapTable":
			// All suffix PCs, NEW identities and exception regions are fixed.
			// Reuse the frame decoder with the identity map, discarding its
			// normalized serialization so the fresh frame bytes stay untouched.
			if _, ok := enumArrayRelocateFrames(object, body, opcodes, packet.secondEnd, 0, work); !ok {
				return fail("unproved constructor frame positions")
			}
		case "LineNumberTable":
			if len(body) != 2+4*rows {
				return fail("invalid line table")
			}
			for j := 0; j < rows; j++ {
				at := 2 + 4*j
				pc := int(binary.BigEndian.Uint16(body[at:]))
				next, ok := pcMap[pc]
				if !ok || pc >= int(n) {
					return fail("line entry is not an instruction")
				}
				binary.BigEndian.PutUint16(body[at:], uint16(next))
			}
		case "LocalVariableTable", "LocalVariableTypeTable":
			if len(body) != 2+10*rows {
				return fail("invalid local table")
			}
			for j := 0; j < rows; j++ {
				at := 2 + 10*j
				start, length := int(binary.BigEndian.Uint16(body[at:])), int(binary.BigEndian.Uint16(body[at+2:]))
				slot := int(binary.BigEndian.Uint16(body[at+8:]))
				descriptor, known := sourceBridgeUTF8(object, binary.BigEndian.Uint16(body[at+6:]))
				_, named := sourceBridgeUTF8(object, binary.BigEndian.Uint16(body[at+4:]))
				param, isParam := slots[slot]
				_, startOK := pcMap[start]
				_, endOK := pcMap[start+length]
				if start >= packet.secondEnd && start < int(n) && start+length <= int(n) && startOK && endOK && named && known && slot < int(packet.code.MaxLocals) {
					continue // unchanged local interval entirely in the suffix
				}
				if start != 0 || length != int(n) || !named || !known || slot != 0 && !isParam {
					return fail("interior or nonparameter local scope")
				}
				if name == "LocalVariableTable" && (slot == 0 && descriptor != "L"+object.GetClassName()+";" || slot != 0 && descriptor != params[param]) {
					return fail("local descriptor and parameter slot disagree")
				}
			}
		default:
			return fail("unproved Code attribute " + name)
		}
		pos += 6 + int(size)
	}
	if pos != len(raw) {
		return fail("trailing Code bytes")
	}
	if permute {
		fresh := packet.code.Code
		out := raw[8:codeEnd]
		copy(out, fresh[packet.firstEnd:packet.secondEnd])
		copy(out[packet.secondEnd-packet.firstEnd:], fresh[:packet.firstEnd])
		copy(out[packet.secondEnd:], fresh[packet.secondEnd:])
	}
	for _, guard := range packet.qualifierGuards {
		start, known := pcMap[guard.start]
		if !known || guard.end-guard.start != 5 || start < 0 || start+5 > packet.secondEnd {
			return fail("unproved qualified SUPER guard span")
		}
		// Only the compiler's extra qualifier check is removed. Keep its
		// width as NOPs, so all suffix frames, handlers and PCs remain fixed.
		for i := start; i < start+5; i++ {
			raw[8+i] = core.OP_NOP
		}
	}
	return work.Check()
}

package javaclassparser

import (
	"crypto/sha256"
	"fmt"
	"slices"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// Ordinary allocations are not THIS/SUPER delegates. This receipt observes
// their existing renderer; it never renders arguments, authorizes a source
// projection, or changes allocation/class-initialization order.
type ordinaryConstructorThrowsReceipt struct {
	scope                               *ordinaryConstructorThrowsScope
	allocation                          *values.NewExpression
	call                                *values.FunctionCallExpression
	owner, descriptor, sourceDescriptor string
	newPC, initPC                       int
	codeHash, inputHash                 [32]byte
	metadata                            map[string][32]byte
	operandMetadata                     map[string][32]byte
	exceptions                          []string
	arguments                           []values.JavaValue
	plan                                *nativeMemberAllocation
	planValue                           nativeMemberAllocation
	constructor                         *nativeMemberConstructor
	constructorValue                    nativeMemberConstructor
	roleChild                           *nativeMemberClass
	roleObject                          *ClassObject
	roleObjectFlags                     uint16
	realMethod                          *MemberInfo
	realMethodFlags                     uint16
	roleOwner, roleName, roleField      string
	roleFlags                           uint16
	roleFormals, roleOuterFormals       int
	bridge                              *nativeConstructorAccessBridge
	bridgeValue                         nativeConstructorAccessBridge
	lexicalObjects                      []*ClassObject
	lexicalSignatures                   []string
}

type ordinaryConstructorThrowsScope struct {
	c                                     *ClassObjectDumper
	method                                *MemberInfo
	code                                  *CodeAttribute
	object                                *ClassObject
	name, descriptor, methodSig, classSig string
	sourceVersion                         int
	flags                                 uint16
	methodAttrs, classAttrs               int
	declared                              []string
	allocations                           map[int]*values.NewExpression
	receipts                              map[int]*ordinaryConstructorThrowsReceipt
	frames                                []*ordinaryConstructorRenderFrame
	renders                               []*ordinaryConstructorRenderRecord
	active, finished, failed              bool
	emitting                              bool
	returnEmissions                       int
	parameterNames                        map[*values.JavaRef]string
	returnStatement                       *statements.ReturnStatement
	sourceContext                         *class_context.ClassContext
	sourceBody, publishedCode             string
}

type ordinaryConstructorRenderFrame struct {
	value     *values.NewExpression
	receipt   *ordinaryConstructorThrowsReceipt
	call      *values.FunctionCallExpression
	viewCount int
}

type ordinaryConstructorRenderRecord struct {
	receipt    *ordinaryConstructorThrowsReceipt
	source     string
	begin, end string
}

// Legacy analysis clients cannot suppress a helper without supplying the
// unique final publisher. Only the dumper entry below gets a staged scope.
func (c *ClassObjectDumper) methodNeedsCheckedEscapeWithOrdinaryConstructors(code *CodeAttribute, body []statements.Statement, method *MemberInfo, plan *constructorSourceBoundary) (bool, *ordinaryConstructorThrowsScope, error) {
	scope := c.ordinaryConstructorThrowsScope(code, body, method)
	needs, err := c.methodNeedsCheckedEscapeProof(code, body, method, scope, plan)
	if err != nil || needs || scope == nil || len(scope.receipts) == 0 {
		return needs, nil, err
	}
	return false, scope, nil
}

func (c *ClassObjectDumper) ordinaryConstructorThrowsScope(code *CodeAttribute, body []statements.Statement, method *MemberInfo) *ordinaryConstructorThrowsScope {
	if c == nil || c.obj == nil || c.FuncCtx == nil || method == nil || code == nil || c.getenv("JDEC_ORDINARY_CTOR_THROWS_OFF") != "" || len(code.Code) > 4096 || !nativeProofWork(c.Work, 1) {
		return nil
	}
	name, ne := c.obj.getUtf8(method.NameIndex)
	desc, de := c.obj.getUtf8(method.DescriptorIndex)
	target := c.options.TargetSourceVersion
	if target == 0 {
		target = core.ClassMajorToSourceVersion(c.obj.MajorVersion)
	}
	if ne != nil || de != nil || strings.HasPrefix(name, "<") || method.AccessFlags&0x1040 != 0 || target < 8 || c.FuncCtx.FunctionName != name || c.FuncCtx.CurrentMethodDesc != desc {
		return nil
	}
	methodSig, mk := c.constructorThrowsOriginalSignature(method.Attributes)
	classSig, ck := c.constructorThrowsOriginalSignature(c.obj.Attributes)
	declared, ek := c.constructorThrowsOriginalExceptions(method)
	if !mk || !ck || !ek || methodSig != c.FuncCtx.CurrentMethodSig || classSig != c.FuncCtx.ClassSig || !c.constructorThrowsHeaderView(methodSig) {
		return nil
	}
	// The first source domain is a flat method with a terminal direct NEW
	// return. No general control-flow walker or multi-allocation transaction is
	// needed to establish this exact return expression's final source extent.
	if len(body) == 0 || len(body) > 128 {
		return nil
	}
	ret, ok := body[len(body)-1].(*statements.ReturnStatement)
	if !ok || ret == nil {
		return nil
	}
	n, ok := values.UnpackSoltValue(ret.JavaValue).(*values.NewExpression)
	if !ok || n == nil || n.IsArray() || n.ConstructorCall == nil || !n.ConstructorCall.HasOriginPC {
		return nil
	}
	if c.Work != nil && c.Work.CheckAlloc(1024+int64(len(n.ConstructorCall.Arguments))*16) != nil {
		return nil
	}
	s := &ordinaryConstructorThrowsScope{c: c, method: method, code: code, object: c.obj, name: name, descriptor: desc, methodSig: methodSig, classSig: classSig, sourceVersion: c.options.TargetSourceVersion, flags: method.AccessFlags, methodAttrs: len(method.Attributes), classAttrs: len(c.obj.Attributes), declared: declared, allocations: map[int]*values.NewExpression{}, receipts: map[int]*ordinaryConstructorThrowsReceipt{}}
	for _, st := range body[:len(body)-1] {
		if !nativeProofWork(c.Work, 1) {
			return nil
		}
		switch st.(type) {
		case *statements.AssignStatement, *statements.ExpressionStatement, *statements.SourceAnchorStatement:
		default:
			return nil
		}
	}
	s.allocations[n.ConstructorCall.OriginPC] = n
	s.returnStatement = ret
	return s
}

func (s *ordinaryConstructorThrowsScope) admit(member *values.JavaClassMember, pc int) (*ordinaryConstructorThrowsReceipt, constructorThrowsAdmissionState) {
	if s == nil || member == nil || member.Member != "<init>" {
		return nil, constructorThrowsNotApplicable
	}
	n := s.allocations[pc]
	if n == nil || n.ConstructorCall == nil {
		return nil, constructorThrowsUnknown
	}
	call := n.ConstructorCall
	owner := strings.ReplaceAll(member.Name, ".", "/")
	ctx := s.c.FuncCtx
	if ctx.SourceAnonymousCandidate != nil && ctx.SourceAnonymousCandidate(owner) || ctx.SourceMethodLocalCandidate != nil && ctx.SourceMethodLocalCandidate(owner) {
		return nil, constructorThrowsUnknown
	}
	if !n.HasOriginPC || call.Object != n || !call.HasOriginPC || call.OriginPC != pc || call.Kind != values.InvokeSpecial || !call.IsSpecialInvoke || call.IsStatic || call.FunctionName != "<init>" || strings.ReplaceAll(call.ClassName, ".", "/") != owner || call.Descriptor != member.Description || len(call.Arguments) > 128 {
		return nil, constructorThrowsRefused
	}
	r := &ordinaryConstructorThrowsReceipt{scope: s, allocation: n, call: call, owner: owner, descriptor: member.Description, sourceDescriptor: member.Description, newPC: n.OriginPC, initPC: pc, arguments: slices.Clone(call.Arguments)}
	provider, scopes, known := r.sourceView(false)
	if !known {
		return nil, constructorThrowsUnknown
	}
	analysis, metadata := s.c.constructorThrowsProofContext()
	metadata.provider = provider
	exceptions, known := ordinaryConstructorThrowsOnly(analysis.FuncCtx.InvocationMetadata, owner, r.sourceDescriptor, scopes, s.c.Work)
	if !known {
		return nil, constructorThrowsUnknown
	}
	// A concrete inferred exception cannot borrow a source formal's erased
	// upper bound. Unchecked inference needs no unrelated caller Signature.
	if !s.c.constructorThrowsHeaderView(s.methodSig) {
		return nil, constructorThrowsRefused
	}
	for _, exception := range exceptions {
		if !nativeProofWork(s.c.Work, 1) {
			return nil, constructorThrowsUnknown
		}
		if analysis.constructorThrowsUnchecked(exception) {
			continue
		}
		concrete, known := s.c.constructorDelegationSourceThrows(s.method, s.declared)
		if !known {
			return nil, constructorThrowsRefused
		}
		covered := false
		for _, allowed := range concrete {
			if !nativeProofWork(s.c.Work, 1) {
				return nil, constructorThrowsUnknown
			}
			covered = covered || originalExceptionCovered(exception, allowed, analysis.FuncCtx.InvocationMetadata)
		}
		if !covered {
			return nil, constructorThrowsRefused
		}
	}
	input, known := s.c.constructorThrowsOriginalInput(s.method, s.code, pc)
	if !known {
		return nil, constructorThrowsUnknown
	}
	operandAnalysis, operandMetadata := s.c.constructorThrowsProofContext()
	if !r.matchInput(input, operandAnalysis) {
		return nil, constructorThrowsUnknown
	}
	r.operandMetadata = operandMetadata.hashes
	r.inputHash = input.hash
	r.codeHash = sha256.Sum256(s.code.Code)
	r.exceptions = exceptions
	r.metadata = metadata.hashes
	if !r.fresh() {
		return nil, constructorThrowsRefused
	}
	s.receipts[pc] = r
	return r, constructorThrowsProved
}

// Return the exact source view of the existing native plan, without granting
// that plan permission. Rendering must later consume its original callback and
// independently prove the same physical/real/logical ports.
func (r *ordinaryConstructorThrowsReceipt) sourceView(fresh bool) (callbinding.Provider, []string, bool) {
	s, c := r.scope, r.scope.c
	plan := c.nativeMemberCalls[s.name+s.descriptor][r.initPC]
	if plan == nil {
		if fresh && r.plan != nil {
			return nil, nil, false
		}
		if c.FuncCtx.SourceMemberCandidate != nil && c.FuncCtx.SourceMemberCandidate(r.owner) {
			return nil, nil, false
		}
		if r.owner == c.obj.GetClassName() && (c.getenv("JDEC_METHOD_TYPEPARAMS_OFF") != "" || c.getenv("JDEC_THROWS_SIG_RECOVERY_OFF") != "") {
			return nil, nil, false
		}
		cl, known := c.FuncCtx.InvocationMetadata(r.owner)
		if !known || len(types.ClassFormalTypeParamNames(cl.Signature)) != 0 {
			return nil, nil, false
		}
		return c.FuncCtx.InvocationMetadata, []string{cl.Signature}, true
	}
	p := c.nativeMemberRoot
	child := plan.child
	if c.getenv("JDEC_METHOD_TYPEPARAMS_OFF") != "" || c.getenv("JDEC_THROWS_SIG_RECOVERY_OFF") != "" {
		return nil, nil, false
	}
	if p == nil || p.failed || child == nil || child.static || plan.rootObject != nil || plan.allocatedObject() == nil || p.lexicalObjects[r.owner] != child.object || plan.newPC != r.newPC || plan.invokePC != r.initPC || plan.descriptor != r.descriptor || plan.freshEnclosing != nil || plan.enclosingReadPC >= 0 || plan.enclosingParameter != nil || plan.implicitReceiverClass != "" || plan.anonymousMemberEnclosing != nil || plan.methodLocalEnclosing != nil {
		return nil, nil, false
	}
	ctor := nativeMemberConstructorForAllocation(child, r.descriptor)
	if ctor == nil || ctor.descriptor == "" || ctor.sourceDescriptor == "" {
		return nil, nil, false
	}
	if child.object.AccessFlags&0x0600 != 0 || len(child.object.Methods) > 256 || len(child.object.ConstantPool) > 2048 {
		return nil, nil, false
	}
	var real *MemberInfo
	for _, m := range child.object.Methods {
		if m == nil || len(m.Attributes) > 128 || !nativeProofWork(c.Work, 1) {
			return nil, nil, false
		}
		name, nk := sourceBridgeUTF8(child.object, m.NameIndex)
		desc, dk := sourceBridgeUTF8(child.object, m.DescriptorIndex)
		if !nk || !dk {
			return nil, nil, false
		}
		if name == "<init>" && desc == ctor.descriptor {
			if real != nil {
				return nil, nil, false
			}
			real = m
		}
	}
	if real == nil {
		return nil, nil, false
	}
	reader := *c
	reader.obj = child.object
	methodSig, mk := reader.constructorThrowsOriginalSignature(real.Attributes)
	methodThrows, tk := reader.constructorThrowsOriginalExceptions(real)
	if !mk || !tk {
		return nil, nil, false
	}
	physical, ret, err := callbinding.Descriptor(ctor.descriptor)
	if err != nil || ret != "V" || len(physical) == 0 || physical[0] != "L"+child.owner+";" || ctor.sourceDescriptor != "("+strings.Join(physical[1:], "")+")V" {
		return nil, nil, false
	}
	bridge := child.accessBridges[r.descriptor]
	if bridge != nil && (!nativeMemberJointBridgeEquivalent(p, child.object, bridge.method, r.descriptor, c.Work) || bridge.target != ctor.descriptor) {
		return nil, nil, false
	}
	// A raw generic class erases its constructor member type. This native
	// renderer must be on its independently established parameterized/diamond
	// path, rather than borrowing throws inference for a raw allocation.
	// JLS 4.8: a member class of a raw enclosing type is raw even when the
	// member declares no formals of its own. Its constructor member type is
	// erased, so the throws-only preference cannot be borrowed from a typed
	// declaration. The existing source writer must supply the typed outer view.
	if child.formalCount > 0 || child.outerFormalCount > 0 {
		if len(r.call.Arguments) == 0 {
			return nil, nil, false
		}
		outer := r.call.Arguments[0]
		ref, isRef := values.UnpackSoltValue(outer).(*values.JavaRef)
		typedOuter := child.outerFormalCount == 0 || plan.implicitEnclosing || isRef && ref.IsThis && c.obj.GetClassName() == child.owner
		if param, ok := types.AsParameterizedType(outer.Type()); ok && len(param.TypeArgs) == child.outerFormalCount {
			typedOuter = true
		}
		if !typedOuter {
			return nil, nil, false
		}
	}
	var objects []*ClassObject
	var signatures []string
	current := child.object
	seen := map[*ClassObject]bool{}
	for current != nil {
		if len(objects) >= 32 || seen[current] || !nativeProofWork(c.Work, 1) {
			return nil, nil, false
		}
		seen[current] = true
		sig, known := c.ordinaryConstructorObjectSignature(current)
		if !known {
			return nil, nil, false
		}
		objects = append(objects, current)
		signatures = append(signatures, sig)
		if current.GetClassName() == p.owner {
			break
		}
		if len(current.Attributes) > 256 {
			return nil, nil, false
		}
		for _, a := range current.Attributes {
			if !nativeProofWork(c.Work, 1) {
				return nil, nil, false
			}
			if inner, ok := a.(*InnerClassesAttribute); ok {
				if inner == nil || len(inner.Classes) > 256 || !nativeProofWork(c.Work, int64(len(inner.Classes))+1) {
					return nil, nil, false
				}
			}
		}
		owner, _, flags, known := originalMemberOwner(current)
		if !known {
			return nil, nil, false
		}
		if flags&0x0008 != 0 {
			break
		}
		current = p.lexicalObjects[owner]
		if current == nil {
			return nil, nil, false
		}
	}
	slices.Reverse(objects)
	slices.Reverse(signatures)
	if fresh {
		if r.plan != plan || r.planValue != *plan || r.constructor != ctor || r.constructorValue != *ctor || r.roleChild != child || r.roleObject != child.object || r.roleObjectFlags != child.object.AccessFlags || r.realMethod != real || r.realMethodFlags != real.AccessFlags || r.roleOwner != child.owner || r.roleName != child.name || r.roleField != child.field || r.roleFlags != child.flags || r.roleFormals != child.formalCount || r.roleOuterFormals != child.outerFormalCount || r.bridge != bridge || bridge != nil && r.bridgeValue != *bridge || !slices.Equal(r.lexicalObjects, objects) || !slices.Equal(r.lexicalSignatures, signatures) || r.sourceDescriptor != ctor.sourceDescriptor {
			return nil, nil, false
		}
	} else {
		r.plan, r.planValue = plan, *plan
		r.constructor, r.constructorValue = ctor, *ctor
		r.roleChild, r.roleObject = child, child.object
		r.roleObjectFlags = child.object.AccessFlags
		r.realMethod, r.realMethodFlags = real, real.AccessFlags
		r.roleOwner, r.roleName, r.roleField = child.owner, child.name, child.field
		r.roleFlags, r.roleFormals, r.roleOuterFormals = child.flags, child.formalCount, child.outerFormalCount
		r.bridge = bridge
		if bridge != nil {
			r.bridgeValue = *bridge
		}
		r.lexicalObjects, r.lexicalSignatures = objects, signatures
		r.sourceDescriptor = ctor.sourceDescriptor
	}
	provider := nativeMemberBinding(c.FuncCtx, p, c.Work).InvocationMetadata
	declaration, known := provider(r.owner)
	if !known || len(declaration.Methods) > 256 {
		return nil, nil, false
	}
	matches := 0
	for _, m := range declaration.Methods {
		if !nativeProofWork(c.Work, 1) {
			return nil, nil, false
		}
		if m.Name == "<init>" && m.Desc == ctor.sourceDescriptor {
			if m.Signature != methodSig || !m.ExceptionsKnown || !slices.Equal(m.Exceptions, methodThrows) || m.Static != (real.AccessFlags&8 != 0) || m.Bridge != (real.AccessFlags&0x40 != 0) || m.Varargs != (real.AccessFlags&0x80 != 0) || m.Generic != (methodSig != "") {
				return nil, nil, false
			}
			matches++
		}
	}
	return provider, signatures, matches == 1
}

func (c *ClassObjectDumper) ordinaryConstructorObjectSignature(obj *ClassObject) (string, bool) {
	if obj == nil || len(obj.Attributes) > 256 {
		return "", false
	}
	copy := *c
	copy.obj = obj
	return copy.constructorThrowsOriginalSignature(obj.Attributes)
}

func (r *ordinaryConstructorThrowsReceipt) matchInput(input *constructorThrowsInput, proof *ClassObjectDumper) bool {
	if input == nil || r.call.Object != r.allocation || !r.allocation.HasOriginPC || r.allocation.OriginPC != r.newPC || len(input.invoke.Uses) != len(r.call.Arguments)+1 || len(input.invoke.UseIDs) == 0 {
		return false
	}
	allocation, known := input.ir.InstrByID(methodir.InstrID(r.newPC))
	invoke, ik := input.ir.InstrByID(methodir.InstrID(r.initPC))
	record := input.records[uint16(r.newPC)]
	if !known || !ik || allocation.Opcode != core.OP_NEW || allocation.Class != r.owner || invoke.Opcode != core.OP_INVOKESPECIAL || invoke.Class != r.owner || invoke.Member != "<init>" || invoke.Desc != r.descriptor || input.invoke.Uses[0] != (ssabuild.Origin{Kind: ssabuild.OriginInstr, PC: uint16(r.newPC)}) || len(record.ResultIDs) != 1 || record.ResultIDs[0] != input.invoke.UseIDs[0] {
		return false
	}
	params, _, err := callbinding.Descriptor(r.descriptor)
	if err != nil {
		return false
	}
	words := 1
	for _, p := range params {
		words++
		if p == "J" || p == "D" {
			words++
		}
	}
	stack := input.invoke.Before.Stack
	if len(stack) < words {
		return false
	}
	token := stack[len(stack)-words]
	if token.Kind != frametransfer.UninitNew || int(token.NewPC) != r.newPC || token.Class != r.owner {
		return false
	}
	owner, known := types.RawClassFQN(r.allocation.Type())
	if !known || strings.ReplaceAll(owner, ".", "/") != r.owner {
		return false
	}
	remaining := 256
	for i, arg := range r.call.Arguments {
		if arg != r.arguments[i] {
			return false
		}
		if ref, ok := values.UnpackSoltValue(arg).(*values.JavaRef); ok && ref != nil && ref.IsThis {
			slot, known := ref.OriginalReceiverSlot()
			if !known || slot != 0 || input.invoke.Uses[i+1].Kind != ssabuild.OriginParam || input.invoke.Uses[i+1].Slot != 0 {
				return false
			}
		} else if !r.matchOperand(input, arg, input.invoke.Uses[i+1], &remaining, proof) {
			return false
		}
	}
	return true
}

func (r *ordinaryConstructorThrowsReceipt) matchOperand(input *constructorThrowsInput, value values.JavaValue, origin ssabuild.Origin, remaining *int, proof *ClassObjectDumper) bool {
	*remaining--
	if *remaining < 0 || value == nil || !nativeProofWork(r.scope.c.Work, 1) {
		return false
	}
	if slot, ok := value.(*values.SlotValue); ok {
		return r.matchOperand(input, slot.GetValue(), origin, remaining, proof)
	}
	call, ok := value.(*values.FunctionCallExpression)
	if !ok {
		return proof.constructorThrowsMatchOperand(input, value, origin, remaining)
	}
	// An exact receiver-free primitive producer needs no generic/poly source
	// argument inference. It stays at the same original argument edge; this
	// receipt never renders, removes or reorders it.
	if call == nil || !call.HasOriginPC || origin.Kind != ssabuild.OriginInstr || call.OriginPC != int(origin.PC) || call.Kind != values.InvokeStatic || !call.IsStatic || call.IsSpecialInvoke || len(call.Arguments) > 32 {
		return false
	}
	ins, known := input.ir.InstrByID(methodir.InstrID(origin.PC))
	record := input.records[origin.PC]
	if !known || ins.Opcode != core.OP_INVOKESTATIC || ins.Member != call.FunctionName || ins.Desc != call.Descriptor || ins.Class != strings.ReplaceAll(call.ClassName, ".", "/") || len(record.Uses) != len(call.Arguments) {
		return false
	}
	_, ret, err := callbinding.Descriptor(call.Descriptor)
	if err != nil || len(ret) != 1 || !strings.Contains("ZBCSIJFD", ret) {
		return false
	}
	cl, known := proof.FuncCtx.InvocationMetadata(ins.Class)
	if !known || !cl.MembersComplete || len(cl.Methods) > 256 {
		return false
	}
	matches := 0
	for _, m := range cl.Methods {
		if !nativeProofWork(r.scope.c.Work, 1) {
			return false
		}
		if m.Name == call.FunctionName {
			if m.Desc != call.Descriptor || !m.Static || m.Generic || m.Varargs || m.Bridge {
				return false
			}
			matches++
		}
	}
	if matches != 1 {
		return false
	}
	for i, arg := range call.Arguments {
		if !r.matchOperand(input, arg, record.Uses[i], remaining, proof) {
			return false
		}
	}
	return true
}

func (r *ordinaryConstructorThrowsReceipt) fresh() bool {
	if r == nil || r.scope == nil {
		return false
	}
	s, c := r.scope, r.scope.c
	if c.obj != s.object || c.CurrentMethod != s.method || c.options.TargetSourceVersion != s.sourceVersion || c.FuncCtx.FunctionName != s.name || c.FuncCtx.CurrentMethodDesc != s.descriptor || c.FuncCtx.CurrentMethodSig != s.methodSig || c.FuncCtx.ClassSig != s.classSig || s.method.AccessFlags != s.flags || len(s.method.Attributes) != s.methodAttrs || len(s.object.Attributes) != s.classAttrs || r.allocation.ConstructorCall != r.call || r.call.OriginPC != r.initPC || !r.call.HasOriginPC || r.call.FunctionName != "<init>" || r.call.Descriptor != r.descriptor || strings.ReplaceAll(r.call.ClassName, ".", "/") != r.owner || r.call.Kind != values.InvokeSpecial || !r.call.IsSpecialInvoke || r.call.IsStatic || len(r.call.Arguments) != len(r.arguments) || !nativeProofWork(c.Work, int64(len(s.code.Code))+1) || sha256.Sum256(s.code.Code) != r.codeHash {
		return false
	}
	if s.returnStatement == nil || values.UnpackSoltValue(s.returnStatement.JavaValue) != r.allocation || len(s.object.Methods) > 65535 {
		return false
	}
	members, codes := 0, 0
	for _, m := range s.object.Methods {
		if !nativeProofWork(c.Work, 1) {
			return false
		}
		if m == s.method {
			members++
		}
	}
	for _, a := range s.method.Attributes {
		if !nativeProofWork(c.Work, 1) {
			return false
		}
		if code, ok := a.(*CodeAttribute); ok {
			if code != s.code {
				return false
			}
			codes++
		}
	}
	if members != 1 || codes != 1 {
		return false
	}
	methodSig, mk := c.constructorThrowsOriginalSignature(s.method.Attributes)
	classSig, ck := c.constructorThrowsOriginalSignature(s.object.Attributes)
	declared, ek := c.constructorThrowsOriginalExceptions(s.method)
	if !mk || !ck || !ek || methodSig != s.methodSig || classSig != s.classSig || !slices.Equal(declared, s.declared) || !c.constructorThrowsHeaderView(methodSig) {
		return false
	}
	input, known := c.constructorThrowsOriginalInput(s.method, s.code, r.initPC)
	operandAnalysis, operandMetadata := c.constructorThrowsProofContext()
	if !known || input.hash != r.inputHash || !r.matchInput(input, operandAnalysis) || !constructorThrowsMetadataEqual(r.operandMetadata, operandMetadata.hashes) {
		return false
	}
	provider, scopes, known := r.sourceView(true)
	if !known {
		return false
	}
	analysis, metadata := c.constructorThrowsProofContext()
	metadata.provider = provider
	throws, known := ordinaryConstructorThrowsOnly(analysis.FuncCtx.InvocationMetadata, r.owner, r.sourceDescriptor, scopes, c.Work)
	if !known || !slices.Equal(throws, r.exceptions) {
		return false
	}
	for _, exception := range throws {
		if analysis.constructorThrowsUnchecked(exception) {
			continue
		}
		concrete, known := c.constructorDelegationSourceThrows(s.method, s.declared)
		if !known {
			return false
		}
		covered := false
		for _, allowed := range concrete {
			if !nativeProofWork(c.Work, 1) {
				return false
			}
			covered = covered || originalExceptionCovered(exception, allowed, analysis.FuncCtx.InvocationMetadata)
		}
		if !covered {
			return false
		}
	}
	return constructorThrowsMetadataEqual(r.metadata, metadata.hashes)
}

func (s *ordinaryConstructorThrowsScope) String() string {
	return fmt.Sprintf("%s%s", s.name, s.descriptor)
}

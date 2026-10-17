package javaclassparser

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// This receipt owns only the source arguments of one already-proved original
// delegation. It grants no receiver, capture, initialization or access permission.
// In particular, generic/poly producers cannot borrow their JVM result type as
// a source inference witness. The receipt renders the exact operands it proves,
// so a later descriptor cast cannot silently broaden a lower bound.
type constructorInvocationThrowsReceipt struct {
	dumper                                  *ClassObjectDumper
	object                                  *ClassObject
	method                                  *MemberInfo
	code                                    *CodeAttribute
	codeHash                                [32]byte
	inputHash                               [32]byte
	parameters                              []constructorThrowsParameter
	parameterBindingCount                   int
	declarationHash                         [32]byte
	metadataHashes                          map[string][32]byte
	methodFlags                             uint16
	methodDesc, methodSig, classSig         string
	declared                                []string
	plan                                    *constructorSourceBoundary
	call                                    *values.FunctionCallExpression
	receiver                                values.JavaValue
	owner, descriptor                       string
	methodAttrCount, classAttrCount         int
	pc, sourceVersion                       int
	operands                                []*constructorThrowsOperand
	arguments                               []values.JavaValue
	exceptions                              []string
	allUnchecked                            bool
	renderedSource                          string
	renderCount, headerCount, emissionCount int
}

type constructorThrowsOperand struct {
	value            values.JavaValue
	typeKey, erasure string
	scalar           any
	refID            any
	isParam          bool
	castPC           int
	originalCast     bool
	children         []*constructorThrowsOperand
}

func constructorThrowsTypeKey(t types.JavaType, ctx *class_context.ClassContext) (string, string, bool) {
	if t == nil || ctx == nil || t.IsArray() {
		return "", "", false
	}
	if _, ok := types.AsParameterizedType(t); ok {
		return "", "", false
	}
	if _, ok := t.(*types.JavaWildcardType); ok {
		return "", "", false
	}
	// JavaClass.String changes the source declaration through this typed
	// projection. It is outside this receipt's bare-source-type permission.
	if raw, ok := t.RawType().(*types.JavaClass); ok && raw != nil {
		if replacement, changed := ctx.StandaloneEraseTypeVar(raw.Name); changed && replacement != "" {
			return "", "", false
		}
	}
	erased, known := values.SourceTypeErasure(t, ctx)
	if !known || erased == "" {
		return "", "", false
	}
	if name, formal := types.TypeVariableName(t); formal {
		return "T:" + name + ":" + erased, erased, true
	}
	if name, ok := types.RawClassFQN(t); ok && ctx.IsTypeParam(name) {
		return "T:" + name + ":" + erased, erased, true
	}
	return erased, erased, true
}

func (c *ClassObjectDumper) snapshotConstructorThrowsOperand(v values.JavaValue, allowed map[*values.JavaRef]bool, active map[values.JavaValue]bool, remaining *int) (*constructorThrowsOperand, bool) {
	*remaining--
	if *remaining < 0 || v == nil || reflect.ValueOf(v).Kind() == reflect.Pointer && reflect.ValueOf(v).IsNil() || active[v] || !nativeProofWork(c.Work, 1) {
		return nil, false
	}
	active[v] = true
	defer delete(active, v)
	key, erasure, known := constructorThrowsTypeKey(v.Type(), c.FuncCtx)
	if !known {
		return nil, false
	}
	result := &constructorThrowsOperand{value: v, typeKey: key, erasure: erasure}
	child := func(value values.JavaValue) bool {
		snapshot, known := c.snapshotConstructorThrowsOperand(value, allowed, active, remaining)
		if known {
			result.children = append(result.children, snapshot)
		}
		return known
	}
	switch x := v.(type) {
	case *values.SlotValue:
		if !child(x.GetValue()) || key != result.children[0].typeKey && result.children[0].erasure != "null" {
			return nil, false
		}
		result.typeKey = result.children[0].typeKey
		result.erasure = result.children[0].erasure
	case *values.JavaRef:
		if x.IsThis || x.Id == nil || x.CustomValue != nil || x.StackVar != nil || !allowed[x] {
			return nil, false
		}
		result.refID = x.Id
		result.isParam = x.IsParam
	case *values.JavaLiteral:
		// Lossless UTF-16 payloads and narrow primitive literals need their
		// own source-expression witness; their IR type alone is insufficient.
		if !constructorThrowsCanonicalLiteral(x, erasure) {
			return nil, false
		}
		switch x.Data.(type) {
		case nil, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, string:
		default:
			return nil, false
		}
		result.scalar = x.Data
		if values.IsNullLiteral(x) {
			result.erasure = "null"
			result.typeKey = "null"
		}
	case *values.CastExpression:
		// Binding casts may own hidden intersection/source permissions. Retain
		// ordinary original cast expressions only, at their original operand edge.
		if x.Binding || !child(x.Value) {
			return nil, false
		}
		result.castPC = x.OriginPC
		result.originalCast = x.OriginalCheckCast
	case *values.TernaryExpression:
		return nil, false // selected original phi edges require their own source certificate

	default:
		return nil, false
	}
	return result, true
}

func constructorThrowsOperandsEqual(a, b *constructorThrowsOperand) bool {
	if a == nil || b == nil || a.value != b.value || a.typeKey != b.typeKey || a.erasure != b.erasure || !reflect.DeepEqual(a.scalar, b.scalar) || a.refID != b.refID || a.isParam != b.isParam || a.castPC != b.castPC || a.originalCast != b.originalCast || len(a.children) != len(b.children) {
		return false
	}
	for i := range a.children {
		if !constructorThrowsOperandsEqual(a.children[i], b.children[i]) {
			return false
		}
	}
	return true
}

// Snapshot the exact original constructor family as well as its target's
// declaration. A fresh proof with a different Signature or rival table is not
// the receipt that the checked analysis published, even if its erasure agrees.
func (c *ClassObjectDumper) constructorThrowsDeclarationHash(owner string) ([32]byte, bool) {
	var zero [32]byte
	if c == nil || c.FuncCtx == nil || c.FuncCtx.InvocationMetadata == nil {
		return zero, false
	}
	metadata, known := c.FuncCtx.InvocationMetadata(strings.ReplaceAll(owner, ".", "/"))
	if !known || len(metadata.Methods) > 256 || len(metadata.Parents) > 128 || len(metadata.Signature) > 8192 {
		return zero, false
	}
	family := metadata
	family.Methods = nil
	retained := len(metadata.Name) + len(metadata.Signature)
	for _, parent := range metadata.Parents {
		if !nativeProofWork(c.Work, 1) {
			return zero, false
		}
		retained += len(parent)
	}
	for _, method := range metadata.Methods {
		if !nativeProofWork(c.Work, 1) {
			return zero, false
		}
		if method.Name != "<init>" {
			continue
		}
		retained += len(method.Desc) + len(method.Signature)
		if len(method.Exceptions) > 128 {
			return zero, false
		}
		for _, exception := range method.Exceptions {
			if !nativeProofWork(c.Work, 1) {
				return zero, false
			}
			retained += len(exception)
		}
		if retained > 65536 {
			return zero, false
		}
		family.Methods = append(family.Methods, method)
	}
	if retained > 65536 || !nativeProofWork(c.Work, int64(retained)+1) || c.Work != nil && c.Work.CheckAlloc(int64(retained)*8+16384) != nil {
		return zero, false
	}
	encoded, err := json.Marshal(family)
	if err != nil {
		return zero, false
	}
	return sha256.Sum256(encoded), true
}

func (c *ClassObjectDumper) constructorThrowsOriginalSignature(attributes []AttributeInfo) (string, bool) {
	var signature string
	seen := false
	for _, attribute := range attributes {
		if !nativeProofWork(c.Work, 1) {
			return "", false
		}
		if s, ok := attribute.(*SignatureAttribute); ok {
			if seen || s == nil {
				return "", false
			}
			seen = true
			value, err := c.obj.getUtf8(s.SignatureIndex)
			if err != nil || len(value) > 8192 {
				return "", false
			}
			signature = value
		}
	}
	return signature, true
}

// A source throws formal is not its erased upper bound. Without a DeclId
// substitution proof, a newly inferred checked exception cannot borrow that
// broader Exceptions entry. Unchecked inference remains independent of it.
func (c *ClassObjectDumper) constructorThrowsUnchecked(exception string) bool {
	if c == nil || c.FuncCtx == nil {
		return false
	}
	provider := c.FuncCtx.InvocationMetadata
	return originalExceptionUnchecked(exception, func(name string) (callbinding.Class, bool) {
		if provider == nil || !nativeProofWork(c.Work, 1) {
			return callbinding.Class{}, false
		}
		return provider(name)
	})
}

// Project only concrete source throws. A formal's erased upper bound is ABI
// metadata, never an additional Java source exception declaration.
func (c *ClassObjectDumper) constructorThrowsSourceProjection(classSig, methodSig string, declared []string) (string, []string, bool) {
	if methodSig == "" {
		return "", declared, true
	}
	if len(classSig)+len(methodSig) > 16384 || !nativeProofWork(c.Work, int64(len(classSig)+len(methodSig))+1) {
		return "", nil, false
	}
	erased, rawThrows, valid := types.EraseLexicalMethodSignatureWithThrows(classSig, methodSig)
	// javac omits nongeneric throws from Signature when only parameters or
	// formals need that attribute. The header then uses original Exceptions.
	if valid && len(rawThrows) == 0 {
		return erased, declared, true
	}
	if !valid || len(rawThrows) != len(declared) {
		return "", nil, false
	}
	for i, exception := range rawThrows {
		if !nativeProofWork(c.Work, 1) || exception != "L"+declared[i]+";" {
			return "", nil, false
		}
	}
	_, _, result, throws := types.ParseMethodSignatureFullWithThrows(methodSig, c.FuncCtx)
	if result == nil || len(throws) != len(declared) {
		return "", nil, false
	}
	var concrete []string
	for _, thrown := range throws {
		if !nativeProofWork(c.Work, 1) {
			return "", nil, false
		}
		if _, formal := types.TypeVariableName(thrown); formal {
			continue
		}
		name, known := types.RawClassFQN(thrown)
		if !known || !constructorInvocationThrowsSimpleType(thrown, false) {
			return "", nil, false
		}
		concrete = append(concrete, strings.ReplaceAll(name, ".", "/"))
	}
	return erased, concrete, true
}

func (c *ClassObjectDumper) constructorThrowsCallerSourceCoverage(classSig, methodSig, descriptor string, declared, inferred []string) bool {
	erased, concrete, known := c.constructorThrowsSourceProjection(classSig, methodSig, declared)
	if !known || methodSig != "" && erased != descriptor {
		return false
	}
	provider := c.FuncCtx.InvocationMetadata
	for _, exception := range inferred {
		if !nativeProofWork(c.Work, 1) {
			return false
		}
		if c.constructorThrowsUnchecked(exception) {
			continue
		}
		covered := false
		for _, allowed := range concrete {
			if !nativeProofWork(c.Work, 1) {
				return false
			}
			covered = covered || originalExceptionCovered(exception, allowed, provider)
		}
		if !covered {
			return false
		}
	}
	return true
}

func (c *ClassObjectDumper) constructorThrowsOriginalSourceProjection(method *MemberInfo, declared []string) ([]string, bool) {
	methodSig, methodKnown := c.constructorThrowsOriginalSignature(method.Attributes)
	classSig, classKnown := c.constructorThrowsOriginalSignature(c.obj.Attributes)
	if !methodKnown || !classKnown {
		return nil, false
	}
	_, concrete, known := c.constructorThrowsSourceProjection(classSig, methodSig, declared)
	return concrete, known
}

type constructorThrowsAdmissionState uint8

const (
	constructorThrowsNotApplicable constructorThrowsAdmissionState = iota
	constructorThrowsUnknown
	constructorThrowsRefused
	constructorThrowsProved
)

// A known source refusal must not disappear into erased Exceptions fallback.
// The bool wrapper is retained for focused evidence/test clients; production
// consumes the four-state admission result below.
func (c *ClassObjectDumper) proveConstructorInvocationThrows(code *CodeAttribute, method *MemberInfo, member *values.JavaClassMember, pc int, plan *constructorSourceBoundary) (*constructorInvocationThrowsReceipt, bool) {
	r, state := c.constructorInvocationThrowsAdmission(code, method, member, pc, plan)
	return r, state == constructorThrowsProved
}

func (c *ClassObjectDumper) constructorInvocationThrowsAdmission(code *CodeAttribute, method *MemberInfo, member *values.JavaClassMember, pc int, plan *constructorSourceBoundary) (*constructorInvocationThrowsReceipt, constructorThrowsAdmissionState) {
	if c == nil || c.obj == nil || c.FuncCtx == nil || code == nil || method == nil || plan == nil || plan.delegate == nil || plan.pc != pc || plan.carrier != "" || c.FuncCtx.SourceMemberDelegation != nil || c.FuncCtx.ConstructorInvokeBridge != nil || c.FuncCtx.DeclarationSourceName != nil || len(plan.params) > 128 || len(plan.delegate.Arguments) > 128 || c.Work != nil && c.Work.CheckAlloc(256<<10) != nil || !nativeProofWork(c.Work, 1) {
		return nil, constructorThrowsUnknown
	}
	analysis, metadata := c.constructorThrowsProofContext()
	if analysis.FuncCtx.InvocationMetadata != nil {
		if declaration, known := analysis.FuncCtx.InvocationMetadata(strings.ReplaceAll(plan.delegate.ClassName, ".", "/")); known && len(declaration.Methods) <= 256 {
			for _, candidate := range declaration.Methods {
				if !nativeProofWork(c.Work, 1) {
					return nil, constructorThrowsUnknown
				}
				if candidate.Name == "<init>" && candidate.Desc == plan.delegate.Descriptor && !candidate.Generic {
					return nil, constructorThrowsNotApplicable
				}
			}
		}
	}
	name, nameErr := c.obj.getUtf8(method.NameIndex)
	desc, descErr := c.obj.getUtf8(method.DescriptorIndex)
	if nameErr != nil || descErr != nil || name != "<init>" || c.FuncCtx.FunctionName != name || c.FuncCtx.CurrentMethodDesc != desc || strings.ReplaceAll(c.FuncCtx.ClassName, ".", "/") != c.obj.GetClassName() {
		return nil, constructorThrowsUnknown
	}
	originalMethodSig, methodSigKnown := c.constructorThrowsOriginalSignature(method.Attributes)
	originalClassSig, classSigKnown := c.constructorThrowsOriginalSignature(c.obj.Attributes)
	if !c.constructorThrowsHeaderView(originalMethodSig) {
		return nil, constructorThrowsRefused
	}
	if !methodSigKnown || !classSigKnown || originalMethodSig != c.FuncCtx.CurrentMethodSig || originalClassSig != c.FuncCtx.ClassSig {
		return nil, constructorThrowsUnknown
	}
	declared, known := c.constructorThrowsOriginalExceptions(method)
	if !known {
		return nil, constructorThrowsUnknown
	}
	exceptions, proved := analysis.constructorDelegationInvocationThrows(member, pc, plan.delegate)
	if !proved {
		return nil, constructorThrowsUnknown
	}
	if !analysis.constructorThrowsCallerSourceCoverage(originalClassSig, originalMethodSig, desc, declared, exceptions) {
		return nil, constructorThrowsRefused
	}
	r := &constructorInvocationThrowsReceipt{dumper: c, object: c.obj, method: method, code: code, methodFlags: method.AccessFlags, methodAttrCount: len(method.Attributes), classAttrCount: len(c.obj.Attributes), methodDesc: desc, methodSig: c.FuncCtx.CurrentMethodSig, classSig: c.FuncCtx.ClassSig, declared: slices.Clone(declared), plan: plan, call: plan.delegate, receiver: plan.delegate.Object, owner: plan.delegate.ClassName, descriptor: plan.delegate.Descriptor, pc: pc, sourceVersion: c.options.TargetSourceVersion}
	if len(code.Code) == 0 || len(code.Code) > 65535 || !nativeProofWork(c.Work, int64(len(code.Code))+1) {
		return nil, constructorThrowsUnknown
	}
	parameters, known := c.constructorThrowsParameters(plan, desc)
	if !known {
		return nil, constructorThrowsUnknown
	}
	r.parameters = parameters
	input, known := c.constructorThrowsOriginalInput(method, code, pc)
	if !known {
		return nil, constructorThrowsUnknown
	}
	r.inputHash = input.hash
	r.codeHash = sha256.Sum256(code.Code)
	declarationHash, known := analysis.constructorThrowsDeclarationHash(r.owner)
	if !known {
		return nil, constructorThrowsUnknown
	}
	r.declarationHash = declarationHash

	allowed := map[*values.JavaRef]bool{}
	for _, param := range plan.params {
		if !nativeProofWork(c.Work, 1) {
			return nil, constructorThrowsUnknown
		}
		if ref, ok := param.(*values.JavaRef); ok && ref != nil && !ref.IsThis {
			allowed[ref] = true
		}
	}
	remaining := 256
	for _, arg := range plan.delegate.Arguments {
		snapshot, known := c.snapshotConstructorThrowsOperand(arg, allowed, map[values.JavaValue]bool{}, &remaining)
		if !known {
			return nil, constructorThrowsUnknown
		}
		r.arguments = append(r.arguments, arg)
		r.operands = append(r.operands, snapshot)
	}
	if !analysis.constructorThrowsMatchOriginal(input, plan) {
		return nil, constructorThrowsRefused
	}
	r.exceptions = slices.Clone(exceptions)
	r.allUnchecked = true
	for _, exception := range exceptions {
		if !nativeProofWork(c.Work, 1) {
			return nil, constructorThrowsUnknown
		}
		r.allUnchecked = r.allUnchecked && analysis.constructorThrowsUnchecked(exception)
	}
	r.metadataHashes = metadata.hashes
	if !r.fresh(c.FuncCtx) {
		return nil, constructorThrowsRefused
	}
	return r, constructorThrowsProved
}

func (r *constructorInvocationThrowsReceipt) fresh(ctx *class_context.ClassContext) bool {
	if r == nil || r.dumper == nil || r.object == nil || r.method == nil || r.code == nil || r.plan == nil || r.call == nil || ctx == nil {
		return false
	}
	c := r.dumper
	analysis, metadata := c.constructorThrowsProofContext()
	if !c.constructorThrowsHeaderView(r.methodSig) {
		return false
	}
	if !r.sourceParametersFresh(ctx) {
		return false
	}
	if c.obj != r.object || c.FuncCtx != ctx || c.options.TargetSourceVersion != r.sourceVersion || ctx.FunctionName != "<init>" || ctx.CurrentMethodDesc != r.methodDesc || ctx.CurrentMethodSig != r.methodSig || ctx.ClassSig != r.classSig || strings.ReplaceAll(ctx.ClassName, ".", "/") != r.object.GetClassName() || ctx.SourceMemberDelegation != nil || ctx.ConstructorInvokeBridge != nil || ctx.DeclarationSourceName != nil || r.method.AccessFlags != r.methodFlags || len(r.method.Attributes) != r.methodAttrCount || len(r.object.Attributes) != r.classAttrCount || len(r.code.Code) == 0 || len(r.code.Code) > 65535 || len(r.object.Methods) > 65535 || len(r.plan.params) > 128 || r.plan.pc != r.pc || r.plan.carrier != "" || r.plan.delegate != r.call || r.call.Object != r.receiver || r.call.ClassName != r.owner || r.call.Descriptor != r.descriptor || len(r.call.Arguments) != len(r.arguments) {
		return false
	}
	matches := 0
	codes := 0
	for _, method := range r.object.Methods {
		if !nativeProofWork(c.Work, 1) || method == nil {
			return false
		}
		name, nerr := r.object.getUtf8(method.NameIndex)
		desc, derr := r.object.getUtf8(method.DescriptorIndex)
		if nerr != nil || derr != nil {
			return false
		}
		if name == "<init>" && desc == r.methodDesc {
			if method != r.method {
				return false
			}
			matches++
		}
	}
	if matches != 1 {
		return false
	}
	for _, attribute := range r.method.Attributes {
		if !nativeProofWork(c.Work, 1) {
			return false
		}
		if code, ok := attribute.(*CodeAttribute); ok {
			if code != r.code {
				return false
			}
			codes++
		}
	}
	originalMethodSig, methodSigKnown := c.constructorThrowsOriginalSignature(r.method.Attributes)
	originalClassSig, classSigKnown := c.constructorThrowsOriginalSignature(r.object.Attributes)
	if !methodSigKnown || !classSigKnown || originalMethodSig != r.methodSig || originalClassSig != r.classSig {
		return false
	}
	declared, known := c.constructorThrowsOriginalExceptions(r.method)
	if codes != 1 || !known || !slices.Equal(declared, r.declared) || !nativeProofWork(c.Work, int64(len(r.code.Code))+1) || sha256.Sum256(r.code.Code) != r.codeHash {
		return false
	}
	input, known := c.constructorThrowsOriginalInput(r.method, r.code, r.pc)
	if !known || input.hash != r.inputHash || !analysis.constructorThrowsMatchOriginal(input, r.plan) {
		return false
	}
	decoder := core.NewDecompiler(r.code.Code, func(index int) values.JavaValue { return GetValueFromCP(c.ConstantPool, index) })
	decoder.Work = c.Work
	if decoder.ParseOpcode() != nil || !constructorOriginalInvoke(decoder, r.call, func(index int) values.JavaValue { return GetValueFromCP(c.ConstantPool, index) }) {
		return false
	}
	allowed := map[*values.JavaRef]bool{}
	for _, param := range r.plan.params {
		if !nativeProofWork(c.Work, 1) {
			return false
		}
		if ref, ok := param.(*values.JavaRef); ok && ref != nil && !ref.IsThis {
			allowed[ref] = true
		}
	}
	remaining := 256
	for i, arg := range r.call.Arguments {
		if arg != r.arguments[i] {
			return false
		}
		snapshot, known := c.snapshotConstructorThrowsOperand(arg, allowed, map[values.JavaValue]bool{}, &remaining)
		if !known || !constructorThrowsOperandsEqual(r.operands[i], snapshot) {
			return false
		}
	}
	declarationHash, known := analysis.constructorThrowsDeclarationHash(r.owner)
	if !known || declarationHash != r.declarationHash {
		return false
	}
	member := constructorMotionMember(r.object, decoder.OpcodeByPC(uint16(r.pc)), core.OP_INVOKESPECIAL)
	exceptions, proved := analysis.constructorDelegationInvocationThrows(member, r.pc, r.call)
	if !proved || !slices.Equal(exceptions, r.exceptions) {
		return false
	}
	if !analysis.constructorThrowsCallerSourceCoverage(r.classSig, r.methodSig, r.methodDesc, r.declared, exceptions) {
		return false
	}
	unchecked := true
	for _, exception := range exceptions {
		if !nativeProofWork(c.Work, 1) {
			return false
		}
		unchecked = unchecked && analysis.constructorThrowsUnchecked(exception)
	}
	return unchecked == r.allUnchecked && nativeProofWork(c.Work, int64(len(metadata.hashes))+1) && constructorThrowsMetadataEqual(metadata.hashes, r.metadataHashes)
}

// Rendering is an affine operation. Only the actual header writer may claim
// its result, for this plan/delegate/PC. A preview, duplicate or sibling claim
// cannot stand in for source consumption of the analyzed invocation.
func (r *constructorInvocationThrowsReceipt) render(ctx *class_context.ClassContext) (string, error) {
	if r.parameterBindingCount != 1 || r.renderCount != 0 || r.headerCount != 0 || r.emissionCount != 0 || r.plan.throwsReceipt != r || !r.fresh(ctx) {
		return "", fmt.Errorf("stale or consumed constructor invocation throws source binding")
	}
	args := make([]string, len(r.arguments))
	for i, arg := range r.arguments {
		args[i] = arg.String(ctx)
		if args[i] == "" {
			return "", fmt.Errorf("unrenderable constructor invocation throws operand")
		}
	}
	if !r.fresh(ctx) {
		return "", fmt.Errorf("stale constructor invocation throws source binding")
	}
	target := "super"
	if strings.ReplaceAll(r.owner, ".", "/") == strings.ReplaceAll(ctx.ClassName, ".", "/") {
		target = "this"
	}
	source := target + "(" + strings.Join(args, ",") + ")"
	r.renderedSource = source
	r.renderCount = 1
	return source, nil
}

func (r *constructorInvocationThrowsReceipt) claimHeader(plan *constructorSourceBoundary, call *values.FunctionCallExpression, pc int, source string, ctx *class_context.ClassContext) bool {
	if r == nil || plan != r.plan || plan.throwsReceipt != r || call != r.call || pc != r.pc || r.renderCount != 1 || r.headerCount != 0 || r.emissionCount != 0 || r.renderedSource == "" || source != r.renderedSource || !r.fresh(ctx) {
		return false
	}
	r.headerCount = 1
	return true
}

// Exact prefix equality is only a renderer-consistency postcondition, after
// the unique structured header claim. It does not infer types from text or
// search comments, siblings or arbitrary occurrences for a delegation.
func (r *constructorInvocationThrowsReceipt) confirmEmission(plan *constructorSourceBoundary, ctx *class_context.ClassContext, body, indent string) bool {
	if r == nil || plan != r.plan || plan.throwsReceipt != r || r.renderCount != 1 || r.headerCount != 1 || r.emissionCount != 0 || !r.fresh(ctx) || !strings.HasPrefix(body, "\n"+indent+r.renderedSource+";\n") {
		return false
	}
	r.emissionCount = 1
	return true
}

func (r *constructorInvocationThrowsReceipt) consumed(ctx *class_context.ClassContext) bool {
	return r != nil && r.plan != nil && r.parameterBindingCount == 1 && r.plan.throwsReceipt == r && r.renderCount == 1 && r.headerCount == 1 && r.emissionCount == 1 && r.fresh(ctx)
}

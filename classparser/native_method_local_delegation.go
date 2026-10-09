package javaclassparser

import (
	"fmt"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// A no-source-argument local constructor can refer to captured declarations in
// its explicit SUPER call. javac regenerates their hidden stores BEFORE that
// call; moving stores past an observing parent is neither needed nor licensed.
// The physical certificate admits only unchanged captured-word loads. The
// source transaction separately closes lexical bindings and exact target types.
func (c *ClassObjectDumper) nativeMethodLocalDelegationSource(method *MemberInfo) (*dumpedMethods, error) {
	local := c.nativeMethodLocalCurrent
	if local == nil || local.constructor == nil || method == nil || c.nativeMemberRoot == nil || c.FuncCtx == nil {
		return nil, fmt.Errorf("missing original local constructor source transaction")
	}
	ctor := local.constructor
	original, proved := originalMethodLocalSourceConstructor(c.obj, c.nativeMemberRoot.lexicalObjects[local.owner.owner], c.Work)
	if !proved || !sameOriginalMethodLocalConstructor(original, ctor, c.Work) {
		return nil, fmt.Errorf("original local delegation packet changed")
	}
	descriptor, known := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
	if !known || descriptor != ctor.descriptor || len(ctor.delegateParams) == 0 {
		return nil, fmt.Errorf("local constructor delegation identity mismatch")
	}
	target, known := c.constructorMotionClass(ctor.delegateOwner)
	// This allocation constructs the concrete local subclass, not its parent.
	// An abstract class still has a source-invocable constructor; interfaces do
	// not. Original constructor access, descriptor and exception proofs below
	// remain necessary regardless of the parent's abstract methods.
	if !known || target.AccessFlags&0x0200 != 0 {
		return nil, fmt.Errorf("unresolved original local parent declaration")
	}
	// A nonstatic member parent needs a qualified enclosing-instance SUPER
	// certificate. A generic constructor needs instantiated source parameter
	// types rather than erased casts guessed from its executable descriptor.
	if _, _, flags, member := originalMemberOwner(target); member && flags&8 == 0 {
		return nil, fmt.Errorf("local parent needs original enclosing-instance binding")
	}
	// Class formals cannot change a constructor's source parameters when that
	// exact constructor has no Signature (proved below). Validate the parent's
	// declaration, but do not require irrelevant instantiation for its arguments.
	parentSignature, valid := nativeMethodLocalOriginalSignature(target, target.Attributes, c.Work)
	if !valid {
		return nil, fmt.Errorf("invalid local parent class signature")
	}
	if parentSignature != "" {
		if _, valid := types.LexicalTypeParameterErasures([]types.LexicalTypeScope{{Signature: parentSignature}}, nil); !valid {
			return nil, fmt.Errorf("local parent needs closed original generic scope")
		}
		parent, interfaces := types.ParseClassSignatureSupers(parentSignature)
		raw, known := types.RawClassFQN(parent)
		if !known || strings.ReplaceAll(raw, ".", "/") != target.GetSupperClassName() || len(interfaces) != len(target.Interfaces) {
			return nil, fmt.Errorf("local parent signature changes physical hierarchy")
		}
		for i, typ := range interfaces {
			raw, known := types.RawClassFQN(typ)
			physical, resolved := sourceBridgeClassName(target, target.Interfaces[i])
			if !known || !resolved || strings.ReplaceAll(raw, ".", "/") != physical || !nativeProofWork(c.Work, 1) {
				return nil, fmt.Errorf("local parent signature changes physical interface")
			}
		}
	}
	matches := 0
	for _, m := range target.Methods {
		if m == nil || !nativeProofWork(c.Work, 1) {
			return nil, fmt.Errorf("unproved original local parent method table")
		}
		n, named := sourceBridgeUTF8(target, m.NameIndex)
		d, typed := sourceBridgeUTF8(target, m.DescriptorIndex)
		if !named || !typed {
			return nil, fmt.Errorf("invalid original local parent method identity")
		}
		if n != "<init>" || d != ctor.delegateDescriptor {
			continue
		}
		matches++
		if m.AccessFlags&(0x0002|0x0008|0x0100|0x0400) != 0 || m.AccessFlags&0x0005 == 0 && nativeAnonymousCallPackage(target.GetClassName()) != nativeAnonymousCallPackage(c.obj.GetClassName()) {
			return nil, fmt.Errorf("local parent constructor is not source-accessible")
		}
		seenExceptions := false
		for _, a := range m.Attributes {
			if !nativeProofWork(c.Work, 1) {
				return nil, c.Work.Err()
			}
			// A cached invocation provider cannot license a newly resolved
			// constructor with different throws. Close the actual declaration
			// as well as the provider before emitting a no-throws constructor.
			if throws, ok := a.(*ExceptionsAttribute); ok {
				if throws == nil || seenExceptions || len(throws.ExceptionIndexTable) != 0 {
					return nil, fmt.Errorf("local parent requires a separate checked-exception source proof")
				}
				seenExceptions = true
			}
			if _, generic := a.(*SignatureAttribute); generic {
				return nil, fmt.Errorf("local parent constructor needs instantiated generic binding")
			}
		}
	}
	exceptions, declared := exactInvocationExceptions(c.FuncCtx.InvocationMetadata, ctor.delegateOwner, "<init>", ctor.delegateDescriptor)
	if matches != 1 || !declared || len(exceptions) != 0 {
		return nil, fmt.Errorf("local parent requires a separate checked-exception source proof")
	}
	if c.Work != nil && c.Work.CheckAlloc(int64(len(ctor.captures)+len(ctor.delegateParams))*128) != nil {
		return nil, c.Work.Err()
	}
	byParam := map[int]string{}
	bindings := map[string]bool{}
	for field, param := range ctor.captures {
		if !nativeProofWork(c.Work, 1) || byParam[param] != "" {
			return nil, fmt.Errorf("ambiguous original captured delegation word")
		}
		byParam[param] = field
	}
	for _, param := range ctor.delegateParams {
		field := byParam[param]
		binding, bound := local.bindings[field]
		ref := local.sourceRefs[field]
		if !bound || binding == "" || ref == nil || ref.Id == nil || local.captureIDs[field] != ref.Id || !nativeProofWork(c.Work, 1) {
			return nil, fmt.Errorf("unproved local captured delegation declaration")
		}
		if field != ctor.enclosingField {
			bindings[binding] = true
		}
	}
	// An inherited source field can shadow a captured declaration in SUPER's
	// argument scope. Never bind a word merely by matching its val$ spelling.
	seen := map[string]bool{}
	for depth := 0; target != nil; depth++ {
		name := target.GetClassName()
		if depth > 16 || seen[name] || !nativeProofWork(c.Work, 1) {
			return nil, fmt.Errorf("unproved local parent source field scope")
		}
		seen[name] = true
		for _, f := range target.Fields {
			if f == nil || !nativeProofWork(c.Work, 1) {
				return nil, fmt.Errorf("unproved local parent source field table")
			}
			name, known := sourceBridgeUTF8(target, f.NameIndex)
			if !known {
				return nil, fmt.Errorf("invalid local parent source field name")
			}
			if bindings[name] {
				return nil, fmt.Errorf("local parent field shadows captured delegation value")
			}
		}
		if name == "java/lang/Object" {
			break
		}
		target, known = c.constructorMotionClass(target.GetSupperClassName())
		if !known {
			return nil, fmt.Errorf("incomplete local parent source field ancestry")
		}
	}
	mt, err := types.ParseMethodDescriptor(ctor.delegateDescriptor)
	if err != nil || len(mt.FunctionType().ParamTypes) != len(ctor.delegateParams) {
		return nil, fmt.Errorf("invalid local delegation source descriptor")
	}
	physicalParams, _, err := callbinding.Descriptor(ctor.descriptor)
	if err != nil {
		return nil, fmt.Errorf("invalid original local capture descriptor")
	}
	targetParams, _, err := callbinding.Descriptor(ctor.delegateDescriptor)
	if err != nil {
		return nil, fmt.Errorf("invalid original local parent descriptor")
	}
	widening := c.nativeMethodLocalDelegationWideningQuery()
	for i, param := range ctor.delegateParams {
		if !c.nativeMethodLocalDelegationArgumentAssignable(widening, physicalParams[param], targetParams[i]) {
			return nil, fmt.Errorf("local captured delegation requires original reference widening")
		}
	}
	args := make([]string, len(ctor.delegateParams))
	outputSize := int64(len(local.owner.name) + 16)
	for i, param := range ctor.delegateParams {
		field := byParam[param]
		if !nativeProofWork(c.Work, 1) {
			return nil, c.Work.Err()
		}
		typeName := mt.FunctionType().ParamTypes[i].String(c.FuncCtx)
		outputSize += int64(len(typeName) + len(c.obj.GetClassName()) + len(field) + len(local.bindings[field]) + 32)
		if err := c.ensureOutput(outputSize); err != nil {
			return nil, err
		}
		args[i] = "(" + typeName + ")(" + "/*jdec-owned-local-capture:" + c.obj.GetClassName() + ":" + field + "*/" + local.bindings[field] + ")"
	}
	body := "super(" + strings.Join(args, ",") + ");"
	code := local.owner.name + "() {" + body + "}"
	if err := c.ensureOutput(int64(len(code))); err != nil {
		return nil, err
	}
	return &dumpedMethods{methodName: "<init>", code: code, bodyCode: body, member: method, descriptor: descriptor}, nil
}

// A fresh declaration provider uses the original archive hierarchy and the
// pinned platform declaration catalog. Constructor bytecode is a purpose-built
// subset, not the complete platform type graph. Do not borrow the caller's
// possibly stale invocation cache for a new source binding proof.
func (c *ClassObjectDumper) nativeMethodLocalDelegationWideningQuery() *constructorWideningQuery {
	original := c.buildInvocationMetadata()
	return newConstructorWideningQuery(func(name string) (callbinding.Class, bool) {
		if !nativeProofWork(c.Work, 1) {
			return callbinding.Class{}, false
		}
		declaration, known := original(name)
		if !known || declaration.Name != name || !declaration.ParentsComplete || len(declaration.Parents) > 63 || !nativeProofWork(c.Work, int64(len(declaration.Parents))) {
			return callbinding.Class{}, false
		}
		if c.Work != nil && c.Work.CheckAlloc(int64(len(declaration.Parents)+1)*64) != nil {
			return callbinding.Class{}, false
		}
		for _, parent := range declaration.Parents {
			if !nativeSourceBinaryName(parent) {
				return callbinding.Class{}, false
			}
		}
		return callbinding.Class{Name: name, Parents: append([]string(nil), declaration.Parents...), ParentsComplete: true}, true
	})
}

func (c *ClassObjectDumper) nativeMethodLocalDelegationArgumentAssignable(q *constructorWideningQuery, actual, formal string) bool {
	if q == nil || !nativeProofWork(c.Work, int64(len(actual)+len(formal))) {
		return false
	}
	// An edge merely naming an absent non-root type cannot close source binding.
	endpoint := strings.TrimLeft(formal, "[")
	if actual != formal && callbinding.Reference(endpoint) && endpoint != "Ljava/lang/Object;" {
		if _, known := q.class(callbinding.Name(endpoint)); !known {
			return false
		}
	}
	return q.assignable(actual, formal)
}

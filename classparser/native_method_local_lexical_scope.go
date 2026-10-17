package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Original named ownership and static flags delimit lexical type scope. Keep
// signatures ordered by declaration instead of flattening equal-spelled names.
func nativeMethodLocalLexicalSignatures(enclosing *ClassObject, owner *nativeMethodLocalOwner, family *nativeMemberFamily, work *workbudget.Budget) ([]string, bool) {
	if enclosing == nil || owner == nil || owner.declaration == nil || owner.owner != enclosing.GetClassName() {
		return nil, false
	}
	if family != nil && (family.owner == "" || family.lexicalObjects[enclosing.GetClassName()] != enclosing) {
		return nil, false
	}
	found := false
	for _, m := range enclosing.Methods {
		if !nativeProofWork(work, 1) {
			return nil, false
		}
		if m == owner.declaration {
			found = true
		}
	}
	if !found {
		return nil, false
	}
	name, named := sourceBridgeUTF8(enclosing, owner.declaration.NameIndex)
	desc, typed := sourceBridgeUTF8(enclosing, owner.declaration.DescriptorIndex)
	if !named || !typed || name != owner.method || desc != owner.descriptor {
		return nil, false
	}

	if owner.declaration.AccessFlags&8 != 0 {
		return nil, true
	}
	signatures := []string{}
	current := enclosing
	seen := map[*ClassObject]bool{}
	for depth := 0; depth < 64; depth++ {
		if current == nil || seen[current] || !nativeProofWork(work, 1) || work != nil && work.CheckAlloc(int64(depth+1)*128) != nil {
			return nil, false
		}
		if family != nil && family.lexicalObjects[current.GetClassName()] != current {
			return nil, false
		}
		seen[current] = true
		sig, ok := nativeMethodLocalOriginalSignature(current, current.Attributes, work)
		if !ok {
			return nil, false
		}
		signatures = append(signatures, sig)
		if family == nil || current.GetClassName() == family.owner {
			for i, j := 0, len(signatures)-1; i < j; i, j = i+1, j-1 {
				signatures[i], signatures[j] = signatures[j], signatures[i]
			}
			return signatures, true
		}
		child := family.children[current.GetClassName()]
		if child == nil || child.object != current || family.lexicalObjects[current.GetClassName()] != current {
			return nil, false
		}
		actual, name, flags, known := originalMemberOwner(current)
		if !known || actual != child.owner || name != child.name || flags != child.flags || child.static != (flags&8 != 0) {
			return nil, false
		}
		if child.static {
			for i, j := 0, len(signatures)-1; i < j; i, j = i+1, j-1 {
				signatures[i], signatures[j] = signatures[j], signatures[i]
			}
			return signatures, true
		}
		current = family.lexicalObjects[actual]
	}
	return nil, false
}

func nativeMethodLocalOriginalSignature(obj *ClassObject, attrs []AttributeInfo, work *workbudget.Budget) (string, bool) {
	result := ""
	seen := false
	for _, a := range attrs {
		if !nativeProofWork(work, 1) {
			return "", false
		}
		if s, ok := a.(*SignatureAttribute); ok {
			if seen || s == nil {
				return "", false
			}
			seen = true
			var valid bool
			result, valid = sourceBridgeUTF8(obj, s.SignatureIndex)
			if !valid || result == "" || !nativeProofWork(work, int64(len(result))) || work != nil && work.CheckAlloc(int64(len(result))*32) != nil {
				return "", false
			}
		}
	}
	return result, true
}

// Only the physically closed method owner can supply erasures for source
// captures. Existing renderer-only erasure remains conservative elsewhere.
func (c *ClassObjectDumper) nativeMethodLocalSourceErasure(t types.JavaType, local *nativeMethodLocalClass) (string, bool) {
	if c == nil || c.FuncCtx == nil || c.obj == nil || local == nil || local.owner == nil || c.obj.GetClassName() != local.owner.owner || c.FuncCtx.FunctionName != local.executionOwner().method || c.FuncCtx.CurrentMethodDesc != local.executionOwner().descriptor {
		return "", false
	}
	prefix := ""
	for depth := 0; depth < 32; depth++ {
		if t == nil || !nativeProofWork(c.Work, 1) {
			return "", false
		}
		if t.IsArray() {
			prefix += "["
			t = t.ElementType()
			continue
		}
		name, formal := types.RawClassFQN(t)
		if !formal || !c.FuncCtx.IsTypeParam(name) {
			d, ok := values.SourceTypeErasure(t, c.FuncCtx)
			return prefix + d, ok
		}
		classes, ok := nativeMethodLocalLexicalSignatures(c.obj, local.owner, c.nativeMemberRoot, c.Work)
		if !ok {
			return "", false
		}
		signature, ok := nativeMethodLocalOriginalSignature(c.obj, local.owner.declaration.Attributes, c.Work)
		if !ok {
			return "", false
		}
		signature, ok = nativeMethodLocalBindingSignature(c.obj, local.owner, classes, signature, c.nativeMemberRoot, c.Work)
		if !ok || !nativeMethodLocalBindingBudget(classes, signature, c.Work) {
			return "", false
		}
		d, descriptor, ok := types.LexicalOwnerTypeVariableErasure(classes, signature, name)
		return prefix + d, ok && descriptor == local.owner.descriptor
	}
	return "", false
}

// Bound the derived environment work/storage before the grammar resolver
// snapshots declaration maps. The original UTF8 length alone understates a
// deep stack of inherited formals retained by successive owner declarations.
func nativeMethodLocalBindingBudget(classes []string, method string, work *workbudget.Budget) bool {
	if len(classes) > 64 {
		return false
	}
	bytes, names, cells := int64(len(method)), int64(0), int64(0)
	for _, sig := range classes {
		if len(sig) > 65535 || !nativeProofWork(work, int64(len(sig))+1) {
			return false
		}
		bytes += int64(len(sig))
		names += int64(len(types.ClassFormalTypeParamNames(sig)))
		cells += names
	}
	names += int64(len(types.MethodFormalTypeParamNames(method)))
	cells += names
	if bytes > 1<<20 || names > 512 {
		return false
	}
	return nativeProofWork(work, bytes*4+cells*4+names*128) && (work == nil || work.CheckAlloc(bytes*16+cells*96+names*128) == nil)
}

// Instantiate the original direct parent declaration before choosing a source
// constructor. This domain has one accessible fixed-arity candidate at the
// original arity, no constructor-owned formals or throws, and exactly the same
// runtime parameter erasures after instantiation. Consequently the typed view
// cannot add a runtime check or select a different overload. Captured operands
// and their pre-SUPER stores are still proved by the ordinary local packet.
func (c *ClassObjectDumper) nativeMethodLocalGenericDelegationParameters(parent *ClassObject, method *MemberInfo, parentSignature string) ([]types.JavaType, bool) {
	if c == nil || c.obj == nil || c.FuncCtx == nil || c.nativeMemberRoot == nil || parent == nil || method == nil || !nativeProofWork(c.Work, 1) {
		return nil, false
	}
	local := c.nativeMethodLocalCurrent
	if local == nil || local.object != c.obj || local.owner == nil || local.constructor == nil || method.AccessFlags&0x0080 != 0 || parent.GetClassName() != local.constructor.delegateOwner {
		return nil, false
	}
	original, proved := originalMethodLocalSourceConstructor(c.obj, c.nativeMemberRoot.lexicalObjects[local.owner.owner], c.Work)
	if !proved || !sameOriginalMethodLocalConstructor(original, local.constructor, c.Work) {
		return nil, false
	}
	signature, known := nativeMethodLocalOriginalSignature(parent, method.Attributes, c.Work)
	if !known || len(parentSignature) > 65535 || !nativeProofWork(c.Work, int64(len(parentSignature)+len(signature))*130+1) || c.Work != nil && c.Work.CheckAlloc(int64(len(parentSignature)+len(signature))*256) != nil {
		return nil, false
	}
	seenThrows := false
	for _, attribute := range method.Attributes {
		if !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		if declared, ok := attribute.(*ExceptionsAttribute); ok {
			if declared == nil || seenThrows || len(declared.ExceptionIndexTable) != 0 {
				return nil, false
			}
			seenThrows = true
		}
	}
	erased, throws, valid := types.EraseClassBoundMethodSignatureWithThrows(parentSignature, signature)
	if !known || !valid || erased != local.constructor.delegateDescriptor || len(throws) != 0 {
		return nil, false
	}
	physical, result, err := callbinding.Descriptor(erased)
	if err != nil || result != "V" || len(physical) == 0 || len(physical) != len(local.constructor.delegateParams) {
		return nil, false
	}
	matches := 0
	for _, candidate := range parent.Methods {
		if candidate == nil || !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		name, named := sourceBridgeUTF8(parent, candidate.NameIndex)
		desc, typed := sourceBridgeUTF8(parent, candidate.DescriptorIndex)
		if !named || !typed {
			return nil, false
		}
		if name != "<init>" {
			continue
		}
		params, ret, err := callbinding.Descriptor(desc)
		if err != nil || ret != "V" || candidate.AccessFlags&(0x0008|0x0100|0x0400) != 0 {
			return nil, false
		}
		if candidate.AccessFlags&2 != 0 || candidate.AccessFlags&5 == 0 && nativeAnonymousCallPackage(parent.GetClassName()) != nativeAnonymousCallPackage(c.obj.GetClassName()) {
			continue
		}
		if len(params) == len(physical) {
			if candidate != method || desc != erased || candidate.AccessFlags&0x0080 != 0 {
				return nil, false
			}
			matches++
		}
	}
	if matches != 1 {
		return nil, false
	}
	// The fresh provider must describe this exact original class/constructor;
	// a cached Signature cannot license a changed resolver declaration.
	metadata, known := c.buildInvocationMetadata()(parent.GetClassName())
	if !known || metadata.Name != parent.GetClassName() || !metadata.MembersComplete || metadata.Signature != parentSignature {
		return nil, false
	}
	matchedMetadata := 0
	for _, candidate := range metadata.Methods {
		if !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		if candidate.Name == "<init>" && candidate.Desc == erased {
			if candidate.Signature != signature || candidate.Static || candidate.Varargs || candidate.Bridge {
				return nil, false
			}
			matchedMetadata++
		}
	}
	if matchedMetadata != 1 {
		return nil, false
	}
	localSignature, known := nativeMethodLocalOriginalSignature(c.obj, c.obj.Attributes, c.Work)
	if !known || localSignature == "" || c.FuncCtx.ClassSig != localSignature {
		return nil, false
	}
	localParent, interfaces := types.ParseClassSignatureSupers(localSignature)
	view, parameterized := types.AsParameterizedType(localParent)
	formals := types.ClassFormalTypeParamNames(parentSignature)
	if !parameterized || len(view.OwnerSegments) > 1 || strings.ReplaceAll(view.RawClassName, ".", "/") != parent.GetClassName() || c.obj.GetSupperClassName() != parent.GetClassName() || len(formals) == 0 || len(view.TypeArgs) != len(formals) || len(interfaces) != len(c.obj.Interfaces) {
		return nil, false
	}
	for i, typ := range interfaces {
		name, valid := types.RawClassFQN(typ)
		original, known := sourceBridgeClassName(c.obj, c.obj.Interfaces[i])
		if !valid || !known || strings.ReplaceAll(name, ".", "/") != original || !nativeProofWork(c.Work, 1) {
			return nil, false
		}
	}
	owner := c.nativeMemberRoot.lexicalObjects[local.owner.owner]
	if _, known := nativeMethodLocalScopeSignature(c.obj, owner, local.owner, c.Work, c.nativeMemberRoot); !known {
		return nil, false
	}
	classes, known := nativeMethodLocalLexicalSignatures(owner, local.owner, c.nativeMemberRoot, c.Work)
	if !known {
		return nil, false
	}
	methodSignature, known := nativeMethodLocalOriginalSignature(owner, local.owner.declaration.Attributes, c.Work)
	if !known {
		return nil, false
	}
	if methodSignature == "" {
		methodSignature = local.owner.descriptor
	}
	// Source and original lexical scopes must agree before a formal's source
	// spelling can stand for the declaration whose erasure we just proved.
	expected := []string{}
	if methodSignature != local.owner.descriptor {
		expected = append(expected, methodSignature)
	}
	for i := len(classes) - 1; i >= 0; i-- {
		if classes[i] != "" {
			expected = append(expected, classes[i])
		}
	}
	observed := []string{}
	for _, signature := range c.FuncCtx.LexicalTypeParamSignatures {
		if signature != "" {
			observed = append(observed, signature)
		}
	}
	if len(expected) != len(observed) {
		return nil, false
	}
	for i := range expected {
		if expected[i] != observed[i] || !nativeProofWork(c.Work, 1) {
			return nil, false
		}
	}
	if !nativeMethodLocalBindingBudget(append(append([]string(nil), classes...), localSignature), methodSignature, c.Work) || !nativeProofWork(c.Work, int64(len(parentSignature)+len(signature)+len(localSignature))*130+1) {
		return nil, false
	}
	scopes := make([]types.LexicalTypeScope, 0, len(classes)+2)
	for _, signature := range classes {
		scopes = append(scopes, types.LexicalTypeScope{Signature: signature})
	}
	boundMethod, known := nativeMethodLocalBindingSignature(owner, local.owner, classes, methodSignature, c.nativeMemberRoot, c.Work)
	if !known {
		return nil, false
	}
	scopes = append(scopes, types.LexicalTypeScope{Signature: boundMethod, Method: true, Descriptor: local.owner.descriptor}, types.LexicalTypeScope{Signature: localSignature})
	_, references, known := types.SignatureTypeVariableReferences(localSignature)
	if !known {
		return nil, false
	}
	bounds, known := types.LexicalTypeParameterErasures(scopes, references)
	if !known {
		return nil, false
	}
	for _, name := range references {
		if !c.FuncCtx.IsTypeParam(name) {
			return nil, false
		}
	}
	sigma := map[string]types.JavaType{}
	for i, name := range formals {
		if view.TypeArgs[i] == nil || types.IsWildcardType(view.TypeArgs[i]) {
			return nil, false
		}
		sigma[name] = view.TypeArgs[i]
	}
	params, ret := types.ParseMethodSignature(signature)
	if ret == nil || len(params) != len(physical) {
		return nil, false
	}
	for i, param := range params {
		params[i] = types.SubstituteTypeVars(param, sigma)
		erasure, known := nativeMethodLocalGenericParameterErasure(params[i], bounds, 0)
		if !known || erasure != physical[i] || !nativeProofWork(c.Work, 1) {
			return nil, false
		}
	}
	return params, true
}

// Runtime erasure depends on a T-reference's original scope, not on whether a
// concrete class happens to share a visible formal's name.
func nativeMethodLocalGenericParameterErasure(t types.JavaType, bounds map[string]string, depth int) (string, bool) {
	if t == nil || depth > 64 {
		return "", false
	}
	if t.IsArray() {
		element, known := nativeMethodLocalGenericParameterErasure(t.ElementType(), bounds, depth+1)
		return "[" + element, known
	}
	if name, variable := types.TypeVariableName(t); variable {
		return bounds[name], bounds[name] != ""
	}
	if primitive, ok := t.RawType().(*types.JavaPrimer); ok {
		descriptor := map[string]string{"boolean": "Z", "byte": "B", "char": "C", "short": "S", "int": "I", "long": "J", "float": "F", "double": "D"}[primitive.Name]
		return descriptor, descriptor != ""
	}
	name, known := types.RawClassFQN(t)
	return "L" + strings.ReplaceAll(name, ".", "/") + ";", known
}

// Signature describes source parameters; EnclosingMethod retains the physical
// constructor descriptor. Lift only independently certified implicit ports for
// binding/erasure checks. Rendering continues to use the original Signature.
// Equal arity or equal type spelling alone never identifies an implicit port.
func nativeMethodLocalBindingSignature(enclosing *ClassObject, owner *nativeMethodLocalOwner, classes []string, signature string, family *nativeMemberFamily, work *workbudget.Budget) (string, bool) {
	if enclosing == nil || owner == nil || owner.declaration == nil || owner.owner != enclosing.GetClassName() || !nativeProofWork(work, 1) {
		return "", false
	}
	if signature == "" {
		signature = owner.descriptor
	}
	if !nativeMethodLocalBindingBudget(classes, signature, work) {
		return "", false
	}
	erased, _, known := types.EraseLexicalOwnerMethodSignatureWithThrows(classes, signature)
	if !known {
		return "", false
	}
	if erased == owner.descriptor {
		return signature, true
	}
	if owner.method != "<init>" {
		return "", false
	}
	prefix, known := nativeMethodLocalConstructorBindingPrefix(enclosing, owner, family, work)
	if !known || len(prefix) == 0 {
		return "", false
	}
	// The original payload erasures must match every remaining physical port.
	physical, result, err := callbinding.Descriptor(owner.descriptor)
	if err != nil || result != "V" || len(physical) < len(prefix) || "("+strings.Join(physical[len(prefix):], "")+")V" != erased {
		return "", false
	}
	for i, p := range prefix {
		if physical[i] != p {
			return "", false
		}
	}
	start := strings.IndexByte(signature, '(')
	if start < 0 || work != nil && work.CheckAlloc(int64(len(signature)+len(strings.Join(prefix, "")))) != nil {
		return "", false
	}
	lifted := signature[:start+1] + strings.Join(prefix, "") + signature[start+1:]
	actual, _, known := types.EraseLexicalOwnerMethodSignatureWithThrows(classes, lifted)
	return lifted, known && actual == owner.descriptor
}

func nativeMethodLocalConstructorBindingPrefix(enclosing *ClassObject, owner *nativeMethodLocalOwner, family *nativeMemberFamily, work *workbudget.Budget) ([]string, bool) {
	if enclosing == nil || owner == nil || owner.method != "<init>" || owner.declaration == nil || owner.owner != enclosing.GetClassName() {
		return nil, false
	}
	if enclosing.AccessFlags&0x4000 != 0 && enclosing.GetSupperClassName() == "java/lang/Enum" {
		constructors := map[string]*MemberInfo{}
		var order []*MemberInfo
		for _, m := range enclosing.Methods {
			if m == nil || !nativeProofWork(work, 1) {
				return nil, false
			}
			name, nk := sourceBridgeUTF8(enclosing, m.NameIndex)
			desc, dk := sourceBridgeUTF8(enclosing, m.DescriptorIndex)
			if !nk || !dk {
				return nil, false
			}
			if name != "<init>" {
				continue
			}
			if len(order) >= 64 || constructors[desc] != nil || m.AccessFlags != 2 && m.AccessFlags != 0x82 {
				return nil, false
			}
			constructors[desc] = m
			order = append(order, m)
		}
		if constructors[owner.descriptor] != owner.declaration || !nativeEnumConstructorHiddenGraphWithOps(enclosing, order, constructors, work, nativeEnumConstructorBindingOps) {
			return nil, false
		}
		return []string{"Ljava/lang/String;", "I"}, true
	}
	if family == nil || family.lexicalObjects[owner.owner] != enclosing {
		return nil, false
	}
	child := family.children[owner.owner]
	actual, name, flags, known := originalMemberOwner(enclosing)
	if !known || child == nil || child.object != enclosing || child.static || flags&8 != 0 || child.owner != actual || child.name != name || child.flags != flags {
		return nil, false
	}
	reader := NewClassObjectDumper(enclosing)
	reader.Work = work
	reader.nativeMemberRoot = family
	reader.nativeMemberCurrent = child
	if !reader.nativeMemberConstructorHasEnclosingParameter(owner.descriptor) {
		return nil, false
	}

	packet := child.constructors[owner.descriptor]
	if !nativeMethodLocalMemberBindingSnapshot(reader, owner, child, packet, work) {
		return nil, false
	}

	return []string{"L" + actual + ";"}, true
}

// Initialization roles depend on the actual Code, not on whether the source
// metadata can be regenerated. This reader grants binding only; ordinary enum
// synthesis still uses its stricter metadata reader and source emission must
// separately retain the original parameter finality.
func nativeEnumConstructorBindingOps(obj *ClassObject, method *MemberInfo, work *workbudget.Budget) ([]*core.OpCode, bool) {
	if obj == nil || method == nil {
		return nil, false
	}
	var code *CodeAttribute
	for _, a := range method.Attributes {
		if !nativeProofWork(work, 1) {
			return nil, false
		}
		if body, ok := a.(*CodeAttribute); ok {
			if body == nil || code != nil {
				return nil, false
			}
			code = body
		}
	}
	if code == nil || len(code.ExceptionTable) != 0 || !nativeProofWork(work, int64(len(code.Code))) {
		return nil, false
	}
	d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
	d.Work = work
	if d.ParseOpcode() != nil {
		return nil, false
	}
	return constructorMotionOps(d), true
}

// Binding a hidden enclosing port needs its original storage/initialization
// identity, not permission to render every argument expression. In particular,
// a legal subtype AASTORE cannot depend on an unrelated source metadata query.
func nativeMethodLocalMemberBindingSnapshot(reader *ClassObjectDumper, owner *nativeMethodLocalOwner, child *nativeMemberClass, packet *nativeMemberConstructor, work *workbudget.Budget) bool {
	if reader == nil || reader.obj == nil || owner == nil || child == nil || packet == nil {
		return false
	}
	// A THIS edge inherits the target's enclosing storage identity. Recheck the
	// entire finite chain against current Code rather than trusting cached
	// target packets; no argument-rendering capability is inferred here.
	seen := map[string]bool{}
	for depth := 0; depth < 64; depth++ {
		if packet == nil || seen[packet.descriptor] || !nativeProofWork(work, 1) || work != nil && work.CheckAlloc(int64(depth+1)*128) != nil {
			return false
		}
		seen[packet.descriptor] = true
		if !reader.nativeMemberConstructorHasEnclosingParameter(packet.descriptor) || !nativeMethodLocalMemberBindingSnapshotOne(reader, owner, child, packet, work) {
			return false
		}
		if packet.delegateOwner != reader.obj.GetClassName() {
			return true
		}
		target := child.constructors[packet.delegateDescriptor]
		if target == nil {
			return false
		}
		var declaration *MemberInfo
		for _, m := range reader.obj.Methods {
			if m == nil || !nativeProofWork(work, 1) {
				return false
			}
			name, nk := sourceBridgeUTF8(reader.obj, m.NameIndex)
			desc, dk := sourceBridgeUTF8(reader.obj, m.DescriptorIndex)
			if !nk || !dk {
				return false
			}
			if name == "<init>" && desc == target.descriptor {
				if declaration != nil {
					return false
				}
				declaration = m
			}
		}
		if declaration == nil {
			return false
		}
		owner = &nativeMethodLocalOwner{owner: reader.obj.GetClassName(), method: "<init>", descriptor: target.descriptor, declaration: declaration}
		packet = target
	}
	return false
}

func nativeMethodLocalMemberBindingSnapshotOne(reader *ClassObjectDumper, owner *nativeMethodLocalOwner, child *nativeMemberClass, packet *nativeMemberConstructor, work *workbudget.Budget) bool {
	if reader == nil || reader.obj == nil || owner == nil || owner.declaration == nil || child == nil || child.object != reader.obj || packet == nil {
		return false
	}
	object := reader.obj
	matches := 0
	for _, m := range object.Methods {
		if m == nil || !nativeProofWork(work, 1) {
			return false
		}
		name, nk := sourceBridgeUTF8(object, m.NameIndex)
		desc, dk := sourceBridgeUTF8(object, m.DescriptorIndex)
		if !nk || !dk {
			return false
		}
		if name == owner.method && desc == owner.descriptor {
			if m != owner.declaration {
				return false
			}
			matches++
		}
	}
	if matches != 1 || owner.method != "<init>" || packet.descriptor != owner.descriptor || owner.declaration.AccessFlags&(0x0008|0x0100|0x0400) != 0 {
		return false
	}
	var code *CodeAttribute
	for _, a := range owner.declaration.Attributes {
		if !nativeProofWork(work, 1) {
			return false
		}
		if body, ok := a.(*CodeAttribute); ok {
			if body == nil || code != nil {
				return false
			}
			code = body
		}
	}
	if code == nil || !nativeProofWork(work, int64(len(code.Code))) {
		return false
	}
	ir, frames, known := reader.nativeOriginalMethodSnapshot(owner.declaration, code)
	if !known {
		return false
	}
	decoder := core.NewDecompiler(code.Code, nil)
	decoder.Work = work
	if decoder.ParseOpcode() != nil {
		return false
	}
	ops := map[int]*core.OpCode{}
	for _, op := range constructorMotionOps(decoder) {
		if !nativeProofWork(work, 1) {
			return false
		}
		pc := int(op.CurrentOffset)
		if ops[pc] != nil {
			return false
		}
		// Even a verifier-valid write after SUPER can replace the original
		// captured enclosing object. The cached packet is stale in that case.
		if op.Instr == nil {
			return false
		}
		if op.Instr.OpCode == core.OP_PUTFIELD {
			field := constructorMotionMember(object, op, core.OP_PUTFIELD)
			if field == nil || field.Name == object.GetClassName() && field.Member == child.field && (field.Description != "L"+child.owner+";" || pc != packet.capturePC) {
				return false
			}
		}
		ops[pc] = op
	}
	if work != nil && work.CheckAlloc(int64(len(ops))*96) != nil {
		return false
	}
	captureSeen := packet.capturePC < 0
	delegates := 0
	origin := func(slot int) ssabuild.Origin { return ssabuild.Origin{Kind: ssabuild.OriginParam, Slot: slot} }
	for _, record := range frames.Instructions {
		if !nativeProofWork(work, 1) {
			return false
		}
		ins, found := ir.InstrByID(methodir.InstrID(record.PC))
		if !found {
			return false
		}
		pc := int(record.PC)
		if packet.capturePC >= 0 && pc == packet.capturePC {
			field := constructorMotionMember(object, ops[pc], core.OP_PUTFIELD)
			if packet.capturePC >= packet.delegatePC || ins.Opcode != core.OP_PUTFIELD || field == nil || field.Name != object.GetClassName() || field.Member != child.field || field.Description != "L"+child.owner+";" || !constructorMotionField(object, field, true, work) || !record.Before.ThisUninitialized || len(record.Before.Stack) != 2 || record.Before.Stack[0].Kind != frametransfer.UninitThis || len(record.Uses) != 2 || record.Uses[0] != origin(0) || record.Uses[1] != origin(1) {
				return false
			}
			captureSeen = true
		}
		if ins.Opcode != core.OP_INVOKESPECIAL || ins.Member != "<init>" {
			continue
		}
		params, result, err := callbinding.Descriptor(ins.Desc)
		index := len(record.Before.Stack) - nativeMemberParameterWidth(params) - 1
		if err != nil || result != "V" || index < 0 {
			return false
		}
		if record.Before.Stack[index].Kind != frametransfer.UninitThis {
			continue
		}
		invoke := constructorMotionMember(object, ops[pc], core.OP_INVOKESPECIAL)
		if delegates != 0 || pc != packet.delegatePC || invoke == nil || invoke.Name != packet.delegateOwner || invoke.Description != packet.delegateDescriptor || index != 0 || !record.Before.ThisUninitialized || len(record.Uses) != len(params)+1 || record.Uses[0] != origin(0) {
			return false
		}
		after, _, err := frametransfer.Transfer(record.Before, frametransfer.FromIR(ins))
		if err != nil || after.ThisUninitialized {
			return false
		}
		if packet.capturePC < 0 {
			target := child.constructors[invoke.Description]
			if invoke.Name != object.GetClassName() || target == nil || target.descriptor != invoke.Description || len(params) == 0 || params[0] != "L"+child.owner+";" || record.Uses[1] != origin(1) {
				return false
			}
		} else if invoke.Name != object.GetSupperClassName() && invoke.Name != object.GetClassName() {
			return false
		}
		delegates++
	}
	return captureSeen && delegates == 1
}

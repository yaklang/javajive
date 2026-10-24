package javaclassparser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"reflect"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Scope metadata has its own receipt: an inert Object constructor may need no
// executable initialization-cut repair at all. This receipt grants no Code
// edits and activates only on the exact public source return.
func (c *ClassObjectDumper) publishMemberGenericScope(source string) bool {
	a := c.options.sourceConstructorLowering
	if a == nil || !c.reconstructionPhysicalHeader() || c.nativeCaptureFields != nil || c.FuncCtx == nil {
		return true
	}
	_, _, flags, member := originalMemberOwner(c.obj)
	signature, valid := nativeMethodLocalOriginalSignature(c.obj, c.obj.Attributes, c.Work)
	if !member || flags&8 != 0 || !valid {
		return true
	}
	if len(types.ClassFormalTypeParamNames(signature)) != 0 && !originalMemberScopeCapturesClosed(c.obj, c.Work) {
		return true
	}
	if len(c.FuncCtx.ClassTypeParams) == 0 {
		// A non-generic flattened member still owes its declaration row.
		// Enroll only a closed, entirely non-generic lexical path with proved
		// original capture packets. This grants no guessed outer type binder.
		scopes, method, known := c.flattenedMethodLexicalScopesResolved(c.nativeAnnotationDeclarationResolver())
		if !known || method || memberScopeHasFormals(scopes) || !originalMemberScopeCapturesClosed(c.obj, c.Work) {
			return true
		}
	}
	if c.sourceDegraded || legacySourceHasFailureMarker(source) {
		return true
	}
	raw, err := a.jar.readJarEntry(c.obj.GetClassName() + ".class")
	if err != nil || !nativeProofWork(c.Work, int64(len(raw)+len(source))+1) || !bytes.Equal(raw, c.obj.Bytes()) {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if c.Work.CheckAlloc(int64(len(a.genericPending)+1)*96) != nil {
		return false
	}
	if a.genericPending == nil {
		a.genericPending = map[string]constructorNativeSourceOrigin{}
	}
	a.genericPending[c.obj.GetClassName()] = constructorNativeSourceOrigin{sha256.Sum256(raw), sha256.Sum256([]byte(source))}
	return true
}

func (a *sourceConstructorLoweringAuthority) memberGenericSourceReturned(owner string, original []byte, work *workbudget.Budget) bool {
	if a == nil || !nativeProofWork(work, 1) {
		return false
	}
	a.mu.Lock()
	origin, active := a.genericActive[owner]
	_, pending := a.genericPending[owner]
	a.mu.Unlock()
	if !active && !pending && a.parent != nil && a.jar != nil && a.parent.jar != nil && a.jar.sourceCompiler.effective() == a.parent.jar.sourceCompiler.effective() && a.jar.targetSourceVersion == a.parent.jar.targetSourceVersion {
		a.parent.mu.Lock()
		origin, active = a.parent.genericActive[owner]
		a.parent.mu.Unlock()
	}
	return active && origin.sourceHash != ([32]byte{}) && origin.originalHash == sha256.Sum256(original)
}

// An independent member unit may redeclare enclosing formals so javac can
// type-check its source. Discharge that representation only after the returned
// constructor source, original lexical scopes, complete fresh bounds and all
// descriptor erasures agree. No executable byte or existing constant changes.
// Original generic supertypes retain their exact signature and enclosing
// binders. Classes with their own formals instead retain their exact class
// signature and separately discharge the hidden occurrences in member types.
func (a *sourceConstructorLoweringAuthority) restoreMemberGenericScope(original, compiled []byte, work *workbudget.Budget) ([]byte, error) {
	required := false
	stage := "source receipt"
	decline := func() ([]byte, error) {
		if err := work.Check(); err != nil {
			return nil, err
		}
		if required {
			return nil, constructorLoweringFailure(work, "member generic scope: unproved "+stage)
		}
		return compiled, nil
	}
	fail := func(s string) ([]byte, error) {
		return nil, constructorLoweringFailure(work, "member generic scope: "+s)
	}
	if a == nil || a.jar == nil {
		return decline()
	}
	if len(original) > 32<<20 || len(compiled) > 32<<20 || !nativeProofWork(work, int64(len(original)+len(compiled))+1) {
		return fail("input budget")
	}
	want, err := ParseWithBudget(original, work)
	if err != nil {
		return nil, err
	}
	owner, _, flags, member := originalMemberOwner(want)
	if !member || flags&8 != 0 || want.MajorVersion < 49 || want.MajorVersion > 52 || want.MinorVersion != 0 {
		return decline()
	}
	oldSignature, valid := nativeMethodLocalOriginalSignature(want, want.Attributes, work)
	if !valid {
		return decline()
	}
	ownFormals := len(types.ClassFormalTypeParamNames(oldSignature)) != 0
	if !a.memberGenericSourceReturned(want.GetClassName(), original, work) {
		return decline()
	}
	required = true
	reader, err := NewClassReaderWithBudget(compiled, work)
	if err != nil {
		return nil, err
	}
	layout := &compiledMetadataLayout{methods: map[*MemberInfo]compiledMetadataSpan{}, fields: map[*MemberInfo]compiledMetadataSpan{}}
	got, err := parseWithMetadataLayout(reader, nil, layout)
	if err != nil {
		return nil, err
	}
	stage = "physical class identity"
	targetMatches := got.MajorVersion == uint16(a.jar.targetSourceVersion+44)
	if selected := reconstructionCaptureMetadataTarget(a.jar); selected.major != 0 {
		// A physical MR namespace can implicitly compile a base declaration
		// at its selected pre-nest target. Use the same original/version/nest
		// proof as capture metadata, without widening the base source profile.
		targetMatches = selected.matches(want, got)
	}
	if !sourceParameterClassIdentityClosed(want, got) || a.jar.targetSourceVersion < 8 || a.jar.targetSourceVersion > 21 || !targetMatches || got.MinorVersion != 0 || len(want.Methods) > 4096 || len(want.Fields) > 4096 {
		return decline()
	}
	stage = "lexical owner scopes"
	d := NewClassObjectDumper(want)
	d.Work, d.foldSiblingResolver, d.declarationResolver = work, a.jar.enumSiblingResolver(), a.jar.declarationResolver
	scopes, methodScope, valid := d.flattenedMethodLexicalScopesResolved(d.nativeAnnotationDeclarationResolver())
	if !valid || methodScope {
		return decline()
	}
	var signatures []string
	for _, scope := range scopes {
		signatures = append(signatures, scope.Signature)
	}
	if !nativeMethodLocalBindingBudget(signatures, "()V", work) {
		return decline()
	}
	stage = "fresh signature"
	freshSignature, valid := nativeMethodLocalOriginalSignature(got, got.Attributes, work)
	if !valid {
		return decline()
	}
	stage = "original member row"
	key, valid := sourceParameterClassKey(want, want.ThisClass)
	if !valid {
		return decline()
	}
	oldRows, err := sourceAnonymousInnerRows(want, work)
	if err != nil {
		return nil, err
	}
	newRows, err := sourceAnonymousInnerRows(got, work)
	if err != nil {
		return nil, err
	}
	row, exists := oldRows[key]
	if !exists || row.outer == "" || row.name == "" || row.anonymous {
		return decline()
	}
	actual, hasRow := newRows[key]
	if hasRow && (actual.outer != row.outer || actual.name != row.name || actual.flags != row.flags || actual.anonymous) {
		return decline()
	}
	stage = "physical generic supertypes"
	// The idempotent path owes the same physical declaration proof.
	if oldSignature != "" {
		// The original lexical grammar has already been validated above.
		// Its complete supertype sequence must also describe the physical
		// superclass/interfaces, not merely share an erased method result.
		super, interfaces := types.ParseClassSignatureSupers(oldSignature)
		declared := append([]types.JavaType{super}, interfaces...)
		physical := append([]uint16{want.SuperClass}, want.Interfaces...)
		if len(declared) != len(physical) {
			return decline()
		}
		for i, typ := range declared {
			name, ok := types.RawClassFQN(typ)
			expected, known := sourceBridgeClassName(want, physical[i])
			if !ok || !known || strings.ReplaceAll(name, ".", "/") != expected {
				return decline()
			}
		}
	}
	stage = "lexical signature projection"
	completed := freshSignature == oldSignature && hasRow
	rowsOnly := ownFormals || freshSignature == oldSignature && !memberScopeHasFormals(scopes)
	if ownFormals && freshSignature != oldSignature && !memberOwnScopeProjectedClassSignature(want, freshSignature, scopes, work) {
		return decline() // Own bounds/arity are never inferred from erased descriptors.
	}
	if !completed && !rowsOnly && freshSignature == "" {
		return decline()
	}
	freshScopes := scopes
	if ownFormals && !completed {
		freshScopes = append([]types.LexicalTypeScope(nil), scopes...)
		freshScopes[len(freshScopes)-1].Signature = freshSignature
	}
	if !completed && !rowsOnly {
		if !memberScopeProjectedClassSignature(want, freshSignature, scopes, d.nativeAnnotationDeclarationResolver(), work) {
			return decline()
		}
		freshScopes = []types.LexicalTypeScope{{Signature: freshSignature}}
	}
	if ownFormals {
		compiled, err = restoreHiddenMemberSignatures(want, got, compiled, layout, scopes, work)
		if err != nil {
			return nil, err
		}
		reader, err = NewClassReaderWithBudget(compiled, work)
		if err != nil {
			return nil, err
		}
		layout = &compiledMetadataLayout{methods: map[*MemberInfo]compiledMetadataSpan{}, fields: map[*MemberInfo]compiledMetadataSpan{}}
		got, err = parseWithMetadataLayout(reader, nil, layout)
		if err != nil {
			return nil, err
		}
	}
	stage = "method metadata"
	if err := sourceMethodMetadataClosed(want, compiled, work); err != nil {
		return decline()
	}
	provider := d.buildInvocationMetadata()
	stage = "member signatures"
	constructors := 0
	// An access constructor forwards its unchanged typed operands to a
	// private constructor in this same class. It has no capture store of its
	// own. The existing original/fresh forwarding proof establishes that
	// edge; the terminal private constructor still owes the capture proof
	// below. A marker name or the synthetic bit alone grants no exception.
	markerProof := syntheticConstructorMarkerProof(want, got, work, syntheticMethodFlag)
	for _, m := range want.Methods {
		name, _ := sourceBridgeUTF8(want, m.NameIndex)
		descriptor, _ := sourceBridgeUTF8(want, m.DescriptorIndex)
		_, present, valid := sourceMethodSignatureKey(want, m)
		if !valid {
			return decline()
		}
		stage = "method signature erasure " + name + descriptor
		if present {
			// Exact metadata keys contain MUTF-8 code units. Lexical scopes
			// contain decoded identifiers, so grammar/erasure must use the
			// decoded Signature too; supplementary formals are not raw bytes.
			signature, valid := nativeMethodLocalOriginalSignature(want, m.Attributes, work)
			if !valid {
				return decline()
			}
			before, _, ok := types.EraseLexicalScopedMethodSignatureWithThrows(scopes, signature)
			after, _, freshOK := types.EraseLexicalScopedMethodSignatureWithThrows(freshScopes, signature)
			if !ok || !freshOK || before != descriptor || after != descriptor {
				// A member constructor Signature excludes the proved hidden outer port.
				// Align through that typed capture proof, never by argument count alone.
				lifted, proved := constructorEnclosingBindingSignature(want, m, work, d.nativeAnnotationDeclarationResolver())
				if name != "<init>" || !proved {
					return decline()
				}
				before, _, ok = types.EraseLexicalScopedMethodSignatureWithThrows(scopes, lifted)
				after, _, freshOK = types.EraseLexicalScopedMethodSignatureWithThrows(freshScopes, lifted)
				if !ok || !freshOK || before != descriptor || after != descriptor {
					return decline()
				}
			}
		}
		if name != "<init>" {
			continue
		}
		var next *MemberInfo
		for _, n := range got.Methods {
			if !nativeProofWork(work, 1) {
				return decline()
			}
			nn, _ := sourceBridgeUTF8(got, n.NameIndex)
			nd, _ := sourceBridgeUTF8(got, n.DescriptorIndex)
			if nn == name && nd == descriptor {
				next = n
			}
		}
		if m.AccessFlags&syntheticMethodFlag != 0 && markerProof != nil && next != nil && markerProof(next) {
			continue
		}
		constructors++
		stage = "constructor capture " + descriptor
		before := constructorLoweringTypedPacketFor(want, m, true, true, work, provider)
		after := constructorLoweringTypedPacketFor(got, next, true, false, work, provider, 1)
		if after == nil {
			after = constructorLoweringTypedPacketFor(got, next, false, false, work, provider, 1)
		}
		if before == nil || after == nil || !reflect.DeepEqual(before.plan, after.plan) || len(before.plan.captures) != 1 || before.plan.captures[0].slot != 1 || before.plan.captures[0].field.descriptor != "L"+owner+";" {
			return decline()
		}
	}
	stage = "field universe"
	if constructors == 0 || len(want.Fields) != len(got.Fields) {
		return decline()
	}
	stage = "field signature erasure"
	for _, field := range want.Fields {
		_, present, valid := sourceMethodSignatureKey(want, field)
		if !valid {
			return decline()
		}
		if present {
			signature, valid := nativeMethodLocalOriginalSignature(want, field.Attributes, work)
			if !valid {
				return decline()
			}
			descriptor, _ := sourceBridgeUTF8(want, field.DescriptorIndex)
			before, _, ok := types.EraseLexicalScopedMethodSignatureWithThrows(scopes, "()"+signature)
			after, _, freshOK := types.EraseLexicalScopedMethodSignatureWithThrows(freshScopes, "()"+signature)
			if !ok || !freshOK || before != "()"+descriptor || after != before {
				return decline()
			}
		}
	}
	if completed {
		return compiled, nil // Independently checked idempotent original scope.
	}
	if work.CheckAlloc(int64(len(compiled))*3+int64(len(got.ConstantPool))*64) != nil {
		return fail("allocation budget")
	}
	count := len(got.ConstantPool) + 1
	var added []byte
	word := func(n uint16) []byte { return []byte{byte(n >> 8), byte(n)} }
	entry := func(tag byte, data []byte) (uint16, bool) {
		if count >= 65535 || len(added)+len(data)+1 > 1<<20 || !nativeProofWork(work, int64(len(data))+1) {
			return 0, false
		}
		index := uint16(count)
		count++
		added = append(append(added, tag), data...)
		return index, true
	}
	text := func(s string) (uint16, bool) {
		if len(s) == 0 || len(s) > 65535 {
			return 0, false
		}
		return entry(CONSTANT_Utf8, append(word(uint16(len(s))), []byte(s)...))
	}
	class := func(s string) (uint16, bool) {
		i, ok := text(s)
		if !ok {
			return 0, false
		}
		return entry(CONSTANT_Class, word(i))
	}
	var originalSignatureIndex uint16
	if oldSignature != "" {
		for _, attr := range want.Attributes {
			if signature, ok := attr.(*SignatureAttribute); ok {
				raw, known := sourceParameterUTF8Key(want, signature.SignatureIndex)
				if !known {
					return decline()
				}
				var valid bool
				originalSignatureIndex, valid = text(raw)
				if !valid {
					return fail("original signature constant budget")
				}
			}
		}
		if originalSignatureIndex == 0 {
			return decline()
		}
	}
	var newRow []byte
	if !hasRow {
		inner, ik := class(key)
		outer, ok := class(row.outer)
		name, nk := text(row.name)
		if !ik || !ok || !nk {
			return fail("constant budget")
		}
		newRow = append(append(append(word(inner), word(outer)...), word(name)...), word(row.flags)...)
	}
	at := layout.classAttributesStart
	if at < layout.poolEnd || at+2 > len(compiled) || int(binary.BigEndian.Uint16(compiled[at:])) != len(got.Attributes) {
		return fail("class attribute span")
	}
	stage = "declaration table rewrite"
	var payload []byte
	kept, removed := 0, 0
	for pos := at + 2; pos < len(compiled); {
		if len(compiled)-pos < 6 {
			return fail("truncated attribute")
		}
		size := int64(binary.BigEndian.Uint32(compiled[pos+2:]))
		if size > int64(len(compiled)-pos-6) || !nativeProofWork(work, size+1) {
			return fail("attribute length")
		}
		end := pos + 6 + int(size)
		name, ok := sourceBridgeUTF8(got, binary.BigEndian.Uint16(compiled[pos:]))
		if !ok || name == "EnclosingMethod" || name == "RuntimeVisibleTypeAnnotations" || name == "RuntimeInvisibleTypeAnnotations" {
			return decline()
		}
		if name == "Signature" {
			if size != 2 {
				return decline()
			}
			removed++
			if originalSignatureIndex != 0 {
				// Retain the original supertype signature after proving that the
				// only fresh addition was the projected enclosing formal list.
				payload = append(payload, compiled[pos:pos+6]...)
				payload = append(payload, word(originalSignatureIndex)...)
				kept++
			}
		} else {
			raw := bytes.Clone(compiled[pos:end])
			if name == "InnerClasses" && !hasRow {
				if size < 2 || size != 2+8*int64(binary.BigEndian.Uint16(raw[6:])) || binary.BigEndian.Uint16(raw[6:]) == 65535 {
					return fail("inner class table")
				}
				binary.BigEndian.PutUint32(raw[2:], uint32(size+8))
				binary.BigEndian.PutUint16(raw[6:], binary.BigEndian.Uint16(raw[6:])+1)
				raw = append(raw, newRow...)
			}
			payload = append(payload, raw...)
			kept++
		}
		pos = end
	}
	if removed != 1 && freshSignature != "" || removed != 0 && freshSignature == "" {
		return decline()
	}
	if layout.innerClasses == nil && !hasRow {
		attribute, ok := text("InnerClasses")
		if !ok {
			return fail("attribute constant")
		}
		payload = append(payload, append(append(word(attribute), 0, 0, 0, 10, 0, 1), newRow...)...)
		kept++
	}
	return applyCompiledMetadataEdits(compiled, []compiledMetadataEdit{
		{compiledMetadataSpan{8, 10}, word(uint16(count))},
		{compiledMetadataSpan{layout.poolEnd, layout.poolEnd}, added},
		{compiledMetadataSpan{at, len(compiled)}, append(word(uint16(kept)), payload...)},
	}, work)
}

// Constructor binding and class-scope completion share the same declaration
// proof. A concrete first-bound erasure does not authorize arbitrary changes to
// own arity, secondary bounds, variance, supertype arguments or binder roles.
func memberOwnScopeProjectedClassSignature(object *ClassObject, fresh string, scopes []types.LexicalTypeScope, work *workbudget.Budget) bool {
	original, valid := nativeMethodLocalOriginalSignature(object, object.Attributes, work)
	if !valid || len(types.ClassFormalTypeParamNames(original)) == 0 || len(scopes) == 0 || scopes[len(scopes)-1].Signature != original || scopes[len(scopes)-1].Method {
		return false
	}
	var signatures []string
	for _, scope := range scopes {
		signatures = append(signatures, scope.Signature)
	}
	if !nativeMethodLocalBindingBudget(signatures, "()V", work) || !nativeProofWork(work, int64(len(fresh))+1) {
		return false
	}
	projected, valid := types.ProjectHiddenLexicalClassSignature(scopes)
	return valid && fresh == projected
}

func memberScopeHasFormals(scopes []types.LexicalTypeScope) bool {
	for _, scope := range scopes {
		if len(types.ClassFormalTypeParamNames(scope.Signature)) != 0 {
			return true
		}
	}
	return false
}

// Non-generic declaration-row repair is optional, so source publication must
// first establish the original executable domain that completion will check.
// Forwarders are edges to the separately checked private capture constructor.
func originalMemberScopeCapturesClosed(obj *ClassObject, work *workbudget.Budget) bool {
	owner, _, flags, member := originalMemberOwner(obj)
	if !member || flags&StaticFlag != 0 || len(obj.Methods) > 4096 {
		return false
	}
	d := NewClassObjectDumper(obj)
	d.Work = work
	bridges := d.originalNativeConstructorAccessBridges()
	count := 0
	for _, method := range obj.Methods {
		if method == nil || !nativeProofWork(work, 1) {
			return false
		}
		name, known := sourceBridgeUTF8(obj, method.NameIndex)
		if !known {
			return false
		}
		if name != "<init>" {
			continue
		}
		descriptor, known := sourceBridgeUTF8(obj, method.DescriptorIndex)
		if !known {
			return false
		}
		if bridge := bridges[descriptor]; bridge != nil && bridge.method == method {
			continue
		}
		packet := constructorLoweringPacketFor(obj, method, true, true, work)
		if packet == nil || len(packet.plan.captures) != 1 || packet.plan.captures[0].slot != 1 || packet.plan.captures[0].field.descriptor != "L"+owner+";" {
			return false
		}
		count++
	}
	return count > 0
}

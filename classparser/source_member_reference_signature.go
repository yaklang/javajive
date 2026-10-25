package javaclassparser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func (c *ClassObjectDumper) publishMemberReferenceSignatures(source string) bool {
	a := c.options.sourceConstructorLowering
	if a == nil || !c.reconstructionPhysicalHeader() || c.sourceDegraded || legacySourceHasFailureMarker(source) || c.FuncCtx == nil || c.FuncCtx.SiblingSourceClassFormals == nil {
		return true
	}
	found := false
	projected := c.buildSiblingReferenceSourceFormals(nil)
	if projected == nil {
		return true
	}
	for _, method := range append(append([]*MemberInfo(nil), c.obj.Methods...), c.obj.Fields...) {
		if method == nil || !nativeProofWork(c.Work, 1) {
			return false
		}
		_, present, valid := sourceMethodSignatureKey(c.obj, method)
		if !valid {
			return true
		}
		if !present {
			continue
		}
		sig, valid := nativeMethodLocalOriginalSignature(c.obj, method.Attributes, c.Work)
		if !valid {
			return true
		}
		refs, valid := types.SignatureClassReferences(sig)
		if !valid {
			return true
		}
		for _, name := range refs {
			if names, known := projected(strings.ReplaceAll(name, ".", "/")); known && len(names) > 0 {
				found = true
			}
		}
	}
	if !found {
		return c.Work.Check() == nil
	}
	raw, err := a.jar.readJarEntry(c.obj.GetClassName() + ".class")
	if err != nil || !nativeProofWork(c.Work, int64(len(raw)+len(source))+1) || !bytes.Equal(raw, c.obj.Bytes()) {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if c.Work.CheckAlloc(int64(len(a.referencePending)+1)*96) != nil {
		return false
	}
	if a.referencePending == nil {
		a.referencePending = map[string]constructorNativeSourceOrigin{}
	}
	a.referencePending[c.obj.GetClassName()] = constructorNativeSourceOrigin{sha256.Sum256(raw), sha256.Sum256([]byte(source))}
	return true
}

func (a *sourceConstructorLoweringAuthority) memberReferenceSourceReturned(owner string, original []byte, work *workbudget.Budget) bool {
	if a == nil || !nativeProofWork(work, 1) {
		return false
	}
	a.mu.Lock()
	origin, active := a.referenceActive[owner]
	_, pending := a.referencePending[owner]
	a.mu.Unlock()
	if !active && !pending && a.parent != nil && a.jar != nil && a.parent.jar != nil && a.jar.sourceCompiler.effective() == a.parent.jar.sourceCompiler.effective() && a.jar.targetSourceVersion == a.parent.jar.targetSourceVersion {
		a.parent.mu.Lock()
		origin, active = a.parent.referenceActive[owner]
		a.parent.mu.Unlock()
	}
	return active && origin.sourceHash != ([32]byte{}) && origin.originalHash == sha256.Sum256(original)
}

// Restore only a present member Signature whose complete type graph is the
// proved relocation of original enclosing arguments. Existing constants and
// every fresh method byte remain unchanged except the Signature's two-byte
// index. The ordinary final metadata gate still checks all other contracts.
func (a *sourceConstructorLoweringAuthority) restoreMemberReferenceSignatures(original, compiled []byte, work *workbudget.Budget) ([]byte, error) {
	fail := func(s string) ([]byte, error) {
		return nil, constructorLoweringFailure(work, "member reference signature: "+s)
	}
	if a == nil || a.jar == nil {
		return compiled, work.Check()
	}
	if len(original) > 32<<20 || len(compiled) > 32<<20 || !nativeProofWork(work, int64(len(original)+len(compiled))+1) {
		return fail("input budget")
	}
	want, err := ParseWithBudget(original, work)
	if err != nil {
		return nil, err
	}
	if !a.memberReferenceSourceReturned(want.GetClassName(), original, work) {
		return compiled, work.Check()
	}
	reader, err := NewClassReaderWithBudget(compiled, work)
	if err != nil {
		return nil, err
	}
	layout := &compiledMetadataLayout{methods: map[*MemberInfo]compiledMetadataSpan{}, fields: map[*MemberInfo]compiledMetadataSpan{}}
	got, err := parseWithMetadataLayout(reader, nil, layout)
	if err != nil {
		return nil, err
	}
	if want.MajorVersion < 49 || want.MajorVersion > 52 || got.MajorVersion < 52 || got.MajorVersion > 65 || want.MinorVersion != 0 || got.MinorVersion != 0 || !sourceParameterClassIdentityClosed(want, got) || len(want.Methods) != len(got.Methods) || len(want.Fields) != len(got.Fields) || len(got.Methods) > 4096 || len(got.Fields) > 4096 {
		return compiled, work.Check()
	}
	ws, wv := nativeMethodLocalOriginalSignature(want, want.Attributes, work)
	gs, gv := nativeMethodLocalOriginalSignature(got, got.Attributes, work)
	d := NewClassObjectDumper(want)
	d.Work, d.foldSiblingResolver, d.declarationResolver = work, a.jar.enumSiblingResolver(), a.jar.declarationResolver
	scopes, methodScope, valid := d.flattenedMethodLexicalScopesResolved(d.nativeAnnotationDeclarationResolver())
	if !valid || methodScope {
		return compiled, work.Check()
	}
	environment := ws == gs
	if wv && gv && !environment && a.memberGenericSourceReturned(want.GetClassName(), original, work) {
		// The later scope transaction independently discharges this same
		// class projection before any bytes become a finished artifact.
		environment = len(types.ClassFormalTypeParamNames(ws)) == 0 && memberScopeProjectedClassSignature(want, gs, scopes, d.nativeAnnotationDeclarationResolver(), work) || memberOwnScopeProjectedClassSignature(want, gs, scopes, work)
	}
	if !wv || !gv || !environment {
		return compiled, work.Check()
	}
	projected := d.buildSiblingReferenceSourceFormals(func(name string, raw []byte) bool {
		// An own-formal member's independent scope transaction must own the
		// exact returned source. Original declarations alone grant no edit.
		return a.memberGenericSourceReturned(name, raw, work)
	})
	resolve := d.nativeAnnotationDeclarationResolver()
	if projected == nil || work.CheckAlloc(int64(len(compiled))*2+int64(len(got.Methods)+len(got.Fields))*256) != nil {
		return compiled, work.Check()
	}
	oldFields, oldFieldsErr := sourceFieldIdentities(want, work)
	newFields, newFieldsErr := sourceFieldIdentities(got, work)
	if oldFieldsErr != nil || newFieldsErr != nil {
		return fail("field identity")
	}
	count := len(got.ConstantPool) + 1
	var added []byte
	var edits []compiledMetadataEdit
	for group, oldMembers := range [][]*MemberInfo{want.Methods, want.Fields} {
		newMembers, spans := got.Methods, layout.methods
		if group == 1 {
			newMembers, spans = got.Fields, layout.fields
		}
		index := map[sourceLambdaMethodIdentity]*MemberInfo{}
		for _, m := range newMembers {
			if m == nil || !nativeProofWork(work, 1) {
				return fail("fresh method inventory")
			}
			n, nk := sourceBridgeUTF8(got, m.NameIndex)
			d, dk := sourceBridgeUTF8(got, m.DescriptorIndex)
			key := sourceLambdaMethodIdentity{n, d}
			if !nk || !dk || index[key] != nil {
				return fail("fresh method identity")
			}
			index[key] = m
		}
		for _, old := range oldMembers {
			if old == nil || !nativeProofWork(work, 1) {
				return fail("original method inventory")
			}
			name, nk := sourceBridgeUTF8(want, old.NameIndex)
			desc, dk := sourceBridgeUTF8(want, old.DescriptorIndex)
			key := sourceLambdaMethodIdentity{name, desc}
			next := index[key]
			delete(index, key)
			if !nk || !dk || next == nil {
				return compiled, work.Check()
			}
			flagsMatch := next.AccessFlags == old.AccessFlags
			if group == 1 {
				// Java cannot spell a field's synthetic marker. Apply the same
				// complete identity/flag contract as RestoreSyntheticFieldMetadata,
				// which discharges only that missing marker later in FinishClass.
				// This edit never changes flags and cannot admit a fresh marker.
				key := sourceFieldIdentity{name, desc}
				before, after := oldFields[key], newFields[key]
				flagsMatch = before & ^syntheticFieldFlag == after & ^syntheticFieldFlag && (after&syntheticFieldFlag == 0 || before&syntheticFieldFlag != 0)
			}
			if !flagsMatch {
				if group == 1 {
					return fail("field flags changed")
				}
				return compiled, work.Check()
			}
			ws, wp, wv := sourceMethodSignatureKey(want, old)
			gs, gp, gv := sourceMethodSignatureKey(got, next)
			if !wv || !gv {
				return fail("malformed method signature")
			}
			if name == "<init>" || wp == gp && ws == gs {
				continue
			}
			oldText, oldValid := nativeMethodLocalOriginalSignature(want, old.Attributes, work)
			newText, newValid := nativeMethodLocalOriginalSignature(got, next.Attributes, work)
			if group == 1 {
				oldText, newText, desc = "()"+oldText, "()"+newText, "()"+desc
				// A separate active lexical-scope transaction can still owe
				// erasure of hidden outer binders. Defer only its exact complete
				// projection; the later scope completion rechecks it atomically.
				ownerSignature, ownerValid := nativeMethodLocalOriginalSignature(want, want.Attributes, work)
				if oldValid && newValid && wp && ownerValid && len(types.ClassFormalTypeParamNames(ownerSignature)) > 0 && a.memberGenericSourceReturned(want.GetClassName(), original, work) {
					expected, known := types.ProjectHiddenLexicalMethodSignature(scopes, oldText)
					if known && (gp && expected == newText || !gp && expected == desc) {
						continue
					}
				}
				if !wp || !gp {
					return fail("field signature presence changed")
				}
			} else if !wp || !gp {
				continue
			}
			if !oldValid || !newValid || !memberReferenceSignatureProjection(oldText, newText, desc, scopes, projected, resolve, work) {
				if group == 1 {
					return fail("unproved field signature projection")
				}
				continue
			}
			if !memberReferenceAttributesUnannotated(old.Attributes) || !memberReferenceAttributesUnannotated(next.Attributes) {
				if group == 1 {
					return fail("unproved annotated field projection")
				}
				return compiled, work.Check()
			}
			span, known := spans[next]
			if !known || span.start < layout.poolEnd || span.end > len(compiled) || span.end-span.start < 8 {
				return fail("method span")
			}
			// Signature keys are already validated MUTF-8 code-unit payloads.
			// Re-encoding them as UTF-8 text would corrupt supplementary names.
			payload := []byte(ws)
			if len(payload) > 65535 || count >= 65535 || len(added)+len(payload)+3 > 1<<20 {
				return fail("signature constant budget")
			}
			sigIndex := uint16(count)
			count++
			added = append(added, CONSTANT_Utf8, byte(len(payload)>>8), byte(len(payload)))
			added = append(added, payload...)
			raw, found := compiled[span.start:span.end], false
			for pos := 8; pos < len(raw); {
				if len(raw)-pos < 6 {
					return fail("truncated attribute")
				}
				size := int64(binary.BigEndian.Uint32(raw[pos+2:]))
				if size > int64(len(raw)-pos-6) || !nativeProofWork(work, size+1) {
					return fail("attribute length")
				}
				name, known := sourceBridgeUTF8(got, binary.BigEndian.Uint16(raw[pos:]))
				if !known || name == "RuntimeVisibleTypeAnnotations" || name == "RuntimeInvisibleTypeAnnotations" {
					return compiled, work.Check() // A relocated type path needs a separate proof.
				}
				if name == "Signature" {
					if found || size != 2 {
						return fail("signature attribute")
					}
					found = true
					edits = append(edits, compiledMetadataEdit{compiledMetadataSpan{span.start + pos + 6, span.start + pos + 8}, []byte{byte(sigIndex >> 8), byte(sigIndex)}})
				}
				pos += 6 + int(size)
			}
			if !found {
				return fail("missing fresh signature")
			}
		}
	}
	if err := work.Check(); err != nil {
		return nil, err
	}
	if len(edits) > 0 {
		edits = append(edits, compiledMetadataEdit{compiledMetadataSpan{8, 10}, []byte{byte(count >> 8), byte(count)}}, compiledMetadataEdit{compiledMetadataSpan{layout.poolEnd, layout.poolEnd}, added})
	}
	return applyCompiledMetadataEdits(compiled, edits, work)
}

func memberReferenceAttributesUnannotated(attrs []AttributeInfo) bool {
	for _, attr := range attrs {
		switch u := attr.(type) {
		case *TypeAnnotationsAttribute, *RuntimeVisibleTypeAnnotationsAttribute:
			return false
		case *UnparsedAttribute:
			if u != nil && (u.Name == "RuntimeVisibleTypeAnnotations" || u.Name == "RuntimeInvisibleTypeAnnotations") {
				return false
			}
		}
	}
	return true
}

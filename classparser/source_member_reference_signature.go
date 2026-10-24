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
	for _, method := range c.obj.Methods {
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
			if names, known := c.FuncCtx.SiblingSourceClassFormals(strings.ReplaceAll(name, ".", "/")); known && len(names) > 0 {
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

// Restore only a present method Signature whose complete type graph is the
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
	layout := &compiledMetadataLayout{methods: map[*MemberInfo]compiledMetadataSpan{}}
	got, err := parseWithMetadataLayout(reader, nil, layout)
	if err != nil {
		return nil, err
	}
	if want.MajorVersion < 49 || want.MajorVersion > 52 || got.MajorVersion < 52 || got.MajorVersion > 65 || want.MinorVersion != 0 || got.MinorVersion != 0 || !sourceParameterClassIdentityClosed(want, got) || len(want.Methods) != len(got.Methods) || len(got.Methods) > 4096 {
		return compiled, work.Check()
	}
	ws, wv := nativeMethodLocalOriginalSignature(want, want.Attributes, work)
	gs, gv := nativeMethodLocalOriginalSignature(got, got.Attributes, work)
	if !wv || !gv || ws != gs {
		return compiled, work.Check() // Changing the current binder needs its own scope proof.
	}
	d := NewClassObjectDumper(want)
	d.Work, d.foldSiblingResolver, d.declarationResolver = work, a.jar.enumSiblingResolver(), a.jar.declarationResolver
	scopes, methodScope, valid := d.flattenedMethodLexicalScopesResolved(d.nativeAnnotationDeclarationResolver())
	if !valid || methodScope {
		return compiled, work.Check()
	}
	projected, resolve := d.buildSiblingSourceClassFormals(), d.nativeAnnotationDeclarationResolver()
	if projected == nil || work.CheckAlloc(int64(len(compiled))*2+int64(len(got.Methods))*256) != nil {
		return compiled, work.Check()
	}
	index := map[sourceLambdaMethodIdentity]*MemberInfo{}
	for _, m := range got.Methods {
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
	count := len(got.ConstantPool) + 1
	var added []byte
	var edits []compiledMetadataEdit
	for _, old := range want.Methods {
		if old == nil || !nativeProofWork(work, 1) {
			return fail("original method inventory")
		}
		name, nk := sourceBridgeUTF8(want, old.NameIndex)
		desc, dk := sourceBridgeUTF8(want, old.DescriptorIndex)
		key := sourceLambdaMethodIdentity{name, desc}
		next := index[key]
		delete(index, key)
		if !nk || !dk || next == nil || next.AccessFlags != old.AccessFlags {
			return compiled, work.Check()
		}
		ws, wp, wv := sourceMethodSignatureKey(want, old)
		gs, gp, gv := sourceMethodSignatureKey(got, next)
		if !wv || !gv {
			return fail("malformed method signature")
		}
		if name == "<init>" || !wp || !gp || ws == gs {
			continue
		}
		oldText, oldValid := nativeMethodLocalOriginalSignature(want, old.Attributes, work)
		newText, newValid := nativeMethodLocalOriginalSignature(got, next.Attributes, work)
		if !oldValid || !newValid || !memberReferenceSignatureProjection(oldText, newText, desc, scopes, projected, resolve, work) {
			continue
		}
		for _, attr := range old.Attributes {
			switch u := attr.(type) {
			case *TypeAnnotationsAttribute, *RuntimeVisibleTypeAnnotationsAttribute:
				return compiled, work.Check()
			case *UnparsedAttribute:
				if u != nil && (u.Name == "RuntimeVisibleTypeAnnotations" || u.Name == "RuntimeInvisibleTypeAnnotations") {
					return compiled, work.Check()
				}
			}
		}
		span, known := layout.methods[next]
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
	if err := work.Check(); err != nil {
		return nil, err
	}
	if len(edits) > 0 {
		edits = append(edits, compiledMetadataEdit{compiledMetadataSpan{8, 10}, []byte{byte(count >> 8), byte(count)}}, compiledMetadataEdit{compiledMetadataSpan{layout.poolEnd, layout.poolEnd}, added})
	}
	return applyCompiledMetadataEdits(compiled, edits, work)
}

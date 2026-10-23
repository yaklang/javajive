package javaclassparser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"reflect"
	"strings"

	"github.com/yaklang/javajive/internal/workbudget"
)

func (c *ClassObjectDumper) publishConstructorBindingSignatures(source string, methods []*dumpedMethods) bool {
	a := c.options.sourceConstructorLowering
	if a == nil || !c.reconstructionPhysicalHeader() {
		return true
	}
	found := false
	for _, m := range methods {
		if m == nil || m.member == nil || m.methodName != "<init>" {
			continue
		}
		if _, ok := constructorEnclosingBindingSignature(c.obj, m.member, c.Work); !ok {
			continue
		}
		if c.sourceDegraded || m.sourceReconstructionFailed() || legacySourceHasFailureMarker(source) {
			return true
		}
		if m.sourceDumper != c || m.sourceObject != c.obj || m.sourceRequest.role != methodSourceDeclaration || c.dumpedMethodsSet[m.sourceCacheKey] != m || m.code == "" || strings.Count(source, m.code) != 1 {
			return false
		}
		found = true
	}
	if !found {
		return c.Work.Check() == nil
	}
	if !nativeProofWork(c.Work, int64(len(source))+1) {
		return false
	}
	original, err := a.jar.readJarEntry(c.obj.GetClassName() + ".class")
	if err != nil || !bytes.Equal(original, c.obj.Bytes()) {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.bindingPending == nil {
		a.bindingPending = map[string]constructorNativeSourceOrigin{}
	}
	a.bindingPending[c.obj.GetClassName()] = constructorNativeSourceOrigin{sha256.Sum256(original), sha256.Sum256([]byte(source))}
	return true
}

func (a *sourceConstructorLoweringAuthority) constructorBindingSourceReturned(owner string, original []byte, work *workbudget.Budget) bool {
	if a == nil || !nativeProofWork(work, 1) {
		return false
	}
	a.mu.Lock()
	origin, active := a.bindingActive[owner]
	_, pending := a.bindingPending[owner]
	a.mu.Unlock()
	if !active && !pending && a.parent != nil && a.jar != nil && a.parent.jar != nil && a.jar.sourceCompiler.effective() == a.parent.jar.sourceCompiler.effective() && a.jar.targetSourceVersion == a.parent.jar.targetSourceVersion {
		a.parent.mu.Lock()
		origin, active = a.parent.bindingActive[owner]
		a.parent.mu.Unlock()
	}
	return active && origin.sourceHash != ([32]byte{}) && origin.originalHash == sha256.Sum256(original)
}

// Flattening exposes the proved enclosing instance as an explicit source
// parameter. javac consequently writes it into Signature. Contract that one
// port back out only for an actual returned source, exact generic environments,
// and a fresh typed initialization packet matching the original. All Code,
// descriptor, parameter tables and shared constants remain byte-for-byte fresh.
func (a *sourceConstructorLoweringAuthority) restoreConstructorBindingSignatures(original, compiled []byte, work *workbudget.Budget) ([]byte, error) {
	fail := func(s string) ([]byte, error) {
		return nil, constructorLoweringFailure(work, "constructor signature: "+s)
	}
	if len(original) > 32<<20 || len(compiled) > 32<<20 || !nativeProofWork(work, int64(len(original)+len(compiled))+1) {
		return fail("input budget")
	}
	want, err := ParseWithBudget(original, work)
	if err != nil {
		return nil, err
	}
	if !a.constructorBindingSourceReturned(want.GetClassName(), original, work) {
		if err := work.Check(); err != nil {
			return nil, err
		}
		return compiled, nil
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
	ws, wv := nativeMethodLocalOriginalSignature(want, want.Attributes, work)
	gs, gv := nativeMethodLocalOriginalSignature(got, got.Attributes, work)
	if !wv || !gv || ws != gs || !sourceParameterClassIdentityClosed(want, got) || len(want.Methods) != len(got.Methods) {
		return fail("class environment changed")
	}
	if work.CheckAlloc(int64(len(compiled))*2+int64(len(got.ConstantPool))*64) != nil {
		return fail("allocation budget")
	}
	count := len(got.ConstantPool) + 1
	var added []byte
	var edits []compiledMetadataEdit
	for _, before := range want.Methods {
		lifted, proved := constructorEnclosingBindingSignature(want, before, work)
		if !proved {
			continue
		}
		desc, _ := sourceBridgeUTF8(want, before.DescriptorIndex)
		var after *MemberInfo
		for _, m := range got.Methods {
			n, nk := sourceBridgeUTF8(got, m.NameIndex)
			d, dk := sourceBridgeUTF8(got, m.DescriptorIndex)
			if !nk || !dk {
				return fail("method identity")
			}
			if n == "<init>" && d == desc {
				if after != nil {
					return fail("duplicate constructor")
				}
				after = m
			}
		}
		if after == nil || before.AccessFlags != after.AccessFlags {
			return fail("constructor identity")
		}
		signature, valid := nativeMethodLocalOriginalSignature(got, after.Attributes, work)
		originalSig, validOriginal := nativeMethodLocalOriginalSignature(want, before.Attributes, work)
		if !valid || !validOriginal || signature != lifted && signature != originalSig {
			return fail("fresh signature is not the proved lifted binding")
		}
		old := constructorLoweringPacketFor(want, before, true, true, work)
		// lower() has already restored any noncommuting initialization cut. The
		// Object-only case may still have javac's inert call before its capture.
		fresh := constructorLoweringPacketFor(got, after, true, false, work, 1)
		if fresh == nil {
			fresh = constructorLoweringPacketFor(got, after, false, false, work, 1)
		}
		if old == nil || fresh == nil || !reflect.DeepEqual(old.plan, fresh.plan) {
			return fail("fresh constructor packet changed")
		}
		if signature == originalSig {
			continue
		} // independently checked idempotence
		rawKey, present, valid := sourceMethodSignatureKey(want, before)
		if !present || !valid || len(rawKey) > 65535 || count >= 65535 || len(added)+len(rawKey) > 1<<20 {
			return fail("signature constant budget")
		}
		index := uint16(count)
		count++
		added = append(added, CONSTANT_Utf8, byte(len(rawKey)>>8), byte(len(rawKey)))
		added = append(added, []byte(rawKey)...)
		span, known := layout.methods[after]
		if !known || span.start < layout.poolEnd || span.end > len(compiled) || span.end-span.start < 8 {
			return fail("method span")
		}
		raw := compiled[span.start:span.end]
		found := false
		for pos := 8; pos < len(raw); {
			if len(raw)-pos < 6 {
				return fail("truncated attribute")
			}
			size := int64(binary.BigEndian.Uint32(raw[pos+2:]))
			if size > int64(len(raw)-pos-6) || !nativeProofWork(work, size+1) {
				return fail("attribute length")
			}
			name, valid := sourceBridgeUTF8(got, binary.BigEndian.Uint16(raw[pos:]))
			if !valid {
				return fail("attribute name")
			}
			if name == "Signature" {
				if found || size != 2 {
					return fail("signature attribute")
				}
				found = true
				edits = append(edits, compiledMetadataEdit{compiledMetadataSpan{span.start + pos + 6, span.start + pos + 8}, []byte{byte(index >> 8), byte(index)}})
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
	if len(added) > 0 {
		edits = append(edits, compiledMetadataEdit{compiledMetadataSpan{8, 10}, []byte{byte(count >> 8), byte(count)}}, compiledMetadataEdit{compiledMetadataSpan{layout.poolEnd, layout.poolEnd}, added})
	}
	return applyCompiledMetadataEdits(compiled, edits, work)
}

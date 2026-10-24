package javaclassparser

import (
	"bytes"
	"encoding/binary"

	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Called only inside an active original/source receipt transaction after the
// member's unchanged own formal bounds and original lexical path are proved.
// Equality of erased descriptors alone is insufficient: every fresh Signature
// must be the exact source projection, including nested arguments/variance.
// The enclosing scope completion still checks constructors and declaration
// rows before any result leaves the transaction. Fresh Code stays opaque.
func restoreHiddenMemberSignatures(want, got *ClassObject, compiled []byte, layout *compiledMetadataLayout, scopes []types.LexicalTypeScope, work *workbudget.Budget) ([]byte, error) {
	fail := func(s string) ([]byte, error) {
		return nil, constructorLoweringFailure(work, "hidden member signature: "+s)
	}
	if len(want.Methods) != len(got.Methods) || len(want.Fields) != len(got.Fields) || len(want.Methods)+len(want.Fields) > 8192 || work.CheckAlloc(int64(len(compiled))*3+int64(len(got.ConstantPool))*64) != nil {
		return fail("member inventory or allocation budget")
	}
	count := len(got.ConstantPool) + 1
	var added []byte
	word := func(n uint16) []byte { return []byte{byte(n >> 8), byte(n)} }
	text := func(raw string) (uint16, bool) {
		if raw == "" || len(raw) > 65535 || count >= 65535 || len(added)+len(raw)+3 > 1<<20 || !nativeProofWork(work, int64(len(raw))+1) {
			return 0, false
		}
		index := uint16(count)
		count++
		added = append(added, CONSTANT_Utf8, byte(len(raw)>>8), byte(len(raw)))
		added = append(added, raw...)
		return index, true
	}
	var edits []compiledMetadataEdit
	for group, oldMembers := range [][]*MemberInfo{want.Methods, want.Fields} {
		newMembers, spans := got.Methods, layout.methods
		if group == 1 {
			newMembers, spans = got.Fields, layout.fields
		}
		byKey := map[sourceLambdaMethodIdentity]*MemberInfo{}
		for _, m := range newMembers {
			n, nk := sourceBridgeUTF8(got, m.NameIndex)
			d, dk := sourceBridgeUTF8(got, m.DescriptorIndex)
			key := sourceLambdaMethodIdentity{n, d}
			if !nk || !dk || byKey[key] != nil || !nativeProofWork(work, 1) {
				return fail("fresh member identity")
			}
			byKey[key] = m
		}
		for _, before := range oldMembers {
			name, nk := sourceBridgeUTF8(want, before.NameIndex)
			desc, dk := sourceBridgeUTF8(want, before.DescriptorIndex)
			key := sourceLambdaMethodIdentity{name, desc}
			after := byKey[key]
			delete(byKey, key)
			if !nk || !dk || after == nil || (before.AccessFlags^after.AccessFlags)&^syntheticMethodFlag != 0 || !nativeProofWork(work, 1) {
				return fail("original member identity")
			}
			ws, wp, wv := sourceMethodSignatureKey(want, before)
			gs, gp, gv := sourceMethodSignatureKey(got, after)
			if !wv || !gv {
				return fail("malformed member signature")
			}
			if wp == gp && ws == gs {
				continue
			}
			if !wp || group == 0 && name == "<init>" {
				return fail("signature outside hidden-binder projection")
			}
			original, valid := nativeMethodLocalOriginalSignature(want, before.Attributes, work)
			fresh, freshValid := nativeMethodLocalOriginalSignature(got, after.Attributes, work)
			target := desc
			if group == 1 {
				original, target = "()"+original, "()"+desc
			}
			projected, proved := types.ProjectHiddenLexicalMethodSignature(scopes, original)
			erased, _, erasureValid := types.EraseLexicalScopedMethodSignatureWithThrows(scopes, original)
			if group == 1 && gp {
				fresh = "()" + fresh
			}
			if !valid || !freshValid || !proved || !erasureValid || erased != target || gp && projected != fresh || !gp && projected != target {
				return fail("fresh signature differs from lexical projection")
			}
			span, known := spans[after]
			if !known || span.start < layout.poolEnd || span.end > len(compiled) || span.end-span.start < 8 {
				return fail("member span")
			}
			raw := compiled[span.start:span.end]
			if int(binary.BigEndian.Uint16(raw[6:])) != len(after.Attributes) {
				return fail("attribute inventory")
			}
			index, ok := text(ws)
			if !ok {
				return fail("signature constant budget")
			}
			result := bytes.Clone(raw[:8])
			found := false
			for pos := 8; pos < len(raw); {
				if len(raw)-pos < 6 {
					return fail("truncated attribute")
				}
				size := int64(binary.BigEndian.Uint32(raw[pos+2:]))
				if size > int64(len(raw)-pos-6) || !nativeProofWork(work, size+1) {
					return fail("attribute length")
				}
				end := pos + 6 + int(size)
				attr, ok := sourceBridgeUTF8(got, binary.BigEndian.Uint16(raw[pos:]))
				if !ok || attr == "RuntimeVisibleTypeAnnotations" || attr == "RuntimeInvisibleTypeAnnotations" {
					return fail("unproved annotated type projection")
				}
				if attr == "Signature" {
					if found || size != 2 {
						return fail("signature attribute")
					}
					found = true
					result = append(result, raw[pos:pos+6]...)
					result = append(result, word(index)...)
				} else {
					result = append(result, raw[pos:end]...)
				}
				pos = end
			}
			if found != gp {
				return fail("signature inventory")
			}
			if !found {
				if len(after.Attributes) == 65535 {
					return fail("attribute count")
				}
				attr, ok := text("Signature")
				if !ok {
					return fail("attribute constant budget")
				}
				binary.BigEndian.PutUint16(result[6:], uint16(len(after.Attributes)+1))
				result = append(result, word(attr)...)
				result = append(result, 0, 0, 0, 2)
				result = append(result, word(index)...)
			}
			edits = append(edits, compiledMetadataEdit{span, result})
		}
	}
	if len(edits) == 0 {
		return compiled, work.Check()
	}
	edits = append(edits, compiledMetadataEdit{compiledMetadataSpan{8, 10}, word(uint16(count))}, compiledMetadataEdit{compiledMetadataSpan{layout.poolEnd, layout.poolEnd}, added})
	return applyCompiledMetadataEdits(compiled, edits, work)
}

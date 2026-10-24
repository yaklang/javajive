package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"sort"

	"github.com/yaklang/javajive/internal/workbudget"
)

// Edits retain the freshly compiled bytes outside parser-certified metadata
// spans. In particular, no Code attribute is serialized from an original class.
func applyCompiledMetadataEdits(compiled []byte, edits []compiledMetadataEdit, work *workbudget.Budget) ([]byte, error) {
	sort.Slice(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
	size, position := int64(len(compiled)), 0
	for _, edit := range edits {
		if edit.start < position || edit.end < edit.start || edit.end > len(compiled) || !nativeProofWork(work, 1) {
			return nil, sourceLambdaMetadataFailure(work, "overlapping metadata byte spans")
		}
		size += int64(len(edit.data)) - int64(edit.end-edit.start)
		position = edit.end
	}
	if size < 0 || size > 34<<20 {
		return nil, sourceLambdaMetadataFailure(work, "metadata output budget exhausted")
	}
	if work.CheckAlloc(size) != nil || work.Charge(workbudget.CounterOutputBytes, size) != nil {
		return nil, work.Check()
	}
	out := make([]byte, 0, int(size))
	position = 0
	for _, edit := range edits {
		out = append(out, compiled[position:edit.start]...)
		out = append(out, edit.data...)
		position = edit.end
	}
	return append(out, compiled[position:]...), nil
}

func compiledMethodWithoutAttributes(compiled []byte, object *ClassObject, method *MemberInfo, layout *compiledMetadataLayout, names map[string]bool, work *workbudget.Budget) (compiledMetadataEdit, error) {
	fail := func(reason string) (compiledMetadataEdit, error) {
		return compiledMetadataEdit{}, sourceLambdaMetadataFailure(work, reason)
	}
	span, recorded := layout.methods[method]
	if !recorded || span.start < layout.poolEnd || span.end > len(compiled) || span.end-span.start < 8 || len(names) == 0 || len(names) > len(method.Attributes) {
		return fail("unavailable compiled attribute span")
	}
	raw := compiled[span.start:span.end]
	if int(binary.BigEndian.Uint16(raw[6:])) != len(method.Attributes) || work.CheckAlloc(int64(len(raw))*2) != nil {
		return fail("compiled attribute inventory changed")
	}
	result := bytes.Clone(raw[:8])
	removed := map[string]bool{}
	for pos := 8; pos < len(raw); {
		if len(raw)-pos < 6 {
			return fail("truncated compiled attribute")
		}
		length := int64(binary.BigEndian.Uint32(raw[pos+2:]))
		if length > int64(len(raw)-pos-6) {
			return fail("invalid compiled attribute length")
		}
		end := pos + 6 + int(length)
		name, known := sourceBridgeUTF8(object, binary.BigEndian.Uint16(raw[pos:]))
		if !known || !nativeProofWork(work, int64(end-pos)) {
			return fail("invalid compiled attribute identity")
		}
		if names[name] {
			if removed[name] {
				return fail("duplicate compiled attribute")
			}
			removed[name] = true
		} else {
			result = append(result, raw[pos:end]...)
		}
		pos = end
	}
	if len(removed) != len(names) {
		return fail("missing compiled attribute")
	}
	binary.BigEndian.PutUint16(result[6:], uint16(len(method.Attributes)-len(removed)))
	return compiledMetadataEdit{span, result}, nil
}

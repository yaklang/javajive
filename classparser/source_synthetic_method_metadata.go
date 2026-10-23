package javaclassparser

import (
	"encoding/binary"

	"github.com/yaklang/javajive/internal/workbudget"
)

const syntheticMethodFlag uint16 = 0x1000

// Ordinary Java declarations cannot spell ACC_SYNTHETIC. This optional
// reconstruction step restores only that missing bit, under a bijection of
// every original/fresh method and equality of every other method/class flag.
// It does not rename a method, grant bridge/constructor recovery, edit Code or
// make a method-universe/visibility mismatch acceptable. If this narrow client
// does not apply, the existing metadata restorer still owns the final refusal.
func restoreDeclaredSyntheticMethodFlags(original, compiled []byte, work *workbudget.Budget) ([]byte, error) {
	if len(original) > 32<<20 || len(compiled) > 32<<20 || !nativeProofWork(work, int64(len(original)+len(compiled))+1) {
		return nil, sourceLambdaMetadataFailure(work, "synthetic method input budget")
	}
	want, err := ParseWithBudget(original, work)
	if err != nil {
		return nil, err
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
	if !sourceParameterClassIdentityClosed(want, got) || len(want.Methods) != len(got.Methods) || len(got.Methods) > 4096 {
		return compiled, nil
	}
	if err := work.CheckAlloc(int64(len(got.Methods)) * 256); err != nil {
		return nil, err
	}
	identity := func(object *ClassObject, method *MemberInfo) (sourceLambdaMethodIdentity, bool) {
		if method == nil {
			return sourceLambdaMethodIdentity{}, false
		}
		name, nk := sourceParameterUTF8Key(object, method.NameIndex)
		desc, dk := sourceParameterUTF8Key(object, method.DescriptorIndex)
		return sourceLambdaMethodIdentity{name, desc}, nk && dk && name != "" && desc != ""
	}
	flags := map[sourceLambdaMethodIdentity]uint16{}
	for _, method := range want.Methods {
		key, known := identity(want, method)
		if !known || !nativeProofWork(work, int64(len(key.name)+len(key.descriptor))+1) {
			return nil, sourceLambdaMetadataFailure(work, "invalid original synthetic method identity")
		}
		if _, duplicate := flags[key]; duplicate {
			return compiled, nil
		}
		flags[key] = method.AccessFlags
	}
	seen := map[sourceLambdaMethodIdentity]bool{}
	var edits []compiledMetadataEdit
	for _, method := range got.Methods {
		key, known := identity(got, method)
		if !known || !nativeProofWork(work, int64(len(key.name)+len(key.descriptor))+1) {
			return nil, sourceLambdaMetadataFailure(work, "invalid compiled synthetic method identity")
		}
		expected, found := flags[key]
		if !found || seen[key] || expected&^syntheticMethodFlag != method.AccessFlags&^syntheticMethodFlag || method.AccessFlags&syntheticMethodFlag != 0 && expected&syntheticMethodFlag == 0 {
			return compiled, nil
		}
		seen[key] = true
		if expected == method.AccessFlags {
			continue
		}
		// Constructor/initializer and bridge protocols have additional source
		// obligations. This client grants no edits to those special methods.
		if key.name == "<init>" || key.name == "<clinit>" || expected&0x0040 != 0 {
			return compiled, nil
		}
		span, recorded := layout.methods[method]
		if !recorded || span.start < layout.poolEnd || span.end > len(compiled) || span.end-span.start < 8 || binary.BigEndian.Uint16(compiled[span.start:]) != method.AccessFlags {
			return nil, sourceLambdaMetadataFailure(work, "invalid synthetic method declaration span")
		}
		data := make([]byte, 2)
		binary.BigEndian.PutUint16(data, expected)
		edits = append(edits, compiledMetadataEdit{compiledMetadataSpan{span.start, span.start + 2}, data})
	}
	if len(edits) == 0 {
		return compiled, work.Check()
	}
	return applyCompiledMetadataEdits(compiled, edits, work)
}

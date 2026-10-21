package javaclassparser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/mutf8"
	"github.com/yaklang/javajive/internal/workbudget"
)

type sourceParameterMetadataRow struct {
	// nil is name_index=0. UTF-16 units distinguish a literal backslash-u name
	// from an unpaired surrogate that has the same legacy display spelling.
	name  []uint16
	flags uint16
}

// MUTF-8 keys retain exact UTF-16 identity, even when legacy display strings
// coincide. Names are data here; no source spelling establishes JVM identity.
func sourceParameterUTF8Key(object *ClassObject, index uint16) (string, bool) {
	if index == 0 || int(index) > len(object.ConstantPool) {
		return "", false
	}
	value, ok := object.ConstantPool[index-1].(*ConstantUtf8Info)
	if !ok || value == nil || value.DecodeErr != nil {
		return "", false
	}
	payload, err := value.mutf8BytesChecked()
	return string(payload), err == nil
}

func sourceParameterClassKey(object *ClassObject, index uint16) (string, bool) {
	if index == 0 || int(index) > len(object.ConstantPool) {
		return "", false
	}
	value, ok := object.ConstantPool[index-1].(*ConstantClassInfo)
	if !ok || value == nil {
		return "", false
	}
	return sourceParameterUTF8Key(object, value.NameIndex)
}

func sourceParameterClassIdentityClosed(left, right *ClassObject) bool {
	if left.AccessFlags != right.AccessFlags || len(left.Interfaces) != len(right.Interfaces) {
		return false
	}
	for _, pair := range [][2]uint16{{left.ThisClass, right.ThisClass}, {left.SuperClass, right.SuperClass}} {
		if pair[0] == 0 || pair[1] == 0 {
			if pair[0] != pair[1] {
				return false
			}
			continue
		}
		a, ak := sourceParameterClassKey(left, pair[0])
		b, bk := sourceParameterClassKey(right, pair[1])
		if !ak || !bk || a != b {
			return false
		}
	}
	for i, index := range left.Interfaces {
		a, ak := sourceParameterClassKey(left, index)
		b, bk := sourceParameterClassKey(right, right.Interfaces[i])
		if !ak || !bk || a != b {
			return false
		}
	}
	return true
}

func sourceParameterRows(object *ClassObject, method *MemberInfo, work *workbudget.Budget, textWork *int64) ([]sourceParameterMetadataRow, bool, error) {
	fail := func(reason string) ([]sourceParameterMetadataRow, bool, error) {
		return nil, false, sourceLambdaMetadataFailure(work, "parameter metadata: "+reason)
	}
	descriptor, known := sourceBridgeUTF8(object, method.DescriptorIndex)
	params, _, err := callbinding.Descriptor(descriptor)
	if !known || err != nil || len(params) > 255 || !nativeProofWork(work, int64(len(descriptor))+1) {
		return fail("invalid method descriptor")
	}
	var table *UnparsedAttribute
	for _, attribute := range method.Attributes {
		if raw, ok := attribute.(*UnparsedAttribute); ok && raw != nil && raw.Name == "MethodParameters" {
			if table != nil {
				return fail("duplicate table")
			}
			table = raw
		}
	}
	if table == nil {
		return nil, false, nil
	}
	if object.MajorVersion < 52 || table.Length != uint32(len(table.Info)) || len(table.Info) != 1+4*len(params) || int(table.Info[0]) != len(params) || work.CheckAlloc(int64(len(params))*64) != nil {
		return fail("invalid table length or parameter count")
	}
	rows := make([]sourceParameterMetadataRow, len(params))
	for i := range rows {
		index := binary.BigEndian.Uint16(table.Info[1+4*i:])
		flags := binary.BigEndian.Uint16(table.Info[3+4*i:])
		if flags & ^uint16(0x9010) != 0 || !nativeProofWork(work, 1) {
			return fail("unknown parameter flags")
		}
		rows[i].flags = flags
		if index == 0 {
			continue
		}
		if int(index) > len(object.ConstantPool) {
			return fail("invalid parameter name index")
		}
		name, ok := object.ConstantPool[index-1].(*ConstantUtf8Info)
		if !ok || name == nil || name.DecodeErr != nil {
			return fail("invalid parameter name constant")
		}
		units := name.semanticUnits()
		cost := int64(len(units))*2 + 1
		if len(units) == 0 || cost > (128<<20)-*textWork || !nativeProofWork(work, cost) {
			return fail("invalid or excessive parameter name")
		}
		*textWork += cost
		for _, unit := range units {
			if unit == '.' || unit == ';' || unit == '[' || unit == '/' {
				return fail("parameter name is not an unqualified JVM name")
			}
		}
		rows[i].name = units
	}
	return rows, true, nil
}

func sourceParameterRowsEqual(left, right []sourceParameterMetadataRow) bool {
	return slices.EqualFunc(left, right, func(a, b sourceParameterMetadataRow) bool {
		return a.flags == b.flags && (a.name == nil) == (b.name == nil) && slices.Equal(a.name, b.name)
	})
}

// RestoreMethodParameterMetadata restores exactly the original MethodParameters
// tables, including their absence, for a bijection of owner/name/descriptor and
// unchanged method flags. Parameter rows follow descriptor positions, not local
// word slots or source spellings. No executable byte or other attribute changes.
// It does not certify a source reconstruction, JVM validity or method behavior.
func RestoreMethodParameterMetadata(original, compiled []byte, work *WorkBudget) ([]byte, error) {
	return restoreMethodParameterMetadata(original, compiled, nil, work)
}

// A helper spelling may differ only under the separately sealed lambda source
// plan. Public standalone use has no renaming projection. The subsequent lambda
// restorer still closes its own original and rebuilt factory/body obligations.
func restoreMethodParameterMetadata(original, compiled []byte, plan *sourceLambdaMetadataPlan, work *workbudget.Budget) ([]byte, error) {
	fail := func(reason string) ([]byte, error) {
		return nil, sourceLambdaMetadataFailure(work, "parameter metadata: "+reason)
	}
	if len(original) > 32<<20 || len(compiled) > 32<<20 || !nativeProofWork(work, int64(len(original)+len(compiled))+1) {
		return fail("input budget exhausted")
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
	if want.GetClassName() == "" || !sourceParameterClassIdentityClosed(want, got) || len(want.Methods) != len(got.Methods) || len(got.Methods) > 4096 {
		return fail("class or method universe changed")
	}
	if work.CheckAlloc(int64(len(compiled))*2+int64(len(got.ConstantPool))*64+int64(len(got.Methods))*256) != nil {
		return nil, work.Check()
	}
	identity := func(object *ClassObject, method *MemberInfo) (sourceLambdaMethodIdentity, bool) {
		if method == nil {
			return sourceLambdaMethodIdentity{}, false
		}
		n, nk := sourceParameterUTF8Key(object, method.NameIndex)
		d, dk := sourceParameterUTF8Key(object, method.DescriptorIndex)
		return sourceLambdaMethodIdentity{n, d}, nk && dk && n != "" && d != ""
	}
	expected := map[sourceLambdaMethodIdentity]*MemberInfo{}
	for _, method := range want.Methods {
		key, known := identity(want, method)
		if !known || expected[key] != nil || !nativeProofWork(work, int64(len(key.name)+len(key.descriptor))+1) {
			return fail("invalid original method identity")
		}
		expected[key] = method
	}
	projection := map[sourceLambdaMethodIdentity]sourceLambdaMethodIdentity{}
	projectedOriginals := map[sourceLambdaMethodIdentity]bool{}
	planIdentity := func(key sourceLambdaMethodIdentity) sourceLambdaMethodIdentity {
		return sourceLambdaMethodIdentity{string(NewUtf8FromString(key.name).mutf8Bytes()), string(NewUtf8FromString(key.descriptor).mutf8Bytes())}
	}
	if plan != nil {
		if plan.owner != want.GetClassName() || plan.originalHash != sha256.Sum256(original) || plan.sourceHash == ([32]byte{}) || len(plan.methods) == 0 || len(plan.methods) > 4096 {
			return fail("unpublished or stale lambda projection")
		}
		for _, method := range plan.methods {
			originalKey, generatedKey := planIdentity(method.original), planIdentity(method.generated)
			if method.original.descriptor != method.generated.descriptor || projection[generatedKey].name != "" || projectedOriginals[originalKey] || expected[originalKey] == nil || !sourceLambdaImplementationFlags(expected[originalKey].AccessFlags) || !nativeProofWork(work, 1) {
				return fail("ambiguous lambda projection")
			}
			projection[generatedKey] = originalKey
			projectedOriginals[originalKey] = true
		}
	}
	// A new UTF8 is encoded from lossless logical name units. Never edit shared
	// existing constants, import an original CP blob, or use its CP indexes.
	poolCount := len(got.ConstantPool) + 1
	utf8 := map[string]uint16{}
	for i, constant := range got.ConstantPool {
		if value, ok := constant.(*ConstantUtf8Info); ok && value != nil {
			payload, err := value.mutf8BytesChecked()
			if err != nil || !nativeProofWork(work, int64(len(payload))+1) {
				return fail("invalid or excessive compiled UTF8 pool")
			}
			utf8[string(payload)] = uint16(i + 1)
		}
	}
	var added []byte
	appendName := func(units []uint16) (uint16, error) {
		if units == nil {
			return 0, nil
		}
		payload := mutf8.Encode(units)
		if len(payload) > 65535 || !nativeProofWork(work, int64(len(payload))+1) {
			return 0, sourceLambdaMetadataFailure(work, "parameter name payload exceeds UTF8 limit")
		}
		key := string(payload)
		if index := utf8[key]; index != 0 {
			return index, nil
		}
		if poolCount >= 65535 || len(added)+3+len(payload) > 1<<20 {
			return 0, sourceLambdaMetadataFailure(work, "parameter name constant-pool budget exhausted")
		}
		index := uint16(poolCount)
		poolCount++
		entry := []byte{CONSTANT_Utf8, byte(len(payload) >> 8), byte(len(payload))}
		added = append(added, entry...)
		added = append(added, payload...)
		utf8[key] = index
		return index, nil
	}
	var edits []compiledMetadataEdit
	seen := map[sourceLambdaMethodIdentity]bool{}
	textWork := int64(0)
	for _, method := range got.Methods {
		key, known := identity(got, method)
		originalKey := key
		if mapped, found := projection[key]; found {
			originalKey = mapped
		}
		originalMethod := expected[originalKey]
		if !known || originalMethod == nil || seen[originalKey] || method.AccessFlags != originalMethod.AccessFlags || !nativeProofWork(work, int64(len(key.name)+len(key.descriptor))+1) {
			return fail("method identity or flags changed")
		}
		seen[originalKey] = true
		before, beforeTable, err := sourceParameterRows(want, originalMethod, work, &textWork)
		if err != nil {
			return nil, err
		}
		after, afterTable, err := sourceParameterRows(got, method, work, &textWork)
		if err != nil {
			return nil, err
		}
		if beforeTable == afterTable && sourceParameterRowsEqual(before, after) {
			continue
		}
		span, recorded := layout.methods[method]
		if !recorded || span.start < layout.poolEnd || span.end > len(compiled) || span.end-span.start < 8 {
			return fail("unavailable compiled method span")
		}
		raw := bytes.Clone(compiled[span.start:span.end])
		attributes := len(method.Attributes)
		if afterTable {
			edit, err := compiledMethodWithoutAttributes(compiled, got, method, layout, map[string]bool{"MethodParameters": true}, work)
			if err != nil {
				return nil, err
			}
			raw = edit.data
			attributes--
		}
		if beforeTable {
			if attributes >= 65535 {
				return fail("method attribute count exhausted")
			}
			nameIndex, err := appendName(NewUtf8FromString("MethodParameters").semanticUnits())
			if err != nil {
				return nil, err
			}
			attribute := make([]byte, 7+4*len(before))
			binary.BigEndian.PutUint16(attribute, nameIndex)
			binary.BigEndian.PutUint32(attribute[2:], uint32(1+4*len(before)))
			attribute[6] = byte(len(before))
			for i, row := range before {
				index, err := appendName(row.name)
				if err != nil {
					return nil, err
				}
				binary.BigEndian.PutUint16(attribute[7+4*i:], index)
				binary.BigEndian.PutUint16(attribute[9+4*i:], row.flags)
			}
			raw = append(raw, attribute...)
			attributes++
		}
		binary.BigEndian.PutUint16(raw[6:], uint16(attributes))
		edits = append(edits, compiledMetadataEdit{span, raw})
	}
	if len(seen) != len(expected) {
		return fail("missing original method identity")
	}
	for key, original := range projection {
		if !seen[original] {
			return fail(fmt.Sprintf("missing generated lambda %s%s", key.name, key.descriptor))
		}
	}
	if len(added) != 0 {
		count := []byte{byte(poolCount >> 8), byte(poolCount)}
		edits = append(edits, compiledMetadataEdit{compiledMetadataSpan{8, 10}, count}, compiledMetadataEdit{compiledMetadataSpan{layout.poolEnd, layout.poolEnd}, added})
	}
	return applyCompiledMetadataEdits(compiled, edits, work)
}

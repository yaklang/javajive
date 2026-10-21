package javaclassparser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"slices"
	"sort"

	"github.com/yaklang/javajive/internal/workbudget"
)

// These positions are collected by the ordinary bounded parser only when a
// metadata editor requests them. Code and unknown attribute payloads remain
// opaque original spans of the newly compiled class, never reserialized.
type compiledMetadataSpan struct{ start, end int }
type compiledMetadataLayout struct {
	poolEnd   int
	constants []compiledMetadataSpan
	methods   map[*MemberInfo]compiledMetadataSpan
}

type sourceLambdaMethodIdentity struct{ name, descriptor string }
type sourceLambdaMetadataMethod struct {
	original, generated sourceLambdaMethodIdentity
}
type sourceLambdaMetadataPlan struct {
	owner                    string
	originalHash, sourceHash [32]byte
	methods                  []sourceLambdaMetadataMethod
}

type compiledMetadataEdit struct {
	compiledMetadataSpan
	data []byte
}

// The final source/factory proof supplies this private plan. Restoration only
// changes generated helper declarations, their own symbolic method references,
// and absent implementation MethodParameters. It never imports original Code,
// a constant-pool blob, a debug table, or a method body.
func restoreLambdaMethodMetadata(original, compiled []byte, plan *sourceLambdaMetadataPlan, work *workbudget.Budget) ([]byte, error) {
	if plan == nil || plan.owner == "" || plan.sourceHash == ([32]byte{}) || sha256.Sum256(original) != plan.originalHash || len(plan.methods) == 0 || len(plan.methods) > 4096 || len(compiled) > 32<<20 {
		return nil, sourceLambdaMetadataFailure(work, "missing or stale source plan")
	}
	if !nativeProofWork(work, int64(len(original)+len(compiled))+int64(len(plan.methods))*8) {
		return nil, work.Check()
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
	if want.GetClassName() != plan.owner || got.GetClassName() != plan.owner || want.MajorVersion != 52 || got.MajorVersion != 52 || want.MinorVersion != got.MinorVersion || want.AccessFlags != got.AccessFlags || want.GetSupperClassName() != got.GetSupperClassName() || !slices.Equal(want.GetInterfacesName(), got.GetInterfacesName()) || len(want.Methods) != len(got.Methods) || len(got.Methods) > 4096 {
		return nil, sourceLambdaMetadataFailure(work, "class or method universe changed")
	}
	if work != nil && work.CheckAlloc(int64(len(compiled))*2+int64(len(got.ConstantPool))*64+int64(len(got.Methods))*256) != nil {
		return nil, work.Check()
	}
	identity := func(obj *ClassObject, method *MemberInfo) (sourceLambdaMethodIdentity, bool) {
		if method == nil {
			return sourceLambdaMethodIdentity{}, false
		}
		n, nk := sourceBridgeUTF8(obj, method.NameIndex)
		d, dk := sourceBridgeUTF8(obj, method.DescriptorIndex)
		return sourceLambdaMethodIdentity{n, d}, nk && dk && n != "" && d != ""
	}
	expected := map[sourceLambdaMethodIdentity]*MemberInfo{}
	for _, method := range want.Methods {
		key, known := identity(want, method)
		if !known || expected[key] != nil || !nativeProofWork(work, 1) {
			return nil, sourceLambdaMetadataFailure(work, "invalid original method identity")
		}
		expected[key] = method
	}
	projection := map[sourceLambdaMethodIdentity]sourceLambdaMethodIdentity{}
	originals := map[sourceLambdaMethodIdentity]bool{}
	for _, method := range plan.methods {
		if method.original.descriptor != method.generated.descriptor || originals[method.original] || projection[method.generated].name != "" || expected[method.original] == nil || expected[method.original].AccessFlags != 0x100a || !nativeProofWork(work, 1) {
			return nil, sourceLambdaMetadataFailure(work, "ambiguous implementation plan")
		}
		projection[method.generated], originals[method.original] = method.original, true
	}
	// Appending constants preserves every existing index, including indexes in
	// Code, stack maps, annotations and opaque attributes. Never rename a shared
	// UTF8 in place: application strings may contain the same spelling.
	poolCount := len(got.ConstantPool) + 1
	added := []byte{}
	utf8 := map[string]uint16{}
	for i, item := range got.ConstantPool {
		if text, ok := item.(*ConstantUtf8Info); ok && text != nil {
			utf8[text.Value] = uint16(i + 1)
		}
	}
	appendEntry := func(data []byte) (uint16, error) {
		if poolCount >= 65535 || len(added)+len(data) > 1<<20 || !nativeProofWork(work, int64(len(data))+1) {
			return 0, sourceLambdaMetadataFailure(work, "constant pool budget exhausted")
		}
		index := uint16(poolCount)
		poolCount++
		added = append(added, data...)
		return index, nil
	}
	appendUTF8 := func(text string) (uint16, error) {
		if index := utf8[text]; index != 0 {
			return index, nil
		}
		payload, err := NewUtf8FromString(text).mutf8BytesChecked()
		if err != nil {
			return 0, err
		}
		data := make([]byte, 3, 3+len(payload))
		data[0] = CONSTANT_Utf8
		binary.BigEndian.PutUint16(data[1:], uint16(len(payload)))
		index, err := appendEntry(append(data, payload...))
		if err == nil {
			utf8[text] = index
		}
		return index, err
	}
	var edits []compiledMetadataEdit
	textWork := int64(0)
	seen := map[sourceLambdaMethodIdentity]bool{}
	for _, method := range got.Methods {
		key, known := identity(got, method)
		originalKey, projected := projection[key]
		if !projected {
			originalKey = key
		}
		originalMethod := expected[originalKey]
		if !known || originalMethod == nil || seen[originalKey] || method.AccessFlags != originalMethod.AccessFlags || !nativeProofWork(work, 1) {
			return nil, sourceLambdaMetadataFailure(work, "compiled method declaration changed")
		}
		seen[originalKey] = true
		span, recorded := layout.methods[method]
		if !recorded || span.start < layout.poolEnd || span.end > len(compiled) || span.end-span.start < 8 {
			return nil, sourceLambdaMetadataFailure(work, "unavailable compiled method span")
		}
		raw := bytes.Clone(compiled[span.start:span.end])
		if projected && key.name != originalKey.name {
			name, err := appendUTF8(originalKey.name)
			if err != nil {
				return nil, err
			}
			binary.BigEndian.PutUint16(raw[2:], name)
		}
		wantNames, wantFlags, wantTable, valid := sourceMethodParameterTable(want, originalMethod)
		gotNames, gotFlags, gotTable, validGot := sourceMethodParameterTable(got, method)
		if !valid || !validGot || !sourceMethodMetadataTextWork(work, &textWork, wantNames, gotNames) {
			return nil, sourceLambdaMetadataFailure(work, "invalid parameter table")
		}
		if projected {
			metadata, valid := readLambdaParameterMetadata(want, originalMethod, work)
			if !valid || !sourceLambdaOrdinaryFactoryOnly(want, originalMethod, metadata, work) {
				return nil, sourceLambdaMetadataFailure(work, "original factory contract changed")
			}
			compiledMetadata, valid := readLambdaParameterMetadata(got, method, work)
			if !valid || !sourceLambdaOrdinaryFactoryOnly(got, method, compiledMetadata, work) {
				return nil, sourceLambdaMetadataFailure(work, "rebuilt factory contract changed")
			}
		}
		if gotTable != wantTable || !slices.Equal(wantNames, gotNames) || !slices.Equal(wantFlags, gotFlags) {
			// An absent table is the one known modern-javac lowering difference.
			// Never erase or overwrite an unexpected generated table.
			if !projected || gotTable || !wantTable || len(wantNames) == 0 || len(wantNames) > 255 || len(method.Attributes) == 65535 {
				return nil, sourceLambdaMetadataFailure(work, "parameter contract changed")
			}
			nameIndex, err := appendUTF8("MethodParameters")
			if err != nil {
				return nil, err
			}
			attribute := make([]byte, 7+4*len(wantNames))
			binary.BigEndian.PutUint16(attribute, nameIndex)
			binary.BigEndian.PutUint32(attribute[2:], uint32(1+4*len(wantNames)))
			attribute[6] = byte(len(wantNames))
			for i, text := range wantNames {
				index, err := appendUTF8(text)
				if err != nil {
					return nil, err
				}
				binary.BigEndian.PutUint16(attribute[7+4*i:], index)
				binary.BigEndian.PutUint16(attribute[9+4*i:], wantFlags[i])
			}
			binary.BigEndian.PutUint16(raw[6:], uint16(len(method.Attributes)+1))
			raw = append(raw, attribute...)
		}
		if !bytes.Equal(raw, compiled[span.start:span.end]) {
			edits = append(edits, compiledMetadataEdit{span, raw})
		}
	}
	for key := range projection {
		if !seen[projection[key]] {
			return nil, sourceLambdaMetadataFailure(work, "missing rebuilt implementation")
		}
	}
	for i, item := range got.ConstantPool {
		var ref *ConstantMemberrefInfo
		switch value := item.(type) {
		case *ConstantMethodrefInfo:
			ref = &value.ConstantMemberrefInfo
		case *ConstantInterfaceMethodrefInfo:
			ref = &value.ConstantMemberrefInfo
		default:
			continue
		}
		owner, known := sourceBridgeClassName(got, ref.ClassIndex)
		if !known {
			return nil, sourceLambdaMetadataFailure(work, "invalid symbolic method owner")
		}
		if owner != plan.owner {
			continue
		}
		nt, ok := got.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
		if !ok || nt == nil {
			return nil, sourceLambdaMetadataFailure(work, "invalid symbolic method identity")
		}
		name, nk := sourceBridgeUTF8(got, nt.NameIndex)
		descriptor, dk := sourceBridgeUTF8(got, nt.DescriptorIndex)
		replacement, mapped := projection[sourceLambdaMethodIdentity{name, descriptor}]
		if !nk || !dk {
			return nil, sourceLambdaMetadataFailure(work, "invalid symbolic method text")
		}
		if !mapped || replacement.name == name {
			continue
		}
		nameIndex, err := appendUTF8(replacement.name)
		if err != nil {
			return nil, err
		}
		entry := []byte{CONSTANT_NameAndType, 0, 0, 0, 0}
		binary.BigEndian.PutUint16(entry[1:], nameIndex)
		binary.BigEndian.PutUint16(entry[3:], nt.DescriptorIndex)
		index, err := appendEntry(entry)
		if err != nil {
			return nil, err
		}
		span := layout.constants[i]
		if span.end-span.start != 5 || span.start < 10 || span.end > layout.poolEnd {
			return nil, sourceLambdaMetadataFailure(work, "invalid symbolic reference span")
		}
		raw := bytes.Clone(compiled[span.start:span.end])
		binary.BigEndian.PutUint16(raw[3:], index)
		edits = append(edits, compiledMetadataEdit{span, raw})
	}
	if len(added) != 0 {
		count := []byte{0, 0}
		binary.BigEndian.PutUint16(count, uint16(poolCount))
		edits = append(edits, compiledMetadataEdit{compiledMetadataSpan{8, 10}, count}, compiledMetadataEdit{compiledMetadataSpan{layout.poolEnd, layout.poolEnd}, added})
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
	size := len(compiled)
	for _, edit := range edits {
		size += len(edit.data) - (edit.end - edit.start)
	}
	if size > 34<<20 {
		return nil, sourceLambdaMetadataFailure(work, "output budget exhausted")
	}
	if err := work.CheckAlloc(int64(size)); err != nil {
		return nil, err
	}
	if err := work.Charge(workbudget.CounterOutputBytes, int64(size)); err != nil {
		return nil, err
	}
	result := make([]byte, 0, size)
	position := 0
	for _, edit := range edits {
		if edit.start < position || edit.end < edit.start || edit.end > len(compiled) {
			return nil, sourceLambdaMetadataFailure(work, "overlapping byte spans")
		}
		result = append(result, compiled[position:edit.start]...)
		result = append(result, edit.data...)
		position = edit.end
	}
	return append(result, compiled[position:]...), nil
}

// Logical rows, not CP indexes, define MethodParameters equality. Nil and an
// empty table are distinct contracts. The parser validates descriptor arity.
func sourceMethodParameterTable(object *ClassObject, method *MemberInfo) ([]string, []uint16, bool, bool) {
	var names []string
	var flags []uint16
	present := false
	for _, attribute := range method.Attributes {
		table, ok := attribute.(*UnparsedAttribute)
		if !ok || table == nil || table.Name != "MethodParameters" {
			continue
		}
		if present || len(table.Info) == 0 || table.Length != uint32(len(table.Info)) || len(table.Info) != 1+4*int(table.Info[0]) {
			return nil, nil, false, false
		}
		present = true
		for i := 0; i < int(table.Info[0]); i++ {
			index := binary.BigEndian.Uint16(table.Info[1+4*i:])
			text, known := sourceBridgeUTF8(object, index)
			if index != 0 && (!known || text == "") {
				return nil, nil, false, false
			}
			names = append(names, text)
			flags = append(flags, binary.BigEndian.Uint16(table.Info[3+4*i:]))
		}
	}
	return names, flags, present, true
}

func sourceLambdaOrdinaryFactoryOnly(object *ClassObject, method *MemberInfo, metadata *lambdaParameterMetadata, work *workbudget.Budget) bool {
	if method == nil || method.AccessFlags != 0x100a {
		return false
	}
	closed := false
	if metadata == nil {
		closed = nativeLambdaBodyOriginalUseClosed(object, method, work)
	} else {
		closed = nativeLambdaBodyOriginalUseClosed(object, method, work, metadata)
	}
	if !closed {
		return false
	}
	return sourceLambdaOrdinaryBootstrapClosed(object, work)
}

func sourceLambdaOrdinaryBootstrapClosed(object *ClassObject, work *workbudget.Budget) bool {
	if object == nil {
		return false
	}
	// Serializable/marker adaptation uses a different protocol and can embed
	// helper names in executable deserializer constants. No rename authority.
	for _, attribute := range object.Attributes {
		if table, ok := attribute.(*BootstrapMethodsAttribute); ok {
			if table == nil || len(table.BootstrapMethods) > 4096 {
				return false
			}
			for _, row := range table.BootstrapMethods {
				if row == nil || row.BootstrapMethodRef == 0 || int(row.BootstrapMethodRef) > len(object.ConstantPool) || !nativeProofWork(work, int64(len(row.BootstrapArguments))+1) {
					return false
				}
				handle, ok := object.ConstantPool[row.BootstrapMethodRef-1].(*ConstantMethodHandleInfo)
				if !ok || handle == nil {
					return false
				}
				target, known := nativeLambdaOriginalHandleTarget(object, handle)
				if !known || target.owner != "java/lang/invoke/LambdaMetafactory" || target.name != "metafactory" || len(row.BootstrapArguments) != 3 {
					return false
				}
			}
		}
	}
	return true
}

// Metadata editing may not silently change another original method, including
// an implicit constructor parameter table. An absent final-source plan grants
// no lambda edits, and a lambda plan grants no constructor/ordinary ABI edits.
func sourceMethodMetadataClosed(original *ClassObject, compiled []byte, work *workbudget.Budget) error {
	rebuilt, err := ParseWithBudget(compiled, work)
	if err != nil {
		return err
	}
	if original.GetClassName() != rebuilt.GetClassName() || len(original.Methods) != len(rebuilt.Methods) || !nativeProofWork(work, int64(len(original.Methods)+len(rebuilt.Methods))+1) {
		return sourceLambdaMetadataFailure(work, "method universe changed without an applicable source plan")
	}
	if err := work.CheckAlloc(int64(len(rebuilt.Methods)) * 128); err != nil {
		return err
	}
	index := map[sourceLambdaMethodIdentity]*MemberInfo{}
	for _, method := range rebuilt.Methods {
		name, nk := sourceBridgeUTF8(rebuilt, method.NameIndex)
		descriptor, dk := sourceBridgeUTF8(rebuilt, method.DescriptorIndex)
		key := sourceLambdaMethodIdentity{name, descriptor}
		if !nk || !dk || index[key] != nil {
			return sourceLambdaMetadataFailure(work, "invalid compiled method declaration")
		}
		index[key] = method
	}
	seen := map[sourceLambdaMethodIdentity]bool{}
	textWork := int64(0)
	for _, method := range original.Methods {
		name, nk := sourceBridgeUTF8(original, method.NameIndex)
		descriptor, dk := sourceBridgeUTF8(original, method.DescriptorIndex)
		key := sourceLambdaMethodIdentity{name, descriptor}
		next := index[key]
		if !nk || !dk || seen[key] || next == nil || next.AccessFlags != method.AccessFlags {
			return sourceLambdaMetadataFailure(work, "method identities or flags changed without an applicable source plan")
		}
		seen[key] = true
		wn, wf, wt, valid := sourceMethodParameterTable(original, method)
		gn, gf, gt, validGot := sourceMethodParameterTable(rebuilt, next)
		if !valid || !validGot || !sourceMethodMetadataTextWork(work, &textWork, wn, gn) || wt != gt || !slices.Equal(wn, gn) || !slices.Equal(wf, gf) {
			return sourceLambdaMetadataFailure(work, "parameter tables changed without an applicable source plan")
		}
	}
	return nil
}

// CP strings can be shared by many parameter rows. Comparing their logical
// text must bill those repeated bytes, rather than only the compact row count.
func sourceMethodMetadataTextWork(work *workbudget.Budget, total *int64, tables ...[]string) bool {
	for _, names := range tables {
		for _, name := range names {
			cost := int64(len(name)) + 1
			if cost > (128<<20)-*total || !nativeProofWork(work, cost) {
				return false
			}
			*total += cost
		}
	}
	return true
}

// A metadata refusal must not hide a sticky resource/cancellation failure from
// a nested parser or original factory proof. No partial class is returned.
func sourceLambdaMetadataFailure(work *workbudget.Budget, reason string) error {
	if err := work.Check(); err != nil {
		return err
	}
	return fmt.Errorf("lambda metadata: %s", reason)
}

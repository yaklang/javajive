package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/yaklang/javajive/internal/workbudget"
)

const syntheticFieldFlag uint16 = 0x1000

type sourceFieldIdentity struct {
	name, descriptor string
}

func sourceFieldIdentities(object *ClassObject, work *workbudget.Budget) (map[sourceFieldIdentity]uint16, error) {
	if object == nil {
		return nil, fmt.Errorf("field metadata: unavailable declaration")
	}
	if !nativeProofWork(work, int64(len(object.Fields))+1) {
		return nil, work.Check()
	}
	if work != nil {
		if err := work.CheckAlloc(int64(len(object.Fields)) * 64); err != nil {
			return nil, err
		}
	}
	fields := make(map[sourceFieldIdentity]uint16, len(object.Fields))
	for _, field := range object.Fields {
		if field == nil {
			return nil, fmt.Errorf("field metadata: nil declaration")
		}
		name, named := sourceBridgeUTF8(object, field.NameIndex)
		descriptor, typed := sourceBridgeUTF8(object, field.DescriptorIndex)
		flags, _, known := nativeMemberEffectiveFieldFlags(field, work)
		if !named || !typed || name == "" || descriptor == "" || !known {
			return nil, fmt.Errorf("field metadata: invalid identity or flags")
		}
		key := sourceFieldIdentity{name, descriptor}
		if _, duplicate := fields[key]; duplicate {
			return nil, fmt.Errorf("field metadata: duplicate field %s %s", name, descriptor)
		}
		fields[key] = flags
	}
	return fields, nil
}

// RestoreSyntheticFieldMetadata restores original JVM field synthetic markers
// after source compilation. Java has no field modifier for this metadata.
// Both inputs must declare exactly the same class and field identities; every
// other field flag must already match. An unexpected generated marker is an
// error. This does not certify method semantics or any other class metadata.
//
// The result is a fresh copy of compiled, edited only at parser-proved field
// access_flags offsets. No original method, constant pool, attribute, debug
// table or executable byte is copied. Legacy Synthetic attributes and flags
// have the same JVM meaning; a missing marker is restored as ACC_SYNTHETIC.
// A nil work budget retains the standalone parser's ordinary behavior.
func RestoreSyntheticFieldMetadata(original, compiled []byte, work *WorkBudget) ([]byte, error) {
	authority, err := ParseWithBudget(original, work)
	if err != nil {
		return nil, fmt.Errorf("field metadata original: %w", err)
	}
	reader, err := NewClassReaderWithBudget(compiled, work)
	if err != nil {
		return nil, err
	}
	offsets := make(map[*MemberInfo]int)
	rebuilt, err := parseWithFieldFlagOffsets(reader, offsets)
	if err != nil {
		return nil, fmt.Errorf("field metadata compiled: %w", err)
	}
	if authority.GetClassName() == "" || authority.GetClassName() != rebuilt.GetClassName() {
		return nil, fmt.Errorf("field metadata: class identity changed")
	}
	expected, err := sourceFieldIdentities(authority, work)
	if err != nil {
		return nil, err
	}
	actual, err := sourceFieldIdentities(rebuilt, work)
	if err != nil {
		return nil, err
	}
	if len(expected) != len(actual) {
		return nil, fmt.Errorf("field metadata: field universe changed")
	}
	// Validate the complete result before allocating or editing output. A
	// mismatch at the last field must not expose a partially repaired class.
	for _, field := range rebuilt.Fields {
		name, _ := sourceBridgeUTF8(rebuilt, field.NameIndex)
		descriptor, _ := sourceBridgeUTF8(rebuilt, field.DescriptorIndex)
		key := sourceFieldIdentity{name, descriptor}
		want, present := expected[key]
		got := actual[key]
		if !present || want & ^syntheticFieldFlag != got & ^syntheticFieldFlag || got&syntheticFieldFlag != 0 && want&syntheticFieldFlag == 0 {
			return nil, fmt.Errorf("field metadata: original contract changed for %s %s", name, descriptor)
		}
		offset, recorded := offsets[field]
		if !recorded || offset < 8 || offset > len(compiled)-2 || binary.BigEndian.Uint16(compiled[offset:offset+2]) != field.AccessFlags {
			return nil, fmt.Errorf("field metadata: invalid original field byte span")
		}
	}
	if work != nil {
		if err := work.CheckAlloc(int64(len(compiled))); err != nil {
			return nil, err
		}
		if err := work.Charge(workbudget.CounterOutputBytes, int64(len(compiled))); err != nil {
			return nil, err
		}
	}
	result := bytes.Clone(compiled)
	for _, field := range rebuilt.Fields {
		name, _ := sourceBridgeUTF8(rebuilt, field.NameIndex)
		descriptor, _ := sourceBridgeUTF8(rebuilt, field.DescriptorIndex)
		key := sourceFieldIdentity{name, descriptor}
		if expected[key]&syntheticFieldFlag != 0 && actual[key]&syntheticFieldFlag == 0 {
			offset := offsets[field]
			binary.BigEndian.PutUint16(result[offset:offset+2], field.AccessFlags|syntheticFieldFlag)
		}
	}
	return result, nil
}

package javaclassparser

import (
	"testing"

	"github.com/yaklang/javajive/classparser/classes"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialAbsentParameterTableDoesNotRequireConstructorNamingProof(t *testing.T) {
	raw, err := classes.FS.ReadFile("LongTest.class")
	if err != nil {
		t.Fatal(err)
	}
	obj, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	var constructor *MemberInfo
	for _, m := range obj.Methods {
		if name, _ := sourceBridgeUTF8(obj, m.NameIndex); name == "<init>" {
			constructor = m
			break
		}
	}
	if constructor == nil {
		t.Fatal("missing original constructor")
	}
	for _, a := range constructor.Attributes {
		if x, ok := a.(*UnparsedAttribute); ok && x.Name == "MethodParameters" {
			t.Fatal("fixture already has a parameter table")
		}
	}
	descriptor, _ := sourceBridgeUTF8(obj, constructor.DescriptorIndex)
	d := NewClassObjectDumper(obj)
	d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: int64(len(constructor.Attributes) + 1)})
	names, known := d.originalOrdinaryMethodParameterNames(constructor, descriptor)
	if !known || names != nil || d.Work.Err() != nil {
		t.Fatal("absent table spent work proving an unused naming domain", names, known, d.Work.Err())
	}
	if d.Work.Used(workbudget.CounterGraphScans) != int64(len(constructor.Attributes)+1) {
		t.Fatal("original attribute inventory was not completely inspected")
	}
	// A real table still requires the complete original constructor domain.
	// The same tiny work allowance cannot authorize skipping that proof.
	constructor.Attributes = append(constructor.Attributes, &UnparsedAttribute{Name: "MethodParameters", Length: 1, Info: []byte{0}})
	d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: int64(len(constructor.Attributes) + 1)})
	d.originalOrdinaryMethodParameterNames(constructor, descriptor)
	if d.Work.Err() == nil {
		t.Fatal("present table bypassed the original constructor domain proof")
	}
}

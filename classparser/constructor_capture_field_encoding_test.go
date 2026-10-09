package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialConstructorCaptureFieldOriginalSyntheticEncodings(t *testing.T) {
	for _, row := range []struct {
		name  string
		flags uint16
		attrs []AttributeInfo
		want  bool
	}{
		{"flag", 0x1010, nil, true},
		{"attribute", 0x10, []AttributeInfo{&SyntheticAttribute{}}, true},
		{"both", 0x1010, []AttributeInfo{&SyntheticAttribute{}}, true},
		{"attribute and signature", 0x10, []AttributeInfo{&SignatureAttribute{}, &SyntheticAttribute{}}, true},
		{"ordinary final", 0x10, nil, false},
		{"mutable synthetic", 0x1000, nil, false},
		{"mutable attribute", 0, []AttributeInfo{&SyntheticAttribute{}}, false},
		{"static capture", 0x18, []AttributeInfo{&SyntheticAttribute{}}, false},
		{"volatile capture", 0x50, []AttributeInfo{&SyntheticAttribute{}}, false},
		{"duplicate", 0x1010, []AttributeInfo{&SyntheticAttribute{}, &SyntheticAttribute{}}, false},
		{"nonempty", 0x1010, []AttributeInfo{&SyntheticAttribute{AttrLen: 1}}, false},
		{"nil marker", 0x1010, []AttributeInfo{(*SyntheticAttribute)(nil)}, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			obj := NewClassObject()
			obj.ThisClass = uint16(obj.ConstantPoolManager.AddUtf8Info("CaptureOwner"))
			field := &MemberInfo{AccessFlags: row.flags, NameIndex: uint16(obj.ConstantPoolManager.AddUtf8Info("stored")), DescriptorIndex: uint16(obj.ConstantPoolManager.AddUtf8Info("J")), Attributes: row.attrs}
			obj.Fields = []*MemberInfo{field}
			member := &values.JavaClassMember{Name: "CaptureOwner", Member: "stored", Description: "J"}
			if got := constructorMotionField(obj, member, true, nil); got != row.want {
				t.Fatalf("capture accepted=%v want=%v", got, row.want)
			}
			if field.AccessFlags != row.flags || len(field.Attributes) != len(row.attrs) {
				t.Fatal("original field mutated")
			}
			member.Description = "I"
			if constructorMotionField(obj, member, true, nil) {
				t.Fatal("wrong erased descriptor accepted")
			}
			member.Description = "J"
			member.Name = "OtherOwner"
			if constructorMotionField(obj, member, true, nil) {
				t.Fatal("wrong declaring owner accepted")
			}
			member.Name = "CaptureOwner"
			obj.Fields = append(obj.Fields, field)
			if constructorMotionField(obj, member, true, nil) {
				t.Fatal("duplicate physical storage accepted")
			}
		})
	}
}

// Field and attribute traversal use the caller's shared proof budget.
func TestAdversarialConstructorCaptureFieldEncodingBudget(t *testing.T) {
	obj := NewClassObject()
	obj.ThisClass = uint16(obj.ConstantPoolManager.AddUtf8Info("CaptureOwner"))
	obj.Fields = []*MemberInfo{{AccessFlags: 0x10, NameIndex: uint16(obj.ConstantPoolManager.AddUtf8Info("stored")), DescriptorIndex: uint16(obj.ConstantPoolManager.AddUtf8Info("J")), Attributes: []AttributeInfo{&SignatureAttribute{}, &SyntheticAttribute{}}}}
	member := &values.JavaClassMember{Name: "CaptureOwner", Member: "stored", Description: "J"}
	for _, limit := range []int64{1, 2, 3} {
		work := workbudget.New(nil, workbudget.Limits{MaxGraphScans: limit})
		if got := constructorMotionField(obj, member, true, work); got != (limit == 3) {
			t.Fatalf("field+attribute scans with budget %d: %v", limit, got)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	work := workbudget.New(ctx, workbudget.Limits{})
	if constructorMotionField(obj, member, true, work) {
		t.Fatal("canceled capture field proof accepted")
	}
	if constructorMotionField(nil, member, true, nil) || constructorMotionField(obj, nil, true, nil) {
		t.Fatal("missing identity accepted")
	}
}

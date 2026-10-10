package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeLegacyMemberMetadataRejectsUnprovedModernFeatures(t *testing.T) {
	files := nativeCompileClasses(t, nativeMemberProofFixture)
	for _, scenario := range []string{"original48", "original45", "unknown minor45", "unknown minor48", "major44", "major55", "class signature", "class annotation", "modern constant", "field signature", "method parameters", "method synthetic", "method bridge", "method varargs", "stack map", "opaque debug", "bad debug length", "class literal ldc", "interface static call", "invokedynamic", "budget", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			obj, err := Parse(files["NativeArchiveOwner$Child.class"])
			if err != nil {
				t.Fatal(err)
			}
			obj.MajorVersion = 48
			for _, m := range obj.Methods {
				attrs := m.Attributes[:0]
				for _, a := range m.Attributes {
					if raw, ok := a.(*UnparsedAttribute); ok && raw.Name == "MethodParameters" {
						continue
					}
					attrs = append(attrs, a)
				}
				m.Attributes = attrs
			}
			var code *CodeAttribute
			for _, a := range obj.Methods[0].Attributes {
				if c, ok := a.(*CodeAttribute); ok {
					code = c
				}
			}
			if code == nil {
				t.Fatal("fixture code")
			}
			var work *workbudget.Budget
			switch scenario {
			case "original45":
				obj.MajorVersion = 45
			case "unknown minor45":
				obj.MajorVersion = 45
				obj.MinorVersion = 4
			case "unknown minor48":
				obj.MinorVersion = 1
			case "major44":
				obj.MajorVersion = 44
			case "major55":
				obj.MajorVersion = 55
			case "class signature":
				obj.Attributes = append(obj.Attributes, &SignatureAttribute{})
			case "class annotation":
				obj.Attributes = append(obj.Attributes, &RuntimeVisibleAnnotationsAttribute{})
			case "modern constant":
				obj.ConstantPool = append(obj.ConstantPool, &ConstantMethodHandleInfo{})
			case "field signature":
				obj.Fields[0].Attributes = append(obj.Fields[0].Attributes, &SignatureAttribute{})
			case "method parameters":
				obj.Methods[0].Attributes = append(obj.Methods[0].Attributes, &UnparsedAttribute{Name: "MethodParameters"})
			case "method synthetic":
				obj.Methods[0].Attributes = append(obj.Methods[0].Attributes, &SyntheticAttribute{})
			case "method bridge":
				obj.Methods[0].AccessFlags |= 0x40
			case "method varargs":
				obj.Methods[0].AccessFlags |= 0x80
			case "stack map":
				code.Attributes = append(code.Attributes, &UnparsedAttribute{Name: "StackMapTable"})
			case "opaque debug":
				code.Attributes = append(code.Attributes, &UnparsedAttribute{Name: "Unknown", Length: 2, Info: []byte{0, 0}})
			case "bad debug length":
				code.Attributes = append(code.Attributes, &UnparsedAttribute{Name: "LocalVariableTable", Length: 2, Info: []byte{0, 1}})
			case "class literal ldc":
				for i, c := range obj.ConstantPool {
					if _, ok := c.(*ConstantClassInfo); ok {
						code.Code = []byte{byte(core.OP_LDC_W), byte((i + 1) >> 8), byte(i + 1), byte(core.OP_POP), byte(core.OP_RETURN)}
						break
					}
				}
			case "interface static call":
				obj.ConstantPool = append(obj.ConstantPool, &ConstantInterfaceMethodrefInfo{})
				i := len(obj.ConstantPool)
				code.Code = []byte{byte(core.OP_INVOKESTATIC), byte(i >> 8), byte(i), byte(core.OP_RETURN)}
			case "invokedynamic":
				code.Code = []byte{byte(core.OP_INVOKEDYNAMIC), 0, 1, 0, 0, byte(core.OP_RETURN)}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			want := scenario == "original48" || scenario == "original45"
			if got := nativeMemberVersionMetadata(obj, work); got != want {
				t.Fatalf("metadata accepted=%v want=%v", got, want)
			}
		})
	}
}

func TestNativeSyntheticCaptureEncodingRequiresOneEmptyOriginalMarker(t *testing.T) {
	for _, row := range []struct {
		name        string
		flags       uint16
		attrs       []AttributeInfo
		want        uint16
		only, known bool
	}{
		{"flag", 0x1010, nil, 0x1010, true, true},
		{"attribute", 0x10, []AttributeInfo{&SyntheticAttribute{}}, 0x1010, true, true},
		{"both", 0x1010, []AttributeInfo{&SyntheticAttribute{}}, 0x1010, true, true},
		{"no evidence", 0x10, nil, 0x10, true, true},
		{"extra metadata", 0x10, []AttributeInfo{&SyntheticAttribute{}, &SignatureAttribute{}}, 0x1010, false, true},
		{"duplicate", 0x10, []AttributeInfo{&SyntheticAttribute{}, &SyntheticAttribute{}}, 0, false, false},
		{"nonempty", 0x10, []AttributeInfo{&SyntheticAttribute{AttrLen: 1}}, 0, false, false},
		{"nil marker", 0x10, []AttributeInfo{(*SyntheticAttribute)(nil)}, 0, false, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			field := &MemberInfo{AccessFlags: row.flags, Attributes: row.attrs}
			flags, only, known := nativeMemberEffectiveFieldFlags(field, nil)
			if flags != row.want || only != row.only || known != row.known {
				t.Fatalf("%x %v %v", flags, only, known)
			}
			if field.AccessFlags != row.flags || len(field.Attributes) != len(row.attrs) {
				t.Fatal("original metadata mutated")
			}
		})
	}
	work := workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
	if _, _, known := nativeMemberEffectiveFieldFlags(&MemberInfo{Attributes: []AttributeInfo{&SignatureAttribute{}, &SignatureAttribute{}}}, work); known {
		t.Fatal("attribute traversal exceeded shared budget")
	}
}

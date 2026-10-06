package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeMethodLocalLegacyMetadataRequiresClosedPhysicalConstructor(t *testing.T) {
	files := nativeCompileDebugClasses(t, `class LegacyMetaOwner{Object make(final long n){class Entry{long get(){return n;}}return new Entry();}}`, "none")
	for _, scenario := range []string{"49", "50", "51", "current missing table", "future", "preenum", "minor", "present table", "absent legacy signature", "wrong signature", "duplicate signature", "unknown attribute", "duplicate code", "changed capture packet", "budget", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			root, err := Parse(append([]byte(nil), files["LegacyMetaOwner.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			obj, err := Parse(append([]byte(nil), files["LegacyMetaOwner$1Entry.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			obj.MajorVersion = 51
			owner, known := originalMethodLocalOwner(obj, root, nil)
			if !known {
				t.Fatal("original local owner")
			}
			var ctor *MemberInfo
			var table *UnparsedAttribute
			var signature *SignatureAttribute
			var code *CodeAttribute
			for _, method := range obj.Methods {
				name, _ := sourceBridgeUTF8(obj, method.NameIndex)
				if name != "<init>" {
					continue
				}
				ctor = method
				keep := []AttributeInfo{}
				for _, a := range method.Attributes {
					switch value := a.(type) {
					case *UnparsedAttribute:
						if value.Name == "MethodParameters" {
							table = value
							continue
						}
					case *SignatureAttribute:
						signature = value
					case *CodeAttribute:
						code = value
					}
					keep = append(keep, a)
				}
				method.Attributes = keep
			}
			if ctor == nil || table == nil || signature == nil || code == nil {
				t.Fatal("fixture metadata")
			}
			descriptor, _ := sourceBridgeUTF8(obj, ctor.DescriptorIndex)
			params, _, err := callbinding.Descriptor(descriptor)
			if err != nil {
				t.Fatal(err)
			}
			var work *workbudget.Budget
			switch scenario {
			case "49":
				obj.MajorVersion = 49
			case "50":
				obj.MajorVersion = 50
			case "current missing table":
				obj.MajorVersion = 52
			case "future":
				obj.MajorVersion = 53
			case "preenum":
				obj.MajorVersion = 48
			case "minor":
				obj.MinorVersion = 1
			case "present table":
				ctor.Attributes = append(ctor.Attributes, table)
			case "absent legacy signature":
				keep := []AttributeInfo{}
				for _, a := range ctor.Attributes {
					if a != signature {
						keep = append(keep, a)
					}
				}
				ctor.Attributes = keep
			case "wrong signature":
				obj.ConstantPool = append(obj.ConstantPool, &ConstantUtf8Info{Value: "(I)V"})
				signature.SignatureIndex = uint16(len(obj.ConstantPool))
			case "duplicate signature":
				ctor.Attributes = append(ctor.Attributes, signature)
			case "unknown attribute":
				ctor.Attributes = append(ctor.Attributes, &UnparsedAttribute{Name: "Opaque"})
			case "duplicate code":
				ctor.Attributes = append(ctor.Attributes, code)
			case "changed capture packet":
				code.Code = append([]byte(nil), code.Code...)
				code.Code[0] = 0x01 // null cannot replace original uninitializedThis capture receiver.
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			metadata := nativeMethodLocalLegacySourceMetadata(obj, ctor, params, owner, work)
			_, physical := originalMethodLocalDefaultConstructor(obj, root, work)
			got := metadata && physical
			want := scenario == "49" || scenario == "50" || scenario == "51" || scenario == "absent legacy signature"
			if got != want {
				t.Fatalf("legacy=%v physical=%v wanted=%v", metadata, physical, want)
			}
			// The exact Java-8 profile remains independently strict for older metadata.
			if nativeMethodLocalConstructorParameters(obj, ctor, params, owner, true, nil) {
				t.Fatal("legacy path weakened exact current metadata proof")
			}
		})
	}
}

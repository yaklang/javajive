package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeMethodLocalLegacyZeroArgumentMetadataKeepsProtocol(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"MethodTypeOwner.java": methodLocalTypeConsumerFixture}, "none", "8")
	for _, scenario := range []string{"49", "50", "51", "pre-generics", "current", "future", "minor", "invented signature", "method parameters", "opaque attribute", "bad constructor", "budget", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			root, e := Parse(append([]byte(nil), files["MethodTypeOwner.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			obj, e := Parse(append([]byte(nil), files["MethodTypeOwner$1Token.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			obj.MajorVersion = 51
			owner, k := originalMethodLocalOwner(obj, root, nil)
			if !k {
				t.Fatal("original method scope")
			}
			var ctor *MemberInfo
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n == "<init>" {
					ctor = m
				}
			}
			if ctor == nil {
				t.Fatal("original default constructor")
			}
			var work *workbudget.Budget
			switch scenario {
			case "49":
				obj.MajorVersion = 49
			case "50":
				obj.MajorVersion = 50
			case "pre-generics":
				obj.MajorVersion = 48
			case "current":
				obj.MajorVersion = 52
			case "future":
				obj.MajorVersion = 53
			case "minor":
				obj.MinorVersion = 1
			case "invented signature":
				obj.ConstantPool = append(obj.ConstantPool, &ConstantUtf8Info{Value: "()V"})
				ctor.Attributes = append(ctor.Attributes, &SignatureAttribute{SignatureIndex: uint16(len(obj.ConstantPool))})
			case "method parameters":
				ctor.Attributes = append(ctor.Attributes, &UnparsedAttribute{Name: "MethodParameters", Length: 1, Info: []byte{0}})
			case "opaque attribute":
				ctor.Attributes = append(ctor.Attributes, &UnparsedAttribute{Name: "Opaque"})
			case "bad constructor":
				for _, a := range ctor.Attributes {
					if code, ok := a.(*CodeAttribute); ok {
						code.Code = []byte{0xb1}
					}
				}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			metadata := nativeMethodLocalLegacySourceMetadata(obj, ctor, nil, owner, work)
			_, physical := originalMethodLocalSourceConstructor(obj, root, work)
			got := metadata && physical
			want := scenario == "49" || scenario == "50" || scenario == "51"
			if got != want {
				t.Fatalf("metadata=%v physical=%v wanted=%v", metadata, physical, want)
			}
		})
	}
}

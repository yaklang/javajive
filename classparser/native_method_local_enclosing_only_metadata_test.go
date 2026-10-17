package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A mandated outer parameter is appended to javac's external method type;
// an ordinary captured variable changes the lowered source method type. They
// therefore have different Signature protocols even with the same word count.
func TestNativeMethodLocalEnclosingOnlyMetadataSeparatesMandatedAndCapturedWords(t *testing.T) {
	for _, release := range []string{"8", "9", "10", "11"} {
		t.Run(release, func(t *testing.T) {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(debug, func(t *testing.T) {
					files := nativeCompileReleaseClasses(t, `class EnclosingOnlyOwner<T>{<U> Object make(){class Entry{}return new Entry();}}`, debug, release)
					for _, variant := range []string{"original", "invented capture signature", "missing mandated table", "named mandated parameter", "synthetic outer parameter", "duplicate table", "budget", "canceled"} {
						t.Run(variant, func(t *testing.T) {
							root, e := Parse(append([]byte(nil), files["EnclosingOnlyOwner.class"]...))
							if e != nil {
								t.Fatal(e)
							}
							local, e := Parse(append([]byte(nil), files["EnclosingOnlyOwner$1Entry.class"]...))
							if e != nil {
								t.Fatal(e)
							}
							owner, known := originalMethodLocalOwner(local, root, nil)
							if !known {
								t.Fatal("original enclosing method")
							}
							var ctor *MemberInfo
							var table *UnparsedAttribute
							for _, m := range local.Methods {
								n, _ := sourceBridgeUTF8(local, m.NameIndex)
								if n == "<init>" {
									ctor = m
									for _, a := range m.Attributes {
										if _, ok := a.(*SignatureAttribute); ok {
											t.Fatal("actual enclosing-only constructor must not have captured-variable signature")
										}
										if raw, ok := a.(*UnparsedAttribute); ok && raw.Name == "MethodParameters" {
											table = raw
										}
									}
								}
							}
							if ctor == nil || table == nil {
								t.Fatal("actual original mandated-parameter protocol")
							}
							desc, _ := sourceBridgeUTF8(local, ctor.DescriptorIndex)
							params, _, err := callbinding.Descriptor(desc)
							if err != nil || len(params) != 1 || params[0] != "LEnclosingOnlyOwner;" {
								t.Fatalf("original outer descriptor=%s", desc)
							}
							var work *workbudget.Budget
							switch variant {
							case "invented capture signature":
								ctor.Attributes = append(ctor.Attributes, &SignatureAttribute{SignatureIndex: sourceBridgePoolString(t, local, "()V")})
							case "missing mandated table":
								keep := []AttributeInfo{}
								for _, a := range ctor.Attributes {
									if a != table {
										keep = append(keep, a)
									}
								}
								ctor.Attributes = keep
							case "named mandated parameter":
								table.Info[1], table.Info[2] = byte(local.ThisClass>>8), byte(local.ThisClass)
							case "synthetic outer parameter":
								table.Info[3], table.Info[4] = 0x10, 0x10
							case "duplicate table":
								ctor.Attributes = append(ctor.Attributes, table)
							case "budget":
								work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
							case "canceled":
								ctx, cancel := context.WithCancel(context.Background())
								cancel()
								work = workbudget.New(ctx, workbudget.Limits{})
							}
							if got := nativeMethodLocalConstructorParameters(local, ctor, params, owner, true, work); got != (variant == "original") {
								t.Fatalf("enclosing-only metadata admitted=%v", got)
							}
						})
					}
				})
			}
		})
	}
}

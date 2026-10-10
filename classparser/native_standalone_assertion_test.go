package javaclassparser

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeStandaloneAssertionRequiresOwnedClosedPacket(t *testing.T) {
	files := nativeCompileDebugClasses(t, strings.NewReplacer("BASE_STATUS", "true", "CHILD_STATUS", "false").Replace(nativeStandaloneAssertionFixture), "none")
	for _, variant := range []string{"original", "ordinary flag", "mutable flag", "private flag", "duplicate flag", "initializer effect", "wrong branch", "source version", "class version", "enclosing method", "member source", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj, err := Parse(files["OwnerBase.class"])
			if err != nil {
				t.Fatal(err)
			}
			var flag *MemberInfo
			var clinit *CodeAttribute
			for _, f := range obj.Fields {
				if name, _ := sourceBridgeUTF8(obj, f.NameIndex); name == nativeAssertionField {
					flag = f
				}
			}
			for _, m := range obj.Methods {
				if name, _ := sourceBridgeUTF8(obj, m.NameIndex); name == "<clinit>" {
					for _, a := range m.Attributes {
						if code, ok := a.(*CodeAttribute); ok {
							clinit = code
						}
					}
				}
			}
			if flag == nil || clinit == nil {
				t.Fatal("original assertion packet absent")
			}
			d := NewClassObjectDumper(obj)
			switch variant {
			case "ordinary flag":
				flag.AccessFlags &^= 0x1000
			case "mutable flag":
				flag.AccessFlags &^= 16
			case "private flag":
				flag.AccessFlags |= 2
			case "duplicate flag":
				obj.Fields = append(obj.Fields, flag)
			case "initializer effect":
				clinit.Code = append([]byte{core.OP_ACONST_NULL, core.OP_POP}, clinit.Code...)
			case "wrong branch":
				clinit.Code[5] = core.OP_IFEQ
			case "source version":
				d.options.TargetSourceVersion = 11
			case "class version":
				obj.MajorVersion = 53
			case "enclosing method":
				obj.Attributes = append(obj.Attributes, &UnparsedAttribute{Name: "EnclosingMethod"})
			case "member source":
				d.nativeMemberCurrent = &nativeMemberClass{}
			case "budget":
				d.Work = workbudget.New(context.Background(), workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			d.prepareNativeStandaloneAssertion()
			if admitted := d.nativeStandaloneAssertion != nil; admitted != (variant == "original") {
				t.Fatalf("original ownership/protocol admission=%t", admitted)
			}
			if variant == "original" && d.nativeStandaloneAssertionClosed("class OwnerBase{}") {
				t.Fatal("unconsumed original assertion reads admitted")
			}
		})
	}
}

func TestNativeStandaloneAssertionDoesNotPublishPartialSource(t *testing.T) {
	packet := &nativeMemberAssertion{reads: map[string]map[int]*nativeAssertionPacket{"check()V": {3: {}}}}
	for _, variant := range []string{"unconsumed", "method stub", "stack placeholder", "complete"} {
		t.Run(variant, func(t *testing.T) {
			d := NewClassObjectDumper(&ClassObject{})
			d.nativeStandaloneAssertion = &nativeStandaloneAssertion{packet: packet, consumed: map[string]bool{"check()V": variant != "unconsumed"}}
			source := "class Root {void check(){assert true;}}"
			if variant == "method stub" {
				source += "/* yak-decompiler: source projection failed */"
			}
			if variant == "stack placeholder" {
				source += "empty slot value"
			}
			if closed := d.nativeStandaloneAssertionClosed(source); closed != (variant == "complete") {
				t.Fatalf("partial source closure=%t", closed)
			}
		})
	}
}

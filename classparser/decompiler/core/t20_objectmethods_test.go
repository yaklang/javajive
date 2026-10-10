package core

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestTaskT20C05BootstrapNeighbor(t *testing.T) {
	t.Log("T20-C05")
	strT := types.NewJavaPrimer(types.JavaString)
	intT := types.NewJavaPrimer(types.JavaInteger)
	rec := values.NewJavaClassValue(types.NewJavaClass("Rec"))
	names := values.NewJavaLiteral("a;b", strT)
	ga := values.NewJavaClassMember("Rec", "a", "I", intT)
	gb := values.NewJavaClassMember("Rec", "b", "I", intT)
	ga.RefKind = RefGetField
	gb.RefKind = RefGetField

	neighbor := CallSiteRequest{
		Identity: BootstrapIdentity{
			Owner:      "com.example.OtherMethods",
			Name:       "bootstrap",
			Descriptor: IdentityObjectMethods.Descriptor,
			RefKind:    RefInvokeStatic,
		},
		CallSiteName:        "toString",
		CallSiteDescriptor:  "(LRec;)Ljava/lang/String;",
		StaticArgs:          []values.JavaValue{rec, names, ga, gb},
		DynamicArgs:         []values.JavaValue{values.JavaNull},
		TargetSourceVersion: 17,
		ClassMajor:          61,
	}
	res := DispatchInvokeDynamic(neighbor, nil, nil, strT)
	if res.Status != "unsupported" {
		t.Fatalf("T20-C05 neighbor must be unsupported, got %+v", res)
	}
	if res.Family == FamilyRecord && res.Status == "" {
		t.Fatalf("T20-C05 neighbor treated as synthetic ObjectMethods: %+v", res)
	}
	if s := ""; res.Value != nil {
		s = res.Value.String(nil)
		if strings.Contains(s, "Rec[") && strings.Contains(s, `a="`) {
			t.Fatalf("T20-C05 neighbor reconstructed as ObjectMethods toString: %s", s)
		}
	}

	wrong := CallSiteRequest{
		Identity:            IdentityObjectMethods,
		CallSiteName:        "equals",
		CallSiteDescriptor:  "(LRec;Ljava/lang/Object;)Z",
		StaticArgs:          []values.JavaValue{rec, names, gb, ga},
		DynamicArgs:         []values.JavaValue{values.JavaNull, values.JavaNull},
		TargetSourceVersion: 17,
		ClassMajor:          61,
	}
	bad := DispatchInvokeDynamic(wrong, nil, nil, types.NewJavaPrimer(types.JavaBoolean))
	if bad.Status != "invalid_input" {
		t.Fatalf("T20-C05 wrong getter order must be invalid_input, got %+v", bad)
	}
	if !strings.Contains(bad.Reason, "order") && !strings.Contains(bad.Reason, "mismatch") {
		t.Fatalf("T20-C05 expected order mismatch reason, got %q", bad.Reason)
	}

	kind := IdentityObjectMethods
	kind.RefKind = RefInvokeVirtual
	kindReq := CallSiteRequest{
		Identity:            kind,
		CallSiteName:        "toString",
		CallSiteDescriptor:  "(LRec;)Ljava/lang/String;",
		StaticArgs:          []values.JavaValue{rec, names, ga, gb},
		DynamicArgs:         []values.JavaValue{values.JavaNull},
		TargetSourceVersion: 17,
		ClassMajor:          61,
	}
	kr := DispatchInvokeDynamic(kindReq, nil, nil, strT)
	if kr.Status != "unsupported" {
		t.Fatalf("T20-C05 wrong refkind must not whitelist-match, got %+v", kr)
	}
}

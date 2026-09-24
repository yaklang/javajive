package core

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestTaskT21C05TypeSwitchNullIndex(t *testing.T) {
	t.Log("T21-C05")
	objT := types.NewJavaClass("java.lang.Object")
	intT := types.NewJavaPrimer(types.JavaInteger)
	sel := values.NewJavaLiteral("x", types.NewJavaPrimer(types.JavaString))
	rst := values.NewJavaLiteral(0, intT)
	req := CallSiteRequest{
		Identity:            IdentityTypeSwitch,
		CallSiteName:        "typeSwitch",
		CallSiteDescriptor:  "(Ljava/lang/Object;I)I",
		StaticArgs:          []values.JavaValue{values.NewJavaClassValue(types.NewJavaClass("java.lang.String"))},
		DynamicArgs:         []values.JavaValue{rst, sel},
		TargetSourceVersion: 21,
		ClassMajor:          65,
	}
	res := DispatchInvokeDynamic(req, nil, nil, intT)
	if res.Status == "invalid_input" {
		t.Fatalf("T21-C05 typeSwitch dispatch: %+v", res)
	}
	s := ""
	if res.Value != nil {
		s = res.Value.String(nil)
	}
	if strings.Contains(s, "case null") {
		t.Fatalf("T21-C05 adapter must not invent case null: %s", s)
	}
	_ = objT

	enumReq := req
	enumReq.Identity = IdentityEnumSwitch
	er := DispatchInvokeDynamic(enumReq, nil, nil, intT)
	if er.Status != "unsupported" {
		t.Fatalf("T21 enumSwitch should stay explicit unsupported, got %+v", er)
	}
}

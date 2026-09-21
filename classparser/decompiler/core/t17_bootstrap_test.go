package core

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func t17CallSiteType(t *testing.T) types.JavaType {
	t.Helper()
	return types.NewJavaClass("java.lang.String")
}

func t17BaseConcatReq() CallSiteRequest {
	recipe := values.NewJavaLiteral("\u0001", types.NewJavaPrimer(types.JavaString))
	return CallSiteRequest{
		Identity:            IdentityMakeConcatWithConstants,
		CallSiteName:        "makeConcatWithConstants",
		CallSiteDescriptor:  "(Ljava/lang/Object;)Ljava/lang/String;",
		StaticArgs:          []values.JavaValue{recipe},
		DynamicArgs:         []values.JavaValue{values.JavaNull},
		TargetSourceVersion: 17,
		ClassMajor:          61,
	}
}

func TestTaskT17C01UnknownBootstrapDoesNotExecute(t *testing.T) {
	t.Log("T17-C01")
	marker := false
	prev := FamilyAdapter(FamilyConcat)
	SetFamilyAdapter(FamilyConcat, func(req CallSiteRequest, d *Decompiler, sim StackSimulation, resultType types.JavaType) DispatchResult {
		marker = true
		return defaultConcatAdapter(req, d, sim, resultType)
	})
	defer SetFamilyAdapter(FamilyConcat, prev)

	req := t17BaseConcatReq()
	req.Identity.Owner = "evil.bootstrap.MarkerFactory"
	typ := t17CallSiteType(t)
	res := DispatchInvokeDynamic(req, nil, nil, typ)
	if marker {
		t.Fatalf("T17-C01 concat adapter ran for unknown owner (would have executed trap/marker)")
	}
	if res.ExecutedBootstrap {
		t.Fatal("T17-C01 ExecutedBootstrap must stay false")
	}
	if res.Status != "unsupported" {
		t.Fatalf("T17-C01 expected unsupported, got %q reason %q", res.Status, res.Reason)
	}
	if res.DiagnosticCode != DiagBootstrapUnknown {
		t.Fatalf("T17-C01 code=%s", res.DiagnosticCode)
	}
	if !strings.Contains(res.Identity.Format(), "evil.bootstrap.MarkerFactory") {
		t.Fatalf("T17-C01 identity not traceable: %s", res.Identity.Format())
	}
	if res.Family != FamilyUnknown {
		t.Fatalf("T17-C01 family=%s", res.Family)
	}
}

func TestTaskT17C02CondyLegalUnknownAndInvalid(t *testing.T) {
	t.Log("T17-C02")
	legal := ClassifyCondy("val", "Ljava/lang/String;", 0, 1)
	if legal.Status != "unsupported" || legal.DiagnosticCode != DiagCondyUnsupported {
		t.Fatalf("T17-C02 legal condy: %+v", legal)
	}
	if legal.ExecutedBootstrap {
		t.Fatal("T17-C02 executed condy bootstrap")
	}
	src := legal.Value.String(nil)
	if strings.Contains(src, "forged") || (!strings.Contains(src, "unsupported condy") && !strings.Contains(src, "condy")) {
		t.Fatalf("T17-C02 legal condy forged a constant: %q", src)
	}

	badIdx := ClassifyCondy("val", "Ljava/lang/String;", 9, 1)
	if badIdx.Status != "invalid_input" || badIdx.DiagnosticCode != DiagCondyInvalid {
		t.Fatalf("T17-C02 bad index should be invalid_input: %+v", badIdx)
	}
	empty := ClassifyCondy("", "", 0, 1)
	if empty.Status != "invalid_input" {
		t.Fatalf("T17-C02 empty name/desc: %+v", empty)
	}
}

func TestTaskT17C03IdentityNeighbors(t *testing.T) {
	t.Log("T17-C03")
	typ := t17CallSiteType(t)
	base := t17BaseConcatReq()
	neighbors := []BootstrapIdentity{
		{Owner: "evil.StringConcatFactory", Name: base.Identity.Name, Descriptor: base.Identity.Descriptor, RefKind: RefInvokeStatic},
		{Owner: base.Identity.Owner, Name: "makeConcatWithConstants", Descriptor: "(Ljava/lang/invoke/MethodHandles$Lookup;Ljava/lang/String;Ljava/lang/invoke/MethodType;)Ljava/lang/invoke/CallSite;", RefKind: RefInvokeStatic},
		{Owner: base.Identity.Owner, Name: base.Identity.Name, Descriptor: base.Identity.Descriptor, RefKind: RefInvokeVirtual},
		{Owner: base.Identity.Owner, Name: "notARealBootstrap", Descriptor: base.Identity.Descriptor, RefKind: RefInvokeStatic},
	}
	called := 0
	prev := FamilyAdapter(FamilyConcat)
	SetFamilyAdapter(FamilyConcat, func(req CallSiteRequest, d *Decompiler, sim StackSimulation, resultType types.JavaType) DispatchResult {
		called++
		return defaultConcatAdapter(req, d, sim, resultType)
	})
	defer SetFamilyAdapter(FamilyConcat, prev)

	for i, id := range neighbors {
		req := base
		req.Identity = id
		res := DispatchInvokeDynamic(req, nil, nil, typ)
		if res.Status != "unsupported" {
			t.Fatalf("T17-C03 neighbor %d entered adapter: status=%s family=%s id=%s", i, res.Status, res.Family, id.Format())
		}
		if _, ok := LookupBuiltin(id); ok {
			t.Fatalf("T17-C03 neighbor %d whitelist-hit: %s", i, id.Format())
		}
	}
	if called != 0 {
		t.Fatalf("T17-C03 concat adapter invoked %d times for neighbors", called)
	}

	hit, ok := LookupBuiltin(IdentityMakeConcatWithConstants)
	if !ok || hit != FamilyConcat {
		t.Fatalf("T17-C03 exact identity must match, got %s ok=%v", hit, ok)
	}
}

func TestTaskT17C04ArgumentValidation(t *testing.T) {
	t.Log("T17-C04")
	typ := t17CallSiteType(t)
	base := t17BaseConcatReq()

	missing := base
	missing.StaticArgs = nil
	res := DispatchInvokeDynamic(missing, nil, nil, typ)
	if res.Status != "invalid_input" {
		t.Fatalf("T17-C04 missing recipe: %+v", res)
	}

	tag := base
	tag.StaticArgs = []values.JavaValue{values.NewJavaLiteral(7, types.NewJavaPrimer(types.JavaInteger))}
	res = DispatchInvokeDynamic(tag, nil, nil, typ)
	if res.Status != "invalid_input" || !strings.Contains(res.Reason, "tag") && !strings.Contains(res.Reason, "string") {
		t.Fatalf("T17-C04 wrong tag: %+v", res)
	}

	extra := base
	extra.StaticArgs = []values.JavaValue{
		values.NewJavaLiteral("\u0001", types.NewJavaPrimer(types.JavaString)),
		values.NewJavaLiteral("extra", types.NewJavaPrimer(types.JavaString)),
	}
	res = DispatchInvokeDynamic(extra, nil, nil, typ)
	if res.Status != "invalid_input" || !strings.Contains(res.Reason, "TAG_CONST") {
		t.Fatalf("T17-C04 extra static const: %+v", res)
	}

	dyn := base
	dyn.CallSiteDescriptor = "(Ljava/lang/Object;Ljava/lang/Object;)Ljava/lang/String;"
	dyn.DynamicArgs = []values.JavaValue{values.JavaNull, values.JavaNull}
	res = DispatchInvokeDynamic(dyn, nil, nil, typ)
	if res.Status != "invalid_input" || !strings.Contains(res.Reason, "TAG_ARG") && !strings.Contains(res.Reason, "arity") {
		t.Fatalf("T17-C04 dynamic mismatch: %+v", res)
	}

	lam := CallSiteRequest{
		Identity:            IdentityLambdaMetafactory,
		CallSiteName:        "apply",
		CallSiteDescriptor:  "()Ljava/util/function/IntUnaryOperator;",
		StaticArgs:          nil,
		DynamicArgs:         nil,
		TargetSourceVersion: 8,
		ClassMajor:          52,
	}
	res = DispatchInvokeDynamic(lam, nil, nil, types.NewJavaClass("java.util.function.IntUnaryOperator"))
	if res.Status != "invalid_input" {
		t.Fatalf("T17-C04 lambda missing static args: %+v", res)
	}
}

func TestTaskT17C05CapabilityTable(t *testing.T) {
	t.Log("T17-C05")
	rec8 := EvaluateCapability(FamilyRecord, 8, 61)
	if rec8.Lossless {
		t.Fatalf("T17-C05 record at source 8 must not be lossless: %+v", rec8)
	}
	rec17 := EvaluateCapability(FamilyRecord, 17, 61)
	if !rec17.Lossless {
		t.Fatalf("T17-C05 record at source 17 should be language-capable: %+v", rec17)
	}
	sw17 := EvaluateCapability(FamilyTypeSwitch, 17, 65)
	if sw17.Lossless {
		t.Fatalf("T17-C05 typeSwitch at 17 must not be lossless: %+v", sw17)
	}
	sw21 := EvaluateCapability(FamilyTypeSwitch, 21, 65)
	if !sw21.Lossless {
		t.Fatalf("T17-C05 typeSwitch at 21 should be language-capable: %+v", sw21)
	}

	req := CallSiteRequest{
		Identity:            IdentityObjectMethods,
		CallSiteName:        "toString",
		CallSiteDescriptor:  "(LRec;)Ljava/lang/String;",
		StaticArgs:          []values.JavaValue{values.NewJavaClassValue(types.NewJavaClass("Rec")), values.NewJavaLiteral("x", types.NewJavaPrimer(types.JavaString))},
		DynamicArgs:         []values.JavaValue{values.JavaNull},
		TargetSourceVersion: 8,
		ClassMajor:          61,
	}
	res := DispatchInvokeDynamic(req, nil, nil, types.NewJavaPrimer(types.JavaString))
	if res.Status != "unsupported" || res.DiagnosticCode != DiagBootstrapVersion {
		t.Fatalf("T17-C05 ObjectMethods at 8: %+v", res)
	}
}

func TestTaskT17M03AdaptersDeclareFullIdentity(t *testing.T) {
	t.Log("T17-M03")
	ids := AllBuiltinIdentities()
	if len(ids) < 7 {
		t.Fatalf("T17-M03 expected 7 builtin identities, got %d", len(ids))
	}
	seen := map[FeatureFamily]bool{}
	for _, id := range ids {
		if id.Owner == "" || id.Name == "" || id.Descriptor == "" || id.RefKind == 0 {
			t.Fatalf("T17-M03 incomplete identity: %+v", id)
		}
		if !strings.HasPrefix(id.Descriptor, "(") {
			t.Fatalf("T17-M03 descriptor not a method desc: %s", id.Descriptor)
		}
		fam, ok := LookupBuiltin(id)
		if !ok {
			t.Fatalf("T17-M03 lookup failed for %s", id.Format())
		}
		cap := EvaluateCapability(fam, 21, 65)
		if cap.Family != fam {
			t.Fatalf("T17-M03 capability family mismatch %s", fam)
		}
		seen[fam] = true
		slash := id
		slash.Owner = strings.ReplaceAll(id.Owner, ".", "/")
		if _, ok := LookupBuiltin(slash); !ok {
			t.Fatalf("T17-M03 slash owner must normalize: %s", slash.Owner)
		}
	}
	for _, fam := range []FeatureFamily{FamilyConcat, FamilyLambda, FamilyRecord, FamilyTypeSwitch, FamilyEnumSwitch} {
		if !seen[fam] {
			t.Fatalf("T17-M03 missing family %s", fam)
		}
	}
}

func TestTaskT17RefKindZeroNeverMatches(t *testing.T) {
	id := IdentityMakeConcatWithConstants
	id.RefKind = 0
	if _, ok := LookupBuiltin(id); ok {
		t.Fatal("refkind 0 must not whitelist-match")
	}
}

package javaclassparser

import (
	"context"
	"errors"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/utils"
	"reflect"
	"testing"
)

func TestMemberLedgerNeverCompletesUnaccountedMember(t *testing.T) {
	for _, state := range []string{"pending", "unsupported", "dropped", "stub"} {
		r := DecompileResult{Status: "complete", Members: []MemberRecord{{Kind: "method", Name: "<clinit>", Descriptor: "()V", State: state, Evidence: "test observation"}}}
		finalizeMemberStatus(&r)
		if r.Status == "complete" {
			t.Fatal(state)
		}
	}
	r := DecompileResult{Status: "complete", Members: []MemberRecord{{State: "regenerated"}}}
	finalizeMemberStatus(&r)
	if r.Status == "complete" {
		t.Fatal("unsubstantiated regeneration")
	}
}
func TestSyntaxObservationRequiresValidator(t *testing.T) {
	r := DecompileResult{Status: "complete", Source: "class X { invalid }"}
	observeSyntax(&r, DecompileOptions{})
	if r.Syntax.Status != "unavailable" {
		t.Fatal(r.Syntax)
	}
	observeSyntax(&r, DecompileOptions{ValidateSyntax: func(context.Context, string) error { return errors.New("invalid token") }})
	if r.Syntax.Status != "invalid" || r.Status == "complete" {
		t.Fatalf("%+v", r)
	}
	if !errors.Is(validateJavaSyntaxOnce("class X {}", 0), ErrSyntaxUnavailable) {
		t.Fatal("missing parser reported valid")
	}
}
func TestRetryRollbackRestoresSharedRenderingState(t *testing.T) {
	ctx := &class_context.ClassContext{ClassName: "Before", TypeParams: []string{"T"}, MethodDescriptors: map[string]bool{"old": true}, KeySet: utils.NewSet[string]([]string{"old"})}
	ctx.Import("java.util.List")
	report := DecompileResult{Diagnostics: []DecompileDiagnostic{{Code: "before"}}, Members: []MemberRecord{{State: "pending"}}}
	c := NewClassObjectDumper(&ClassObject{})
	c.FuncCtx = ctx
	c.report = &report
	c.lambdaMethods["m"] = []string{"()V"}
	c.fieldDefaultValue["f"] = "old"
	c.deepStack.Push(3)
	c.dumpedMethodsSet["m"] = &dumpedMethods{bodyCode: "before"}
	undo := c.snapshotRetryState()
	c.lambdaMethods["m"][0] = "bad"
	c.fieldDefaultValue["f"] = "bad"
	c.dumpedMethodsSet["m"].bodyCode = "bad"
	c.deepStack.Pop()
	c.deepStack.Push(9)
	ctx.ClassName = "After"
	ctx.TypeParams[0] = "BAD"
	ctx.MethodDescriptors["bad"] = true
	ctx.KeySet.Add("bad")
	ctx.Import("java.util.Set")
	report.Diagnostics[0].Code = "bad"
	report.Members[0].State = "stub"
	c.lambdaLocalSeq = 42
	undo()
	if c.FuncCtx != ctx || ctx.ClassName != "Before" || ctx.TypeParams[0] != "T" || ctx.MethodDescriptors["bad"] || ctx.KeySet.Has("bad") || c.lambdaLocalSeq != 0 || c.deepStack.Peek() != 3 || c.fieldDefaultValue["f"] != "old" || c.dumpedMethodsSet["m"].bodyCode != "before" || c.lambdaMethods["m"][0] != "()V" || report.Diagnostics[0].Code != "before" || report.Members[0].State != "pending" {
		t.Fatalf("rollback leaked state: %+v %+v", c, report)
	}
	if !reflect.DeepEqual(ctx.GetAllImported(), []string{"java.util.List"}) {
		t.Fatal(ctx.GetAllImported())
	}
}
func TestShadowBuilderFailureVisibleWithoutChangingSource(t *testing.T) {
	old := core.ShadowIRBuilder
	defer func() { core.ShadowIRBuilder = old }()
	_, classes := t04CompileRun(t, "8", "ShadowMain", map[string]string{"ShadowMain.java": `public class ShadowMain { public static void main(String[] x){System.out.println(7);} }`})
	off, e := DecompileWithOptions(classes["ShadowMain"], DecompileOptions{Mode: Precision})
	if e != nil {
		t.Fatal(e)
	}
	for _, builder := range []func(core.ShadowIRRequest) (string, uint64, error){nil, func(core.ShadowIRRequest) (string, uint64, error) { return "", 0, errors.New("forced shadow failure") }} {
		core.ShadowIRBuilder = builder
		on, e := DecompileWithOptions(classes["ShadowMain"], DecompileOptions{Mode: Precision, EnableShadowIR: true})
		if e != nil {
			t.Fatal(e)
		}
		if on.Source != off.Source || len(on.Shadow) == 0 {
			t.Fatal("shadow changed source or disappeared")
		}
		want := "failed"
		if builder == nil {
			want = "unavailable"
		}
		found := false
		for _, o := range on.Shadow {
			if o.Status == want && o.Error != "" {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing %s: %+v", want, on.Shadow)
		}
	}
}

func TestMemberLedgerProductionDetectsUnconsumedSynthetic(t *testing.T) {
	_, classes := t04CompileRun(t, "8", "LedgerMain", map[string]string{"LedgerMain.java": `public class LedgerMain { public int kept=7; public static void lambda$unused(){System.out.println(9);} public static void main(String[] x){System.out.println("= Exception; /* yak-decompiler: data */");} }`})
	obj, e := Parse(classes["LedgerMain"])
	if e != nil {
		t.Fatal(e)
	}
	for _, m := range obj.Methods {
		n, _ := obj.getUtf8(m.NameIndex)
		if n == "lambda$unused" {
			m.AccessFlags |= 0x1000
		}
	}
	r, e := DecompileWithOptions(obj.Bytes(), DecompileOptions{Mode: Precision})
	if e != nil {
		t.Fatal(e)
	}
	if len(r.Members) != len(obj.Methods)+len(obj.Fields) || r.Status == "complete" || len(r.StubMethods) != 0 {
		t.Fatalf("unaccounted member or literal false stub: %+v", r)
	}
	found := false
	for _, m := range r.Members {
		if m.Name == "lambda$unused" {
			found = true
			if m.State != "unsupported" {
				t.Fatal(m)
			}
		}
	}
	if !found {
		t.Fatal("missing member record")
	}
}

func TestProductionShadowReceivesDeclaredCodeLimits(t *testing.T) {
	old := core.ShadowIRBuilder
	defer func() { core.ShadowIRBuilder = old }()
	_, classes := t04CompileRun(t, "8", "LimitsMain", map[string]string{"LimitsMain.java": `public class LimitsMain { public static void main(String[] x){System.out.println(7);} }`})
	seen := false
	core.ShadowIRBuilder = func(r core.ShadowIRRequest) (string, uint64, error) {
		if r.MethodName == "main" {
			seen = true
			if !r.Limits.Present || r.Limits.MaxLocals != 1 || r.Limits.MaxStack != 2 || r.Limits.DirectSuperClass != "java/lang/Object" {
				t.Errorf("Code metadata lost: %+v", r.Limits)
			}
		}
		return old(r)
	}
	_, e := DecompileWithOptions(classes["LimitsMain"], DecompileOptions{Mode: Precision, EnableShadowIR: true})
	if e != nil {
		t.Fatal(e)
	}
	if !seen {
		t.Fatal("production hook not reached")
	}
}

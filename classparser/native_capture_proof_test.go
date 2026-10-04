package javaclassparser

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeCaptureRequiresStableDominatingIdentity(t *testing.T) {
	for _, scenario := range []string{"parameter", "same ID different ref", "different ID same spelling", "parameter reassigned", "folded write", "for header write", "sole local initializer", "local reassigned", "branch local escapes", "self initializer", "opaque closure", "value cycle", "statement cycle", "canceled proof", "proof work cap", "proof depth cap", "caller name collision", "declaration erasure differs"} {
		t.Run(scenario, func(t *testing.T) {
			typ := types.NewJavaPrimer(types.JavaInteger)
			ref := values.NewJavaRef(coreutils.NewRootVariableId(), nil, typ)
			ref.IsParam = true
			ref.Id.SetName("x")
			captured := ref
			params := []values.JavaValue{ref}
			child := &nativeAnonymousClass{descriptor: "(I)V", method: "pick(I)Ljava/lang/Object;", fields: map[string]int{"val$x": 0}}
			family := &nativeAnonymousFamily{owner: "Proof", children: map[string]*nativeAnonymousClass{"Proof$1": child}}
			c := &ClassObjectDumper{nativeAnonymousRoot: family, FuncCtx: &class_context.ClassContext{ClassName: "Proof", FunctionName: "pick", CurrentMethodDesc: "(I)Ljava/lang/Object;"}}
			literal := values.NewJavaLiteral(0, typ)
			call := &values.FunctionCallExpression{ClassName: "Proof$1", FunctionName: "<init>", Descriptor: "(I)V", Arguments: []values.JavaValue{captured}, OriginPC: 7, HasOriginPC: true}
			alloc := &values.NewExpression{JavaType: types.NewJavaClass("Proof$1"), ConstructorCall: call, OriginPC: 3, HasOriginPC: true}
			body := []statements.Statement{&statements.ReturnStatement{JavaValue: alloc}}
			good := scenario == "parameter" || scenario == "same ID different ref" || scenario == "sole local initializer" || scenario == "caller name collision"
			switch scenario {
			case "same ID different ref":
				copy := *ref
				copy.IsParam = false
				call.Arguments[0] = &copy
			case "different ID same spelling":
				other := values.NewJavaRef(coreutils.NewRootVariableId(), nil, typ)
				other.Id.SetName("x")
				call.Arguments[0] = other
			case "parameter reassigned":
				body = append(body, statements.NewAssignStatement(ref, literal, false))
			case "folded write":
				body = append(body, &statements.ExpressionStatement{Expression: &values.AssignmentExpression{Target: ref, Value: literal}})
			case "for header write":
				body = append(body, &statements.ForStatement{InitVar: statements.NewAssignStatement(ref, literal, false)})
			case "sole local initializer", "local reassigned", "branch local escapes", "self initializer", "declaration erasure differs":
				ref.IsParam = false
				params = nil
				c.FuncCtx.CurrentMethodDesc = "()Ljava/lang/Object;"
				child.method = "pick()Ljava/lang/Object;"
				declaration := statements.NewAssignStatement(ref, literal, true)
				body = append([]statements.Statement{declaration}, body...)
				if scenario == "local reassigned" {
					body = append(body, statements.NewAssignStatement(ref, literal, false))
				}
				if scenario == "branch local escapes" {
					body = []statements.Statement{&statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), IfBody: []statements.Statement{declaration}}, body[1]}
				}
				if scenario == "self initializer" {
					declaration.JavaValue = alloc
					body = []statements.Statement{declaration}
				}
				if scenario == "declaration erasure differs" {
					declaration.JavaValue = values.NewJavaLiteral(int64(0), types.NewJavaPrimer(types.JavaLong))
				}
			case "opaque closure":
				body = append(body, &statements.ExpressionStatement{Expression: values.NewCustomValue(func(*class_context.ClassContext) string { return "opaque" }, func() types.JavaType { return typ })})
			case "value cycle":
				cycle := &values.JavaExpression{}
				cycle.Values = []values.JavaValue{cycle}
				body = append(body, &statements.ExpressionStatement{Expression: cycle})
			case "statement cycle":
				cycle := &statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))}
				cycle.IfBody = []statements.Statement{cycle}
				body = append(body, cycle)
			case "canceled proof":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				c.Work = workbudget.New(ctx, workbudget.Limits{})
			case "proof work cap":
				c.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "proof depth cap":
				c.Work = workbudget.New(nil, workbudget.Limits{MaxASTDepth: 1})
			case "caller name collision":
				other := values.NewJavaRef(coreutils.NewRootVariableId(), nil, typ)
				other.Id.SetName("x")
				other.IsParam = true
				params = append(params, other)
			}
			c.prepareNativeCaptureBindings(body, params)
			if family.failed == good {
				t.Fatalf("stable capture verdict: failed=%v expected accepted=%v", family.failed, good)
			}
			if good && (c.FuncCtx.SourceCaptureStable == nil || !c.FuncCtx.SourceCaptureStable(7, ref.Id) || c.FuncCtx.SourceCaptureStable(8, ref.Id)) {
				t.Fatal("capture not bound to exact call site")
			}
			if scenario == "caller name collision" && !strings.HasPrefix(params[1].String(c.FuncCtx), "jdec$local$") {
				t.Fatal("distinct identity shadowed capture")
			}
			if c.Work != nil && c.Work.Err() == nil {
				t.Fatal("resource/cancellation refusal is not sticky")
			}
		})
	}
}

func TestNativeAnonymousNumberingRequiresActualSourceOrder(t *testing.T) {
	p := &nativeAnonymousFamily{children: map[string]*nativeAnonymousClass{"A$1": {}, "A$2": {}}}
	for _, c := range []struct {
		source string
		valid  bool
	}{
		{`/*jdec-owned-anonymous-ordinal:1*/new A(){}; /*jdec-owned-anonymous-ordinal:2*/new A(){}`, true},
		{`String s="/*jdec-owned-anonymous-ordinal:2*/";/*jdec-owned-anonymous-ordinal:1*/new A(){};/*jdec-owned-anonymous-ordinal:2*/new A(){}`, true},
		{`// /*jdec-owned-anonymous-ordinal:1*/`, false},
		{`/*jdec-owned-anonymous-ordinal:2*/new A(){};/*jdec-owned-anonymous-ordinal:1*/new A(){}`, false},
		{`/*jdec-owned-anonymous-ordinal:1*/new A(){}`, false},
		{`/*jdec-owned-anonymous-ordinal:1*/new A(){};/*jdec-owned-anonymous-ordinal:1*/new A(){}`, false},
		{`/*jdec-owned-anonymous-ordinal:1*/new A(){};/*jdec-owned-anonymous-ordinal:2*/new A(){};"unterminated`, false},
	} {
		if got := p.completeSource(c.source); got != c.valid {
			t.Fatalf("layout %q: %v", c.source, got)
		}
	}
}

func TestNativeAnonymousMemberScopeIsOpaqueToCallerRecovery(t *testing.T) {
	body := `String text = "/*jdec-owned-anonymous-ordinal:1*/new Fake() {}";
return /*jdec-owned-anonymous-ordinal:1*/new Parent(new int[]{1,2}) {
 int read(Object var1) { NumericDocValues var3 = lookup(var1); return var3.get(); }
};`
	rule := func(s string) string {
		s = hoistSameTypeEscapedLocals(s)
		s = hoistCastGuardedEscapedLocals(s)
		return addMissingGeneratedLocalDecls(s, "Object var1, float var3", "Owner", nil, "Parent")
	}
	got := nativeRewriteEnclosingMethod(body, rule)
	if got != body {
		t.Fatalf("caller rewrote member scope:\n%s", got)
	}
	if got := nativeRewriteEnclosingMethod(body, func(s string) string { return strings.ReplaceAll(s, "/*jdec lexical members 0*/", "") }); got != body {
		t.Fatal("accepted lost member fragment")
	}
	if got := nativeRewriteEnclosingMethod(body, func(s string) string { return s + s }); got != body {
		t.Fatal("accepted duplicated member fragment")
	}
}

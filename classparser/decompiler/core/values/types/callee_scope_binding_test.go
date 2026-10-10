package types

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"testing"
)

// A class with no own formals may still have original references to outer
// formals. Without an actual receiver substitution, a same-spelled variable
// in the caller does not bind that callee declaration.
func TestCalleeLexicalVariablesCannotBorrowCallerNames(t *testing.T) {
	ctx := &class_context.ClassContext{ClassName: "caller.Owner", TypeParams: []string{"T", "U"}}
	for _, variable := range []string{"T", "U"} {
		t.Run(variable, func(t *testing.T) {
			descriptor := "(Ljava/lang/Number;)Ljava/lang/Number;"
			provider := func(name string) (string, map[string]string, bool) {
				if name != "callee/Outer$Inner" {
					return "", nil, false
				}
				return "", map[string]string{class_context.MethodDescKey("apply", descriptor): "(T" + variable + ";)T" + variable + ";", class_context.MethodSigKey("apply", 1): "(T" + variable + ";)T" + variable + ";"}, true
			}
			for _, api := range []string{"parameter", "exact signature", "arity signature", "return", "field"} {
				t.Run(api, func(t *testing.T) {
					switch api {
					case "parameter":
						if p := ResolveInstantiatedParamType(ctx, provider, "callee.Outer$Inner", nil, "apply", descriptor, 1, 0); p != nil {
							t.Fatalf("foreign parameter bound to caller: %s", p.String(ctx))
						}
					case "exact signature":
						if p, r, _ := ResolveInstantiatedSignatureExact(ctx, provider, "callee.Outer$Inner", nil, "apply", descriptor, 1); p != nil || r != nil {
							t.Fatal("foreign exact signature borrowed caller scope")
						}
					case "arity signature":
						if p, r := ResolveInstantiatedSignature(ctx, provider, "callee.Outer$Inner", nil, "apply", 1); p != nil || r != nil {
							t.Fatal("foreign arity signature borrowed caller scope")
						}
					case "return":
						if r := ResolveInstantiatedReturnType(ctx, provider, "callee.Outer$Inner", nil, "apply", 1); r != nil {
							t.Fatal("foreign return borrowed caller scope")
						}
					case "field":
						fields := func(owner, name string) (string, bool) { return "T" + variable + ";", true }
						if p := ResolveInstantiatedFieldType(ctx, provider, fields, "callee.Outer$Inner", nil, "value"); p != nil {
							t.Fatal("foreign field borrowed caller scope")
						}
					}
				})
			}

		})
	}
}

func TestCalleeLexicalVariablesRequireActualReceiverSubstitution(t *testing.T) {
	ctx := &class_context.ClassContext{ClassName: "caller.Owner", TypeParams: []string{"T"}}
	desc := "(Ljava/lang/Object;)Ljava/lang/Object;"
	provider := func(name string) (string, map[string]string, bool) {
		return "<T:Ljava/lang/Object;>Ljava/lang/Object;", map[string]string{class_context.MethodDescKey("apply", desc): "(TT;)TT;", class_context.MethodSigKey("apply", 1): "(TT;)TT;"}, true
	}
	for _, row := range []struct {
		name string
		args []JavaType
		want string
	}{
		{"concrete receiver", []JavaType{NewJavaClass("java.lang.String")}, "String"},
		{"explicit caller formal", []JavaType{NewJavaClass("T")}, "T"},
		{"raw receiver", nil, ""}, {"missing argument", []JavaType{nil}, ""},
	} {
		t.Run(row.name, func(t *testing.T) {
			p, r, _ := ResolveInstantiatedSignatureExact(ctx, provider, "callee.Owner", row.args, "apply", desc, 1)
			if row.want == "" {
				if p != nil || r != nil {
					t.Fatal("receiver binding was invented")
				}
				return
			}
			if len(p) != 1 || p[0].String(ctx) != row.want || r == nil || r.String(ctx) != row.want {
				t.Fatal("actual receiver substitution lost", p, r)
			}
		})
	}
	t.Run("method declaration shadows receiver", func(t *testing.T) {
		own := func(name string) (string, map[string]string, bool) {
			return "<T:Ljava/lang/Number;>Ljava/lang/Object;", map[string]string{class_context.MethodDescKey("apply", desc): "<T::Ljava/lang/CharSequence;>(TT;)TT;"}, true
		}
		p, r, f := ResolveInstantiatedSignatureExact(ctx, own, "callee.Owner", []JavaType{NewJavaClass("java.lang.Integer")}, "apply", desc, 1)
		if len(p) != 1 || p[0].String(ctx) != "T" || r == nil || r.String(ctx) != "T" || len(f) != 1 || f[0] != "T" {
			t.Fatal("independent method declaration lost", p, r, f)
		}
	})
}

func TestCalleeSourceProjectionUsesReceiverArguments(t *testing.T) {
	ctx := &class_context.ClassContext{ClassName: "caller.Owner", TypeParams: []string{"K", "V"}}
	desc := "(Ljava/lang/Object;)Ljava/lang/Object;"
	provider := func(string) (string, map[string]string, bool) {
		return "Ljava/lang/Object;", map[string]string{class_context.MethodDescKey("apply", desc): "(TK;)TV;", class_context.MethodSigKey("apply", 1): "(TK;)TV;"}, true
	}
	ctx.SiblingSourceClassFormals = func(string) ([]string, bool) { return []string{"K", "V"}, true }
	args := []JavaType{NewJavaClass("java.lang.String"), NewJavaClass("java.lang.Long")}
	if got := ResolveInstantiatedParamType(ctx, provider, "callee.Flat", args, "apply", desc, 1, 0); got == nil || got.String(ctx) != "String" {
		t.Fatal("parameter receiver substitution", got)
	}
	if got := ResolveInstantiatedReturnType(ctx, provider, "callee.Flat", args, "apply", 1); got == nil || got.String(ctx) != "Long" {
		t.Fatal("return receiver substitution", got)
	}
	for _, exact := range []bool{false, true} {
		var ps []JavaType
		var ret JavaType
		if exact {
			ps, ret, _ = ResolveInstantiatedSignatureExact(ctx, provider, "callee.Flat", args, "apply", desc, 1)
		} else {
			ps, ret = ResolveInstantiatedSignature(ctx, provider, "callee.Flat", args, "apply", 1)
		}
		if len(ps) != 1 || ps[0].String(ctx) != "String" || ret == nil || ret.String(ctx) != "Long" {
			t.Fatal("complete source signature", ps, ret)
		}
	}
	fields := func(string, string) (string, bool) { return "TV;", true }
	if got := ResolveInstantiatedFieldType(ctx, provider, fields, "callee.Flat", args, "value"); got == nil || got.String(ctx) != "Long" {
		t.Fatal("field receiver substitution", got)
	}
	if ps, ret, _ := ResolveInstantiatedSignatureExact(ctx, provider, "callee.Flat", nil, "apply", desc, 1); ps != nil || ret != nil {
		t.Fatal("raw source projection borrowed caller variables")
	}
}

func TestCalleeMethodReturnFormalShadowsReceiver(t *testing.T) {
	ctx := &class_context.ClassContext{ClassName: "caller.Owner"}
	provider := func(string) (string, map[string]string, bool) {
		return "<T:Ljava/lang/Number;>Ljava/lang/Object;", map[string]string{class_context.MethodSigKey("apply", 0): "<T::Ljava/lang/CharSequence;>()TT;"}, true
	}
	got := ResolveInstantiatedReturnType(ctx, provider, "callee.Owner", []JavaType{NewJavaClass("java.lang.Integer")}, "apply", 0)
	if got == nil || got.String(ctx) != "T" {
		t.Fatal("callee method T substituted by receiver's independent T", got)
	}
}

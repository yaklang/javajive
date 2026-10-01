package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestDeclaredSamInstantiationProof(t *testing.T) {
	const obj = "Ljava/lang/Object;"
	const text = "Ljava/lang/String;"
	const integer = "Ljava/lang/Integer;"
	const abc = "<A:Ljava/lang/Object;B:Ljava/lang/Object;C:Ljava/lang/Object;>Ljava/lang/Object;"
	const one = "<T:Ljava/lang/Object;>Ljava/lang/Object;"
	const two = "<A:Ljava/lang/Object;B:Ljava/lang/Object;>Ljava/lang/Object;"
	for _, tt := range []struct {
		name, class, sig, erased, actual string
		want                             []string
	}{
		{"three", abc, "(TA;TB;TC;)V", "(" + obj + obj + obj + ")V", "(" + text + obj + "Ljava/util/Map;)V", []string{"java.lang.String", "java.lang.Object", "java.util.Map"}},
		{"reversed", two, "(TB;TA;)V", "(" + obj + obj + ")V", "(" + integer + text + ")V", []string{"java.lang.String", "java.lang.Integer"}},
		{"repeated consistent", one, "(TT;TT;)V", "(" + obj + obj + ")V", "(" + text + text + ")V", []string{"java.lang.String"}},
		{"return only", one, "()TT;", "()" + obj, "()" + text, []string{"java.lang.String"}},
		{"concrete throws", one, "(TT;)V^Ljava/io/IOException;", "(" + obj + ")V", "(" + text + ")V", []string{"java.lang.String"}},
		{"array actual", one, "(TT;)V", "(" + obj + ")V", "([I)V", []string{"int[]"}},
		{"concrete class named T", one, "(LT;TT;)V", "(LT;" + obj + ")V", "(LT;" + text + ")V", []string{"java.lang.String"}},
		{"repeated conflict", one, "(TT;TT;)V", "(" + obj + obj + ")V", "(" + text + integer + ")V", nil},
		{"erasure mismatch", one, "(TT;)V", "(" + text + ")V", "(" + text + ")V", nil},
		{"fixed parameter mismatch", one, "(I)TT;", "(I)" + obj, "(J)" + text, nil},
		{"fixed return mismatch", one, "(TT;)I", "(" + obj + ")I", "(" + text + ")J", nil},
		{"arity mismatch", one, "(TT;)V", "(" + obj + ")V", "()V", nil},
		{"primitive generic argument", one, "(TT;)V", "(" + obj + ")V", "(I)V", nil},
		{"unused formal", two, "(TA;)V", "(" + obj + ")V", "(" + text + ")V", nil},
		{"unknown variable", one, "(TU;)V", "(" + obj + ")V", "(" + text + ")V", nil},
		{"strong bound", "<T:Ljava/lang/Number;>Ljava/lang/Object;", "(TT;)V", "(Ljava/lang/Number;)V", "(" + integer + ")V", nil},
		{"method variable", one, "<T:Ljava/lang/Object;>(TT;)V", "(" + obj + ")V", "(" + text + ")V", nil},
		{"nested generic signature", one, "(Ljava/util/List<TT;>;)V", "(Ljava/util/List;)V", "(Ljava/util/List;)V", nil},
		{"array of formal unsupported", one, "([TT;)V", "([" + obj + ")V", "([" + text + ")V", nil},
		{"no declaration", one, "", "(" + obj + ")V", "(" + text + ")V", nil},
		{"bad actual trailing", one, "(TT;)V", "(" + obj + ")V", "(" + text + ")Vjunk", nil},
		{"void parameter", one, "(TT;)V", "(" + obj + ")V", "(V)V", nil},
		{"malformed class", one, "(TT;)V", "(" + obj + ")V", "(L;)V", nil},
		{"no class signature", "", "(TT;)V", "(" + obj + ")V", "(" + text + ")V", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := inferDeclaredSamInstantiation("fixture.Function", tt.class, tt.sig, tt.erased, tt.actual)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("unproved target accepted: %v", got)
				}
				return
			}
			p, ok := types.AsParameterizedType(got)
			if !ok || len(p.TypeArgs) != len(tt.want) {
				t.Fatalf("missing target: %v", got)
			}
			for i, arg := range p.TypeArgs {
				actual := arg.String(&class_context.ClassContext{})
				if jc, ok := arg.RawType().(*types.JavaClass); ok {
					actual = jc.Name
				}
				if actual != tt.want[i] {
					t.Fatalf("argument %d: %s want %s", i, actual, tt.want[i])
				}
			}
		})
	}
}

func TestDeclaredSamThrowsOnlyVariablesKeepExistentialBinding(t *testing.T) {
	for _, tc := range []struct {
		name, class, sig, actual string
		want                     bool
	}{
		{"consumer", "<T:Ljava/lang/Object;E:Ljava/lang/Throwable;>Ljava/lang/Object;", "(TT;)V^TE;", "(Ljava/lang/Throwable;)V", true},
		{"two inputs", "<A:Ljava/lang/Object;B:Ljava/lang/Object;E:Ljava/lang/Throwable;>Ljava/lang/Object;", "(TA;TB;)V^TE;", "(Ljava/lang/Long;Ljava/lang/Integer;)V", true},
		{"return", "<T:Ljava/lang/Object;E:Ljava/lang/Exception;>Ljava/lang/Object;", "()TT;^TE;", "()Ljava/lang/String;", true},
		{"unused without throws", "<T:Ljava/lang/Object;E:Ljava/lang/Throwable;>Ljava/lang/Object;", "(TT;)V", "(Ljava/lang/String;)V", false},
		{"dependent throws bound", "<T:Ljava/lang/Object;E:TT;>Ljava/lang/Object;", "(TT;)V^TE;", "(Ljava/lang/String;)V", false},
		{"intersection throws bound", "<T:Ljava/lang/Object;E:Ljava/lang/Throwable;:Ljava/io/Serializable;>Ljava/lang/Object;", "(TT;)V^TE;", "(Ljava/lang/String;)V", false},
		{"strong input bound", "<T:Ljava/lang/Number;E:Ljava/lang/Throwable;>Ljava/lang/Object;", "(TT;)V^TE;", "(Ljava/lang/Integer;)V", false},
		{"foreign throws variable", "<T:Ljava/lang/Object;E:Ljava/lang/Throwable;>Ljava/lang/Object;", "(TT;)V^TX;", "(Ljava/lang/String;)V", false},
		{"malformed throws", "<T:Ljava/lang/Object;E:Ljava/lang/Throwable;>Ljava/lang/Object;", "(TT;)V^TE;junk", "(Ljava/lang/String;)V", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actual, ok := directSamTokens(tc.actual, false)
			if !ok {
				t.Fatal("invalid test descriptor")
			}
			erased := "("
			for range actual[:len(actual)-1] {
				erased += "Ljava/lang/Object;"
			}
			erased += ")" + actual[len(actual)-1]
			if tc.name == "return" {
				erased = "()Ljava/lang/Object;"
			}
			got := inferDeclaredSamInstantiation("example.Effect", tc.class, tc.sig, erased, tc.actual)
			if (got != nil) != tc.want {
				t.Fatalf("target=%v want=%v", got, tc.want)
			}
			if tc.want {
				pt, ok := types.AsParameterizedType(got)
				if !ok {
					t.Fatal("missing target")
				}
				if _, wildcard := pt.TypeArgs[len(pt.TypeArgs)-1].(*types.JavaWildcardType); !wildcard {
					t.Fatal("throws-only variable was guessed instead of captured")
				}
			}
		})
	}
}

func TestDeclaredSamExactDeclaration(t *testing.T) {
	const classSig = "<T:Ljava/lang/Object;>Ljava/lang/Object;"
	const erased = "(Ljava/lang/Object;)V"
	constant := func(desc string) values.JavaValue {
		return values.NewCustomValue(func(*class_context.ClassContext) string { return desc }, func() types.JavaType { return types.NewJavaClass("java.lang.invoke.MethodType") })
	}
	for _, tt := range []struct {
		name, method    string
		metadata        map[string]string
		available, want bool
	}{
		{"exact", "accept", map[string]string{class_context.MethodDescKey("accept", erased): "(TT;)V"}, true, true},
		{"other name", "different", map[string]string{class_context.MethodDescKey("accept", erased): "(TT;)V"}, true, false},
		{"arity alone insufficient", "accept", map[string]string{class_context.MethodSigKey("accept", 1): "(TT;)V"}, true, false},
		{"other overload", "accept", map[string]string{class_context.MethodDescKey("accept", "(Ljava/lang/String;)V"): "(TT;)V"}, true, false},
		{"no signature", "accept", map[string]string{class_context.MethodDescKey("accept", erased): ""}, true, false},
		{"no metadata", "accept", nil, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			ctx := &class_context.ClassContext{SiblingClassSig: func(name string) (string, map[string]string, bool) {
				called = true
				if name != "example/Sam" {
					t.Fatalf("wrong metadata owner %s", name)
				}
				return classSig, tt.metadata, tt.available
			}}
			d := &Decompiler{FunctionContext: ctx, InvokeDynamicName: tt.method}
			got := inferDeclaredLambdaTarget(d, types.NewJavaClass("example.Sam"), constant(erased), constant("(Ljava/lang/String;)V"))
			if (got != nil) != tt.want || !called {
				t.Fatalf("target=%v metadata called=%v", got, called)
			}
		})
	}
	if got := inferDeclaredLambdaTarget(nil, types.NewJavaClass("example.Sam"), constant(erased), constant("(Ljava/lang/String;)V")); got != nil {
		t.Fatalf("missing resolver accepted: %v", got)
	}
}

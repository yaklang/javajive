package javaclassparser

import (
	"strings"
	"testing"
)

func TestNativeGeneratedLocalRecoveryRespectsSourceBinding(t *testing.T) {
	for _, row := range []struct {
		name, body, params string
		bindings           map[string]bool
		unchanged          bool
	}{
		{"qualified this", "return this.var0;", "", nil, true},
		{"qualified receiver", "return owner.var0;", "", nil, true},
		{"qualified trivia", "return owner . /* gap var2 */ var0;", "", nil, true},
		{"call member", "return var0();", "", nil, true},
		{"call trivia", "return var0 /* member */ ();", "", nil, true},
		{"method reference", "return Owner::var0;", "", nil, true},
		{"string and comments", "return \"var0 var3\"; /* var0 */ // var2\n", "", nil, true},
		{"extended identifiers", "return $var0 + 中文var0 + var0$tail + var0_中文;", "", nil, true},
		{"lexical field", "return (var0/* original field */ = input);", "Object input", map[string]bool{"var0": true}, true},
		{"real parameter", "return var0;", "Object var0", nil, true},
		{"real declaration", "Owner var0 = this;return var0;", "", nil, true},
		{"receiver candidate", "return var0.member;", "", nil, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			got := addMissingGeneratedLocalDecls(row.body, row.params, "Owner", nil, "Object", row.bindings)
			if (got == row.body) != row.unchanged {
				t.Fatalf("source binding lost:\n%s", got)
			}
			if !row.unchanged && !strings.Contains(got, "Owner var0 = this;") {
				t.Fatal("lost existing receiver candidate")
			}
		})
	}
}
func TestAdversarialGeneratedFieldNameKeepsAnonymousReceiver(t *testing.T) {
	sources := map[string]string{"p/Base.java": `package p;public abstract class Base{protected Object var0;public Base(Object token){var0=token;}public abstract Object get();}`,
		"q/LocalOwner.java":  `package q;public class LocalOwner{public static p.Base build(Object token){return new p.Base(token){public Object get(){return var0;}};}}`,
		"q/LocalDriver.java": `package q;public class LocalDriver{public static void main(String[]args)throws Exception{int rows=0;for(Object token:new Object[]{null,new Object(),new StringBuilder("identity")}){p.Base value=LocalOwner.build(token);if(value.get()!=token||!value.getClass().isAnonymousClass()||value.getClass().getEnclosingMethod().getDeclaringClass()!=LocalOwner.class)throw new AssertionError("original field/receiver/identity/scope");rows++;}System.out.println(rows+":field:receiver:identity:scope");}}`}
	testNativePrivateSetterCompiledFixtureWithPolicies(t, "q/LocalOwner", "q.LocalDriver", "3:field:receiver:identity:scope\n", func(t *testing.T, debug string) map[string][]byte {
		return nativeCompileSourceReleaseClasses(t, sources, debug, "8")
	}, nil, []string{"normal"})
}

func TestNativeGeneratedLocalCandidateTokenBoundaries(t *testing.T) {
	for _, row := range []struct{ source, want string }{
		{"consume(var0)", "var0"}, {"owner.var0(var1,var2.member)", "var1,var2"},
		{"Owner::var0", ""}, {"var0::member", "var0"}, {"cond ? var0 : var1", "var0,var1"},
		{"array[var0]", "var0"}, {"$var0 + 中文var0 + var0_中文 + var0$tail", ""},
		{"var1_2 + var1_2_3", "var1_2"}, {"this . /* field */ var0", ""},
		{"var0 /* receiver */ .member", "var0"}, {"var0 /* method */ ()", ""},
		{"@var0 class X {}", ""}, {"break var0; continue var1;", ""},
		{`String s="var0"; char c='v'; // var1
 /* var2 */ consume(var3);`, "var3"},
		{`String s="""
 var0 " var1
 """; consume(var2);`, "var2"},
	} {
		t.Run(row.source, func(t *testing.T) {
			if got := strings.Join(generatedLocalVariableCandidates(row.source), ","); got != row.want {
				t.Fatalf("candidates=%q want=%q", got, row.want)
			}
		})
	}
}

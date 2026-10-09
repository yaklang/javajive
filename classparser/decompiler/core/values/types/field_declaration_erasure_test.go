package types

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"testing"
)

func TestInstantiatedFieldChecksDeclaringErasureBeforeSubstitution(t *testing.T) {
	for _, scenario := range []string{"concrete", "bounded", "array", "nested", "wrong descriptor", "wrong bound", "free formal", "dependent bound", "malformed field", "raw generic", "missing argument", "too many arguments", "missing declaration", "shadow", "cycle", "unknown ancestor"} {
		t.Run(scenario, func(t *testing.T) {
			cs, fs, desc := "<T:Ljava/lang/Object;>Ljava/lang/Object;", "TT;", "Ljava/lang/Object;"
			args := []JavaType{NewJavaClass("java.lang.String")}
			want := "String"
			switch scenario {
			case "bounded":
				cs = "<T:Ljava/lang/Number;>Ljava/lang/Object;"
				args[0] = NewJavaClass("java.lang.Integer")
				desc = "Ljava/lang/Number;"
				want = "Integer"
			case "array":
				fs = "[TT;"
				desc = "[Ljava/lang/Object;"
				want = "String[]"
			case "nested":
				fs = "Ljava/util/List<TT;>;"
				desc = "Ljava/util/List;"
				want = "List<String>"
			case "wrong descriptor":
				desc = "Ljava/lang/String;"
				want = ""
			case "wrong bound":
				cs = "<T:Ljava/lang/Number;>Ljava/lang/Object;"
				want = ""
			case "free formal":
				fs = "TU;"
				want = ""
			case "dependent bound":
				cs = "<T:TU;U:Ljava/lang/Object;>Ljava/lang/Object;"
				args = append(args, NewJavaClass("java.lang.Object"))
				want = ""
			case "malformed field":
				fs = "TT;garbage"
				want = ""
			case "raw generic":
				args = nil
				want = ""
			case "missing argument":
				args[0] = nil
				want = ""
			case "too many arguments":
				args = append(args, args[0])
				want = ""
			case "missing declaration", "shadow", "cycle", "unknown ancestor":
				want = ""
			}
			classes := func(n string) (string, map[string]string, bool) {
				if n == "sample/Child" {
					if scenario == "cycle" {
						return "Lsample/Child;", nil, true
					}
					if scenario == "unknown ancestor" {
						return "Lsample/Missing;", nil, true
					}
					return "Lsample/Box<Ljava/lang/String;>;", nil, true
				}
				return cs, nil, n == "sample/Box"
			}
			fields := func(n, f string) (string, bool) {
				if n == "sample/Child" && scenario == "shadow" {
					return "Ljava/lang/Number;", true
				}
				return fs, n == "sample/Box" && scenario != "missing declaration" && f == "value"
			}
			raw := "sample.Box"
			if scenario == "shadow" || scenario == "cycle" || scenario == "unknown ancestor" {
				raw = "sample.Child"
				args = nil
			}
			ctx := &class_context.ClassContext{TypeParams: []string{"U"}, ClassSig: "<U:Ljava/lang/Object;>Ljava/lang/Object;"}
			got := ResolveInstantiatedFieldTypeWithErasure(ctx, classes, fields, raw, args, "value", desc)
			if want == "" {
				if got != nil {
					t.Fatalf("unproved declaration recovered %s", got.String(ctx))
				}
			} else if got == nil || got.String(ctx) != want {
				t.Fatalf("source=%v want %s", got, want)
			}
		})
	}
}

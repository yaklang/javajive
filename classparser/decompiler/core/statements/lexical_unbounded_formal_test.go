package statements

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestUnboundedFormalQueryRetainsOriginalLexicalDeclaration(t *testing.T) {
	for _, test := range []struct {
		name, method, current string
		outer                 []string
		want                  bool
	}{
		{"own class unbounded", "", "<T:Ljava/lang/Object;>Ljava/lang/Object;", nil, true},
		{"own class bound", "", "<T:Ljava/lang/Comparable<TT;>;>Ljava/lang/Object;", nil, false},
		{"own method shadows bound", "<T:Ljava/lang/Object;>()V", "<T:Ljava/lang/Number;>Ljava/lang/Object;", nil, true},
		{"own class shadows outer bound", "", "<T:Ljava/lang/Object;>Ljava/lang/Object;", []string{"<T:Ljava/lang/Number;>Ljava/lang/Object;"}, true},
		{"outer bound", "", "Ljava/lang/Object;", []string{"<T:Ljava/lang/Number;>Ljava/lang/Object;"}, false},
		{"outer unbounded", "", "Ljava/lang/Object;", []string{"<T:Ljava/lang/Object;>Ljava/lang/Object;"}, true},
		{"empty method scope then bounded class", "", "Ljava/lang/Object;", []string{"", "<T:Ljava/lang/Number;>Ljava/lang/Object;"}, false},
		{"empty method scope then unbounded class", "", "Ljava/lang/Object;", []string{"", "<T:Ljava/lang/Object;>Ljava/lang/Object;"}, true},
		{"outer method shadows bound class", "", "Ljava/lang/Object;", []string{"<T:Ljava/lang/Object;>()V", "<T:Ljava/lang/Number;>Ljava/lang/Object;"}, true},
		{"nearest bounded", "", "Ljava/lang/Object;", []string{"<T:Ljava/lang/Number;>Ljava/lang/Object;", "<T:Ljava/lang/Object;>Ljava/lang/Object;"}, false},
		{"near class shadows outer method", "", "Ljava/lang/Object;", []string{"<T:Ljava/lang/Object;>Ljava/lang/Object;", "<T:Ljava/lang/Number;>()V"}, true},
		{"unknown dependent bound", "", "", []string{"<T:TU;>Ljava/lang/Object;", "<T:Ljava/lang/Object;>Ljava/lang/Object;"}, false},
		{"cyclic dependent bound", "", "", []string{"<T:TU;U:TT;>Ljava/lang/Object;"}, false},
		{"interface bound", "", "", []string{"<T::Ljava/lang/Runnable;>Ljava/lang/Object;"}, false},
		{"Object plus interface", "", "", []string{"<T:Ljava/lang/Object;:Ljava/io/Serializable;>Ljava/lang/Object;"}, false},
		{"unrelated nearer formal", "", "<U:Ljava/lang/Number;>Ljava/lang/Object;", []string{"<T:Ljava/lang/Object;>Ljava/lang/Object;"}, true},
		{"incomplete nearest declaration", "", "", []string{"<T:Ljava/lang/Number;", "<T:Ljava/lang/Object;>Ljava/lang/Object;"}, false},
		{"duplicate nearest declaration", "", "", []string{"<T:Ljava/lang/Object;T:Ljava/lang/Object;>Ljava/lang/Object;"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := &class_context.ClassContext{TypeParams: []string{"T"}, CurrentMethodSig: test.method, ClassSig: test.current, LexicalTypeParamSignatures: test.outer}
			ctx.DeclarationSourceName = func(string) (string, bool) { t.Fatal("proof formatted a declaration name"); return "", false }
			if got := typeVarIsUnbounded("T", ctx); got != test.want {
				t.Fatalf("unbounded=%v want=%v", got, test.want)
			}
			if len(ctx.GetAllImported()) != 0 {
				t.Fatal("proof imported bound type")
			}
		})
	}
	for _, variant := range []string{"legacy bare injected", "legacy bounded injected", "unknown formal", "work", "memory", "canceled", "depth", "length", "nil context", "empty name"} {
		t.Run(variant, func(t *testing.T) {
			name := "T"
			ctx := &class_context.ClassContext{TypeParams: []string{"T"}, LexicalTypeParamSignatures: []string{"<T:Ljava/lang/Object;>Ljava/lang/Object;"}}
			switch variant {
			case "legacy bare injected":
				ctx.LexicalTypeParamSignatures = nil
			case "legacy bounded injected":
				ctx.LexicalTypeParamSignatures = nil
				ctx.InjectedTypeParamBounds = map[string]string{"T": "Number"}
			case "unknown formal":
				ctx.TypeParams = nil
				ctx.LexicalTypeParamSignatures = nil
			case "work":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				c, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Work = workbudget.New(c, workbudget.Limits{})
			case "depth":
				ctx.LexicalTypeParamSignatures = make([]string, 129)
			case "length":
				ctx.LexicalTypeParamSignatures[0] = strings.Repeat("X", 65536)
			case "nil context":
				ctx = nil
			case "empty name":
				name = ""
			}
			if got := typeVarIsUnbounded(name, ctx); got != (variant == "legacy bare injected") {
				t.Fatalf("unbounded=%v", got)
			}
		})
	}
}

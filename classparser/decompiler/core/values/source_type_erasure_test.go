package values

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestSourceTypeErasureRequiresExactLexicalBounds(t *testing.T) {
	ctx := &class_context.ClassContext{ClassSig: "<T:Ljava/lang/Number;>Ljava/lang/Object;", CurrentMethodSig: "<T:Ljava/lang/CharSequence;U::Ljava/lang/Runnable;>()V", TypeParams: []string{"T", "E", "U", "Unknown"}}
	for _, test := range []struct {
		name, signature, want string
		known                 bool
	}{
		{"primitive", "I", "I", true}, {"parameterized", "Ljava/util/List<TT;>;", "Ljava/util/List;", true},
		{"method shadows class", "TT;", "Ljava/lang/CharSequence;", true},
		{"bound array", "[[TT;", "[[Ljava/lang/CharSequence;", true},
		{"primitive array", "[[[I", "[[[I", true},
		{"interface first bound", "TU;", "Ljava/lang/Runnable;", true},
		{"dependent bound", "TE;", "Ljava/lang/Number;", true}, {"missing bound", "TUnknown;", "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			testCtx := *ctx
			if test.name == "dependent bound" {
				testCtx.CurrentMethodSig = "<E:TT;>()V"
			}
			got, known := SourceTypeErasure(types.ParseSignature(test.signature), &testCtx)
			if known != test.known || got != test.want {
				t.Fatalf("erasure %q/%v expected %q/%v", got, known, test.want, test.known)
			}
		})
	}
}

func TestSourceTypeErasureUsesProvedEnclosingDeclarationOrder(t *testing.T) {
	for _, row := range []struct {
		name, method, current, signature, want string
		outer                                  []string
		known                                  bool
	}{
		{"outer unbounded", "", "<Z:Ljava/lang/Object;>Ljava/lang/Object;", "TT;", "Ljava/lang/Object;", []string{"<T:Ljava/lang/Object;>Ljava/lang/Object;"}, true},
		{"outer bound", "", "<Z:Ljava/lang/Object;>Ljava/lang/Object;", "TT;", "Ljava/lang/Number;", []string{"<T:Ljava/lang/Number;>Ljava/lang/Object;"}, true},
		{"outer bound array", "", "", "[[TT;", "[[Ljava/lang/Number;", []string{"<T:Ljava/lang/Number;>Ljava/lang/Object;"}, true},
		{"nearest method", "", "", "TT;", "Ljava/lang/CharSequence;", []string{"<T:Ljava/lang/CharSequence;>()V", "<T:Ljava/lang/Number;>Ljava/lang/Object;"}, true},
		{"own class shadows", "", "<T:Ljava/lang/String;>Ljava/lang/Object;", "TT;", "Ljava/lang/String;", []string{"<T:Ljava/lang/Number;>Ljava/lang/Object;"}, true},
		{"own method shadows", "<T::Ljava/lang/Runnable;>()V", "<T:Ljava/lang/String;>Ljava/lang/Object;", "TT;", "Ljava/lang/Runnable;", []string{"<T:Ljava/lang/Number;>Ljava/lang/Object;"}, true},
		{"unknown own bound is authoritative", "<T:TU;>()V", "", "TT;", "", []string{"<T:Ljava/lang/Number;>Ljava/lang/Object;"}, false},
		{"unknown nearest bound is authoritative", "", "", "TT;", "", []string{"<T:TU;>()V", "<T:Ljava/lang/Number;>Ljava/lang/Object;"}, false},
		{"static cut missing bound", "", "<Z:Ljava/lang/Object;>Ljava/lang/Object;", "TT;", "", nil, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			ctx := &class_context.ClassContext{TypeParams: []string{"T", "Z", "U"}, CurrentMethodSig: row.method, ClassSig: row.current, LexicalTypeParamSignatures: row.outer}
			got, known := SourceTypeErasure(types.ParseSignature(row.signature), ctx)
			if got != row.want || known != row.known {
				t.Fatalf("erasure %q/%v != %q/%v", got, known, row.want, row.known)
			}
		})
	}
	for _, kind := range []string{"depth", "length", "budget", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			ctx := &class_context.ClassContext{TypeParams: []string{"T"}, LexicalTypeParamSignatures: []string{"<T:Ljava/lang/Number;>Ljava/lang/Object;"}}
			switch kind {
			case "depth":
				ctx.LexicalTypeParamSignatures = make([]string, 129)
			case "length":
				ctx.LexicalTypeParamSignatures[0] = strings.Repeat("x", 65536)
			case "budget":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				c, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Work = workbudget.New(c, workbudget.Limits{})
			}
			if _, known := SourceTypeErasure(types.ParseSignature("TT;"), ctx); known {
				t.Fatal("unproved enclosing erasure accepted")
			}
		})
	}
}

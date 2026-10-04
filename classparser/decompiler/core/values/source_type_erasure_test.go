package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
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
		{"dependent bound", "TE;", "", false}, {"missing bound", "TUnknown;", "", false},
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

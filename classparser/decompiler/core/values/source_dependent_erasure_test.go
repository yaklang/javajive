package values

import (
	"context"
	"reflect"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestSourceDependentTypeErasureRetainsOriginalBinderIdentity(t *testing.T) {
	for _, test := range []struct {
		name, current, method, descriptor, subject, want string
		lexical                                          []string
		known                                            bool
	}{
		{name: "same method dependency", method: "<S:Ljava/lang/Object;T:TS;>(TT;)TT;", descriptor: "(Ljava/lang/Object;)Ljava/lang/Object;", subject: "TT;", want: "Ljava/lang/Object;", known: true},
		{name: "forward multihop bound", method: "<T:TU;U:TV;V:Ljava/lang/Number;>()V", descriptor: "()V", subject: "TT;", want: "Ljava/lang/Number;", known: true},
		{name: "interface first bound", method: "<T:TS;S::Ljava/lang/CharSequence;>()V", subject: "[TT;", want: "[Ljava/lang/CharSequence;", known: true},
		{name: "class dependent bound", current: "<U:TT;T:Ljava/lang/Number;>Ljava/lang/Object;", subject: "TU;", want: "Ljava/lang/Number;", known: true},
		{name: "method shadows class", current: "<T:Ljava/lang/Number;>Ljava/lang/Object;", method: "<S:Ljava/lang/CharSequence;T:TS;>()V", subject: "TT;", want: "Ljava/lang/CharSequence;", known: true},
		{name: "hidden bound keeps original identity", current: "<T:Ljava/lang/CharSequence;>Ljava/lang/Object;", lexical: []string{"<U:TT;>()V", "<T:Ljava/lang/Number;>Ljava/lang/Object;"}, subject: "TU;", want: "Ljava/lang/Number;", known: true},
		{name: "method dependency on class", current: "<T:Ljava/lang/Number;>Ljava/lang/Object;", method: "<U:TT;>()V", subject: "TU;", want: "Ljava/lang/Number;", known: true},
		{name: "unknown bound shadows known outer", current: "<T:Ljava/lang/Number;>Ljava/lang/Object;", method: "<T:TS;>()V", subject: "TT;"},
		{name: "cyclic bound", method: "<T:TU;U:TT;>()V", subject: "TT;"},
		{name: "self cycle", method: "<T:TT;>()V", subject: "TT;"},
		{name: "wrong physical method descriptor", method: "<T:TU;U:Ljava/lang/Number;>(TT;)V", descriptor: "(Ljava/lang/Object;)V", subject: "TT;"},
		{name: "malformed declaration", current: "<T:Ljava/lang/Number;>Ljava/lang/Object;bad", method: "<U:TT;>()V", subject: "TU;"},
		{name: "static cut cannot borrow missing scope", method: "<U:TT;>()V", subject: "TU;"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := &class_context.ClassContext{TypeParams: []string{"T", "S", "U", "V"}, ClassSig: test.current, CurrentMethodSig: test.method, CurrentMethodDesc: test.descriptor, LexicalTypeParamSignatures: test.lexical}
			before := append([]string(nil), test.lexical...)
			got, known := SourceTypeErasure(types.ParseSignature(test.subject), ctx)
			if got != test.want || known != test.known || !reflect.DeepEqual(before, ctx.LexicalTypeParamSignatures) {
				t.Fatalf("original erasure=%q/%v want=%q/%v or declaration stack mutated", got, known, test.want, test.known)
			}
		})
	}
}

func TestSourceDependentTypeErasureRefusesExhaustedProof(t *testing.T) {
	for _, mode := range []string{"work", "allocation", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			ctx := &class_context.ClassContext{TypeParams: []string{"T", "U"}, CurrentMethodSig: "<T:TU;U:Ljava/lang/Object;>()V"}
			switch mode {
			case "work":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "allocation":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				c, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Work = workbudget.New(c, workbudget.Limits{})
			}
			if _, known := SourceTypeErasure(types.ParseSignature("TT;"), ctx); known {
				t.Fatal("exhausted proof defaulted to an Object binding")
			}
		})
	}
}

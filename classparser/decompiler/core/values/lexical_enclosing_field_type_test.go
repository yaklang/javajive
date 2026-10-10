package values

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestLexicalEnclosingFieldTypeRequiresOriginalInstanceAndDeclaration(t *testing.T) {
	for _, change := range []string{"original", "no PC", "no projection", "wrong operand", "wrong PC", "wrong field", "unknown declaration", "changed bounds", "raw outer", "no parent", "foreign type", "unnamed source", "method shadow", "member shadow", "missing formal", "cyclic context", "duplicate class", "too deep", "work", "memory", "cancelled"} {
		t.Run(change, func(t *testing.T) {
			sig := "<K:Ljava/lang/Number;>Ljava/lang/Object;"
			outer := &class_context.ClassContext{ClassName: "scope.Owner", ClassSig: sig}
			ctx := &class_context.ClassContext{ClassName: "scope.Owner$Reader", ClassSig: "Ljava/lang/Object;", LexicalClassName: "Reader", TypeParams: []string{"K"}, SourceLexicalParent: outer}
			field := NewRefMember(&JavaRef{IsThis: true}, "capture", types.NewJavaClass("scope.Owner"))
			field.OriginPC, field.HasOriginPC = 7, true
			ctx.SourceLexicalCapturedField = func(v any, pc int, name string) (string, bool) {
				return "Owner.this", v == field && pc == 7 && name == "capture"
			}
			ctx.SiblingClassSig = func(name string) (string, map[string]string, bool) {
				return sig, nil, name == "scope/Owner" && change != "unknown declaration"
			}
			switch change {
			case "no PC":
				field.HasOriginPC = false
			case "no projection":
				ctx.SourceLexicalCapturedField = nil
			case "wrong operand":
				ctx.SourceLexicalCapturedField = func(any, int, string) (string, bool) { return "", false }
			case "wrong PC":
				field.OriginPC++
			case "wrong field":
				field.Member = "other"
			case "changed bounds":
				outer.ClassSig = "<K:Ljava/lang/Object;>Ljava/lang/Object;"
			case "raw outer":
				sig = "Ljava/lang/Object;"
				outer.ClassSig = sig
			case "no parent":
				ctx.SourceLexicalParent = nil
			case "foreign type":
				field.JavaType = types.NewJavaClass("scope.Foreign")
			case "unnamed source":
				ctx.LexicalClassName = ""
			case "method shadow":
				ctx.CurrentMethodSig = "<K:Ljava/lang/Number;>()V"
			case "member shadow":
				ctx.ClassSig = "<K:Ljava/lang/Number;>Ljava/lang/Object;"
			case "missing formal":
				ctx.TypeParams = nil
			case "cyclic context":
				outer.SourceLexicalParent = ctx
			case "duplicate class":
				outer.SourceLexicalParent = &class_context.ClassContext{ClassName: outer.ClassName, ClassSig: sig}
			case "too deep":
				for i := 0; i < 65; i++ {
					outer.SourceLexicalParent = &class_context.ClassContext{ClassName: string(rune('A' + i)), SourceLexicalParent: outer.SourceLexicalParent}
				}
			case "work":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "cancelled":
				c, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Work = workbudget.New(c, workbudget.Limits{})
			}
			got := recoverLexicalEnclosingFieldReceiver(ctx, field)
			if (got != nil) != (change == "original") {
				t.Fatalf("lexical receiver=%v", got)
			}
			if got != nil && got.String(ctx) != "Owner<K>" {
				t.Fatal(got.String(ctx))
			}
		})
	}
}

func TestSourceFieldTypeRecoversLexicalArrayFormalsWithoutNarrowing(t *testing.T) {
	for _, change := range []string{"original", "unknown bound", "dependent bound", "wrong erasure", "wrong rank", "method shadow", "foreign receiver", "opaque this", "missing declaration"} {
		t.Run(change, func(t *testing.T) {
			ctx := &class_context.ClassContext{ClassName: "scope.Owner$Reader", ClassSig: "Ljava/lang/Object;", TypeParams: []string{"K"}, LexicalTypeParamSignatures: []string{"<K:Ljava/lang/Number;>Ljava/lang/Object;"}, FieldTypeVars: map[string]string{"rows": "K[][]"}}
			ref := &JavaRef{IsThis: true}
			field := NewRefMember(ref, "rows", types.NewJavaArrayType(types.NewJavaArrayType(types.NewJavaClass("java.lang.Number"))))
			switch change {
			case "unknown bound":
				ctx.LexicalTypeParamSignatures = nil
			case "dependent bound":
				ctx.LexicalTypeParamSignatures = []string{"<K:TU;U:Ljava/lang/Number;>Ljava/lang/Object;"}
			case "wrong erasure":
				field.JavaType = types.NewJavaArrayType(types.NewJavaArrayType(types.NewJavaClass("java.lang.Object")))
			case "wrong rank":
				field.JavaType = types.NewJavaArrayType(types.NewJavaClass("java.lang.Number"))
			case "method shadow":
				ctx.CurrentMethodSig = "<K:Ljava/lang/Number;>()V"
			case "foreign receiver":
				ref.IsThis = false
			case "opaque this":
				ref.CustomValue = NewCustomValue(nil, nil)
			case "missing declaration":
				ctx.FieldTypeVars = nil
			}
			before := field.Type().String(ctx)
			got := SourceFieldType(ctx, field)
			if (got != nil) != (change == "original") {
				t.Fatalf("source array=%v", got)
			}
			if got != nil && got.String(ctx) != "K[][]" {
				t.Fatal(got.String(ctx))
			}
			if field.Type().String(ctx) != before {
				t.Fatal("shared computational type changed")
			}
		})
	}
}

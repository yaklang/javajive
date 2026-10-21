package values

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestUncapturedMethodReferenceSourceOwnsCanonicalRendererAndCreationTarget(t *testing.T) {
	identity := MethodReferenceSource{Kind: 6, Owner: "Sample", Member: "format", Descriptor: "(Ljava/lang/Object;)Ljava/lang/String;", CallName: "apply", CallDescriptor: "()Ljava/util/function/Function;", ErasedSAM: "(Ljava/lang/Object;)Ljava/lang/Object;", Instantiated: "(Ljava/lang/Object;)Ljava/lang/String;"}
	raw := types.NewJavaClass("java.util.function.Function")
	makeValue := func() *CustomValue {
		value := NewUncapturedMethodReferenceSource(identity, raw)
		if value == nil {
			t.Fatal("typed ordinary reference was refused")
		}
		value.OriginPC, value.HasOriginPC = 7, true
		value.MarkOriginalLambdaFactory(7)
		return value
	}
	for _, scenario := range []string{"original", "opaque callbacks", "serialized source authority", "lost factory", "changed origin", "changed kind flag", "changed instantiated type", "unknown capture", "new capture", "output cap", "retained target", "target bound"} {
		t.Run(scenario, func(t *testing.T) {
			value := makeValue()
			ctx := &class_context.ClassContext{}
			called := false
			value.StringFunc = func(*class_context.ClassContext) string { called = true; return "wrong()" }
			value.WriteFunc = func(*class_context.ClassContext, *workbudget.Writer) error { called = true; return nil }
			wantKnown := true
			switch scenario {
			case "serialized source authority":
				value = NewSerializableReferenceSource(value, new(int))
				if value == nil {
					t.Fatal("separate original serialization receipt was refused")
				}
				wantKnown = false
			case "lost factory":
				value.originalLambdaFactory = nil
				wantKnown = false
			case "changed origin":
				value.OriginPC++
				wantKnown = false
			case "changed kind flag":
				value.IsMethodRef = false
				wantKnown = false
			case "changed instantiated type":
				value.InstantiatedMtdDesc = "()V"
				wantKnown = false
			case "unknown capture":
				value.CapturesKnown = false
				wantKnown = false
			case "new capture":
				value.Captures = []JavaValue{makeValue()}
				wantKnown = false
			case "output cap":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 7})
			case "retained target", "target bound":
				count := 1
				if scenario == "target bound" {
					count = 17
					wantKnown = false
				}
				for i := 0; i < count; i++ {
					var known bool
					value, known = value.RetainMethodReferenceTarget(raw, types.NewJavaClass("java.util.function.UnaryOperator"))
					if !known || value == nil {
						t.Fatal("lost structural target renderer")
					}
				}
			}
			got, known := value.UncapturedMethodReferenceSource()
			if known != wantKnown || known && got != identity {
				t.Fatalf("reference tuple=%v known=%v", got, known)
			}
			text := value.String(ctx)
			if scenario == "output cap" {
				if text != "" || ctx.Work.Err() == nil {
					t.Fatal("unbounded canonical reference", text)
				}
			} else if scenario == "target bound" {
				if text != "" {
					t.Fatal("target overflow rendered a partial target", text)
				}
			} else if scenario == "retained target" {
				if text != "(Function) ((UnaryOperator) (Sample::format))" {
					t.Fatal("creation target changed", text)
				}
			} else if text != "Sample::format" {
				t.Fatal("canonical original source changed", text)
			}
			if called {
				t.Fatal("an opaque callback supplied reference source")
			}
		})
	}
}

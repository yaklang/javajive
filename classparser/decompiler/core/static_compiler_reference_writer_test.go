package core

import (
	"errors"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestStaticCompilerReferenceWriterReopensFinalTypeNamespace(t *testing.T) {
	for _, scenario := range []string{"original", "unrelated local", "late value shadow", "late type and package shadow", "changed qualifier"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := &class_context.ClassContext{ClassName: "p.Owner", PackageName: "p"}
			pc, observations := 0, 0
			d := &Decompiler{FunctionContext: ctx, lambdaFactoryOrigin: &pc}
			d.OriginalLambdaDeclaration = func(string, string, string) LambdaOriginalDeclaration { return LambdaDeclarationSyntheticBody }
			d.DumpClassLambdaSourceAtOrigin = func(string, string, *utils.VariableId, []values.JavaValue, *LambdaReferenceAdapter, int) (LambdaBodySource, error) {
				return LambdaBodySource{Code: "Owner::sum", ReferenceTarget: t19IntOpType(), ReferenceOwner: "p.Owner", ReferenceQualifier: "Owner", RecordRenderedWithCaptures: func(render LambdaSourceRender) {
					observations++
					if !render.Complete || len(render.Captures) != 0 || !strings.Contains(render.Packet, "Owner::sum") {
						t.Fatal("static source observation is not the completed zero-capture packet", render)
					}
				}}, nil
			}
			bootstrap := buildinBootstrapMethods["java.lang.invoke.LambdaMetafactory.metafactory"]
			value, err := bootstrap(t19SAM(), t19Impl("p.Owner", "lambda$factory$0", "(I)I", RefInvokeStatic), t19SAM())(d, nil, t19IntOpType())
			if err != nil {
				t.Fatal("original callback failed before the final namespace change", err)
			}
			writer, known := value.(*values.CustomValue)
			if !known || writer.WriteFunc == nil {
				t.Fatal("reference did not retain its actual bounded writer")
			}
			switch scenario {
			case "unrelated local":
				ctx.Arguments = []string{"unrelated"}
			case "late value shadow":
				ctx.Arguments = []string{"Owner"}
			case "late type and package shadow":
				ctx.TypeParams, ctx.Arguments = []string{"Owner"}, []string{"p"}
			case "changed qualifier":
				// Same original type, but a final namespace policy now requires
				// qualification. The retained simple source packet is stale.
				ctx.LexicalTypeNames = map[string]bool{"Owner": true}
			}
			out := workbudget.NewWriter(nil)
			err = writer.WriteFunc(ctx, out)
			want := scenario == "original" || scenario == "unrelated local"
			if want {
				if err != nil || observations != 1 || !strings.Contains(out.String(), "Owner::sum") {
					t.Fatalf("unchanged static source lost: err=%v observations=%d source=%s", err, observations, out.String())
				}
			} else if !errors.Is(err, ErrLambdaSourceUnsupported) || observations != 0 || out.Len() != 0 {
				t.Fatalf("stale type name acquired output/receipt: err=%v observations=%d source=%s", err, observations, out.String())
			}
		})
	}
}

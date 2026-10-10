package values

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestSourceFormalLocalResultRequiresExactOriginalSignatureAndCallerErasure(t *testing.T) {
	for _, scenario := range []string{"original", "renamed callee formal", "shadowed class formal", "same-class cache conflict", "checked operand", "raw witness", "conflicting witnesses", "foreign caller formal", "caller shadow changes erasure", "different result erasure", "wrong signature erasure", "missing signature", "class result formal", "missing origin", "wrong invoke kind", "parameterized consumer", "widened consumer", "array consumer", "narrowed consumer", "nested call", "poly operand", "inline ref", "cast cycle", "type cycle", "type depth", "missing metadata", "foreign metadata", "long signature", "budget", "memory", "canceled", "disabled"} {
		t.Run(scenario, func(t *testing.T) {
			f, ctx, meta, sig := widenedLocalResultFixture()
			ctx.CurrentMethodSig = "<T:Ljava/lang/Number;>()Ljava/lang/Number;"
			consumer := types.NewJavaClass("java.lang.Number")
			first := f.Arguments[0]
			f.Arguments[0] = &CastExpression{Value: first, TargetType: types.NewJavaClass("T"), OriginPC: 7}
			switch scenario {
			case "renamed callee formal":
				*sig = strings.ReplaceAll(strings.ReplaceAll(*sig, "<N:", "<U:"), "TN;", "TU;")
			case "shadowed class formal":
				ctx.ClassSig = "<T:Ljava/lang/Object;>Ljava/lang/Object;"
			case "same-class cache conflict":
				ctx.ClassName = f.ClassName
				ctx.MethodSignaturesByDesc = map[string]string{class_context.MethodDescKey(f.FunctionName, f.Descriptor): "<T:Ljava/lang/Object;>(Ljava/lang/Object;Ljava/lang/Class<TT;>;)TT;"}
			case "checked operand":
				f.Arguments[0] = &CastExpression{Value: NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object")), TargetType: types.NewJavaClass("T"), OriginPC: 7}
			case "raw witness":
				f.Arguments[1] = NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Class"))
			case "conflicting witnesses":
				*sig = strings.ReplaceAll(*sig, "(Ljava/lang/Number;", "(TN;")
				f.Arguments[0] = NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Integer"))
			case "foreign caller formal":
				ctx.TypeParams = []string{"U"}
			case "caller shadow changes erasure":
				ctx.ClassSig = "<T:Ljava/lang/Number;>Ljava/lang/Object;"
				ctx.CurrentMethodSig = "<T:Ljava/lang/Object;>()Ljava/lang/Object;"
			case "different result erasure":
				ctx.CurrentMethodSig = "<T:Ljava/lang/Integer;>()Ljava/lang/Integer;"
			case "wrong signature erasure":
				*sig = strings.ReplaceAll(*sig, "<N:Ljava/lang/Number;>", "<N:Ljava/lang/Object;>")
			case "missing signature":
				*sig = ""
			case "class result formal":
				*sig = "(Ljava/lang/Number;Ljava/lang/Class<TN;>;)TN;"
			case "missing origin":
				f.HasOriginPC = false
			case "wrong invoke kind":
				f.Kind = InvokeSpecial
			case "parameterized consumer":
				consumer = types.NewParameterizedType("java.lang.Number", []types.JavaType{types.NewJavaClass("T")})
			case "widened consumer":
				consumer = types.NewJavaClass("java.lang.Object")
			case "array consumer":
				consumer = types.NewJavaArrayType(consumer)
			case "narrowed consumer":
				consumer = types.NewJavaClass("java.lang.Integer")
			case "nested call":
				f.Arguments[1] = &FunctionCallExpression{FuncType: &types.JavaFuncType{ReturnType: f.Arguments[1].Type()}}
			case "poly operand":
				f.Arguments[1] = &CustomValue{Flag: "lambda"}
			case "inline ref":
				f.Arguments[1].(*JavaRef).CustomValue = &CustomValue{Flag: "lambda"}
			case "cast cycle":
				cast := &CastExpression{TargetType: types.NewJavaClass("T")}
				cast.Value = cast
				f.Arguments[0] = cast
			case "type cycle":
				p := types.NewParameterizedType("java.lang.Class", nil)
				v, _ := types.AsParameterizedType(p)
				v.TypeArgs = []types.JavaType{p}
				f.Arguments[1] = NewJavaRef(utils.NewRootVariableId(), nil, p)
			case "type depth":
				var p types.JavaType = types.NewJavaClass("T")
				for i := 0; i < 34; i++ {
					p = types.NewParameterizedType("java.lang.Class", []types.JavaType{p})
				}
				f.Arguments[1] = NewJavaRef(utils.NewRootVariableId(), nil, p)
			case "missing metadata":
				ctx.InvocationMetadata = nil
			case "foreign metadata":
				ctx.SiblingClassSig = func(string) (string, map[string]string, bool) { return "", nil, false }
				c := meta["probe/Owner"]
				c.Name = "foreign/Owner"
				meta["probe/Owner"] = c
			case "long signature":
				*sig += strings.Repeat("x", 8193)
			case "budget":
				ctx.Work = workbudget.New(context.Background(), workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				ctx.Work = workbudget.New(context.Background(), workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				c, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Work = workbudget.New(c, workbudget.Limits{})
			case "disabled":
				ctx.Env = func(k string) string {
					if k == "JDEC_SOURCE_FORMAL_LOCAL_RESULT_OFF" {
						return "1"
					}
					return ""
				}
			}
			before := append([]JavaValue(nil), f.Arguments...)
			witness := f.Witness()
			got := f.SourceFormalLocalResult(ctx, consumer)
			want := scenario == "original" || scenario == "renamed callee formal" || scenario == "shadowed class formal" || scenario == "same-class cache conflict" || scenario == "checked operand"
			if (got != nil) != want || got != nil && got.String(ctx) != "T" {
				t.Fatalf("formal=%v want=%v", got, want)
			}
			if !reflect.DeepEqual(before, f.Arguments) || f.Witness() != witness {
				t.Fatal("source consumer mutated original arguments or invocation")
			}
		})
	}
}

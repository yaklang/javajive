package values

import (
	"context"
	"reflect"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// The return view follows every original instantiated parent edge. It does not
// change argument evaluation, the computational result, or the original types.
func TestSourceInvocationResultProjectsOriginalArgumentSupertypes(t *testing.T) {
	for _, variant := range []string{"direct", "transitive", "conflicting diamond", "duplicate parent", "missing parent", "incomplete parent", "foreign identity", "missing Signature", "wrong physical edge", "cycle", "raw argument", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			desc := "(Lprobe/Box;)Lprobe/Cursor;"
			sig := "<E:Ljava/lang/Object;>(Lprobe/Box<TE;>;)Lprobe/Cursor<TE;>;"
			metas := map[string]callbinding.Class{
				"probe/Factory": {Name: "probe/Factory", ParentsComplete: true, MembersComplete: true, Methods: []callbinding.Method{{Name: "make", Desc: desc, Signature: sig, Static: true, Generic: true, Public: true}}},
				"probe/Derived": {Name: "probe/Derived", ParentsComplete: true, MembersComplete: true, Parents: []string{"probe/Box"}, Signature: "<T:Ljava/lang/Object;>Lprobe/Box<TT;>;"},
				"probe/Middle":  {Name: "probe/Middle", ParentsComplete: true, MembersComplete: true, Parents: []string{"probe/Box"}, Signature: "<T:Ljava/lang/Object;>Lprobe/Box<TT;>;"},
				"probe/Box":     {Name: "probe/Box", ParentsComplete: true, MembersComplete: true, Parents: []string{"java/lang/Object"}, Signature: "<T:Ljava/lang/Object;>Ljava/lang/Object;"},
			}
			token := types.NewJavaClass("java.lang.String")
			argument := NewJavaRef(utils.NewRootVariableId(), nil, types.NewParameterizedType("probe.Derived", []types.JavaType{token}))
			call := &FunctionCallExpression{ClassName: "probe.Factory", FunctionName: "make", Descriptor: desc, IsStatic: true, Kind: InvokeStatic, OriginPC: 7, HasOriginPC: true, Arguments: []JavaValue{argument}, FuncType: &types.JavaFuncType{ReturnType: types.NewJavaClass("probe.Cursor")}}
			ctx := &class_context.ClassContext{ClassName: "probe.Caller", InvocationMetadata: func(n string) (callbinding.Class, bool) { m, ok := metas[n]; return m, ok }}
			derived := metas["probe/Derived"]
			switch variant {
			case "transitive":
				derived.Parents, derived.Signature = []string{"probe/Middle"}, "<T:Ljava/lang/Object;>Lprobe/Middle<TT;>;"
			case "conflicting diamond":
				derived.Parents, derived.Signature = []string{"probe/Box", "probe/Middle"}, "<T:Ljava/lang/Object;>Lprobe/Box<TT;>;Lprobe/Middle<Ljava/lang/Integer;>;"
			case "duplicate parent":
				derived.Parents = []string{"probe/Box", "probe/Box"}
			case "missing parent":
				delete(metas, "probe/Box")
			case "incomplete parent":
				box := metas["probe/Box"]
				box.ParentsComplete = false
				metas["probe/Box"] = box
			case "foreign identity":
				derived.Name = "probe/Foreign"
			case "missing Signature":
				derived.Signature = ""
			case "wrong physical edge":
				derived.Parents = []string{"probe/Middle"}
			case "cycle":
				derived.Parents, derived.Signature = []string{"probe/Derived"}, "<T:Ljava/lang/Object;>Lprobe/Derived<TT;>;"
			case "raw argument":
				argument = NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("probe.Derived"))
				call.Arguments[0] = argument
			case "work":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				canceled, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Work = workbudget.New(canceled, workbudget.Limits{})
			}
			metas["probe/Derived"] = derived
			originalType, originalResult := argument.Type().Copy(), call.FuncType.ReturnType.Copy()
			got := call.SourceInvocationResultType(ctx)
			positive := variant == "direct" || variant == "transitive"
			if (got != nil) != positive || got != nil && got.String(ctx) != "Cursor<String>" {
				t.Fatalf("original hierarchy %s returned %v, want positive=%v", variant, got, positive)
			}
			if call.Arguments[0] != argument || !reflect.DeepEqual(argument.Type(), originalType) || !reflect.DeepEqual(call.FuncType.ReturnType, originalResult) {
				t.Fatal("source proof changed original computational operands/types")
			}
		})
	}
}

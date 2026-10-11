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

func TestSourceInvocationResultRequiresSelectedDeclarationAndInvariantWitnesses(t *testing.T) {
	for _, mode := range []string{"static witness", "derived raw header", "wrong derived raw header", "invented generic header", "class receiver", "missing origin", "wrong opcode", "wrong staticness", "bridge declaration", "wrong computational result", "wrong descriptor", "missing metadata", "wrong owner", "incomplete methods", "incomplete parents", "missing Signature", "wrong Signature erasure", "contradictory sibling Signature", "same-class cache conflict", "raw witness", "unsatisfied bound", "parameterized bound", "custom witness", "stack witness", "cyclic value", "typed nil value", "typed nil type", "cyclic type", "deep type", "free caller formal", "work", "memory", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			desc := "(Lprobe/Box;)Lprobe/Cursor;"
			sig := "<E:Ljava/lang/Object;>(Lprobe/Box<TE;>;)Lprobe/Cursor<TE;>;"
			args := []JavaValue{NewJavaRef(utils.NewRootVariableId(), nil, types.NewParameterizedType("probe.Box", []types.JavaType{types.NewJavaClass("java.lang.String")}))}
			f := &FunctionCallExpression{ClassName: "probe.Factory", FunctionName: "make", Descriptor: desc, IsStatic: true, Kind: InvokeStatic, OriginPC: 7, HasOriginPC: true, Arguments: args, FuncType: &types.JavaFuncType{ReturnType: types.NewJavaClass("probe.Cursor")}}
			meta := callbinding.Class{Name: "probe/Factory", MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{{Name: "make", Desc: desc, Signature: sig, Static: true, Generic: true, Public: true}}}
			ctx := &class_context.ClassContext{ClassName: "probe.Caller"}
			siblingSig := sig
			sourceClassSig := ""
			ctx.InvocationMetadata = func(n string) (callbinding.Class, bool) {
				if n == meta.Name {
					return meta, true
				}
				return callbinding.Class{}, false
			}
			ctx.SiblingClassSig = func(n string) (string, map[string]string, bool) {
				if n == meta.Name {
					return func() string {
						if sourceClassSig != "" {
							return sourceClassSig
						}
						return meta.Signature
					}(), map[string]string{class_context.MethodDescKey("make", desc): siblingSig}, true
				}
				return "", nil, false
			}
			switch mode {
			case "derived raw header":
				meta.Parents = []string{"java/lang/Object"}
				sourceClassSig = "Ljava/lang/Object;"
			case "wrong derived raw header":
				meta.Parents = []string{"java/lang/Object"}
				sourceClassSig = "Ljava/lang/Number;"
			case "invented generic header":
				meta.Parents = []string{"java/lang/Object"}
				sourceClassSig = "<T:Ljava/lang/Object;>Ljava/lang/Object;"

			case "class receiver":
				meta.Signature = "<E:Ljava/lang/Object;>Ljava/lang/Object;"
				sig = "()Lprobe/Cursor<TE;>;"
				desc = "()Lprobe/Cursor;"
				siblingSig = sig
				meta.Methods[0].Desc = desc
				meta.Methods[0].Signature = sig
				meta.Methods[0].Static = false
				meta.Methods[0].Generic = false
				f.Descriptor = desc
				f.IsStatic = false
				f.Kind = InvokeVirtual
				f.Arguments = nil
				f.Object = NewJavaRef(utils.NewRootVariableId(), nil, types.NewParameterizedType("probe.Factory", []types.JavaType{types.NewJavaClass("java.lang.String")}))
			case "missing origin":
				f.HasOriginPC = false
			case "wrong opcode":
				f.Kind = InvokeSpecial
			case "wrong staticness":
				meta.Methods[0].Static = false
			case "bridge declaration":
				meta.Methods[0].Bridge = true
			case "wrong computational result":
				f.FuncType.ReturnType = types.NewJavaClass("java.lang.Object")

			case "wrong descriptor":
				f.Descriptor = "()Lprobe/Cursor;"
			case "missing metadata":
				ctx.InvocationMetadata = nil
			case "wrong owner":
				meta.Name = "foreign/Factory"
			case "incomplete methods":
				meta.MembersComplete = false
			case "incomplete parents":
				meta.ParentsComplete = false
			case "missing Signature":
				meta.Methods[0].Signature = ""
				siblingSig = ""
			case "wrong Signature erasure":
				meta.Methods[0].Signature = "<E:Ljava/lang/Object;>(Ljava/lang/Object;)Lprobe/Cursor<TE;>;"
				siblingSig = meta.Methods[0].Signature
			case "contradictory sibling Signature":
				siblingSig = "<E:Ljava/lang/Object;>(Lprobe/Box<TE;>;)Lprobe/Cursor<Ljava/lang/Object;>;"
			case "same-class cache conflict":
				ctx.ClassName = f.ClassName
				ctx.MethodSignaturesByDesc = map[string]string{class_context.MethodDescKey("make", desc): "<E:Ljava/lang/Object;>(Lprobe/Box<TE;>;)Lprobe/Cursor<Ljava/lang/Object;>;"}
			case "unsatisfied bound":
				meta.Methods[0].Signature = "<E:Ljava/lang/Number;>(Lprobe/Box<TE;>;)Lprobe/Cursor<TE;>;"
				siblingSig = meta.Methods[0].Signature
			case "parameterized bound":
				meta.Methods[0].Signature = "<E::Ljava/lang/Comparable<TE;>;>(Lprobe/Box<TE;>;)Lprobe/Cursor<TE;>;"
				siblingSig = meta.Methods[0].Signature

			case "raw witness":
				f.Arguments[0] = NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("probe.Box"))
			case "custom witness":
				args[0].(*JavaRef).CustomValue = &CustomValue{}
			case "stack witness":
				args[0].(*JavaRef).StackVar = JavaNull
			case "cyclic value":
				v := &JavaExpression{}
				v.Values = []JavaValue{v}
				f.Arguments[0] = v
			case "typed nil value":
				f.Arguments[0] = (*JavaRef)(nil)
			case "typed nil type":
				f.Arguments[0] = NewJavaRef(utils.NewRootVariableId(), nil, (*types.JavaTypeWrap)(nil))

			case "cyclic type":
				p := types.NewParameterizedType("probe.Box", nil)
				raw, _ := types.AsParameterizedType(p)
				raw.TypeArgs = []types.JavaType{p}
				f.Arguments[0] = NewJavaRef(utils.NewRootVariableId(), nil, p)
			case "deep type":
				var typ types.JavaType = types.NewJavaClass("java.lang.String")
				for i := 0; i < 34; i++ {
					typ = types.NewParameterizedType("probe.Box", []types.JavaType{typ})
				}
				f.Arguments[0] = NewJavaRef(utils.NewRootVariableId(), nil, typ)
			case "free caller formal":
				f.Arguments[0] = NewJavaRef(utils.NewRootVariableId(), nil, types.NewParameterizedType("probe.Box", []types.JavaType{types.NewJavaClass("T")}))
			case "work":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				c, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Work = workbudget.New(c, workbudget.Limits{})
			}
			before := f.Type().Copy()
			operands := append([]JavaValue(nil), f.Arguments...)
			got := f.SourceInvocationResultType(ctx)
			want := mode == "static witness" || mode == "derived raw header" || mode == "class receiver" || mode == "same-class cache conflict"
			if (got != nil) != want {
				t.Fatalf("source result=%v want=%v", got, want)
			}
			if got != nil {
				p, ok := types.AsParameterizedType(got)
				if !ok || len(p.TypeArgs) != 1 || p.TypeArgs[0].String(ctx) != "String" {
					t.Fatal("wrong invariant result", got)
				}
			}
			if !reflect.DeepEqual(before.RawType(), f.Type().RawType()) || !reflect.DeepEqual(operands, f.Arguments) {
				t.Fatal("source proof changed computational type or operands")
			}
		})
	}
}

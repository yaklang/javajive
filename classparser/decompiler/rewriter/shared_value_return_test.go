package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestSharedValueReturnRequiresOriginalEffectDomainAndCopyIdentity(t *testing.T) {
	for _, variant := range []string{"call", "bound receiver", "original conversion", "original cast", "same handler", "unknown handlers", "different handler", "nested argument outside", "missing call origin", "missing return origin", "wrong return origin", "missing branch origin", "missing descriptor", "missing result metadata", "dynamic call", "constructor", "opaque", "fake conversion", "allocation", "cyclic slot", "cyclic call", "deep conversion", "handler budget", "hidden", "normal successor", "encoded", "owned"} {
		t.Run(variant, func(t *testing.T) {
			root, external, tail := jumpTestNode("root"), jumpTestNode("external"), jumpTestNode("tail")
			branch := core.NewNode(&statements.ConditionStatement{})
			branch.OriginPC, branch.HasOriginPC = 10, true
			call := layerTestCall(15)
			call.Descriptor = "()J"
			call.FuncType = &types.JavaFuncType{ReturnType: types.NewJavaPrimer(types.JavaLong)}
			ret := &statements.ReturnStatement{JavaValue: call, OriginPC: 20, HasOriginPC: true}
			target := core.NewNode(ret)
			target.OriginPC, target.HasOriginPC = 20, true
			root.AddNext(branch)
			root.AddNext(external)
			branch.AddNext(target)
			branch.AddNext(tail)
			external.AddNext(target)
			m := NewRootStatementManager(root)
			m.SetOriginalExceptionTable(nil)
			want := variant == "call" || variant == "bound receiver" || variant == "original conversion" || variant == "original cast" || variant == "same handler"
			switch variant {
			case "bound receiver":
				call.IsStatic, call.Kind, call.Object = false, values.InvokeVirtual, layerTestRef("example.Owner")
			case "original conversion":
				ret.JavaValue = values.NewOriginalPrimitiveConversion(call, types.NewJavaPrimer(types.JavaInteger), 19)
			case "original cast":
				call.Descriptor = "()Ljava/lang/Object;"
				call.FuncType.ReturnType = types.NewJavaClass("java.lang.Object")
				ret.JavaValue = &values.CastExpression{Value: call, TargetType: types.NewJavaClass("java.lang.String"), OriginalCheckCast: true, OriginPC: 19}
			case "same handler", "different handler", "nested argument outside":
				m.SetOriginalExceptionTable([]*core.ExceptionTableEntry{{StartPc: 10, EndPc: 21, HandlerPc: 40, CatchType: 1}})
				if variant == "different handler" {
					call.OriginPC = 9
				} else if variant == "nested argument outside" {
					call.Descriptor = "(Ljava/lang/Object;)J"
					call.Arguments = []values.JavaValue{layerTestCall(9)}
				}
			case "unknown handlers":
				m.originalHandlersKnown = false
			case "missing call origin":
				call.HasOriginPC = false
			case "missing return origin":
				ret.HasOriginPC = false
			case "wrong return origin":
				ret.OriginPC++
			case "missing branch origin":
				branch.HasOriginPC = false
			case "missing descriptor":
				call.Descriptor = ""
			case "missing result metadata":
				ret.JavaValue = values.NewOriginalPrimitiveConversion(call, types.NewJavaPrimer(types.JavaInteger), 19)
				call.FuncType = nil
			case "dynamic call":
				call.Kind = values.InvokeDynamic
			case "constructor":
				call.FunctionName = "<init>"
			case "opaque":
				ret.JavaValue = values.NewCustomValue(nil, nil)
			case "fake conversion":
				ret.JavaValue = &values.CustomValue{Flag: "primitive_cast", CapturesKnown: true, Captures: []values.JavaValue{call}}
			case "allocation":
				ret.JavaValue = &values.NewExpression{}
			case "cyclic slot":
				v := values.NewSlotValue(call, types.NewJavaPrimer(types.JavaLong))
				v.ResetValue(v)
				ret.JavaValue = v
			case "cyclic call":
				call.Arguments = []values.JavaValue{call}
			case "deep conversion":
				for i := 0; i < 17; i++ {
					ret.JavaValue = values.NewOriginalPrimitiveConversion(ret.JavaValue, types.NewJavaPrimer(types.JavaInteger), 19)
				}
			case "handler budget":
				m.originalHandlers = make([]core.HandlerRange, 513)
			case "hidden":
				target.HideNext = tail
			case "normal successor":
				target.AddNext(tail)
			case "encoded":
				branch.EncodedJumps = map[*core.Node]bool{target: true}
			case "owned":
				root.RemoveNext(external)
				branch.AddNext(external)
			}
			m.DominatorMap = GenerateDominatorTree(root)
			splitSharedTerminalLeaves(m, branch)
			if (branch.Next[0] != target) != want {
				t.Fatalf("private terminal=%v want=%v", branch.Next[0] != target, want)
			}
			if external.Next[0] != target || branch.Next[1] != tail {
				t.Fatal("changed unrelated incoming edge or branch polarity")
			}
			if want {
				copy := branch.Next[0].Statement.(*statements.ReturnStatement)
				if copy == ret || copy.JavaValue != ret.JavaValue || !copy.HasOriginPC || copy.OriginPC != ret.OriginPC {
					t.Fatal("changed original returned evaluation or its origin")
				}
			}
		})
	}
}

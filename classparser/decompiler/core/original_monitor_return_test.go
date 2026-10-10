package core

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestOriginalMonitorReturnSnapshotRequiresUniqueReleaseEdge(t *testing.T) {
	for _, category := range []struct {
		name       string
		op         int
		typ        types.JavaType
		descriptor string
	}{
		{"reference", OP_ARETURN, types.NewJavaClass("Object"), "Ljava/lang/Object;"},
		{"boolean", OP_IRETURN, types.NewJavaPrimer(types.JavaBoolean), "Z"},
		{"byte", OP_IRETURN, types.NewJavaPrimer(types.JavaByte), "B"},
		{"short", OP_IRETURN, types.NewJavaPrimer(types.JavaShort), "S"},
		{"char", OP_IRETURN, types.NewJavaPrimer(types.JavaChar), "C"},
		{"int", OP_IRETURN, types.NewJavaPrimer(types.JavaInteger), "I"},
		{"long", OP_LRETURN, types.NewJavaPrimer(types.JavaLong), "J"},
		{"float", OP_FRETURN, types.NewJavaPrimer(types.JavaFloat), "F"},
		{"double", OP_DRETURN, types.NewJavaPrimer(types.JavaDouble), "D"},
	} {
		t.Run(category.name, func(t *testing.T) {
			for _, scenario := range []string{"field", "call", "missing method type", "missing result type", "wrong operand category", "wrong result category", "void result", "local", "literal", "opaque", "missing owner", "missing CFG", "missing simulation", "edited return", "two successors", "return join", "exception entry", "taken edge", "missing stack", "work cap", "memory cap", "canceled"} {
				t.Run(scenario, func(t *testing.T) {
					// JVM input: acquire; read typed holder.value; release; typed return; catch-all release.
					code := []byte{OP_ALOAD_0, OP_DUP, OP_ASTORE_1, OP_MONITORENTER, OP_ALOAD_2, OP_GETFIELD, 0, 1, OP_ALOAD_1, OP_MONITOREXIT, byte(category.op), OP_ASTORE_3, OP_ALOAD_1, OP_MONITOREXIT, OP_ALOAD_3, OP_ATHROW}
					d := NewDecompiler(code, nil)
					d.FunctionType = types.NewJavaFuncType("()"+category.descriptor, nil, category.typ.Copy())
					d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 4, EndPc: 10, HandlerPc: 11}, {StartPc: 11, EndPc: 14, HandlerPc: 11}}
					if e := d.ParseOpcode(); e != nil {
						t.Fatal(e)
					}
					g, e := d.buildSemanticCFG()
					if e != nil {
						t.Fatal(e)
					}
					d.semanticCFG = g
					exit, ret := d.OpcodeByPC(9), d.OpcodeByPC(10)
					owners := d.originalMonitorOwners()
					if owners[exit] != 3 {
						t.Fatal("original balanced monitor not proved")
					}
					holder := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("Holder"))
					var value values.JavaValue = &values.RefMember{Object: holder, Member: "value", JavaType: category.typ.Copy()}
					ret.stackConsumed = []values.JavaValue{value}
					d.opcodeToSimulateStack = map[*OpCode]*StackSimulationImpl{exit: NewStackSimulation(nil, map[int]*values.JavaRef{}, utils.NewRootVariableId())}
					switch scenario {
					case "call":
						value = &values.FunctionCallExpression{Object: holder, FunctionName: "read", ClassName: "Holder", Descriptor: "()" + category.descriptor, FuncType: types.NewJavaFuncType("read", nil, category.typ.Copy())}
						ret.stackConsumed[0] = value
					case "missing method type":
						d.FunctionType = nil
					case "missing result type":
						d.FunctionType.ReturnType = nil
					case "wrong operand category":
						if category.op == OP_ARETURN {
							value.(*values.RefMember).JavaType = types.NewJavaPrimer(types.JavaInteger)
						} else {
							value.(*values.RefMember).JavaType = types.NewJavaClass("Object")
						}
					case "wrong result category":
						if category.op == OP_ARETURN {
							d.FunctionType.ReturnType = types.NewJavaPrimer(types.JavaInteger)
						} else {
							d.FunctionType.ReturnType = types.NewJavaClass("Object")
						}
					case "void result":
						d.FunctionType.ReturnType = types.NewJavaPrimer(types.JavaVoid)
					case "local":
						ret.stackConsumed[0] = holder
					case "literal":
						ret.stackConsumed[0] = values.JavaNull
					case "opaque":
						ret.stackConsumed[0] = values.NewCustomValue(nil, func() types.JavaType { return types.NewJavaClass("Object") })
					case "missing owner":
						delete(owners, exit)
					case "missing CFG":
						d.semanticCFG = nil
					case "missing simulation":
						d.opcodeToSimulateStack = nil
					case "edited return":
						ret.Instr = InstrInfos[OP_RETURN]
					case "two successors":
						i := g.outgoing[exit][0]
						g.outgoing[exit] = append(g.outgoing[exit], i)
					case "return join":
						g.incoming[ret] = append(g.incoming[ret], g.incoming[ret][0])
					case "exception entry":
						g.Edges[g.incoming[ret][0]].Kind = EdgeException
					case "taken edge":
						g.Edges[g.incoming[ret][0]].Kind = EdgeTaken
					case "missing stack":
						ret.stackConsumed = nil
					case "work cap":
						d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
					case "memory cap":
						d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
					case "canceled":
						ctx, cancel := context.WithCancel(context.Background())
						cancel()
						d.Work = workbudget.New(ctx, workbudget.Limits{})
					}
					returns, snapshots, err := d.snapshotOriginalMonitorReturns(owners)
					accept := scenario == "field" || scenario == "call"
					if accept {
						if err != nil || len(returns) != 1 || len(snapshots) != 1 {
							t.Fatalf("original snapshot refused %v", err)
						}
						snapshot := snapshots[exit]
						if snapshot.Value != value || snapshot.Ref != returns[ret] || snapshot.OriginPC != 9 || ret.stackConsumed[0] != value || values.SameLocal(snapshot.Ref, holder) {
							t.Fatal("snapshot changed original operand/binding")
						}
					}
					if !accept && err == nil && (len(returns) != 0 || len(snapshots) != 0) {
						t.Fatal("unproved snapshot admitted")
					}
				})
			}
		})
	}
}
func TestOriginalMonitorSnapshotEmissionRequiresExactTwoNodePlan(t *testing.T) {
	for _, scenario := range []string{"owned", "no plan", "no owner", "changed receiver", "wrong owner", "wrong PC", "unknown release", "changed value", "changed ref", "nonlocal store", "nonfirst", "missing origin", "third node", "wrong primary", "edited opcode"} {
		t.Run(scenario, func(t *testing.T) {
			value := values.NewJavaLiteral("original", types.NewJavaClass("String"))
			ref := values.NewJavaRef(utils.NewRootVariableId(), value, value.Type())
			op := &OpCode{Instr: InstrInfos[OP_MONITOREXIT], CurrentOffset: 20}
			snapshot := EvaluationSnapshot{Ref: ref, Value: value, OriginPC: 20}
			snapshots := map[*OpCode]EvaluationSnapshot{op: snapshot}
			owners := map[*OpCode]int{op: 3}
			assign := statements.NewAssignStatement(ref, value, true)
			assign.HasOriginPC = true
			assign.OriginPC = 20
			capture := NewNode(assign)
			exit := statements.NewOriginalMonitorStatement("monitor_exit", nil, 20, 3)
			primary := NewNode(exit)
			chain := []*Node{capture, primary}
			switch scenario {
			case "no plan":
				snapshots = nil
			case "no owner":
				owners = nil
			case "changed receiver":
				exit.Data = values.JavaNull
			case "wrong owner":
				primary.Statement = statements.NewOriginalMonitorStatement("monitor_exit", nil, 20, 4)
			case "wrong PC":
				primary.Statement = statements.NewOriginalMonitorStatement("monitor_exit", nil, 21, 3)
			case "unknown release":
				primary.Statement = statements.NewMiddleStatement("monitor_exit", nil)
			case "changed value":
				assign.JavaValue = values.JavaNull
			case "changed ref":
				assign.LeftValue = values.NewJavaRef(utils.NewRootVariableId(), nil, ref.Type())
			case "nonlocal store":
				assign.LeftValue = &values.RefMember{Object: ref, Member: "value", JavaType: ref.Type()}
			case "nonfirst":
				assign.IsFirst = false
			case "missing origin":
				assign.HasOriginPC = false
			case "third node":
				chain = append([]*Node{NewNode(statements.NewMiddleStatement("start", nil))}, chain...)
			case "wrong primary":
				primary = capture
			case "edited opcode":
				op.Instr = InstrInfos[OP_NOP]
			}
			if originalMonitorSnapshotChain(chain, primary, op, owners, snapshots) != (scenario == "owned") {
				t.Fatal("incorrect monitor emission license")
			}
		})
	}
}

func TestOriginalMonitorReturnCategoryRejectsCrossCategoryAndUnknownTypes(t *testing.T) {
	cases := []struct {
		op  int
		typ types.JavaType
	}{
		{OP_IRETURN, types.NewJavaPrimer(types.JavaInteger)},
		{OP_IRETURN, types.NewJavaPrimer(types.JavaBoolean)},
		{OP_IRETURN, types.NewJavaPrimer(types.JavaChar)},
		{OP_LRETURN, types.NewJavaPrimer(types.JavaLong)},
		{OP_FRETURN, types.NewJavaPrimer(types.JavaFloat)},
		{OP_DRETURN, types.NewJavaPrimer(types.JavaDouble)},
		{OP_ARETURN, types.NewJavaClass("Object")},
		{OP_ARETURN, types.NewJavaArrayType(types.NewJavaPrimer(types.JavaLong))},
		{OP_ARETURN, types.NewParameterizedType("java.util.List", []types.JavaType{types.NewJavaClass("String")})},
		{-1, nil}, {-1, types.NewJavaPrimer(types.JavaVoid)}, {-1, types.NewJavaPrimer("unknown")},
	}
	for _, left := range cases {
		for _, right := range cases {
			for _, op := range []int{OP_IRETURN, OP_LRETURN, OP_FRETURN, OP_DRETURN, OP_ARETURN, OP_RETURN, OP_ATHROW} {
				want := left.op == op && right.op == op
				if originalMonitorReturnCategory(op, left.typ, right.typ) != want {
					t.Fatalf("opcode%d operand%T result%T allowed=%v", op, left.typ, right.typ, !want)
				}
			}
		}
	}
}

func TestOriginalMonitorNestedReturnSnapshotsBeforeAllReleases(t *testing.T) {
	for _, scenario := range []string{"nested", "unknown inner", "unknown outer", "joined load", "extra release edge", "exception load"} {
		t.Run(scenario, func(t *testing.T) {
			code := []byte{OP_ALOAD_0, OP_DUP, OP_ASTORE_1, OP_MONITORENTER, OP_ALOAD_2, OP_DUP, OP_ASTORE_3, OP_MONITORENTER, OP_ALOAD_0, OP_GETFIELD, 0, 1, OP_ALOAD_3, OP_MONITOREXIT, OP_ALOAD_1, OP_MONITOREXIT, OP_IRETURN, OP_ASTORE, 4, OP_ALOAD_3, OP_MONITOREXIT, OP_ALOAD, 4, OP_ATHROW, OP_ASTORE, 5, OP_ALOAD_1, OP_MONITOREXIT, OP_ALOAD, 5, OP_ATHROW}
			d := NewDecompiler(code, nil)
			d.FunctionType = types.NewJavaFuncType("()I", nil, types.NewJavaPrimer(types.JavaInteger))
			d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 8, EndPc: 14, HandlerPc: 17}, {StartPc: 17, EndPc: 21, HandlerPc: 17}, {StartPc: 4, EndPc: 16, HandlerPc: 24}, {StartPc: 17, EndPc: 24, HandlerPc: 24}, {StartPc: 24, EndPc: 28, HandlerPc: 24}}
			if err := d.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			g, err := d.buildSemanticCFG()
			if err != nil {
				t.Fatal(err)
			}
			d.semanticCFG = g
			inner, outer, ret, load := d.OpcodeByPC(13), d.OpcodeByPC(15), d.OpcodeByPC(16), d.OpcodeByPC(14)
			owners := d.originalMonitorOwners()
			if owners[inner] != 7 || owners[outer] != 3 {
				t.Fatalf("original nested owners not proved: %v", owners)
			}
			value := &values.RefMember{Object: values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("Holder")), Member: "number", JavaType: types.NewJavaPrimer(types.JavaInteger)}
			ret.stackConsumed = []values.JavaValue{value}
			d.opcodeToSimulateStack = map[*OpCode]*StackSimulationImpl{inner: NewStackSimulation(nil, map[int]*values.JavaRef{}, utils.NewRootVariableId()), outer: NewStackSimulation(nil, map[int]*values.JavaRef{}, utils.NewRootVariableId())}
			switch scenario {
			case "unknown inner":
				delete(owners, inner)
			case "unknown outer":
				delete(owners, outer)
			case "joined load":
				g.incoming[load] = append(g.incoming[load], g.incoming[load][0])
			case "extra release edge":
				g.outgoing[inner] = append(g.outgoing[inner], g.outgoing[inner][0])
			case "exception load":
				g.Edges[g.incoming[load][0]].Kind = EdgeException
			}
			returns, snapshots, err := d.snapshotOriginalMonitorReturns(owners)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "nested" {
				if len(returns) != 1 || len(snapshots) != 1 || snapshots[inner].Value != value || snapshots[outer].Ref != nil {
					t.Fatal("evaluation moved outside inner monitor")
				}
			} else if len(returns) != 0 || len(snapshots) != 0 {
				t.Fatal("unproved release corridor admitted")
			}
		})
	}
}

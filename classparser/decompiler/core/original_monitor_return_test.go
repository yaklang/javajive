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
	for _, scenario := range []string{"field", "call", "local", "literal", "opaque", "missing owner", "missing CFG", "missing simulation", "edited return", "two successors", "return join", "exception entry", "taken edge", "missing stack", "work cap", "memory cap", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			// JVM input: acquire; read holder.value; release; areturn; catch-all release.
			code := []byte{OP_ALOAD_0, OP_DUP, OP_ASTORE_1, OP_MONITORENTER, OP_ALOAD_2, OP_GETFIELD, 0, 1, OP_ALOAD_1, OP_MONITOREXIT, OP_ARETURN, OP_ASTORE_3, OP_ALOAD_1, OP_MONITOREXIT, OP_ALOAD_3, OP_ATHROW}
			d := NewDecompiler(code, nil)
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
			var value values.JavaValue = &values.RefMember{Object: holder, Member: "value", JavaType: types.NewJavaClass("Object")}
			ret.stackConsumed = []values.JavaValue{value}
			d.opcodeToSimulateStack = map[*OpCode]*StackSimulationImpl{exit: NewStackSimulation(nil, map[int]*values.JavaRef{}, utils.NewRootVariableId())}
			switch scenario {
			case "call":
				value = &values.FunctionCallExpression{Object: holder, FunctionName: "read", ClassName: "Holder", Descriptor: "()Ljava/lang/Object;", FuncType: types.NewJavaFuncType("read", nil, types.NewJavaClass("Object"))}
				ret.stackConsumed[0] = value
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

package core

import (
	"errors"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestByteArrayReadRequiresOriginalClosedReferenceDomain(t *testing.T) {
	for _, name := range []string{"parameter", "instance parameter", "wide parameter layout", "boolean parameter", "reference parameter", "nested byte array", "receiver", "missing descriptor", "malformed descriptor", "missing context", "copy", "seeded copy cycle", "unseeded copy cycle", "mixed unknown", "wrong store slot", "custom store", "malformed store", "missing definition", "field", "static field", "boolean field", "unsealed field", "stale field", "call", "boolean call", "stale call", "wrong call descriptor", "malformed call", "missing call symbol", "allocation", "boolean allocation", "stale allocation", "inferred array only", "missing producer", "foreign producer", "duplicate cell", "missing original", "missing decoded", "custom read", "wide read", "payload read", "missing array operand", "other array", "other index", "stale result", "missing result origin", "forward limit", "nil array", "nil result", "failed graph", "missing graph", "graph cap", "work cap", "allocation cap", "proof cap"} {
		t.Run(name, func(t *testing.T) {
			arrayType := types.NewJavaArrayType(types.NewJavaPrimer(types.JavaByte))
			integer := types.NewJavaPrimer(types.JavaInteger)
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, arrayType)
			cell := values.NewSlotValue(ref, arrayType)
			index := values.NewJavaLiteral(0, integer)
			load, read := op(OP_ALOAD_0, 0), op(OP_BALOAD, 3)
			load.stackProduced = []values.JavaValue{cell}
			x := values.NewJavaArrayMember(cell, index)
			x.OriginPC = 3
			x.HasOriginPC = true
			read.stackConsumed = []values.JavaValue{index, cell}
			read.stackProduced = []values.JavaValue{x}
			facts := &slotDefinitions{in: map[*OpCode]definitionSet{load: {nil: {}}}}
			g := &SemanticCFG{Nodes: []*OpCode{load, read}, order: map[*OpCode]int{load: 0, read: 1}, slots: map[int]*slotDefinitions{0: facts}}
			d := &Decompiler{opCodes: []*OpCode{load, read}, semanticCFG: g, FunctionContext: &class_context.ClassContext{IsStatic: true, CurrentMethodDesc: "([B)V"}, stackValueProducers: map[values.JavaValue]*OpCode{cell: load}, invokeFuncCall: map[*OpCode]*values.FunctionCallExpression{}}
			add := func(o *OpCode) {
				d.opCodes = append(d.opCodes, o)
				g.order[o] = len(g.Nodes)
				g.Nodes = append(g.Nodes, o)
				if len(o.stackProduced) == 1 {
					d.stackValueProducers[o.stackProduced[0]] = o
				}
			}
			want := name == "parameter"
			switch name {
			case "instance parameter", "wide parameter layout":
				load.Instr = op(OP_ALOAD, 0).Instr
				slot := 1
				d.FunctionContext.IsStatic = false
				if name == "wide parameter layout" {
					slot = 4
					d.FunctionContext.IsStatic = true
					d.FunctionContext.CurrentMethodDesc = "(JD[B)V"
				}
				load.Data = []byte{byte(slot)}
				g.slots[slot] = facts
				want = true
			case "boolean parameter":
				d.FunctionContext.CurrentMethodDesc = "([Z)V"
			case "reference parameter":
				d.FunctionContext.CurrentMethodDesc = "(Ljava/lang/Object;)V"
			case "nested byte array":
				d.FunctionContext.CurrentMethodDesc = "([[B)V"
			case "receiver":
				d.FunctionContext.IsStatic = false
			case "missing descriptor":
				d.FunctionContext.CurrentMethodDesc = ""
			case "malformed descriptor":
				d.FunctionContext.CurrentMethodDesc = "([B)Vtrailing"
			case "missing context":
				d.FunctionContext = nil
			case "copy", "seeded copy cycle", "unseeded copy cycle", "mixed unknown", "wrong store slot", "custom store", "malformed store", "missing definition":
				entry, store := op(OP_ALOAD_1, 4), op(OP_ASTORE_0, 5)
				v := values.NewSlotValue(ref, arrayType)
				entry.stackProduced = []values.JavaValue{v}
				store.stackConsumed = []values.JavaValue{v}
				add(entry)
				add(store)
				d.FunctionContext.CurrentMethodDesc = "(I[B)V"
				g.slots[1] = &slotDefinitions{in: map[*OpCode]definitionSet{entry: {nil: {}}}}
				facts.in[load] = definitionSet{store: {}}
				want = name == "copy" || name == "seeded copy cycle"
				if name == "seeded copy cycle" || name == "unseeded copy cycle" {
					store.stackConsumed[0] = cell
					if name == "seeded copy cycle" {
						extra := op(OP_ASTORE_0, 6)
						extra.stackConsumed = []values.JavaValue{v}
						add(extra)
						facts.in[load][extra] = struct{}{}
					}
				}
				if name == "mixed unknown" {
					extra := op(OP_ASTORE_0, 6)
					extra.stackConsumed = []values.JavaValue{ref}
					add(extra)
					facts.in[load][extra] = struct{}{}
				}
				if name == "wrong store slot" {
					store.Instr = op(OP_ASTORE_2, 5).Instr
				}
				if name == "custom store" {
					store.IsCustom = true
				}
				if name == "malformed store" {
					store.Data = []byte{0}
				}
				if name == "missing definition" {
					facts.in[load] = definitionSet{}
				}
			case "field", "static field", "boolean field", "unsealed field", "stale field":
				member := &values.JavaClassMember{Name: "sample/Owner", Member: "payload", Description: "[B", JavaType: arrayType}
				if name == "boolean field" {
					member.Description = "[Z"
				}
				load.Instr = op(OP_GETFIELD, 0).Instr
				load.Data = []byte{0, 1}
				load.stackConsumed = []values.JavaValue{ref}
				field := values.NewRefMember(ref, "payload", arrayType)
				field.HasOriginPC = true
				field.OriginPC = 0
				if name != "unsealed field" {
					field.MarkOriginalFieldRead(member, 0)
				}
				if name == "stale field" {
					field.Member = "other"
				}
				cell.ResetValue(field)
				if name == "static field" {
					load.Instr = op(OP_GETSTATIC, 0).Instr
					load.stackConsumed = nil
					f := *member
					f.HasOriginPC = true
					f.MarkOriginalFieldRead(member, 0)
					cell.ResetValue(&f)
				}
				d.constantPoolGetter = func(int) values.JavaValue { return member }
				want = name == "field" || name == "static field"
			case "call", "boolean call", "stale call", "wrong call descriptor", "malformed call", "missing call symbol":
				load.Instr = op(OP_INVOKESTATIC, 0).Instr
				load.Data = []byte{0, 1}
				call := &values.FunctionCallExpression{Descriptor: "()[B", FuncType: &types.JavaFuncType{ReturnType: arrayType}, HasOriginPC: true, OriginPC: 0}
				if name == "boolean call" {
					call.Descriptor = "()[Z"
				}
				if name == "stale call" {
					call.OriginPC = 1
				}
				cell.ResetValue(call)
				symbol := &values.JavaClassMember{Description: call.Descriptor}
				d.constantPoolGetter = func(int) values.JavaValue { return symbol }
				if name == "wrong call descriptor" {
					symbol.Description = "()[Z"
				}
				if name == "malformed call" {
					load.Data = nil
				}
				if name == "missing call symbol" {
					d.constantPoolGetter = nil
				}
				d.invokeFuncCall[load] = call
				want = name == "call"
			case "allocation", "boolean allocation", "stale allocation":
				load.Instr = op(OP_NEWARRAY, 0).Instr
				load.Data = []byte{8}
				load.stackConsumed = []values.JavaValue{index}
				array := values.NewNewArrayExpression(arrayType, index)
				array.HasOriginPC = true
				array.OriginPC = 0
				cell.ResetValue(array)
				if name == "boolean allocation" {
					load.Data[0] = 4
				}
				if name == "stale allocation" {
					array.OriginPC = 1
				}
				want = name == "allocation"
			case "inferred array only":
				load.Instr = op(OP_NOP, 0).Instr
			case "missing producer":
				delete(d.stackValueProducers, cell)
			case "foreign producer":
				d.stackValueProducers[cell] = op(OP_ALOAD_0, 10)
			case "duplicate cell":
				o := op(OP_ALOAD_1, 8)
				o.stackProduced = []values.JavaValue{cell}
				add(o)
			case "missing original":
				delete(g.order, load)
				g.Nodes = g.Nodes[1:]
			case "missing decoded":
				d.opCodes = d.opCodes[1:]
			case "custom read":
				read.IsCustom = true
			case "wide read":
				read.IsWide = true
			case "payload read":
				read.Data = []byte{1}
			case "missing array operand":
				read.stackConsumed = read.stackConsumed[:1]
			case "other array":
				x.Object = ref
			case "other index":
				x.Index = values.NewJavaLiteral(1, integer)
			case "stale result":
				x.OriginPC = 4
			case "missing result origin":
				x.HasOriginPC = false
			case "forward limit":
				for i := 0; i < 33; i++ {
					x.Object = values.NewStackLifetimeUseView(x.Object)
				}
				read.stackConsumed[1] = x.Object
			case "nil array":
				read.stackConsumed[1] = nil
				x.Object = nil
			case "nil result":
				read.stackProduced[0] = (*values.JavaArrayMember)(nil)
			case "failed graph":
				g.Err = errors.New("invalid graph")
			case "missing graph":
				d.semanticCFG = nil
			case "graph cap":
				d.opCodes = make([]*OpCode, 8193)
			case "work cap":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "allocation cap":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			}
			p := d.byteArrayProvenance()
			if name == "proof cap" && p != nil {
				p.remaining = 0
			}
			got := p.numericRead(read)
			if (got != nil) != want {
				t.Fatalf("proved=%v want=%v", got != nil, want)
			}
			if got != nil && got != x {
				t.Fatal("substituted array read")
			}
		})
	}
}

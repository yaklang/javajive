package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNumericStackDuplicateRequiresSealedProducer(t *testing.T) {
	for _, change := range []string{"proved", "transitive", "inferred type only", "missing witness", "changed seed", "wrong pc", "wrong opcode", "wide", "operand bytes", "custom", "missing original", "missing decoded", "unknown cell", "different copy", "different ref", "missing conversion", "wrong conversion", "not a declaration", "partial packet", "missing graph", "budget", "cycle"} {
		t.Run(change, func(t *testing.T) {
			typ := types.NewJavaPrimer(types.JavaInteger)
			seed := values.NewJavaLiteral(19, typ)
			duplicate := op(OP_DUP, 4)
			ref := values.NewJavaRef(utils.NewRootVariableId(), seed, typ)
			ref.MarkOriginalStackMaterialization(4, OP_DUP, seed)
			cell := values.NewSlotValue(ref, typ)
			duplicate.stackConsumed, duplicate.stackProduced = []values.JavaValue{seed}, []values.JavaValue{cell, cell}
			d := &Decompiler{opCodes: []*OpCode{duplicate}, semanticCFG: &SemanticCFG{Nodes: []*OpCode{duplicate}}, opcodeIdToRef: map[*OpCode][][2]any{duplicate: {{ref, true}}}, dupConvertedRefValue: map[*OpCode][]values.JavaValue{duplicate: {seed}}, stackValueProducers: map[values.JavaValue]*OpCode{cell: duplicate}}
			roots := map[values.JavaValue]bool{seed: true}
			target := ref
			switch change {
			case "transitive":
				next := op(OP_DUP, 8)
				target = values.NewJavaRef(utils.NewRootVariableId(), cell, typ)
				target.MarkOriginalStackMaterialization(8, OP_DUP, cell)
				out := values.NewSlotValue(target, typ)
				next.stackConsumed, next.stackProduced = []values.JavaValue{cell}, []values.JavaValue{out, out}
				d.opCodes = append([]*OpCode{next}, d.opCodes...) // Not dependent on traversal order.
				d.semanticCFG.Nodes = append(d.semanticCFG.Nodes, next)
				d.opcodeIdToRef[next], d.dupConvertedRefValue[next], d.stackValueProducers[out] = [][2]any{{target, true}}, []values.JavaValue{cell}, next
			case "inferred type only":
				delete(roots, seed)
			case "missing witness":
				ref = values.NewJavaRef(ref.Id, seed, typ)
				target = ref
				cell.ResetValue(ref)
				d.opcodeIdToRef[duplicate][0][0] = ref
			case "changed seed":
				ref.Val = values.NewJavaLiteral(19, typ)
			case "wrong pc":
				duplicate.CurrentOffset++
			case "wrong opcode":
				duplicate.Instr = op(OP_DUP_X1, 4).Instr
			case "wide":
				duplicate.IsWide = true
			case "operand bytes":
				duplicate.Data = []byte{0}
			case "custom":
				duplicate.IsCustom = true
			case "missing original":
				d.semanticCFG.Nodes = nil
			case "missing decoded":
				d.opCodes = nil
			case "unknown cell":
				delete(d.stackValueProducers, cell)
			case "different copy":
				duplicate.stackProduced[1] = values.NewSlotValue(ref, typ)
			case "different ref":
				d.opcodeIdToRef[duplicate][0][0] = values.NewJavaRef(ref.Id, seed, typ)
			case "missing conversion":
				delete(d.dupConvertedRefValue, duplicate)
			case "wrong conversion":
				d.dupConvertedRefValue[duplicate][0] = values.NewJavaLiteral(19, typ)
			case "not a declaration":
				d.opcodeIdToRef[duplicate][0][1] = false
			case "partial packet":
				duplicate.stackConsumed = nil
			case "missing graph":
				d.semanticCFG = nil
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "cycle":
				ref.Val = cell
				duplicate.stackConsumed[0] = cell
				d.dupConvertedRefValue[duplicate][0] = cell
				delete(roots, seed)
			}
			got := d.extendNumericStackProducers(roots)[target]
			if want := change == "proved" || change == "transitive"; got != want {
				t.Fatalf("numeric=%v want=%v", got, want)
			}
		})
	}
}

func TestNumericNegationRequiresExactOriginalOperand(t *testing.T) {
	for _, change := range []string{"proved", "consumer view", "wrong opcode", "custom", "wide", "data", "wrong operator", "binary", "missing operand", "changed operand", "missing result", "wrong type"} {
		t.Run(change, func(t *testing.T) {
			typ := types.NewJavaPrimer(types.JavaInteger)
			value := values.NewJavaLiteral(-2147483648, typ)
			x := values.NewUnaryExpression(value, values.SUB, typ)
			n := op(OP_INEG, 3)
			n.stackConsumed, n.stackProduced = []values.JavaValue{value}, []values.JavaValue{x}
			switch change {
			case "consumer view":
				x.Values[0] = values.NewStackLifetimeUseView(value)
			case "wrong opcode":
				n.Instr = op(OP_LNEG, 3).Instr
			case "custom":
				n.IsCustom = true
			case "wide":
				n.IsWide = true
			case "data":
				n.Data = []byte{0}
			case "wrong operator":
				x.Op = values.ADD
			case "binary":
				x.Values = append(x.Values, value)
			case "missing operand":
				n.stackConsumed = nil
			case "changed operand":
				n.stackConsumed[0] = values.NewJavaLiteral(-2147483648, typ)
			case "missing result":
				n.stackProduced = nil
			case "wrong type":
				x.Typ = types.NewJavaPrimer(types.JavaLong)
			}
			_, got := originalIntNegationResult(n)
			if want := change == "proved" || change == "consumer view"; got != want {
				t.Fatalf("numeric=%v want=%v", got, want)
			}
		})
	}
}

func TestNumericFieldPostResultRequiresOriginalReadUpdateWrite(t *testing.T) {
	for _, static := range []bool{false, true} {
		for _, decrement := range []bool{false, true} {
			for _, change := range []string{"proved", "no fold", "wrong descriptor", "other field", "other receiver", "wrong read", "missing read witness", "missing duplicate witness", "stale duplicate", "unknown arithmetic", "wrong arithmetic", "wrong delta", "different delta", "other retained word", "missing original", "custom write", "wide write", "malformed write", "partial write", "copied result", "missing conversion", "partial duplicate", "foreign duplicate cell", "duplicate receiver", "unknown read", "read payload", "unknown write"} {
				name := change
				if static {
					name += "/static"
				}
				if decrement {
					name += "/decrement"
				}
				t.Run(name, func(t *testing.T) {
					typ := types.NewJavaPrimer(types.JavaInteger)
					member := &values.JavaClassMember{Name: "Counter", Member: "word", Description: "I", JavaType: typ}
					readCode, writeCode, dupCode := OP_GETFIELD, OP_PUTFIELD, OP_DUP_X1
					if static {
						readCode, writeCode, dupCode = OP_GETSTATIC, OP_PUTSTATIC, OP_DUP
					}
					read, duplicate, arithmetic, write := op(readCode, 1), op(dupCode, 4), op(OP_IADD, 6), op(writeCode, 7)
					operator, post := values.ADD, values.INC
					if decrement {
						arithmetic.Instr = op(OP_ISUB, 6).Instr
						operator, post = values.SUB, values.DEC
					}
					read.Data, write.Data = []byte{0, 1}, []byte{0, 1}
					write.SelfOpFolded = true
					receiver := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("Counter"))
					var field values.JavaValue
					if static {
						f := *member
						f.OriginPC, f.HasOriginPC = 1, true
						if change != "missing read witness" {
							f.MarkOriginalFieldRead(member, 1)
						}
						field = &f
					} else {
						f := values.NewRefMember(receiver, member.Member, typ)
						f.OriginPC, f.HasOriginPC = 1, true
						if change != "missing read witness" {
							f.MarkOriginalFieldRead(member, 1)
						}
						field = f
						read.stackConsumed = []values.JavaValue{receiver}
					}
					read.stackProduced = []values.JavaValue{field}
					ref := values.NewJavaRef(utils.NewRootVariableId(), field, typ)
					if change != "missing duplicate witness" {
						ref.MarkOriginalStackMaterialization(4, dupCode, field)
					}
					old := values.NewSlotValue(ref, typ)
					one := values.NewJavaLiteral(1, typ)
					rhs := values.NewBinaryExpression(old, one, operator, typ)
					x := values.NewBinaryExpression(field, one, post, typ)
					arithmetic.stackConsumed, arithmetic.stackProduced = []values.JavaValue{one, old}, []values.JavaValue{rhs}
					write.stackConsumed, write.stackProduced = []values.JavaValue{rhs, old}, []values.JavaValue{x}
					duplicate.stackConsumed, duplicate.stackProduced = []values.JavaValue{field}, []values.JavaValue{old, old}
					if !static {
						write.stackConsumed = []values.JavaValue{rhs, receiver, old}
						duplicate.stackConsumed = []values.JavaValue{field, old, receiver}
						duplicate.stackProduced = []values.JavaValue{old, old, receiver, old}
					}
					d := &Decompiler{opCodes: []*OpCode{read, duplicate, arithmetic, write}, opcodeIdToRef: map[*OpCode][][2]any{duplicate: {{ref, true}}}, dupConvertedRefValue: map[*OpCode][]values.JavaValue{duplicate: {field}}, stackValueProducers: map[values.JavaValue]*OpCode{field: read, old: duplicate, rhs: arithmetic}, constantPoolGetter: func(int) values.JavaValue { return member }}
					d.semanticCFG = &SemanticCFG{Nodes: append([]*OpCode{}, d.opCodes...)}
					switch change {
					case "no fold":
						write.SelfOpFolded = false
					case "wrong descriptor":
						member.Description = "Z"
					case "other field":
						member.Member = "other"
					case "other receiver":
						if !static {
							write.stackConsumed[1] = values.NewJavaRef(utils.NewRootVariableId(), nil, receiver.Type())
						} else {
							member.Name = "Other"
						}
					case "wrong read":
						read.Instr = op(OP_ILOAD_1, 1).Instr
					case "stale duplicate":
						ref.Val = one
					case "unknown arithmetic":
						delete(d.stackValueProducers, rhs)
					case "wrong arithmetic":
						arithmetic.Instr = op(OP_IOR, 6).Instr
					case "wrong delta":
						one.Data = 2
					case "different delta":
						x.Values[1] = values.NewJavaLiteral(1, typ)
					case "other retained word":
						write.stackConsumed[len(write.stackConsumed)-1] = one
					case "missing original":
						d.semanticCFG.Nodes = d.semanticCFG.Nodes[1:]
					case "custom write":
						write.IsCustom = true
					case "wide write":
						write.IsWide = true
					case "malformed write":
						write.Data = []byte{1}
					case "partial write":
						write.stackConsumed = nil
					case "copied result":
						write.stackProduced[0] = values.NewBinaryExpression(field, one, post, typ)
					case "missing conversion":
						delete(d.dupConvertedRefValue, duplicate)
					case "partial duplicate":
						duplicate.stackConsumed = nil
					case "foreign duplicate cell":
						duplicate.stackProduced[0] = values.NewSlotValue(ref, typ)
					case "duplicate receiver":
						if !static {
							duplicate.stackConsumed[2] = one
						} else {
							duplicate.stackProduced[1] = one
						}
					case "unknown read":
						delete(d.stackValueProducers, field)
					case "read payload":
						read.Data = nil
					case "unknown write":
						d.opCodes = d.opCodes[:3]
					}
					got := d.extendNumericStackProducers(map[values.JavaValue]bool{})[x]
					if want := change == "proved"; got != want {
						t.Fatalf("numeric=%v want=%v", got, want)
					}
				})
			}
		}
	}
}

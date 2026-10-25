package core

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/fieldbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestInitializerFinalJoinRequiresCompleteTerminalOwnStores(t *testing.T) {
	for _, desc := range []string{"Z", "B", "C", "S", "I", "J", "F", "D"} {
		for _, variant := range []string{"control", "ordinary method", "foreign field", "not final", "volatile", "incomplete declaration", "duplicate declaration", "missing arm", "finally", "missing handlers", "effect after write", "nested control", "missing origin", "wrong origin", "duplicate origin", "additional write", "wrong CP member", "missing physical proof", "physical identity mismatch", "work", "canceled"} {
			t.Run(desc+"/"+variant, func(t *testing.T) {
				typ, _ := types.ParseDescriptor(desc)
				field := values.NewJavaClassMember("proof.Owner", "value", desc, typ)
				decl := fieldbinding.Class{Name: "proof/Owner", Complete: true, Fields: []fieldbinding.Field{{Name: "value", Descriptor: desc, Access: 0x18}}}
				ctx := &class_context.ClassContext{ClassName: "proof.Owner", FunctionName: "<clinit>", IsStatic: true, OriginalFieldDeclarations: func(string) (fieldbinding.Class, bool) { return decl, true }}
				d := &Decompiler{FunctionContext: ctx, constantPoolGetter: func(int) values.JavaValue { return field }, offsetToOpcodeIndex: map[uint16]int{0: 0, 3: 1}}
				d.opCodes = []*OpCode{{Instr: &Instruction{OpCode: OP_PUTSTATIC}, CurrentOffset: 0, Data: []byte{0, 1}}, {Instr: &Instruction{OpCode: OP_PUTSTATIC}, CurrentOffset: 3, Data: []byte{0, 1}}}
				d.opcodeToSimulateStack = map[*OpCode]*StackSimulationImpl{d.opCodes[0]: nil, d.opCodes[1]: nil}
				d.OriginalOwnStaticFinalStore = func(pc int, owner, name, descriptor string) bool {
					return (pc == 0 || pc == 3) && owner == "proof/Owner" && name == "value" && descriptor == desc
				}
				a := &statements.AssignStatement{LeftValue: field, JavaValue: values.NewJavaLiteral(1, typ), HasOriginPC: true, OriginPC: 0}
				b := &statements.AssignStatement{LeftValue: field, JavaValue: values.NewJavaLiteral(2, typ), HasOriginPC: true, OriginPC: 3}
				tr := &statements.TryCatchStatement{TryBody: []statements.Statement{a}, CatchBodies: [][]statements.Statement{{b}}, Exception: []*values.JavaRef{values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Exception"))}, Handlers: []statements.CatchHandler{{EntryPC: 3, ProtectedRanges: [][2]int{{0, 3}}}}}
				body := []statements.Statement{tr}
				switch variant {
				case "ordinary method":
					ctx.FunctionName = "execute"
				case "foreign field":
					field.Name = "proof.Other"
				case "not final":
					decl.Fields[0].Access = 8
				case "volatile":
					decl.Fields[0].Access |= 0x40
				case "incomplete declaration":
					decl.Complete = false
				case "duplicate declaration":
					decl.Fields = append(decl.Fields, decl.Fields[0])
				case "missing arm":
					tr.CatchBodies[0] = nil
				case "finally":
					tr.Handlers[0].CatchAll = true
				case "missing handlers":
					tr.Handlers = nil
				case "effect after write":
					tr.TryBody = append(tr.TryBody, statements.NewExpressionStatement(values.NewJavaLiteral(0, typ)))
				case "nested control":
					tr.TryBody = append([]statements.Statement{&statements.IfStatement{}}, tr.TryBody...)
				case "missing origin":
					a.HasOriginPC = false
				case "wrong origin":
					a.OriginPC = 1
				case "duplicate origin":
					b.OriginPC = 0
				case "additional write":
					d.opCodes = append(d.opCodes, &OpCode{Instr: &Instruction{OpCode: OP_PUTSTATIC}, CurrentOffset: 6, Data: []byte{0, 1}})
				case "wrong CP member":
					d.constantPoolGetter = func(int) values.JavaValue { return values.NewJavaClassMember(field.Name, "other", desc, typ) }
				case "missing physical proof":
					d.OriginalOwnStaticFinalStore = nil
				case "physical identity mismatch":
					d.OriginalOwnStaticFinalStore = func(int, string, string, string) bool { return false }
				case "work":
					d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
				case "canceled":
					c, cancel := context.WithCancel(context.Background())
					cancel()
					d.Work = workbudget.New(c, workbudget.Limits{})
				}
				d.JoinInitializerFinalStores(&body)
				if variant != "control" {
					if len(body) != 1 || a.LeftValue != field || b.LeftValue != field {
						t.Fatal("unproved store motion")
					}
					return
				}
				if len(body) != 3 || body[1] != tr {
					t.Fatal("missing joined initializer")
				}
				local, ok := a.LeftValue.(*values.JavaRef)
				if !ok || b.LeftValue != local || body[0].(*statements.AssignStatement).LeftValue != local || body[2].(*statements.AssignStatement).JavaValue != local || body[2].(*statements.AssignStatement).LeftValue != field {
					t.Fatal("local identity / field lost")
				}
				d.JoinInitializerFinalStores(&body)
				if len(body) != 3 {
					t.Fatal("not idempotent")
				}
			})
		}
	}
}

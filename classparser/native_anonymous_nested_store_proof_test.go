package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	u "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeAnonymousInitializerEmbeddedStoreRequiresOriginalOwnTarget(t *testing.T) {
	files := nativeCompileClasses(t, nativeAnonymousRepeatedStores)
	for _, change := range []string{"original", "synthetic", "missing origin", "other origin", "foreign owner", "wrong name", "wrong descriptor", "replaced RHS", "wrong target type", "foreign receiver", "custom receiver", "stack receiver", "budget"} {
		t.Run(change, func(t *testing.T) {
			obj, e := Parse(files["RepeatedInitOwner$1.class"])
			if e != nil {
				t.Fatal(e)
			}
			child := nativeAnonymousConstructor(obj, "RepeatedInitOwner", "make", nil)
			if child == nil || child.expressionInitializer == nil {
				t.Fatal("original repeated store proof")
			}
			plan := child.expressionInitializer
			pc := -1
			var member *values.JavaClassMember
			for _, op := range plan.ops[plan.start:] {
				m := constructorMotionMember(obj, op, core.OP_PUTFIELD)
				if m != nil && m.Member == "stage" {
					pc = int(op.CurrentOffset)
					member = m
					break
				}
			}
			if member == nil {
				t.Fatal("original stage store")
			}
			copyMember := *member
			member = &copyMember
			intType := types.NewJavaPrimer(types.JavaInteger)
			receiver := &values.JavaRef{IsThis: true}
			target := values.NewRefMember(receiver, "stage", intType)
			value := values.NewJavaLiteral(17, intType)
			var work *workbudget.Budget
			switch change {
			case "foreign owner":
				member.Name = "Other"
			case "wrong name":
				member.Member = "saved"
			case "wrong descriptor":
				member.Description = "J"
			case "wrong target type":
				target.JavaType = types.NewJavaPrimer(types.JavaLong)
			case "foreign receiver":
				receiver.IsThis = false
			case "custom receiver":
				receiver.CustomValue = &values.CustomValue{}
			case "stack receiver":
				receiver.StackVar = value
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
				work.Charge(workbudget.CounterGraphScans, 1)
			}
			render := func(*class_context.ClassContext) string { return "this.stage = 17" }
			assignment := values.NewOriginalFieldAssignmentExpression(target, value, member, pc, render)
			switch change {
			case "synthetic":
				assignment = values.NewAssignmentExpression(target, value, pc, render)
			case "missing origin":
				assignment.HasOriginPC = false
			case "other origin":
				assignment.OriginPC++
			case "replaced RHS":
				assignment.Value = values.NewJavaLiteral(18, intType)
			}
			events := []int{}
			got := nativeAnonymousInitializerExpressionEvents(child, plan, assignment, &events, nil, work)
			if got != (change == "original") {
				t.Fatalf("accepted=%v", got)
			}
			if got && (len(events) != 1 || events[0] != pc) {
				t.Fatalf("write became extra read or lost original store: %v", events)
			}
		})
	}
}
func TestNativeAnonymousInitializerMaterializationRequiresActualDupDefinition(t *testing.T) {
	for _, change := range []string{"original", "wrong PC", "wrong kind", "local store", "operand bytes", "custom opcode", "missing opcode", "not first", "declaration only", "other left", "no origin", "parameter", "budget"} {
		t.Run(change, func(t *testing.T) {
			typ := types.NewJavaPrimer(types.JavaInteger)
			value := values.NewJavaLiteral(17, typ)
			ref := values.NewJavaRef(u.NewRootVariableId().Next(), value, typ)
			ref.MarkOriginalStackMaterialization(12, core.OP_DUP_X1, value)
			assign := statements.NewAssignStatement(ref, value, true)
			assign.OriginPC, assign.HasOriginPC = 12, true
			op := &core.OpCode{Instr: &core.Instruction{OpCode: core.OP_DUP_X1}, CurrentOffset: 12}
			plan := &nativeAnonymousExpressionInitializer{byPC: map[int]*core.OpCode{12: op}}
			var work *workbudget.Budget
			switch change {
			case "wrong PC":
				assign.OriginPC = 13
			case "wrong kind":
				op.Instr.OpCode = core.OP_DUP
			case "local store":
				op.Instr.OpCode = core.OP_ISTORE_1
			case "operand bytes":
				op.Data = []byte{0}
			case "custom opcode":
				op.IsCustom = true
			case "missing opcode":
				plan.byPC = nil
			case "not first":
				assign.IsFirst = false
			case "declaration only":
				assign.IsDeclare = true
			case "other left":
				assign.LeftValue = values.NewJavaRef(ref.Id, value, typ)
			case "no origin":
				assign.HasOriginPC = false
			case "parameter":
				ref.IsParam = true
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
				work.Charge(workbudget.CounterGraphScans, 1)
			}
			if got := nativeAnonymousInitializerStackMaterialization(plan, assign, ref, work); got != (change == "original") {
				t.Fatalf("accepted=%v", got)
			}
		})
	}
}
func TestNativeAnonymousInitializerMaterializedReadClosesLogicalIdentity(t *testing.T) {
	for _, change := range []string{"original", "shallow clone", "synthetic", "same name different ID", "different UID", "empty UID", "changed value", "parameter", "different original PC", "different original kind", "missing definition", "rebound read type", "rebound declaration type"} {
		t.Run(change, func(t *testing.T) {
			typ := types.NewJavaPrimer(types.JavaInteger)
			value := values.NewJavaLiteral(17, typ)
			original := values.NewJavaRef(u.NewRootVariableId().Next(), value, typ)
			original.MarkOriginalStackMaterialization(12, core.OP_DUP_X1, value)
			copy := *original
			ref := &copy
			switch change {
			case "original":
				ref = original
			case "synthetic":
				ref = values.NewJavaRef(original.Id, value, typ)
				ref.VarUid = original.VarUid
			case "same name different ID":
				ref.Id = u.NewRootVariableId().Next()
			case "different UID":
				ref.VarUid = "different"
			case "empty UID":
				ref.VarUid = ""
			case "changed value":
				ref.Val = values.NewJavaLiteral(18, typ)
			case "parameter":
				ref.IsParam = true
			case "different original PC", "different original kind":
				ref = values.NewJavaRef(original.Id, value, typ)
				ref.VarUid = original.VarUid
				pc, kind := 12, core.OP_DUP_X1
				if change == "different original PC" {
					pc++
				} else {
					kind = core.OP_DUP
				}
				ref.MarkOriginalStackMaterialization(pc, kind, value)
			case "missing definition":
				original = nil
			case "rebound read type":
				ref.ResetVarType(types.NewJavaPrimer(types.JavaLong))
			case "rebound declaration type":
				ref.WebDeclType = types.NewJavaPrimer(types.JavaLong)
			}
			if got := nativeAnonymousInitializerMaterializedRead(ref, original); got != (change == "original" || change == "shallow clone") {
				t.Fatalf("accepted=%v", got)
			}
		})
	}
}
func TestNativeAnonymousInitializerLocalSymbolsCloseCaptureClassAndFormalNames(t *testing.T) {
	files := nativeCompileClasses(t, nativeAnonymousRepeatedStores)
	obj, e := Parse(files["RepeatedInitOwner$1.class"])
	if e != nil {
		t.Fatal(e)
	}
	ctx := &class_context.ClassContext{ClassSig: "<$jdec$stack4:Ljava/lang/Object;>Ljava/lang/Object;"}
	bindings := map[string]string{"val$value": "$jdec$stack1", "val$qualified": "$jdec$stack5.item", "val$parenthesized": "($jdec$stack6)"}
	fields := map[string]bool{"$jdec$stack3": true}
	obj.ConstantPoolManager.AddNewClassInfo("$jdec$stack2")
	names, known := nativeAnonymousInitializerReservedLocalNames(obj, ctx, bindings, fields, nil)
	if !known {
		t.Fatal("complete scope")
	}
	for _, name := range []string{"$jdec$stack1", "$jdec$stack2", "$jdec$stack3", "$jdec$stack4", "$jdec$stack5", "$jdec$stack6"} {
		if !names[name] {
			t.Fatalf("unreserved %s", name)
		}
	}
	for _, change := range []string{"budget", "cancelled", "malformed signature"} {
		t.Run(change, func(t *testing.T) {
			var work *workbudget.Budget
			copy := *ctx
			if change == "budget" {
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
				work.Charge(workbudget.CounterGraphScans, 1)
			}
			if change == "cancelled" {
				c, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(c, workbudget.Limits{})
			}
			if change == "malformed signature" {
				copy.CurrentMethodSig = "("
			}
			if _, known := nativeAnonymousInitializerReservedLocalNames(obj, &copy, bindings, fields, work); known {
				t.Fatal("unclosed scope budget/signature")
			}
		})
	}
}

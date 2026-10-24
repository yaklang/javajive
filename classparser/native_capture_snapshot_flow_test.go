package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeCaptureSnapshotRequiresAssignedWordAndOriginalAllocation(t *testing.T) {
	for _, mode := range []string{"loop writes", "conditional writes", "sequential writes", "standalone increment", "wrapped decrement", "postfix update marker", "boolean increment", "reference increment", "cross-category update", "nonunit update marker", "noncomparable update marker", "effectful update marker", "uninitialized increment", "blank zero-trip loop", "missing branch assignment", "read before assignment", "capture before assignment", "ordinary argument writes captured storage", "ordinary argument increments captured storage", "not direct return", "allocation in loop", "parameter storage", "wrong NEW", "wrong constructor", "wrong erasure", "opaque loop effect", "work", "depth", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			typ := types.NewJavaPrimer(types.JavaLong)
			ref := values.NewJavaRef(coreutils.NewRootVariableId(), nil, typ)
			literal := func(n int64) values.JavaValue { return values.NewJavaLiteral(n, typ) }
			condition := values.NewJavaLiteral(false, types.NewJavaPrimer(types.JavaBoolean))
			decl := &statements.AssignStatement{LeftValue: ref, JavaValue: literal(3), IsFirst: true}
			write := &statements.AssignStatement{LeftValue: ref, JavaValue: literal(-9)}
			loop := &statements.WhileStatement{ConditionValue: condition, Body: []statements.Statement{write}}
			child := &nativeAnonymousClass{descriptor: "(J)V", method: "pick()Ljava/lang/Object;", fields: map[string]int{"val$snapshot": 0}, newPC: 3, invokePC: 7}
			allocation := &values.NewExpression{JavaType: types.NewJavaClass("Snapshot$1"), OriginPC: 3, HasOriginPC: true}
			call := &values.FunctionCallExpression{Object: allocation, ClassName: "Snapshot$1", FunctionName: "<init>", Descriptor: child.descriptor, Arguments: []values.JavaValue{ref}, Kind: values.InvokeSpecial, IsSpecialInvoke: true, OriginPC: 7, HasOriginPC: true}
			allocation.ConstructorCall = call
			ret := statements.NewReturnStatement(allocation)
			body := []statements.Statement{decl, loop, ret}
			family := &nativeAnonymousFamily{owner: "Snapshot", children: map[string]*nativeAnonymousClass{"Snapshot$1": child}}
			c := &ClassObjectDumper{nativeAnonymousRoot: family, FuncCtx: &class_context.ClassContext{ClassName: "Snapshot", FunctionName: "pick", CurrentMethodDesc: "()Ljava/lang/Object;"}}
			good := mode == "loop writes" || mode == "conditional writes" || mode == "sequential writes" || mode == "standalone increment" || mode == "wrapped decrement" || mode == "postfix update marker"
			switch mode {
			case "conditional writes":
				body[1] = &statements.IfStatement{Condition: condition, IfBody: []statements.Statement{write}}
			case "sequential writes":
				body[1] = write
			case "standalone increment":
				loop.Body = []statements.Statement{values.NewUnaryExpression(ref, values.INC, typ)}
			case "wrapped decrement":
				loop.Body = []statements.Statement{&statements.ExpressionStatement{Expression: values.NewUnaryExpression(ref, values.DEC, typ)}}
			case "cross-category update":
				integer := types.NewJavaPrimer(types.JavaInteger)
				other := values.NewJavaRef(ref.Id, nil, integer)
				loop.Body = []statements.Statement{values.NewUnaryExpression(other, values.INC, integer)}
			case "boolean increment", "reference increment":
				var other types.JavaType = types.NewJavaPrimer(types.JavaBoolean)
				descriptor := "(Z)V"
				var initial values.JavaValue = values.NewJavaLiteral(false, other)
				if mode == "reference increment" {
					other = types.NewJavaClass("java.lang.Object")
					descriptor = "(Ljava/lang/Object;)V"
					initial = values.NewJavaLiteral(nil, other)
				}
				ref.ResetVarType(other)
				decl.JavaValue = initial
				child.descriptor, call.Descriptor = descriptor, descriptor
				loop.Body = []statements.Statement{values.NewUnaryExpression(ref, values.INC, other)}
			case "postfix update marker", "nonunit update marker", "noncomparable update marker", "effectful update marker":
				var marker values.JavaValue = values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))
				if mode == "nonunit update marker" {
					marker = values.NewJavaLiteral(2, types.NewJavaPrimer(types.JavaInteger))
				}
				if mode == "noncomparable update marker" {
					marker = values.NewJavaLiteral([]byte{1}, types.NewJavaPrimer(types.JavaInteger))
				}
				if mode == "effectful update marker" {
					marker = values.NewCustomValue(func(*class_context.ClassContext) string { return "touch()" }, func() types.JavaType { return typ })
				}
				loop.Body = []statements.Statement{values.NewBinaryExpression(ref, marker, values.INC, typ)}
			case "uninitialized increment":
				decl.JavaValue, decl.IsFirst, decl.IsDeclare = nil, false, true
				loop.Body = []statements.Statement{values.NewUnaryExpression(ref, values.INC, typ)}
			case "blank zero-trip loop":
				decl.JavaValue, decl.IsFirst, decl.IsDeclare = nil, false, true
			case "missing branch assignment":
				decl.JavaValue, decl.IsFirst, decl.IsDeclare = nil, false, true
				body[1] = &statements.IfStatement{Condition: condition, IfBody: []statements.Statement{write}}
			case "read before assignment":
				decl.JavaValue = ref
			case "capture before assignment":
				body = []statements.Statement{ret, decl, loop}
			case "ordinary argument writes captured storage", "ordinary argument increments captured storage":
				child.descriptor, call.Descriptor = "(JJ)V", "(JJ)V"
				child.fields["val$snapshot"] = 1
				call.Arguments = []values.JavaValue{values.NewAssignmentExpression(ref, literal(11), 6, nil), ref}
				if mode == "ordinary argument increments captured storage" {
					call.Arguments[0] = values.NewUnaryExpression(ref, values.INC, typ)
				}
			case "not direct return":
				body[2] = &statements.ExpressionStatement{Expression: allocation}
			case "allocation in loop":
				loop.Body = append(loop.Body, ret)
				body = body[:2]
			case "parameter storage":
				ref.IsParam = true
			case "wrong NEW":
				allocation.OriginPC++
			case "wrong constructor":
				call.OriginPC++
			case "wrong erasure":
				ref.ResetVarType(types.NewJavaPrimer(types.JavaInteger))
			case "opaque loop effect":
				loop.Body = append(loop.Body, &statements.ExpressionStatement{Expression: values.NewCustomValue(func(*class_context.ClassContext) string { return "touch()" }, func() types.JavaType { return typ })})
			case "work":
				c.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "depth":
				c.Work = workbudget.New(nil, workbudget.Limits{MaxASTDepth: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				c.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if good {
				if _, stable := nativeCaptureJoinedDeclaration(body, ref, allocation, nil); stable {
					t.Fatal("mutable source storage must not directly satisfy effective finality")
				}
			}
			before := append([]values.JavaValue(nil), call.Arguments...)
			projected, closed := c.prepareNativeCaptureIdentitySnapshots(body, nil)
			if good && !closed {
				t.Fatalf("closed=%v want=%v", closed, good)
			}
			if !good {
				if (mode == "not direct return" || mode == "allocation in loop" || mode == "parameter storage") && !closed {
					t.Fatal("an inapplicable optional projection poisoned the ordinary method")
				}
				for i := range before {
					if call.Arguments[i] != before[i] {
						t.Fatal("refused snapshot changed an original operand")
					}
				}
				if closed {
					if len(projected) != len(body) {
						t.Fatal("refused optional projection introduced a statement")
					}
					c.prepareNativeCaptureBindings(projected, nil)
					if !family.failed && c.FuncCtx.SourceCaptureStable != nil && c.FuncCtx.SourceCaptureStable(call.OriginPC, ref.Id) {
						t.Fatal("unproved original storage was authorized by a refused projection")
					}
				}
				return
			}
			alias, ok := call.Arguments[0].(*values.JavaRef)
			if !ok || alias.Id == ref.Id || write.LeftValue != ref || len(projected) != len(body)+1 || projected[len(projected)-1] != ret {
				t.Fatal("snapshot changed original mutable storage or allocation position")
			}
			copy, ok := projected[len(projected)-2].(*statements.AssignStatement)
			if !ok || copy.LeftValue != alias || copy.JavaValue != ref || !copy.IsFirst {
				t.Fatal("snapshot did not read exactly the original word before allocation")
			}
			c.prepareNativeCaptureBindings(projected, nil)
			if family.failed || c.FuncCtx.LocalNames[alias.Id] != "snapshot" || c.FuncCtx.SourceCaptureStable == nil || !c.FuncCtx.SourceCaptureStable(7, alias.Id) || c.FuncCtx.SourceCaptureStable(7, ref.Id) {
				t.Fatal("only the fresh effectively final binder may authorize capture")
			}
		})
	}
}

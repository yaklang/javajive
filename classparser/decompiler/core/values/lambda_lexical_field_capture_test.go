package values

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestLambdaLexicalFieldPortRetainsExactPendingAndFactoryIdentity(t *testing.T) {
	for _, variant := range []string{"original", "slot transport", "receiver slot transport", "renamed snapshot", "renamed receiver", "source slice detached", "copied receipt", "wrong field pc", "changed field name", "changed receiver", "changed seed", "changed field type", "borrowed snapshot", "wrong original factory", "wrong query factory", "wrong index", "foreign context", "static context", "missing factory", "changed source identity", "already declared", "already inlined", "method reference", "missing source identity", "missing metadata constraint", "duplicate port", "second binding"} {
		t.Run(variant, func(t *testing.T) {
			typ := types.NewJavaPrimer(types.JavaLong)
			receiver := NewJavaRef(utils.NewRootVariableId(), JavaNull, types.NewJavaClass("fixture.Owner"))
			receiver.IsParam, receiver.IsThis = true, true
			receiver.MarkOriginalParameter(0)
			receiver.MarkOriginalReceiver()
			field := NewRefMember(receiver, "capture", typ)
			field.OriginPC, field.HasOriginPC = 2, true
			field.MarkOriginalFieldRead(NewJavaClassMember("fixture.Owner", "capture", "J", typ), 2)
			var seed JavaValue = field
			if variant == "slot transport" {
				seed = NewSlotValue(field, typ)
			} else if variant == "receiver slot transport" {
				field.Object = NewSlotValue(receiver, receiver.Type())
			}
			snapshot := NewJavaRef(utils.NewRootVariableId(), seed, typ)
			snapshot.MarkOriginalDynamicOperand(10, 0, seed)
			port := NewLambdaLexicalFieldCapture(snapshot, "fixture.Owner", "capture", "J", 2)
			if port == nil {
				t.Fatal("original typed field/source port")
			}
			value := NewStreamingCustomValue(func(*class_context.ClassContext, *workbudget.Writer) error { panic("proof rendered lambda") }, func() types.JavaType { panic("proof queried lambda type") })
			value.Flag, value.OriginPC, value.HasOriginPC, value.CapturesKnown = "lambda", 10, true, true
			value.Captures = []JavaValue{snapshot}
			value.RequireLambdaParameterNames()
			value.BindLambdaSourceIdentity(NewLambdaSourceIdentity())
			ports := []*LambdaLexicalFieldCapture{port}
			if variant == "copied receipt" {
				copy := *port
				ports[0] = &copy
			} else if variant == "duplicate port" {
				ports = append(ports, port)
			} else if variant == "method reference" {
				value.IsMethodRef = true
			} else if variant == "missing source identity" {
				value.lambdaSourceIdentity = nil
			} else if variant == "missing metadata constraint" {
				value.lambdaParameterNamesRequired = false
			}
			bound := value.BindLambdaLexicalFieldCaptures(ports)
			wantBound := variant != "copied receipt" && variant != "duplicate port" && variant != "method reference" && variant != "missing source identity" && variant != "missing metadata constraint"
			if bound != wantBound {
				t.Fatal("detached, duplicated or role-free source port was bound")
			}
			if variant != "missing factory" {
				value.MarkOriginalLambdaFactory(10)
			}
			ctx := &class_context.ClassContext{ClassName: "fixture.Owner"}
			pc, index := 10, 0
			switch variant {
			case "source slice detached":
				ports[0] = nil
			case "renamed snapshot":
				snapshot.Id.SetName("anotherSpelling")
			case "renamed receiver":
				receiver.Id.SetName("anotherThisSpelling")
			case "wrong field pc":
				field.OriginPC++
			case "changed field name":
				field.Member = "other"
			case "changed receiver":
				field.Object = JavaNull
			case "changed seed":
				snapshot.Val = NewSlotValue(field, typ)
			case "changed field type":
				field.JavaType = types.NewJavaPrimer(types.JavaDouble)
			case "borrowed snapshot":
				other := NewJavaRef(snapshot.Id, seed, typ)
				other.VarUid = snapshot.VarUid
				other.MarkOriginalDynamicOperand(10, 0, seed)
				value.Captures[0] = other
			case "wrong original factory":
				value.OriginPC++
			case "wrong query factory":
				pc++
			case "wrong index":
				index++
			case "foreign context":
				ctx.ClassName = "fixture.Other"
			case "static context":
				ctx.IsStatic = true
			case "changed source identity":
				value.lambdaSourceIdentity = NewLambdaSourceIdentity()
			case "already declared":
				snapshot.MarkOriginalDynamicOperandDeclaration(10, seed)
			case "already inlined":
				snapshot.StackVar = seed
				snapshot.MarkOriginalDynamicOperandInline(10, seed)
			case "second binding":
				if value.BindLambdaLexicalFieldCaptures(ports) {
					t.Fatal("a second call overwrote an immutable binding")
				}
			}
			got, known := value.OriginalLambdaLexicalFieldReceiver(pc, index, snapshot, seed, ctx)
			want := variant == "original" || variant == "slot transport" || variant == "receiver slot transport" || variant == "renamed snapshot" || variant == "renamed receiver" || variant == "source slice detached" || variant == "second binding"
			if known != want || known && got != receiver {
				t.Fatalf("original eager field/source identity=%v want=%v", known, want)
			}
		})
	}
}

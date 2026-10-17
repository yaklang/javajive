package statements

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func sourceAssertionFixture(desc string, message values.JavaValue) (*SourceAssertionStatement, bool) {
	a := &values.NewExpression{JavaType: types.NewJavaClass("java.lang.AssertionError"), OriginPC: 10, HasOriginPC: true}
	c := &values.FunctionCallExpression{Object: a, ClassName: "java.lang.AssertionError", FunctionName: "<init>", Descriptor: desc, Kind: values.InvokeSpecial, OriginPC: 20, HasOriginPC: true}
	var params []types.JavaType
	if message != nil {
		c.Arguments = []values.JavaValue{message}
		params = []types.JavaType{message.Type()}
	}
	c.FuncType = types.NewJavaFuncType(desc, params, types.NewJavaPrimer(types.JavaVoid))
	a.ConstructorCall = c
	return NewSourceAssertionStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), c, 23)
}

func TestSourceAssertionRetainsEveryOriginalConstructorOverload(t *testing.T) {
	for _, desc := range []string{"()V", "(Ljava/lang/Object;)V", "(Z)V", "(C)V", "(I)V", "(J)V", "(F)V", "(D)V"} {
		t.Run(desc, func(t *testing.T) {
			var message values.JavaValue
			if desc != "()V" {
				message = values.JavaNull
				if desc[1] != 'L' {
					argType, err := types.ParseDescriptor(desc[1:2])
					if err != nil {
						t.Fatal(err)
					}
					message = values.NewJavaLiteral(1, argType)
				}
			}
			s, ok := sourceAssertionFixture(desc, message)
			if !ok {
				t.Fatal("valid original constructor refused")
			}
			condition, call, pc, known := s.SourceAssertionProtocol()
			if !known || condition != s.condition || call != s.constructor || pc != 23 || call.Descriptor != desc {
				t.Fatal("original packet changed")
			}
		})
	}
}

func TestSourceAssertionRevalidatesMutatedPackets(t *testing.T) {
	for _, variant := range []string{"new origin", "missing new origin", "invoke origin", "missing invoke origin", "allocation type", "array allocation", "hidden initializer", "allocation constructor", "receiver", "owner", "name", "opcode", "static", "descriptor", "arity", "message", "typed nil message", "typed nil condition", "new before invoke", "invoke before throw"} {
		t.Run(variant, func(t *testing.T) {
			s, ok := sourceAssertionFixture("(Ljava/lang/Object;)V", values.JavaNull)
			if !ok {
				t.Fatal("fixture")
			}
			switch variant {
			case "new origin":
				s.allocation.OriginPC++
			case "missing new origin":
				s.allocation.HasOriginPC = false
			case "invoke origin":
				s.constructor.OriginPC++
			case "missing invoke origin":
				s.constructor.HasOriginPC = false
			case "allocation type":
				s.allocation.JavaType = types.NewJavaClass("java.lang.RuntimeException")
			case "array allocation":
				s.allocation.Length = []values.JavaValue{values.JavaNull}
			case "hidden initializer":
				s.allocation.Initializer = []values.JavaValue{values.JavaNull}
			case "allocation constructor":
				s.allocation.ConstructorCall = s.constructor.Clone()
			case "receiver":
				s.constructor.Object = values.JavaNull
			case "owner":
				s.constructor.ClassName = "java.lang.RuntimeException"
			case "name":
				s.constructor.FunctionName = "factory"
			case "opcode":
				s.constructor.Kind = values.InvokeVirtual
			case "static":
				s.constructor.IsStatic = true
			case "descriptor":
				s.constructor.Descriptor = "(I)V"
			case "arity":
				s.constructor.Arguments = nil
			case "message":
				s.constructor.Arguments[0] = values.NewJavaLiteral("other", types.NewJavaClass("java.lang.String"))
			case "typed nil message":
				s.message = (*values.JavaRef)(nil)
				s.constructor.Arguments[0] = s.message
			case "typed nil condition":
				s.condition = (*values.JavaRef)(nil)
			case "new before invoke":
				s.newPC = 21
				s.allocation.OriginPC = 21
			case "invoke before throw":
				s.invokePC = 24
				s.constructor.OriginPC = 24
			}
			if _, _, _, known := s.SourceAssertionProtocol(); known {
				t.Fatal("changed physical packet accepted")
			}
		})
	}
	for _, desc := range []string{"", "(B)V", "(S)V", "(Ljava/lang/String;)V", "(II)V", "()Z"} {
		if _, ok := sourceAssertionFixture(desc, values.JavaNull); ok {
			t.Fatalf("non-AssertionError constructor %s", desc)
		}
	}
	if _, ok := NewSourceAssertionStatement(nil, nil, -1); ok {
		t.Fatal("missing packet")
	}
}

func TestSourceAssertionRenderingAndBindingUseStructuredOperands(t *testing.T) {
	oldCondition, nextCondition := utils.NewRootVariableId(), utils.NewRootVariableId()
	oldMessage, nextMessage := utils.NewRootVariableId(), utils.NewRootVariableId()
	condition := values.NewJavaRef(oldCondition, nil, types.NewJavaPrimer(types.JavaBoolean))
	message := values.NewJavaRef(oldMessage, nil, types.NewJavaClass("java.lang.Object"))
	s, ok := sourceAssertionFixture("(Ljava/lang/Object;)V", message)
	if !ok {
		t.Fatal("fixture")
	}
	s.condition = condition
	s.allocation.ArgumentsGetter = func() string { t.Fatal("opaque allocation renderer used"); return "hidden()" }
	s.ReplaceVar(oldCondition, nextCondition)
	s.ReplaceVar(oldMessage, nextMessage)
	if condition.Id != nextCondition || message.Id != nextMessage {
		t.Fatal("hidden assertion operand was not rebound")
	}
	ctx := &class_context.ClassContext{}
	if got, want := s.String(ctx), "assert "+condition.String(ctx)+" : "+message.String(ctx); got != want {
		t.Fatalf("rendered %q want %q", got, want)
	}
	if _, _, _, known := s.SourceAssertionProtocol(); !known {
		t.Fatal("binding changed original invocation")
	}
}

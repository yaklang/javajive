package statements

import (
	"reflect"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// SourceAssertionStatement retains the complete typed failure packet after the
// original compiler flag has been proved and projected by the caller. Exception
// domain and local-write visitors must see its conditional operands and throw,
// rather than treating source assert as an opaque rendering leaf.
type SourceAssertionStatement struct {
	condition                values.JavaValue
	constructor              *values.FunctionCallExpression
	allocation               *values.NewExpression
	message                  values.JavaValue
	descriptor               string
	newPC, invokePC, throwPC int
}

func NewSourceAssertionStatement(condition values.JavaValue, call *values.FunctionCallExpression, throwPC int) (*SourceAssertionStatement, bool) {
	if sourceAssertionNil(condition) || call == nil || len(call.Arguments) > 1 || throwPC < 0 {
		return nil, false
	}
	allocation, ok := call.Object.(*values.NewExpression)
	if !ok || allocation == nil {
		return nil, false
	}
	s := &SourceAssertionStatement{condition: condition, constructor: call, allocation: allocation, descriptor: call.Descriptor, newPC: allocation.OriginPC, invokePC: call.OriginPC, throwPC: throwPC}
	if len(call.Arguments) == 1 {
		s.message = call.Arguments[0]
	}
	if _, _, _, valid := s.SourceAssertionProtocol(); !valid {
		return nil, false
	}
	return s, true
}

func (s *SourceAssertionStatement) SourceAssertionProtocol() (values.JavaValue, *values.FunctionCallExpression, int, bool) {
	if s == nil || sourceAssertionNil(s.condition) || s.constructor == nil || s.allocation == nil || !s.allocation.HasOriginPC || s.allocation.OriginPC != s.newPC || s.allocation.ConstructorCall != s.constructor || !s.constructor.HasOriginPC || s.constructor.OriginPC != s.invokePC || s.constructor.Object != s.allocation || s.constructor.Descriptor != s.descriptor || s.constructor.ClassName != "java.lang.AssertionError" || s.constructor.FunctionName != "<init>" || s.constructor.Kind != values.InvokeSpecial || s.constructor.IsStatic {
		return nil, nil, 0, false
	}
	owner, known := types.RawClassFQN(s.allocation.Type())
	if !known || owner != "java.lang.AssertionError" || len(s.allocation.Length) != 0 || len(s.allocation.Initializer) != 0 || s.newPC < 0 || s.newPC >= s.invokePC || s.invokePC >= s.throwPC {
		return nil, nil, 0, false
	}
	arity := -1
	switch s.descriptor {
	case "()V":
		arity = 0
	case "(Ljava/lang/Object;)V", "(Z)V", "(C)V", "(I)V", "(J)V", "(F)V", "(D)V":
		arity = 1
	}
	if arity != len(s.constructor.Arguments) || arity == 1 && sourceAssertionNil(s.message) {
		return nil, nil, 0, false
	}
	if s.message == nil && len(s.constructor.Arguments) != 0 || s.message != nil && (len(s.constructor.Arguments) != 1 || s.constructor.Arguments[0] != s.message) {
		return nil, nil, 0, false
	}
	return s.condition, s.constructor, s.throwPC, true
}

func (s *SourceAssertionStatement) ReplaceVar(old, next *utils.VariableId) {
	s.condition.ReplaceVar(old, next)
	if s.message != nil {
		s.message.ReplaceVar(old, next)
	}
}

func (s *SourceAssertionStatement) String(ctx *class_context.ClassContext) string {
	text := "assert " + values.SimplifyConditionValue(s.condition).String(ctx)
	if s.message != nil {
		text += " : " + s.constructor.ArgumentStrings(ctx)[0]
	}
	return text
}

func sourceAssertionNil(v values.JavaValue) bool {
	return v == nil || reflect.ValueOf(v).Kind() == reflect.Ptr && reflect.ValueOf(v).IsNil()
}

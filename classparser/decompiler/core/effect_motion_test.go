package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestInlineMotionChecksEntirePath(t *testing.T) {
	typ := types.NewJavaPrimer(types.JavaInteger)
	x := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
	saved := values.NewJavaRef(utils.NewRootVariableId().Next(), x, typ)
	source := NewNode(statements.NewAssignStatement(saved, x, true))
	source.Id = 0
	middle := NewNode(statements.NewAssignStatement(x, values.NewJavaLiteral(99, typ), false))
	middle.Id = 1
	target := NewNode(statements.NewReturnStatement(saved))
	target.Id = 2
	source.AddNext(middle)
	middle.AddNext(target)
	origins := map[int]*OpCode{0: {CurrentOffset: 0}, 1: {CurrentOffset: 1}, 2: {CurrentOffset: 2}}
	d := &Decompiler{}
	if d.canInlineValue(x, source, target, origins) {
		t.Fatal("saved read crossed write of original local")
	}
	middle.Statement = statements.NewAssignStatement(values.NewJavaRef(utils.NewRootVariableId().Next().Next(), nil, typ), values.NewJavaLiteral(99, typ), false)
	if !d.canInlineValue(x, source, target, origins) {
		t.Fatal("pure independent linear motion rejected")
	}
	d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 1, HandlerPc: 4}}
	if d.canInlineValue(x, source, target, origins) {
		t.Fatal("read crossed handler scope")
	}
	d.ExceptionTable = nil
	middle.AddNext(source)
	if d.canInlineValue(x, source, target, origins) {
		t.Fatal("ambiguous/cyclic path accepted")
	}
}

func TestAdjacentCallInliningRespectsEvaluatedPrefix(t *testing.T) {
	typ := types.NewJavaPrimer(types.JavaInteger)
	saved := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
	effect := values.TagEffects(values.NewJavaLiteral(1, typ), values.EffectCall)
	source := NewNode(statements.NewAssignStatement(saved, effect, true))
	source.Id = 0
	call := &values.FunctionCallExpression{Arguments: []values.JavaValue{values.NewJavaLiteral(0, typ), saved}}
	target := NewNode(statements.NewExpressionStatement(call))
	target.Id = 1
	source.AddNext(target)
	d := &Decompiler{}
	origins := map[int]*OpCode{0: {CurrentOffset: 0}, 1: {CurrentOffset: 1}}
	if !d.canInlineValue(effect, source, target, origins) {
		t.Fatal("adjacent use with pure prefix rejected")
	}
	call.Arguments[0] = values.TagEffects(values.NewJavaLiteral(0, typ), values.EffectCall)
	if d.canInlineValue(effect, source, target, origins) {
		t.Fatal("reordered two calls")
	}
	call.Arguments = []values.JavaValue{saved, saved}
	if d.canInlineValue(effect, source, target, origins) {
		t.Fatal("duplicated effectful value")
	}
}

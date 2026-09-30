package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/jdecenv"
)

// Java supplies enum name/ordinal implicitly to a delegated this() constructor.
// Strip only the exact two incoming synthetic locals, keeping descriptor-indexed
// rendering for every payload argument. A matching printed name is insufficient.
func enumThisDelegateSource(statement *statements.ExpressionStatement, synthetic []values.JavaValue, ctx *class_context.ClassContext) (string, bool) {
	if statement == nil || ctx == nil || ctx.FunctionName != "<init>" || len(synthetic) != 2 {
		return "", false
	}
	call, ok := values.UnpackSoltValue(statement.Expression).(*values.FunctionCallExpression)
	if !ok || call == nil || call.FunctionName != "<init>" || !call.IsSpecialInvoke ||
		call.ClassName != ctx.ClassName || len(call.Arguments) < 2 {
		return "", false
	}
	receiver, ok := values.UnpackSoltValue(call.Object).(*values.JavaRef)
	if !ok || receiver == nil || !receiver.IsThis {
		return "", false
	}
	method, err := types.ParseMethodDescriptor(call.Descriptor)
	if err != nil || method.FunctionType() == nil || len(method.FunctionType().ParamTypes) != len(call.Arguments) {
		return "", false
	}
	params := method.FunctionType().ParamTypes
	nameType, _ := types.ClassFQNOf(params[0])
	ordinalType, primitive := params[1].RawType().(*types.JavaPrimer)
	if nameType != "java.lang.String" || !primitive || ordinalType.Name != types.JavaInteger {
		return "", false
	}
	for i := 0; i < 2; i++ {
		arg, argOK := values.UnpackSoltValue(call.Arguments[i]).(*values.JavaRef)
		param, paramOK := values.UnpackSoltValue(synthetic[i]).(*values.JavaRef)
		if !argOK || !paramOK || !values.SameLocal(arg, param) {
			return "", false
		}
	}
	args := call.ArgumentStrings(ctx)
	return "this(" + strings.Join(args[2:], ",") + ")", true
}

// fixEnumNoArgCtorThisAfterLocals rewrites a synthetic enum no-arg constructor
// that materializes name/ordinal as Object locals and then calls
// `this(var1,var2, args)` — illegal (this() must be first) and the
// (Object,Object,…) overload does not exist. The real this() is the
// payload ctor: `this(args)`.
// Kill-switch: JDEC_ENUM_CTOR_THIS_FIRST_OFF=1.
func fixEnumNoArgCtorThisAfterLocals(body string) string {
	if jdecenv.Get("JDEC_ENUM_CTOR_THIS_FIRST_OFF") == "1" {
		return body
	}
	const mid = "{\n\tObject var1 = null;\n\tObject var2 = null;\n\t\tthis(var1,var2,"
	from := 0
	for {
		rel := strings.Index(body[from:], mid)
		if rel < 0 {
			return body
		}
		i := from + rel
		argsStart := i + len(mid)
		thisOpen := argsStart - len("var1,var2,") - 1
		if thisOpen < 0 || thisOpen >= len(body) || body[thisOpen] != '(' {
			from = i + 1
			continue
		}
		depth := 1
		j := thisOpen + 1
		for j < len(body) && depth > 0 {
			switch body[j] {
			case '(':
				depth++
			case ')':
				depth--
			}
			j++
		}
		if depth != 0 || !strings.HasPrefix(body[j:], ";\n\t}") {
			from = i + 1
			continue
		}
		rest := body[argsStart : j-1]
		repl := "{\n\t\tthis(" + rest + ");\n\t}"
		end := j + len(";\n\t}")
		body = body[:i] + repl + body[end:]
		from = i + len(repl)
	}
}

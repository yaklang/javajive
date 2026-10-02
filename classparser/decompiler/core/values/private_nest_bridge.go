package values

import (
	"fmt"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func (f *FunctionCallExpression) privateNestBridgeCall(ctx *class_context.ClassContext) (string, bool) {
	if f == nil || ctx == nil || ctx.PrivateNestBridge == nil || !f.HasOriginPC || f.IsStatic || f.Object == nil || f.Kind == InvokeStatic || f.Kind == InvokeDynamic || f.FunctionName == "<init>" {
		return "", false
	}
	name, ok := ctx.PrivateNestBridge(f.ClassName, f.FunctionName, f.Descriptor, uint8(f.Kind), f.OriginPC)
	if !ok {
		return "", false
	}
	method, err := types.ParseMethodDescriptor(f.Descriptor)
	if err != nil || method == nil || method.FunctionType() == nil || len(method.FunctionType().ParamTypes) != len(f.Arguments) {
		return "", false
	}
	owner := types.NewJavaClass(strings.ReplaceAll(f.ClassName, "/", ".")).String(ctx)
	// An instance bridge keeps JVM receiver/argument evaluation and null timing.
	// A static bridge on the owner would initialize that owner even when this
	// receiver is null, which an original invokevirtual must not do.
	receiver := fmt.Sprintf("((%s)(%s))", owner, f.Object.String(ctx))
	var args []string
	for i, value := range f.Arguments {
		if value == nil {
			return "", false
		}
		arg := value.String(ctx)
		param := method.FunctionType().ParamTypes[i]
		if _, primitive := param.RawType().(*types.JavaPrimer); primitive {
			// Keep the existing JVM word-to-boolean argument conversion.
			arg = f.renderArgAt(i, ctx)
		} else {
			arg = fmt.Sprintf("((%s)(%s))", param.String(ctx), arg)
		}
		args = append(args, arg)
	}
	return receiver + "." + name + "(" + strings.Join(args, ", ") + ")", true
}

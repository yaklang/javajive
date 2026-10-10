package values

import (
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"strings"
)

// The planner approves this exact original tuple. The source view keeps every
// receiver/argument evaluation outside the helper catch, in JVM order, once.
func (f *FunctionCallExpression) constructorCheckedInvokeView(ctx *class_context.ClassContext) (string, bool) {
	if f == nil || ctx == nil || ctx.ConstructorInvokeBridge == nil || !f.HasOriginPC || f.FunctionName == "<init>" || (f.Kind != InvokeStatic && f.Kind != InvokeVirtual && f.Kind != InvokeInterface) {
		return "", false
	}
	name, ok := ctx.ConstructorInvokeBridge(f.ClassName, f.FunctionName, f.Descriptor, uint8(f.Kind), f.OriginPC)
	if !ok || name == "" {
		return "", false
	}
	method, err := types.ParseMethodDescriptor(f.Descriptor)
	if err != nil || len(method.FunctionType().ParamTypes) != len(f.Arguments) {
		return "", false
	}
	args := []string{}
	if !f.IsStatic {
		if f.Object == nil {
			return "", false
		}
		owner := types.NewJavaClass(strings.ReplaceAll(f.ClassName, "/", ".")).String(ctx)
		args = append(args, fmt.Sprintf("((%s)(%s))", owner, f.Object.String(ctx)))
	}
	for i, arg := range f.Arguments {
		if arg == nil {
			return "", false
		}
		param := method.FunctionType().ParamTypes[i]
		source := arg.String(ctx)
		if _, primitive := param.RawType().(*types.JavaPrimer); primitive {
			source = f.renderArgAt(i, ctx)
		} else {
			source = fmt.Sprintf("((%s)(%s))", param.String(ctx), source)
		}
		args = append(args, source)
	}
	return name + "(" + strings.Join(args, ",") + ")", true
}

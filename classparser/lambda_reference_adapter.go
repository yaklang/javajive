package javaclassparser

import (
	"fmt"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func lambdaReferenceAdapterParams(adapter *core.LambdaReferenceAdapter, params []values.JavaValue, ctx *class_context.ClassContext, indent string) ([]string, string, error) {
	inst, err := types.ParseMethodDescriptor(adapter.InstantiatedDescriptor)
	if err != nil || inst.FunctionType() == nil || len(inst.FunctionType().ParamTypes) != len(params) {
		return nil, "", fmt.Errorf("unsupported lambda reference adapter parameter shape")
	}
	var names, checks []string
	for i, value := range params {
		ref, ok := value.(*values.JavaRef)
		if !ok || ref == nil || ref.Id == nil || ref.Type() == nil || ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil {
			return nil, "", fmt.Errorf("unsupported lambda reference adapter parameter identity")
		}
		name := ref.String(ctx)
		// Lambda parameters use lN/lDEPTH_N; body locals use lvSEQ_N. The
		// '$sam' suffix is outside those namespaces and never replaces body
		// names or captures. Even an unused specialized argument is checked.
		argument := name + "$sam"
		names = append(names, argument)
		actual := inst.FunctionType().ParamTypes[i].String(ctx)
		checks = append(checks, indent+renderMethodParamType(ref.Type(), ctx)+" "+name+" = (("+actual+")("+argument+"));")
	}
	return names, "\n" + strings.Join(checks, "\n") + "\n", nil
}

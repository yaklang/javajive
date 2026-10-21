package values

import (
	"fmt"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

type originalInstanceOfSource struct {
	operand    JavaValue
	target     types.JavaType
	descriptor string
	pc         int
}

// Retain the decoded type test as an operation, not an opaque callback whose
// Flag happens to say instanceof. The canonical target and renderer are private;
// later formatting/capture annotations cannot change the operation being proved.
func NewOriginalInstanceOf(operand JavaValue, target types.JavaType, pc int) *CustomValue {
	descriptor := ReferenceTypeDescriptor(target, nil)
	if isNilJavaValue(operand) || descriptor == "" || len(descriptor) > 65535 || pc < 0 || pc > 65535 {
		return nil
	}
	name := strings.TrimLeft(descriptor, "[")
	if strings.HasPrefix(name, "L") {
		for _, segment := range strings.Split(strings.TrimSuffix(name[1:], ";"), "/") {
			if !class_context.PreservesJava8Identifier(segment) {
				return nil
			}
		}
	}
	canonical, err := types.ParseDescriptor(descriptor)
	if err != nil || canonical == nil || ReferenceTypeDescriptor(canonical, nil) != descriptor {
		return nil
	}
	role := &originalInstanceOfSource{operand: operand, target: canonical, descriptor: descriptor, pc: pc}
	v := NewStreamingCustomValue(role.write, func() types.JavaType { return types.NewJavaPrimer(types.JavaBoolean) })
	v.sourceInstanceOf = role
	v.Flag, v.CapturesKnown, v.Captures = "instanceof", true, []JavaValue{operand}
	v.OriginPC, v.HasOriginPC = pc, true
	return v
}

func (v *CustomValue) OriginalInstanceOfSource() (operand JavaValue, descriptor string, pc int, known bool) {
	if v == nil || v.sourceInstanceOf == nil {
		return nil, "", 0, false
	}
	role := v.sourceInstanceOf
	return role.operand, role.descriptor, role.pc, true
}

func (role *originalInstanceOfSource) write(ctx *class_context.ClassContext, out *workbudget.Writer) error {
	if role == nil || out == nil || isNilJavaValue(role.operand) || role.target == nil {
		return fmt.Errorf("incomplete original instanceof source")
	}
	if ctx == nil {
		ctx = &class_context.ClassContext{}
	}
	if err := out.WriteString(AssignmentOperand(role.operand, ctx)); err != nil {
		return err
	}
	if err := out.WriteString(" instanceof "); err != nil {
		return err
	}
	return out.WriteString(role.target.String(ctx))
}

package values

import (
	"fmt"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// MethodReferenceSource describes a decoded, uncaptured ordinary reference.
// It grants no bootstrap, member-selection or operand-motion permission. Its
// producer still validates those facts; an original-bytecode consumer reopens
// this complete tuple instead of trusting public CustomValue callbacks.
type MethodReferenceSource struct {
	Kind                                              uint8
	Owner, Member, Descriptor                         string
	CallName, CallDescriptor, ErasedSAM, Instantiated string
}

type methodReferenceTarget struct {
	raw, target types.JavaType
	markers     []types.JavaType
}

type uncapturedMethodReferenceSource struct {
	identity MethodReferenceSource
	targets  []methodReferenceTarget
}

// NewUncapturedMethodReferenceSource owns a canonical bounded renderer. No
// arbitrary text, type or writer callback can impersonate its source body.
// Constructors and bound/special references retain their existing path.
func NewUncapturedMethodReferenceSource(identity MethodReferenceSource, typ types.JavaType) *CustomValue {
	if typ == nil || identity.Kind != 5 && identity.Kind != 6 && identity.Kind != 9 || identity.Owner == "" || len(identity.Owner) > 65535 || strings.ContainsAny(identity.Owner, "[;<>") || !class_context.PreservesJava8Identifier(identity.Member) || !class_context.PreservesJava8Identifier(identity.CallName) {
		return nil
	}
	for _, descriptor := range []string{identity.Descriptor, identity.CallDescriptor, identity.ErasedSAM, identity.Instantiated} {
		if len(descriptor) > 65535 || !strings.HasPrefix(descriptor, "(") {
			return nil
		}
		if _, err := types.ParseMethodDescriptor(descriptor); err != nil {
			return nil
		}
	}
	identity.Owner = strings.ReplaceAll(identity.Owner, ".", "/")
	role := &uncapturedMethodReferenceSource{identity: identity}
	value := NewStreamingCustomValue(role.write, func() types.JavaType { return typ })
	value.sourceMethodReference = role
	value.Flag, value.IsMethodRef, value.CapturesKnown, value.NoOuterCapture = "lambda", true, true, true
	value.InstantiatedMtdDesc = identity.Instantiated
	return value
}

func (value *CustomValue) UncapturedMethodReferenceSource() (MethodReferenceSource, bool) {
	if value == nil || value.sourceMethodReference == nil || !value.IsMethodRef || !value.NoOuterCapture || value.InstantiatedMtdDesc != value.sourceMethodReference.identity.Instantiated {
		return MethodReferenceSource{}, false
	}
	_, captures, known := value.OriginalLambdaFactoryEvaluation()
	if !known || len(captures) != 0 {
		return MethodReferenceSource{}, false
	}
	return value.sourceMethodReference.identity, true
}

// Preserve creation-target casts structurally. A generic functional view may
// wrap the same source more than once; public writer slots stay non-authoritative.
func (value *CustomValue) RetainMethodReferenceTarget(raw, target types.JavaType, markers ...types.JavaType) (*CustomValue, bool) {
	if value == nil || value.sourceMethodReference == nil {
		return nil, false
	}
	refuse := func() (*CustomValue, bool) {
		copy := *value
		copy.sourceMethodReference = nil
		copy.WriteFunc = func(*class_context.ClassContext, *workbudget.Writer) error {
			return fmt.Errorf("method reference target view exceeds its closed source domain")
		}
		return &copy, true
	}
	if raw == nil || target == nil || len(markers) > 64 || len(value.sourceMethodReference.targets) >= 16 {
		return refuse()
	}
	for _, marker := range markers {
		if marker == nil {
			return refuse()
		}
	}
	role := *value.sourceMethodReference
	role.targets = append(append([]methodReferenceTarget(nil), role.targets...), methodReferenceTarget{raw: raw, target: target, markers: append([]types.JavaType(nil), markers...)})
	copy := *value
	copy.sourceMethodReference, copy.WriteFunc = &role, role.write
	copy.TypeFunc = func() types.JavaType { return raw }
	return &copy, true
}

func (role *uncapturedMethodReferenceSource) write(ctx *class_context.ClassContext, out *workbudget.Writer) error {
	if out == nil {
		return fmt.Errorf("method reference requires source context")
	}
	if ctx == nil {
		ctx = &class_context.ClassContext{}
	}
	for i := len(role.targets) - 1; i >= 0; i-- {
		view := role.targets[i]
		if err := out.WriteString("(" + view.raw.String(ctx) + ") ((" + view.target.String(ctx)); err != nil {
			return err
		}
		for _, marker := range view.markers {
			if err := out.WriteString(" & " + marker.String(ctx)); err != nil {
				return err
			}
		}
		if err := out.WriteString(") ("); err != nil {
			return err
		}
	}
	identity := role.identity
	owner, known := ctx.StaticMethodReferenceOwner(strings.ReplaceAll(identity.Owner, "/", "."), identity.Member, identity.Descriptor)
	if !known {
		return fmt.Errorf("original method reference type name is obscured")
	}
	if err := out.WriteString(owner + "::" + identity.Member); err != nil {
		return err
	}
	for range role.targets {
		if err := out.WriteString("))"); err != nil {
			return err
		}
	}
	return nil
}

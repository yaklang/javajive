package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

func writeObservedLambdaSource(ctx *class_context.ClassContext, out *workbudget.Writer, source LambdaBodySource, template string, captured []values.JavaValue, target, bridge string) error {
	start := out.Len()
	if err := t19WriteLambdaBodyWithBridge(ctx, out, template, captured, target, bridge); err != nil {
		return err
	}
	if source.RecordRendered != nil {
		source.RecordRendered(out.String()[start:])
	}
	return nil
}

// LambdaOriginalDeclaration describes original member identity, separately
// from the requested source representation and its capture namespace.
type LambdaOriginalDeclaration uint8

const (
	LambdaDeclarationUnknown LambdaOriginalDeclaration = iota
	LambdaDeclarationOrdinary
	LambdaDeclarationSyntheticBody
)

func (d *Decompiler) originalLambdaDeclaration(impl *values.JavaClassMember) LambdaOriginalDeclaration {
	if d == nil || impl == nil || d.OriginalLambdaDeclaration == nil {
		return LambdaDeclarationUnknown
	}
	return d.OriginalLambdaDeclaration(impl.Name, impl.Member, impl.Description)
}

// A Java method reference can bind an instance receiver, but cannot bind extra
// physical parameters of an ordinary declaration. Do not silently discard such
// captures when an unproved source-body request falls back to a method reference.
func t19OrdinaryMethodReferenceShape(impl *values.JavaClassMember, captures, sam int) bool {
	if impl == nil || captures < 0 || sam < 0 {
		return false
	}
	parameters, err := ParamCountOfDescriptor(impl.Description)
	if err != nil {
		return false
	}
	switch impl.RefKind {
	case RefInvokeStatic:
		return captures == 0 && parameters == sam
	case RefInvokeVirtual, RefInvokeInterface, RefInvokeSpecial:
		return captures == 0 && parameters+1 == sam || captures == 1 && parameters == sam
	default:
		return false
	}
}

package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

func writeObservedLambdaSource(ctx *class_context.ClassContext, out *workbudget.Writer, source LambdaBodySource, template string, captured []values.JavaValue, target, bridge string) error {
	start := out.Len()
	var projection []LambdaCaptureRender
	complete := len(captured) <= 255
	var observe func(int, string)
	if source.RecordRenderedWithCaptures != nil {
		// The physical descriptor bounds distinct capture ports. Repeated reads
		// retain their real writer behavior; inconsistent bytes grant no receipt.
		seen := map[int]int{}
		observe = func(index int, text string) {
			if !complete {
				return
			}
			if index < 0 || index >= len(captured) || len(text) > 1<<20 {
				complete = false
				return
			}
			if ctx != nil && ctx.Work != nil && (ctx.Work.Charge(workbudget.CounterGraphScans, int64(len(text))+1) != nil || ctx.Work.CheckAlloc(int64(len(text))+int64(len(projection)+1)*32) != nil) {
				complete = false
				return
			}
			if prior, known := seen[index]; known {
				complete = projection[prior].Text == text
				return
			}
			seen[index] = len(projection)
			projection = append(projection, LambdaCaptureRender{Index: index, Text: text})
		}
	}
	if err := t19WriteLambdaBodyWithCaptureObserver(ctx, out, template, captured, target, bridge, observe); err != nil {
		return err
	}
	if ctx != nil && ctx.Work != nil && ctx.Work.Err() != nil {
		return ctx.Work.Err()
	}
	if source.RecordRendered != nil {
		source.RecordRendered(out.String()[start:])
	}
	if source.RecordRenderedWithCaptures != nil {
		source.RecordRenderedWithCaptures(LambdaSourceRender{Packet: out.String()[start:], Captures: projection, Complete: complete})
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

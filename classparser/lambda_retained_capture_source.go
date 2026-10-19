package javaclassparser

import (
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A named logical local can intentionally have no historical Val when its
// physical slot has other lifetimes. Its actual retained STORE still carries
// the original RHS and target identity. Reuse that source/point proof rather
// than installing an initializer in Val or guessing a binder from its name.
func (c *ClassObjectDumper) lambdaRetainedCaptureSource(site *nativeLambdaLocalCaptureSite, captured []values.JavaValue, body []statements.Statement) *nativeLambdaLocalCaptureSource {
	if c == nil || c.FuncCtx == nil || site == nil || site.method == nil || site.code == nil || c.CurrentMethod != site.method || site.pc < 0 || site.pc >= len(site.code.Code) || len(captured) == 0 || len(captured) > 64 || len(body) == 0 || len(body) > 4096 || !nativeProofWork(c.Work, int64(len(captured)+len(body))+1) || c.Work != nil && c.Work.CheckAlloc(int64(len(captured)+len(body))*16+256) != nil {
		return nil
	}
	name, named := sourceBridgeUTF8(c.obj, site.method.NameIndex)
	descriptor, typed := sourceBridgeUTF8(c.obj, site.method.DescriptorIndex)
	if !named || !typed || name != c.FuncCtx.FunctionName || descriptor != c.FuncCtx.CurrentMethodDesc || c.FuncCtx.IsStatic != (site.method.AccessFlags&8 != 0) {
		return nil
	}
	context, known := c.FuncCtx.SnapshotForSourceProof(c.Work)
	if !known {
		return nil
	}
	return &nativeLambdaLocalCaptureSource{method: site.method, code: site.code, factoryPC: site.pc, hasFactoryPC: true, context: *context, values: slices.Clone(captured), body: slices.Clone(body)}
}

func (b *lambdaParameterBinding) retainedCaptureSourceClosed(work *workbudget.Budget) bool {
	if b == nil || b.metadata == nil || b.metadata.factory == nil || !b.metadata.factory.closed() || b.captureBody == nil || b.captureEmission == nil || b.captureEmission.body == nil || b.captureEmission.body.member != b.metadata.factory.method || b.captureEmission.request != b.captureEmission.body.sourceRequest {
		return false
	}
	source, factory := b.captureBody, b.metadata.factory
	if source.method != factory.method || source.code != factory.code || !source.hasFactoryPC || source.factoryPC != factory.pc || len(source.values) != len(b.captured)+b.metadata.receiverWords || !nativeProofWork(work, int64(len(source.values))+1) {
		return false
	}
	if b.metadata.receiverWords != 0 && (b.metadata.receiverWords != 1 || source.values[0] != b.receiver) {
		return false
	}
	for i, capture := range b.captured {
		if source.values[i+b.metadata.receiverWords] != capture {
			return false
		}
	}
	return true
}

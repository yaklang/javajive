package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

type nativeLambdaLocalCaptureSite struct {
	method   *MemberInfo
	code     *CodeAttribute
	pc       int
	operands []*nativeEnumSelectorProducer
}
type nativeLambdaLocalCaptureSource struct {
	method  *MemberInfo
	code    *CodeAttribute
	context class_context.ClassContext
	values  []values.JavaValue
}

// Java captures an effectively-final local reference once at the factory
// site. Reuse the original typed LOAD/reaching-STORE proof, then additionally
// exclude every other overlapping write in this method. Mutating the referenced
// object is allowed; reassignment, slot reuse and a wide interior write are not.
func nativeLambdaLocalSingleStore(ops []*core.OpCode, read *nativeEnumLocalRead, work *workbudget.Budget) bool {
	if read == nil || read.slot < 0 || len(ops) > 8192 {
		return false
	}
	width := 1
	if read.descriptor == "J" || read.descriptor == "D" {
		width = 2
	}
	stores := 0
	for _, op := range ops {
		if op == nil || op.Instr == nil || !nativeProofWork(work, 1) {
			return false
		}
		access := core.LocalAccessOf(op.Instr.OpCode)
		if !access.Write {
			continue
		}
		slot := core.GetStoreIdx(op)
		if slot < 0 || access.Width < 1 || access.Width > 2 {
			return false
		}
		if slot < read.slot+width && read.slot < slot+access.Width {
			if int(op.CurrentOffset) != read.storePC || slot != read.slot || access.Width != width {
				return false
			}
			stores++
		}
	}
	return stores == 1
}

// Record the actual captured AST operands before the recursive body dump, and
// revalidate them after the complete source body is rendered. Names, source
// strings and equal declared types cannot substitute another local declaration.
func (c *ClassObjectDumper) recordNativeLambdaLocalCaptureSource(name, desc string, code *CodeAttribute, captured []values.JavaValue) bool {
	if c == nil {
		return false
	}
	if c.nativeMemberCurrent == nil {
		return true
	}
	site := c.nativeMemberCurrent.lambdaContext.localCaptures[name+desc]
	if site == nil {
		return true
	}
	if c.nativeMemberCurrent.object != c.obj || c.FuncCtx == nil || c.CurrentMethod != site.method || code != site.code || c.nativeLambdaLocalSources[name+desc] != nil || len(captured) != len(site.operands) || !nativeProofWork(c.Work, int64(len(captured))) || c.Work != nil && c.Work.CheckAlloc(int64(len(captured))*64+512) != nil {
		return false
	}
	record := &nativeLambdaLocalCaptureSource{method: c.CurrentMethod, code: code, context: *c.FuncCtx, values: append([]values.JavaValue(nil), captured...)}
	// Stack simulation requests this body before the enclosing method builds
	// its STORE declaration nodes. Keep the actual operands now; the family's
	// publication certificate checks them only after that full render.
	if c.nativeLambdaLocalSources == nil {
		c.nativeLambdaLocalSources = map[string]*nativeLambdaLocalCaptureSource{}
	}
	c.nativeLambdaLocalSources[name+desc] = record
	return true
}

func nativeLambdaLocalCaptureSourceClosed(site *nativeLambdaLocalCaptureSite, source *nativeLambdaLocalCaptureSource, work *workbudget.Budget) bool {
	if site == nil || source == nil || site.method == nil || site.code == nil || site.pc < 0 || site.pc >= len(site.code.Code) || len(site.operands) == 0 || len(site.operands) > 64 || source.method != site.method || source.code != site.code || len(source.values) != len(site.operands) {
		return false
	}
	for i, operand := range site.operands {
		if !nativeLambdaLocalCaptureValue(operand, source.values[i], site.pc, i, &source.context, work) {
			return false
		}
	}
	return true
}

func nativeLambdaLocalCaptureValue(operand *nativeEnumSelectorProducer, value values.JavaValue, pc, index int, ctx *class_context.ClassContext, work *workbudget.Budget) bool {
	if nativeEnumSelectorSource(operand, value, ctx, work, 0) {
		return true
	}
	v, bounded := nativeMemberEnclosingUnpack(value, work)
	ref, known := v.(*values.JavaRef)
	if !bounded || !known || ref == nil || operand == nil {
		return false
	}
	location, position, known := ref.OriginalDynamicOperandWitness(ref.Val)
	erasure, typed := values.SourceTypeErasure(ref.Type(), ctx)
	return known && location == pc && position == index && typed && erasure == operand.result && nativeEnumSelectorSource(operand, ref.Val, ctx, work, 0)
}

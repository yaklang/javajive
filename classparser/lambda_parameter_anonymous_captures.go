package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

func lambdaParameterFieldCapture(operand *nativeEnumSelectorProducer) bool {
	return operand != nil && operand.opcode == core.OP_GETFIELD && operand.owner != ""
}

func (c *ClassObjectDumper) lambdaParameterLexicalFieldCaptures(binding *lambdaParameterBinding) ([]*values.LambdaLexicalFieldCapture, bool) {
	if binding == nil {
		return nil, true
	}
	p := binding.metadata
	if c == nil || p == nil || len(binding.captured) != len(p.captures) || len(p.captures) > 64 {
		return nil, false
	}
	field := false
	for _, operand := range p.captures {
		field = field || lambdaParameterFieldCapture(operand)
	}
	if !field {
		return nil, true
	}
	if !nativeProofWork(c.Work, int64(len(p.captures))+1) || c.Work != nil && c.Work.CheckAlloc(int64(len(p.captures))*256) != nil {
		return nil, false
	}
	var ports []*values.LambdaLexicalFieldCapture
	for i, operand := range p.captures {
		if !lambdaParameterFieldCapture(operand) {
			continue
		}
		v, known := nativeMemberEnclosingUnpack(binding.captured[i], c.Work)
		ref, typed := v.(*values.JavaRef)
		if !known || !typed || ref == nil || !c.lambdaParameterAnonymousFieldClosed(p, operand, p.names[i], c.Work) {
			return nil, false
		}
		port := values.NewLambdaLexicalFieldCapture(ref, operand.owner, operand.member, operand.descriptor, operand.pc)
		if port == nil {
			return nil, false
		}
		ports = append(ports, port)
	}
	return ports, true
}

// A physical prefix name may denote an enclosing captured source variable.
// Reuse only the actual anonymous renderer's original constructor/field port
// and completed allocation binding. An ordinary field or same-spelled local
// cannot acquire that role. Evaluation and final emission remain separate.
func (c *ClassObjectDumper) lambdaParameterAnonymousFieldClosed(p *lambdaParameterMetadata, operand *nativeEnumSelectorProducer, name string, work *workbudget.Budget) bool {
	if c == nil || c.obj == nil || p == nil || p.object != c.obj || p.parent != nil || p.factory == nil || !p.factory.closed() || p.factory.method.AccessFlags&8 != 0 || !lambdaParameterFieldCapture(operand) || operand.owner != c.obj.GetClassName() || !class_context.PreservesJava8Identifier(name) || !nativeProofWork(work, int64(len(name))+1) {
		return false
	}
	view := c.nativeAnonymousLambdaCurrent
	if view == nil || view.object != c.obj || !view.lambdaImplementations[p.method] {
		return false
	}
	child := view.lambdaContext.anonymousCaptures
	if child == nil || child.object != c.obj || child.lambdaImplementation != view || operand.member == child.enclosingField || c.nativeCaptureFields[operand.member] != name {
		return false
	}
	_, captured := child.fields[operand.member]
	method, named := sourceBridgeUTF8(c.obj, p.factory.method.NameIndex)
	descriptor, typed := sourceBridgeUTF8(c.obj, p.factory.method.DescriptorIndex)
	return captured && named && typed && c.nativeCapturedReads[method+descriptor][operand.pc] == operand.member
}

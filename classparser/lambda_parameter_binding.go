package javaclassparser

import (
	"slices"
	"strconv"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

type lambdaParameterBinding struct {
	metadata       *lambdaParameterMetadata
	request        methodSourceRequest
	refs           []*values.JavaRef
	context        *class_context.ClassContext
	arrow          string
	captureContext *class_context.ClassContext
	captured       []values.JavaValue
}

func (c *ClassObjectDumper) bindLambdaParameterMetadata(p *lambdaParameterMetadata, request methodSourceRequest, parameters []values.JavaValue) (*lambdaParameterBinding, bool) {
	if c == nil || p == nil || c.FuncCtx == nil || c.CurrentMethod != p.method || c.obj != p.object || request.role != methodSourceLambdaBody || !request.valid() || request.captureCount != len(p.captures) || !p.factory.same(request.factory) || c.FuncCtx.FunctionName != p.name || c.FuncCtx.CurrentMethodDesc != p.descriptor || !c.FuncCtx.IsStatic || len(parameters)+len(p.captures) != len(p.names) || c.Work != nil && c.Work.CheckAlloc(int64(len(parameters))*256) != nil {
		return nil, false
	}
	parentDescriptor, typed := sourceBridgeUTF8(c.obj, p.factory.method.DescriptorIndex)
	parentNames, known := c.originalOrdinaryMethodParameterNames(p.factory.method, parentDescriptor)
	if !typed || !known || !slices.Equal(parentNames, p.parentNames) {
		return nil, false
	}
	binding := &lambdaParameterBinding{metadata: p, request: request}
	protected := map[*coreutils.VariableId]bool{}
	offset := len(p.captures)
	slot := p.samSlot()
	for i, parameter := range parameters {
		physical := offset + i
		ref, ok := parameter.(*values.JavaRef)
		if !ok || ref == nil || ref.Id == nil || !ref.IsParam || ref.IsThis || protected[ref.Id] || slices.Contains(parentNames, p.names[physical]) || !nativeMethodLocalParameterOperand(ref, slot, p.parameters[physical], c.FuncCtx, c.Work) {
			return nil, false
		}
		if prior, exists := c.FuncCtx.LocalNames[ref.Id]; exists && prior != p.names[physical] {
			return nil, false
		}
		binding.refs = append(binding.refs, ref)
		protected[ref.Id] = true
		slot++
		if p.parameters[physical] == "J" || p.parameters[physical] == "D" {
			slot++
		}
	}
	prior := c.nativeLambdaFactoryParameterNames[p.factory.method]
	names := len(prior)
	for _, name := range p.names[offset:] {
		if !prior[name] {
			names++
		}
	}
	if prior == nil && len(c.nativeLambdaFactoryParameterNames) >= 4096 || names > 65536 {
		return nil, false
	}
	// Validate every entry before changing names. Reserve these SAM binders in
	// the actual enclosing method so its generated locals can be renamed there.
	if c.FuncCtx.LocalNames == nil {
		c.FuncCtx.LocalNames = map[*coreutils.VariableId]string{}
	}
	if c.nativeLambdaFactoryParameterNames == nil {
		c.nativeLambdaFactoryParameterNames = map[*MemberInfo]map[string]bool{}
	}
	if c.nativeLambdaFactoryParameterNames[p.factory.method] == nil {
		c.nativeLambdaFactoryParameterNames[p.factory.method] = map[string]bool{}
	}
	for i, ref := range binding.refs {
		ref.Id.SetName(p.names[offset+i])
		c.FuncCtx.LocalNames[ref.Id] = p.names[offset+i]
		c.nativeLambdaFactoryParameterNames[p.factory.method][p.names[offset+i]] = true
	}
	return binding, true
}

func (c *ClassObjectDumper) prepareLambdaParameterMetadataNamespace(binding *lambdaParameterBinding, body []statements.Statement, parameters []values.JavaValue) bool {
	if binding == nil || c == nil || c.FuncCtx == nil {
		return false
	}
	reserved := c.nativeLexicalCaptures()
	protected := map[*coreutils.VariableId]bool{}
	for _, name := range c.FuncCtx.Arguments {
		reserved[name] = true
	}
	for i, ref := range binding.refs {
		reserved[binding.metadata.names[len(binding.metadata.captures)+i]] = true
		protected[ref.Id] = true
	}
	// Allocate over declaration identities before rendering. Text matching varN
	// would also rename an original SAM formal or application string contents.
	namespace := "jdec$lambda$" + strconv.FormatUint(binding.request.serial, 10) + "$"
	return c.prepareNativeSourceNamesInNamespace(body, parameters, reserved, protected, namespace)
}

func (b *lambdaParameterBinding) parametersClosed(ctx *class_context.ClassContext, work *workbudget.Budget) bool {
	if b == nil || b.metadata == nil || ctx == nil || len(b.refs)+len(b.metadata.captures) != len(b.metadata.parameters) {
		return false
	}
	offset := len(b.metadata.captures)
	slot := b.metadata.samSlot()
	for i, ref := range b.refs {
		physical := offset + i
		if ref == nil || !ref.IsParam || ref.IsThis || ref.String(ctx) != b.metadata.names[physical] || !nativeMethodLocalParameterOperand(ref, slot, b.metadata.parameters[physical], ctx, work) {
			return false
		}
		slot++
		if b.metadata.parameters[physical] == "J" || b.metadata.parameters[physical] == "D" {
			slot++
		}
	}
	return true
}

func (c *ClassObjectDumper) lambdaParameterSourcePacketClosed(body *dumpedMethods) bool {
	if body == nil {
		return false
	}
	fresh, known := c.originalLambdaParameterMetadata(body.member, c.Work)
	if !known {
		return false
	}
	b := body.lambdaParameterBinding
	if fresh == nil {
		return b == nil
	}
	return b != nil && b.metadata != nil && b.metadata.object == c.obj && b.metadata.method == body.member && b.request == body.sourceRequest && b.request.captureCount == len(fresh.captures) && b.metadata.factory.same(fresh.factory) && b.metadata.factory.same(b.request.factory) && slices.Equal(b.metadata.captureTypes, fresh.captureTypes) && b.metadata.samDescriptor == fresh.samDescriptor && slices.Equal(b.metadata.names, fresh.names) && slices.Equal(b.metadata.flags, fresh.flags) && slices.Equal(b.metadata.parentNames, fresh.parentNames) && body.parameterNameStamp != nil && body.parameterNameStamp.lambdaParameters && c.methodParameterNameCacheClosed(body.parameterNameStamp) && b.parametersClosed(b.context, c.Work) && b.arrow != "" && b.arrow == body.code
}

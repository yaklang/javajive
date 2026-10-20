package javaclassparser

import (
	"encoding/binary"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"strings"
)

// JVM implementation descriptors retain only erasures. An explicitly typed
// Java lambda must instead match its original SAM's source formal types. This
// projection changes the header alone: physical parameter refs, entry checks,
// method parameter metadata and original body binding remain authoritative.
// Concrete member Signatures of a complete, monomorphic direct interface are
// sufficient; generic owners, inherited substitutions and method variables
// need their own substitution proof and do not acquire permission here.
func (c *ClassObjectDumper) originalLambdaSourceFormals(binding *lambdaParameterBinding) ([]types.JavaType, bool) {
	if c == nil || c.FuncCtx == nil || c.FuncCtx.InvocationMetadata == nil || binding == nil || binding.metadata == nil {
		return nil, false
	}
	p := binding.metadata
	f := p.factory
	if p.object != c.obj || p.method != c.CurrentMethod || binding.request.role != methodSourceLambdaBody || !binding.request.valid() || binding.request.captureCount != p.factoryCaptureCount() || !f.same(binding.request.factory) || !f.sealed || len(binding.refs) > 255 || !binding.parametersClosed(c.FuncCtx, c.Work) || !nativeProofWork(c.Work, 1) {
		return nil, false
	}
	code := f.code.Code
	pc := f.pc
	if pc < 0 || pc+5 > len(code) || code[pc] != core.OP_INVOKEDYNAMIC || code[pc+3] != 0 || code[pc+4] != 0 {
		return nil, false
	}
	index := int(binary.BigEndian.Uint16(code[pc+1 : pc+3]))
	if index <= 0 || index > len(c.obj.ConstantPool) {
		return nil, false
	}
	dynamic, ok := c.obj.ConstantPool[index-1].(*ConstantInvokeDynamicInfo)
	if !ok || dynamic == nil || dynamic.NameAndTypeIndex == 0 || int(dynamic.NameAndTypeIndex) > len(c.obj.ConstantPool) {
		return nil, false
	}
	nt, ok := c.obj.ConstantPool[dynamic.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
	if !ok || nt == nil {
		return nil, false
	}
	name, named := sourceBridgeUTF8(c.obj, nt.NameIndex)
	descriptor, typed := sourceBridgeUTF8(c.obj, nt.DescriptorIndex)
	args, result, err := callbinding.Descriptor(descriptor)
	if !named || !typed || err != nil || len(args) != p.factoryCaptureCount() || len(result) < 3 || result[0] != 'L' || result[len(result)-1] != ';' {
		return nil, false
	}
	owner := result[1 : len(result)-1]
	declaration, known := c.FuncCtx.InvocationMetadata(owner)
	if !known || strings.ReplaceAll(declaration.Name, ".", "/") != owner || !declaration.Public || !declaration.IsInterface || !declaration.MembersComplete || !declaration.ParentsComplete || strings.HasPrefix(declaration.Signature, "<") || len(declaration.Methods) > 4096 || !nativeProofWork(c.Work, int64(len(declaration.Methods))+1) {
		return nil, false
	}
	signature := ""
	found := 0
	for _, method := range declaration.Methods {
		if method.Name == name && method.Desc == p.samDescriptor && method.Public && !method.Static && !method.Bridge {
			signature = method.Signature
			found++
		}
	}
	if found != 1 || signature == "" || len(signature) > 65535 || !nativeProofWork(c.Work, int64(len(signature))+1) || c.Work != nil && c.Work.CheckAlloc(int64(len(signature))*16+int64(len(binding.refs))*128) != nil {
		return nil, false
	}
	erased, _, valid := types.EraseConcreteMethodSignatureWithThrows(signature)
	if !valid || erased != p.samDescriptor {
		return nil, false
	}
	formals, ret := types.ParseMethodSignature(signature)
	if ret == nil || len(formals) != len(binding.refs) {
		return nil, false
	}
	_, returnDescriptor, err := callbinding.Descriptor(p.samDescriptor)
	if err != nil {
		return nil, false
	}
	actualReturn, known := values.SourceTypeErasure(ret, c.FuncCtx)
	if !known || actualReturn != returnDescriptor {
		return nil, false
	}
	for i, formal := range formals {
		if formal == nil {
			return nil, false
		}
		actual, known := values.SourceTypeErasure(formal, c.FuncCtx)
		if !known || actual != p.parameters[len(p.captures)+i] {
			return nil, false
		}
	}
	return formals, true
}

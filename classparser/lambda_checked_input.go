package javaclassparser

import (
	"bytes"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// An unbound reference adapter can accept Object although the instantiated
// SAM accepts a narrower receiver. Its original CHECKCAST belongs to that
// adapter's invocation, not to factory creation. An artifact may use an arrow
// with the instantiated input only if completion restores this checked entry.
// This initial packet has one reference input, one static forward and one
// reference result. It grants no general parameter/descriptor rewriting.
type lambdaCheckedInputAdapter struct {
	method                     *MemberInfo
	code                       *CodeAttribute
	bytes                      []byte
	owner, generatedDescriptor string
	member                     values.JavaClassMember
	checked                    bool
}

func lambdaCheckedInputPacket(object *ClassObject, method *MemberInfo, instantiated string, original bool, work *workbudget.Budget) *lambdaCheckedInputAdapter {
	if object == nil || method == nil || object.MajorVersion != 52 || object.MinorVersion != 0 || method.AccessFlags != 0x100a || !nativeProofWork(work, 1) {
		return nil
	}
	desc, known := sourceBridgeUTF8(object, method.DescriptorIndex)
	params, result, err := callbinding.Descriptor(desc)
	actual, returned, e := callbinding.Descriptor(instantiated)
	if !known || err != nil || e != nil || len(params) != 1 || len(actual) != 1 || result != returned || !strings.HasPrefix(result, "L") || !strings.HasPrefix(actual[0], "L") || actual[0] == "Ljava/lang/Object;" {
		return nil
	}
	if original && params[0] != "Ljava/lang/Object;" || !original && params[0] != actual[0] {
		return nil
	}
	metadata, valid := readLambdaParameterMetadata(object, method, work)
	if !valid || metadata != nil || !sourceLambdaOrdinaryFactoryOnly(object, method, nil, work) {
		return nil
	}
	code := constructorLoweringCode(method)
	if code == nil || len(code.ExceptionTable) != 0 || code.MaxStack != 1 || code.MaxLocals != 1 || len(code.Code) > 8 || !nativeAccessorCodeMetadata(object, code, params, work) {
		return nil
	}
	decoder := core.NewDecompiler(code.Code, nil)
	decoder.Work = work
	if decoder.ParseOpcode() != nil {
		return nil
	}
	ops := constructorMotionOps(decoder)
	if len(ops) != 3 && len(ops) != 4 || !enumArrayInstructionSpans(code.Code, ops) || !nativeEnumOpcode(ops[0], core.OP_ALOAD_0) || !nativeEnumOpcode(ops[len(ops)-1], core.OP_ARETURN) {
		return nil
	}
	owner := actual[0][1 : len(actual[0])-1]
	checked := len(ops) == 4
	if original && !checked {
		return nil
	}
	if checked {
		index, ok := nativeEnumCPIndex(ops[1])
		name, named := sourceBridgeClassName(object, index)
		if !ok || ops[1].Instr.OpCode != core.OP_CHECKCAST || !named || name != owner {
			return nil
		}
	}
	member := constructorMotionMember(object, ops[len(ops)-2], core.OP_INVOKESTATIC)
	if member == nil || member.Description != instantiated || member.Member == "<init>" || member.Member == "<clinit>" || !nativeProofWork(work, int64(len(code.Code))) || work != nil && work.CheckAlloc(int64(len(code.Code))+256) != nil {
		return nil
	}
	return &lambdaCheckedInputAdapter{method: method, code: code, bytes: bytes.Clone(code.Code), owner: owner, generatedDescriptor: instantiated, member: *member, checked: checked}
}

func (c *ClassObjectDumper) proveLambdaCheckedInputAdapter(name, descriptor string, request methodSourceRequest) *lambdaCheckedInputAdapter {
	if c == nil || c.obj == nil || !c.artifactLambdaMetadataAllowed() || c.options.sourceConstructorLowering == nil || request.role != methodSourceLambdaBody || request.captureCount != 0 || !request.factory.closed() || !request.factory.sealed || request.factory.object != c.obj || len(c.obj.Methods) > 4096 {
		return nil
	}
	site, known := c.originalCompilerReferenceSite(request.factory)
	if !known || site.captureCount != 0 || site.implementation.Name != c.obj.GetClassName() || site.implementation.Member != name || site.implementation.Description != descriptor {
		return nil
	}
	var method *MemberInfo
	for _, m := range c.obj.Methods {
		if m == nil || !nativeProofWork(c.Work, 1) {
			return nil
		}
		n, nk := sourceBridgeUTF8(c.obj, m.NameIndex)
		d, dk := sourceBridgeUTF8(c.obj, m.DescriptorIndex)
		if !nk || !dk {
			return nil
		}
		if n == name && d == descriptor {
			if method != nil {
				return nil
			}
			method = m
		}
	}
	return lambdaCheckedInputPacket(c.obj, method, site.actual, true, c.Work)
}

func (c *ClassObjectDumper) lambdaCheckedInputRequestClosed(name, descriptor string, request methodSourceRequest) bool {
	p := request.checkedInput
	q := c.proveLambdaCheckedInputAdapter(name, descriptor, request)
	return p != nil && q != nil && p.method == q.method && p.code == q.code && p.owner == q.owner && p.generatedDescriptor == q.generatedDescriptor && p.member == q.member && bytes.Equal(p.bytes, q.bytes)
}

// Reopen the real factory instead of matching an unused MethodHandle constant.
// Single-site ownership and the ordinary metafactory protocol are required.
func lambdaCheckedInputFactory(object *ClassObject, name, descriptor string, work *workbudget.Budget) (*lambdaSourceFactory, compilerReferenceSite, bool) {
	var found *lambdaSourceFactory
	var result compilerReferenceSite
	if object == nil || len(object.Methods) > 4096 {
		return nil, result, false
	}
	c := NewClassObjectDumper(object)
	c.Work = work
	bytesLeft := 1 << 20
	for _, method := range object.Methods {
		if method == nil {
			return nil, result, false
		}
		for _, attr := range method.Attributes {
			code, ok := attr.(*CodeAttribute)
			if !ok {
				continue
			}
			if code == nil || len(code.Code) > bytesLeft || !nativeProofWork(work, int64(len(code.Code))+1) {
				return nil, result, false
			}
			bytesLeft -= len(code.Code)
			decoder := core.NewDecompiler(code.Code, nil)
			decoder.Work = work
			if decoder.ParseOpcode() != nil {
				return nil, result, false
			}
			for _, op := range constructorMotionOps(decoder) {
				if op.Instr.OpCode != core.OP_INVOKEDYNAMIC {
					continue
				}
				factory := &lambdaSourceFactory{object: object, method: method, code: code, pc: int(op.CurrentOffset)}
				if !factory.seal(work) {
					return nil, result, false
				}
				site, known := c.originalCompilerReferenceSite(factory)
				if !known || site.implementation.Name != object.GetClassName() || site.implementation.Member != name || site.implementation.Description != descriptor {
					continue
				}
				if found != nil || site.captureCount != 0 {
					return nil, result, false
				}
				found, result = factory, site
			}
		}
	}
	return found, result, found != nil
}

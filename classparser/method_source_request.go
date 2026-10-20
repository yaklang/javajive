package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/internal/workbudget"
)

type methodSourceRole uint8

const (
	methodSourceDeclaration methodSourceRole = iota
	methodSourceLambdaBody
	methodSourceCompilerReference
	methodSourceLambdaDepthLimit   = 64
	methodSourceLambdaRequestLimit = 4096
)

// This request identifies the source representation being rendered. It grants
// no allocation, invocation, metadata, or source-ownership permission.
// The actual factory callback supplies captureCount; physical parameter names
// and method-name conventions cannot establish that source role or its arity.
type methodSourceRequest struct {
	role         methodSourceRole
	captureCount int
	serial       uint64
	factory      *lambdaSourceFactory
}

func (r methodSourceRequest) valid() bool {
	switch r.role {
	case methodSourceDeclaration:
		return r.captureCount == 0 && r.serial == 0 && r.factory == nil
	case methodSourceCompilerReference:
		return r.captureCount == 1 && r.serial != 0 && r.factory.closed() && r.factory.sealed
	case methodSourceLambdaBody:
		return r.captureCount >= 0 && r.captureCount <= 255 && r.serial != 0 && (r.factory == nil || r.factory.closed())
	default:
		return false
	}
}

// A factory body also depends on its current lexical namespace. Even a nil
// native ownership site and identical capture arity do not prove two callback
// contexts interchangeable. Never share a packet across callback invocations.
func (c *ClassObjectDumper) newLambdaMethodSourceRequest(captures int) (methodSourceRequest, bool) {
	if c == nil || captures < 0 || captures > 255 || c.lambdaDepth >= methodSourceLambdaDepthLimit || c.methodSourceRequestSerial >= methodSourceLambdaRequestLimit || !nativeProofWork(c.Work, 1) || c.Work != nil && c.Work.CheckAlloc(128) != nil {
		return methodSourceRequest{}, false
	}
	c.methodSourceRequestSerial++
	return methodSourceRequest{role: methodSourceLambdaBody, captureCount: captures, serial: c.methodSourceRequestSerial}, true
}

func (c *ClassObjectDumper) dumpMethodWithInitialId(methodName, desc string, id *coreutils.VariableId, adapter *core.LambdaReferenceAdapter) (*dumpedMethods, error) {
	return c.dumpMethodWithSourceRequest(methodName, desc, id, adapter, methodSourceRequest{role: methodSourceDeclaration})
}

type lambdaMethodSourceReceipt struct {
	object  *ClassObject
	method  *MemberInfo
	request methodSourceRequest
	key     string
	body    *dumpedMethods
	// Optional completed render observations for private-access lowering.
	// A cached template containing capture placeholders is not final source.
	renderedTemplate       string
	renderedPackets        []string
	renderedBytes          int
	renderedUnknown        bool
	captureProjections     []*lambdaCaptureProjection
	captureCompositions    []*lambdaCaptureComposition
	captureProjectionBytes int
}

// The render cache also contains ordinary declarations. Source consumption
// requires a completed body from this dumper's actual lambda request instead.
func (c *ClassObjectDumper) lambdaMethodSourcePacketClosed(method *MemberInfo, body *dumpedMethods, work *workbudget.Budget) bool {
	if c == nil || c.obj == nil || len(c.obj.Methods) > 4096 || len(c.obj.ConstantPool) > 65535 || method == nil || !isSyntheticMethod(method.AccessFlags) || method.AccessFlags&(0x0020|0x0040|0x0100|0x0400) != 0 || body == nil || body.member != method || body.sourceDumper != c || body.sourceObject != c.obj || body.sourceRequest.role != methodSourceLambdaBody && body.sourceRequest.role != methodSourceCompilerReference || !body.sourceRequest.valid() || body.sourceRequest.serial > c.methodSourceRequestSerial || body.sourceCacheKey == "" || c.dumpedMethodsSet[body.sourceCacheKey] != body || body.sourceReconstructionFailed() {
		return false
	}
	if body.sourceRequest.role == methodSourceCompilerReference {
		if !c.compilerReferenceSourcePacketClosed(body) {
			return false
		}
	} else if body.compilerReference != nil || !c.lambdaParameterSourcePacketClosed(body) {
		return false
	}
	owned := 0
	name, named := sourceBridgeUTF8(c.obj, method.NameIndex)
	descriptor, typed := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
	if !named || !typed {
		return false
	}
	for _, original := range c.obj.Methods {
		if original == nil || !nativeProofWork(work, 1) {
			return false
		}
		n, nk := sourceBridgeUTF8(c.obj, original.NameIndex)
		d, dk := sourceBridgeUTF8(c.obj, original.DescriptorIndex)
		if !nk || !dk {
			return false
		}
		if n == name && d == descriptor {
			if original != method {
				return false
			}
			owned++
		}
	}
	if body.sourceRequest.role == methodSourceCompilerReference {
		return owned == 1 && c.lambdaOriginalDeclarationUsesClosed(method, work)
	}
	return owned == 1 && c.lambdaBodyOriginalUseClosed(method, work)
}

func (c *ClassObjectDumper) recordLambdaMethodSourceBody(name, descriptor string, request methodSourceRequest, body *dumpedMethods) bool {
	if body == nil || body.sourceRequest != request || !c.lambdaMethodSourcePacketClosed(body.member, body, c.Work) {
		return false
	}
	originalName, named := sourceBridgeUTF8(c.obj, body.member.NameIndex)
	originalDescriptor, typed := sourceBridgeUTF8(c.obj, body.member.DescriptorIndex)
	if !named || !typed || originalName != name || originalDescriptor != descriptor || c.Work != nil && c.Work.CheckAlloc(128) != nil {
		return false
	}
	if c.lambdaMethodSourceBodies == nil {
		c.lambdaMethodSourceBodies = map[*MemberInfo]*lambdaMethodSourceReceipt{}
	}
	c.lambdaMethodSourceBodies[body.member] = &lambdaMethodSourceReceipt{object: c.obj, method: body.member, request: request, key: body.sourceCacheKey, body: body}
	return true
}

func (c *ClassObjectDumper) lambdaMethodSourceBody(method *MemberInfo, work *workbudget.Budget) *dumpedMethods {
	if c == nil || method == nil {
		return nil
	}
	receipt := c.lambdaMethodSourceBodies[method]
	if receipt == nil || receipt.object != c.obj || receipt.method != method || receipt.body == nil || receipt.body.sourceRequest != receipt.request || receipt.key != receipt.body.sourceCacheKey || !c.lambdaMethodSourcePacketClosed(method, receipt.body, work) {
		return nil
	}
	return receipt.body
}

// Read the actual original declaration anew. A global lambda marker, an arity
// match, or the source request role cannot make an ordinary method a body.
func (c *ClassObjectDumper) originalLambdaDeclaration(owner, name, descriptor string) core.LambdaOriginalDeclaration {
	if c == nil || c.obj == nil || len(c.obj.Methods) > 4096 || len(c.obj.ConstantPool) > 65535 || core.NormalizeBootstrapOwner(owner) != core.NormalizeBootstrapOwner(c.obj.GetClassName()) {
		return core.LambdaDeclarationUnknown
	}
	var original *MemberInfo
	for _, method := range c.obj.Methods {
		if method == nil || !nativeProofWork(c.Work, 1) {
			return core.LambdaDeclarationUnknown
		}
		n, nk := sourceBridgeUTF8(c.obj, method.NameIndex)
		d, dk := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
		if !nk || !dk {
			return core.LambdaDeclarationUnknown
		}
		if n == name && d == descriptor {
			if original != nil {
				return core.LambdaDeclarationUnknown
			}
			original = method
		}
	}
	if original == nil || name == "<init>" || name == "<clinit>" {
		return core.LambdaDeclarationUnknown
	}
	if !isSyntheticMethod(original.AccessFlags) {
		return core.LambdaDeclarationOrdinary
	}
	// These modifiers have invocation or declaration effects that copying the
	// bytecode body into an arrow does not reproduce.
	if original.AccessFlags&(0x0020|0x0040|0x0100|0x0400) != 0 {
		return core.LambdaDeclarationUnknown
	}
	codes := 0
	for _, attribute := range original.Attributes {
		if code, ok := attribute.(*CodeAttribute); ok {
			if code == nil {
				return core.LambdaDeclarationUnknown
			}
			codes++
		}
	}
	if codes != 1 || !c.lambdaOriginalDeclarationUsesClosed(original, c.Work) {
		return core.LambdaDeclarationUnknown
	}
	return core.LambdaDeclarationSyntheticBody
}

package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
)

// A static implementation method is not the lexical source context of its
// lambda. This query recognizes only a receiver-free implementation owned by
// one actual metafactory in a constructor before THIS initialization. It grants
// no anonymous ownership, capture movement or permission to publish source.
func (c *ClassObjectDumper) nativeAnonymousPreinitLambdaContext(child *ClassObject, lexical string, metadata callbinding.Provider) bool {
	if c == nil || c.obj == nil || child == nil || c.options.SourceCompiler != NativeJavac8 || c.options.TargetSourceVersion != 8 || c.obj.MajorVersion < 51 || c.obj.MajorVersion > 52 || child.MajorVersion != c.obj.MajorVersion || len(lexical) > 2*65535 || len(c.obj.Methods) > 4096 || !nativeProofWork(c.Work, int64(len(c.obj.Methods))+1) {
		return false
	}
	owner, original, known := originalAnonymousOwner(child)
	if !known || owner != c.obj.GetClassName() || original != lexical {
		return false
	}
	var implementation *MemberInfo
	var scanned int64
	for _, declaration := range c.obj.Methods {
		if declaration == nil || !nativeProofWork(c.Work, 1) {
			return false
		}
		name, nk := sourceBridgeUTF8(c.obj, declaration.NameIndex)
		desc, dk := sourceBridgeUTF8(c.obj, declaration.DescriptorIndex)
		if !nk || !dk {
			return false
		}
		cost := int64(len(name)) + int64(len(desc)) + 1
		if cost > (128<<20)-scanned || !nativeProofWork(c.Work, cost) {
			return false
		}
		scanned += cost
		if len(name)+len(desc) == len(lexical) && lexical[:len(name)] == name && lexical[len(name):] == desc {
			if implementation != nil {
				return false
			}
			implementation = declaration
		}
	}
	if implementation == nil || implementation.AccessFlags != 0x100a {
		return false
	}
	view := &nativeMemberClass{object: c.obj, lambdaContext: nativeLambdaImplementationContext{resolve: c.nativeAnnotationDeclarationResolver(), metadata: metadata}}
	if !nativeMemberLambdaImplementation(view, implementation, c.Work) {
		return false
	}
	sites := view.lambdaContext.factorySites[lexical]
	if len(sites) != 1 || sites[0] == nil || sites[0].method == nil || sites[0].code == nil {
		return false
	}
	site := sites[0]
	name, named := sourceBridgeUTF8(c.obj, site.method.NameIndex)
	if !named || name != "<init>" || site.method.AccessFlags&(StaticFlag|0x0400|0x0100) != 0 {
		return false
	}
	ir, frames, known := c.nativeOriginalMethodSnapshot(site.method, site.code)
	if !known {
		return false
	}
	for _, frame := range frames.Instructions {
		if !nativeProofWork(c.Work, 1) {
			return false
		}
		if int(frame.PC) != site.pc {
			continue
		}
		op, known := ir.InstrByID(methodir.InstrID(frame.PC))
		if !known || op.Opcode != core.OP_INVOKEDYNAMIC || !frame.Before.ThisUninitialized {
			return false
		}
		params, _, err := callbinding.Descriptor(op.Desc)
		start := len(frame.Before.Stack) - nativeMemberParameterWidth(params)
		if err != nil || start < 0 || !nativeProofWork(c.Work, int64(len(frame.Before.Stack)-start)+1) {
			return false
		}
		for _, value := range frame.Before.Stack[start:] {
			if value.Kind == frametransfer.UninitThis || value.Kind == frametransfer.UninitNew {
				return false
			}
		}
		return true
	}
	return false
}

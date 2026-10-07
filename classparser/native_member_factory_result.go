package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// A direct original member factory returns a fresh child capturing this exact
// lexical instance. Its enclosing type arguments are consequently implicit in
// the source result. Printing those outer formals in an inner shadowing scope
// would rebind equally spelled names. Signature text alone cannot establish
// that receiver: a factory capturing a parameter/field needs its full type.
func (c *ClassObjectDumper) nativeMemberImplicitFactoryResultSource(method *MemberInfo, result types.JavaType) (string, bool) {
	if c == nil || c.obj == nil || c.FuncCtx == nil || method == nil || method.AccessFlags&(8|0x1000|0x0040|0x0100|0x0400) != 0 {
		return "", false
	}
	p := c.nativeMemberRoot
	if p == nil || p.failed || p.lexicalObjects[c.obj.GetClassName()] != c.obj {
		return "", false
	}
	pt, known := types.AsParameterizedType(result)
	if !known || pt == nil || len(pt.OwnerSegments) < 2 || len(pt.OwnerSegments) > 64 {
		return "", false
	}
	binary := strings.ReplaceAll(pt.RawClassName, ".", "/")
	child := p.children[binary]
	if child == nil || child.static || child.owner != c.obj.GetClassName() || child.object != p.lexicalObjects[binary] {
		return "", false
	}
	owner, name, flags, owned := originalMemberOwner(child.object)
	if !owned || owner != child.owner || name != child.name || flags != child.flags || flags&8 != 0 {
		return "", false
	}
	found := 0
	for _, original := range c.obj.Methods {
		if !nativeProofWork(c.Work, 1) {
			return "", false
		}
		if original == method {
			found++
		}
	}
	if found != 1 {
		return "", false
	}
	desc, known := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
	if !known || desc != "()L"+binary+";" {
		return "", false
	}
	signature, known := nativeMethodLocalOriginalSignature(c.obj, method.Attributes, c.Work)
	if !known || signature == "" || len(signature) > 4096 {
		return "", false
	}
	signatures, path, known := nativeMemberOriginalLexicalScope(c.obj, p.lexicalObjects, c.Work)
	if !known || len(path)+1 != len(pt.OwnerSegments) {
		return "", false
	}
	if c.Work != nil && c.Work.CheckAlloc(int64(len(signature))*256) != nil {
		return "", false
	}
	prefix, expected := "()L", path[0]
	for i, sig := range signatures {
		if i > 0 {
			expected += "$" + path[i]
		}
		if strings.ReplaceAll(pt.OwnerSegments[i].BinaryName, ".", "/") != expected {
			return "", false
		}
		if i == 0 {
			prefix += path[0]
		} else {
			prefix += "." + path[i]
		}
		names := types.ClassFormalTypeParamNames(sig)
		if len(names) > 0 {
			prefix += "<"
			for _, n := range names {
				prefix += "T" + n + ";"
			}
			prefix += ">"
		}
	}
	prefix += "." + child.name
	// Preserve concrete/wildcard/formal child arguments; only the implicit
	// original receiver's owner prefix is replaced. No substring name heuristic.
	if !strings.HasPrefix(signature, prefix) || signature[len(prefix):] == "" {
		return "", false
	}
	tail := signature[len(prefix):]
	if tail[0] != '<' && tail[0] != ';' {
		return "", false
	}
	if !nativeMethodLocalBindingBudget(signatures, signature, c.Work) {
		return "", false
	}
	erased, throws, known := types.EraseLexicalOwnerMethodSignatureWithThrows(signatures, signature)
	if !known || erased != desc || !nativeOriginalSignatureThrows(c.obj, method, throws, c.Work) {
		return "", false
	}
	_, params, originalResult := types.ParseMethodSignatureFull(signature, c.FuncCtx)
	originalType, known := types.AsParameterizedType(originalResult)
	if !known || len(params) != 0 || originalType.RawClassName != pt.RawClassName || len(originalType.OwnerSegments) != len(pt.OwnerSegments) {
		return "", false
	}
	pt = originalType
	var code *CodeAttribute
	for _, a := range method.Attributes {
		switch a := a.(type) {
		case *RuntimeVisibleTypeAnnotationsAttribute, *TypeAnnotationsAttribute:
			return "", false
		case *UnparsedAttribute:
			if a != nil && strings.Contains(a.Name, "TypeAnnotations") {
				return "", false
			}
		}
		if v, ok := a.(*CodeAttribute); ok {
			if v == nil || code != nil {
				return "", false
			}
			code = v
		}
	}
	if code == nil || len(code.ExceptionTable) != 0 || len(code.Code) > 32 || code.MaxLocals != 1 || code.MaxStack != 3 || !nativeProofWork(c.Work, int64(len(code.Code))) {
		return "", false
	}
	decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(c.obj.ConstantPool, i) })
	decoder.Work = c.Work
	if decoder.ParseOpcode() != nil {
		return "", false
	}
	ops := constructorMotionOps(decoder)
	if len(ops) != 5 || ops[0].Instr.OpCode != core.OP_NEW || ops[1].Instr.OpCode != core.OP_DUP || ops[2].Instr.OpCode != core.OP_ALOAD_0 || ops[4].Instr.OpCode != core.OP_ARETURN {
		return "", false
	}
	allocated, known := sourceBridgeClassName(c.obj, core.Convert2bytesToInt(ops[0].Data))
	invoke := constructorMotionMember(c.obj, ops[3], core.OP_INVOKESPECIAL)
	if !known || allocated != binary || invoke == nil || invoke.Name != binary || invoke.Member != "<init>" || invoke.Description != "(L"+c.obj.GetClassName()+";)V" {
		return "", false
	}
	ctor := child.constructors[invoke.Description]
	if ctor == nil || ctor.sourceDescriptor != "()V" {
		return "", false
	}
	last := pt.OwnerSegments[len(pt.OwnerSegments)-1]
	short := &types.JavaParameterizedType{RawClassName: child.name, TypeArgs: last.TypeArgs}
	return short.String(c.FuncCtx), true
}

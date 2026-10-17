package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// javac can capture a member constructor's hidden enclosing parameter again
// when an anonymous allocation's arguments use that parameter. Its source
// spelling is already qualified THIS, not a new local named after val$....
// Preserve that binding only for the exact constructor parameter certificate;
// ordinary captures still require their own stable local name and declaration.
func (c *ClassObjectDumper) nativeCaptureEnclosingParameter(child *nativeAnonymousClass, field string, ref, parameter *values.JavaRef) bool {
	if c == nil || c.obj == nil || c.FuncCtx == nil || c.CurrentMethod == nil || child == nil || child.object == nil || c.nativeAnonymousRoot == nil || !nativeProofWork(c.Work, 4) {
		return false
	}
	ctx, member := c.FuncCtx, c.nativeMemberCurrent
	if member == nil || member.static || member.object != c.obj || member.field == "" || field != "val$"+member.field || ref == nil || ref != parameter || parameter != c.nativeConstructorEnclosing || ctx.FunctionName != "<init>" || ctx.IsStatic || strings.ReplaceAll(ctx.ClassName, ".", "/") != c.obj.GetClassName() {
		return false
	}
	group := c.nativeAnonymousRoot
	if group.owner != c.obj.GetClassName() || group.children[child.object.GetClassName()] != child {
		return false
	}
	if _, present := child.fields[field]; !present {
		return false
	}
	ctor := member.constructors[ctx.CurrentMethodDesc]
	if ctor == nil || ctor.descriptor != ctx.CurrentMethodDesc || !nativeMemberSuperCaptureDeclaration(member, c.Work) {
		return false
	}
	name, nk := sourceBridgeUTF8(c.obj, c.CurrentMethod.NameIndex)
	desc, dk := sourceBridgeUTF8(c.obj, c.CurrentMethod.DescriptorIndex)
	if !nk || !dk || name != "<init>" || desc != ctx.CurrentMethodDesc {
		return false
	}
	declarations := 0
	for _, method := range c.obj.Methods {
		if method == nil || !nativeProofWork(c.Work, 1) {
			return false
		}
		n, nk := sourceBridgeUTF8(c.obj, method.NameIndex)
		d, dk := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
		if !nk || !dk {
			return false
		}
		if n == name && d == desc {
			if method != c.CurrentMethod {
				return false
			}
			declarations++
		}
	}
	if declarations != 1 {
		return false
	}
	proof := &nativeMemberEnclosingParameter{member: member, method: c.CurrentMethod, constructor: ctor, descriptor: desc}
	return c.nativeMemberConstructorEnclosingOperand(ref, proof, ctx)
}

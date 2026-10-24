package javaclassparser

import "github.com/yaklang/javajive/classparser/decompiler/core/class_context"

// This proves a naming domain, not lexical ownership or permission to remove
// hidden arguments. Only top-level declarations and named static members have
// source formals in the same positions as every physical constructor formal.
// Local/anonymous/enum, record, and nonstatic member constructors keep their
// separate projection. Record canonical formals belong to the component
// header and can carry compiler-owned mandated flags. A dollar in an
// identifier conveys no declaration role.
func (c *ClassObjectDumper) constructorHasOnlySourceParameters(method *MemberInfo, descriptor string) bool {
	if c == nil || c.obj == nil || method == nil || c.obj.AccessFlags & ^uint16(0x0431) != 0 || method.AccessFlags & ^uint16(0x0887) != 0 || len(descriptor) < 3 || descriptor[len(descriptor)-1] != 'V' {
		return false
	}
	owner, valid := sourceBridgeClassName(c.obj, c.obj.ThisClass)
	if !valid || !nativeProofWork(c.Work, int64(len(owner))+1) {
		return false
	}
	selfRows, tables := 0, 0
	for _, attribute := range c.obj.Attributes {
		if !nativeProofWork(c.Work, 1) {
			return false
		}
		switch a := attribute.(type) {
		case *UnparsedAttribute:
			if a == nil || a.Name == "EnclosingMethod" || a.Name == "InnerClasses" || a.Name == "Record" {
				return false
			}
		case *InnerClassesAttribute:
			if a == nil || tables != 0 || len(a.Classes) > 65535 || int(a.NumberOfClasses) != len(a.Classes) || a.AttrLen != uint32(2+8*len(a.Classes)) {
				return false
			}
			tables++
			for _, row := range a.Classes {
				if row == nil || !nativeProofWork(c.Work, 1) {
					return false
				}
				name, known := sourceBridgeClassName(c.obj, row.InnerClassInfoIndex)
				if !known || !nativeProofWork(c.Work, int64(len(name))+1) {
					return false
				}
				if name != owner {
					continue
				}
				selfRows++
				outer, outerKnown := sourceBridgeClassName(c.obj, row.OuterClassInfoIndex)
				inner, innerKnown := sourceBridgeUTF8(c.obj, row.InnerNameIndex)
				if selfRows != 1 || !outerKnown || !innerKnown || !nativeProofWork(c.Work, int64(len(outer)+len(inner))+1) || row.InnerClassAccessFlags&8 == 0 || row.InnerClassAccessFlags & ^uint16(0x041f) != 0 || !class_context.PreservesJava8Identifier(inner) || owner != outer+"$"+inner {
					return false
				}
			}
		}
	}
	return true
}

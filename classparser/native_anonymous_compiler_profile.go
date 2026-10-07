package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

func nativeAnonymousSelfAccessFlags(obj *ClassObject, expected uint16, work *workbudget.Budget) bool {
	if obj == nil {
		return false
	}
	matches := 0
	for _, attribute := range obj.Attributes {
		if table, ok := attribute.(*InnerClassesAttribute); ok {
			if table == nil {
				return false
			}
			for _, row := range table.Classes {
				if row == nil || !nativeProofWork(work, 1) {
					return false
				}
				name, known := sourceBridgeClassName(obj, row.InnerClassInfoIndex)
				if !known {
					return false
				}
				if name == obj.GetClassName() {
					if row.OuterClassInfoIndex != 0 || row.InnerNameIndex != 0 || row.InnerClassAccessFlags != expected {
						return false
					}
					matches++
				}
			}
		}
	}
	return matches == 1
}

// The same Java8 source is lowered differently by native javac8 and modern
// javac --release 8. In a static method, native javac8 emits a final anonymous
// class, a static self InnerClasses row, and no constructor Exceptions
// attribute. Merely accepting ACC_FINAL under the modern profile would admit
// metadata that the selected compiler cannot regenerate.
func (c *ClassObjectDumper) nativeAnonymousConstructorForCompiler(obj *ClassObject, owner, method, assertionRoot string, members *nativeMemberFamily, forest *nativeAnonymousForest, metadata callbinding.Provider, access map[string]*nativeConstructorAccessBridge) *nativeAnonymousClass {
	if c == nil || c.options.SourceCompiler.validate(c.options.TargetSourceVersion) != nil {
		return nil
	}
	if c.options.SourceCompiler != NativeJavac8 {
		return nativeAnonymousConstructorWithDeclarations(obj, owner, method, assertionRoot, c.Work, members, forest, metadata, access)
	}
	// Native javac8 emits the same anonymous ownership/capture profile when
	// its input source is Java7 or Java8. The selected output remains Java8.
	// Admit only the independently compiled major51/52 domains, with matching
	// original owner/child versions. Version membership does not replace the
	// exact owning method, self row, constructor and capture proofs below.
	if c.options.TargetSourceVersion != 8 || c.obj == nil || obj == nil || c.obj.GetClassName() != owner || (c.obj.MajorVersion != 51 && c.obj.MajorVersion != 52) || obj.MajorVersion != c.obj.MajorVersion || c.obj.MinorVersion != 0 || obj.MinorVersion != 0 || method == "" {
		return nil
	}
	originalOwner, originalMethod, known := originalAnonymousOwner(obj)
	if !known || originalOwner != owner || originalMethod != method {
		return nil
	}
	static, matches := false, 0
	for _, declaration := range c.obj.Methods {
		if declaration == nil || !nativeProofWork(c.Work, 1) {
			return nil
		}
		name, nk := sourceBridgeUTF8(c.obj, declaration.NameIndex)
		desc, dk := sourceBridgeUTF8(c.obj, declaration.DescriptorIndex)
		if !nk || !dk {
			return nil
		}
		if name+desc == method {
			static = declaration.AccessFlags&8 != 0
			matches++
		}
	}
	if matches != 1 {
		return nil
	}
	expected, selfFlags := uint16(0x0020), uint16(0)
	if static {
		expected, selfFlags = 0x0030, 0x0008
	}
	if !nativeAnonymousSelfAccessFlags(obj, selfFlags, c.Work) {
		return nil
	}
	if static {
		for _, declaration := range obj.Methods {
			if declaration == nil || !nativeProofWork(c.Work, 1) {
				return nil
			}
			name, known := sourceBridgeUTF8(obj, declaration.NameIndex)
			if !known {
				return nil
			}
			if name == "<init>" {
				for _, attribute := range declaration.Attributes {
					if _, exceptions := attribute.(*ExceptionsAttribute); exceptions {
						return nil
					}
				}
			}
		}
	}
	packet := nativeAnonymousConstructorWithFlags(obj, owner, method, assertionRoot, c.Work, members, forest, metadata, expected, access)
	if packet == nil || static != (packet.enclosingField == "") {
		return nil
	}
	return packet
}

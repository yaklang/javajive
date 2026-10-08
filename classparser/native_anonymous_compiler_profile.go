package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
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
		packet := nativeAnonymousConstructorWithDeclarations(obj, owner, method, assertionRoot, c.Work, members, forest, metadata, access)
		if packet != nil {
			// Moving an older stored capture to a modern source target can
			// erase the field even though the physical constructor survives.
			// Keep it only when actual use or complete serializable ancestry
			// establishes that the selected lowering retains it.
			if packet.enclosingField != "" && c.options.TargetSourceVersion >= 18 && obj.MajorVersion < 62 && !nativeAnonymousEnclosingFieldRead(obj, packet.enclosingField, owner, c.Work) {
				serializable, known := nativeAnonymousSerialization(obj, metadata, c.Work)
				if !known || !serializable {
					return nil
				}
			}
			return packet
		}
		// JDK18+ retains the hidden enclosing constructor parameter while
		// omitting its unused field. Older output targets must regenerate the
		// field and cannot preserve that original declaration shape.
		if c.options.TargetSourceVersion < 18 || c.obj == nil || obj == nil || c.obj.GetClassName() != owner || obj.MajorVersion < 62 || obj.MajorVersion != c.obj.MajorVersion || obj.MinorVersion != 0 || c.obj.MinorVersion != 0 || !nativeAnonymousSelfAccessFlags(obj, 0, c.Work) {
			return nil
		}
		static, known := nativeAnonymousOriginalContext(c.obj, obj, method, c.Work)
		if !known || static || !nativeAnonymousNonserializable(obj, metadata, c.Work) {
			return nil
		}
		return nativeAnonymousConstructorWithRoles(obj, owner, method, assertionRoot, c.Work, members, forest, metadata, 0x0020, true, access)
	}
	// Native javac8 emits the same anonymous ownership/capture profile when
	// its input source is Java7 or Java8. The selected output remains Java8.
	// Admit only the independently compiled major51/52 domains, with matching
	// original owner/child versions. Version membership does not replace the
	// exact owning method, self row, constructor and capture proofs below.
	if c.options.TargetSourceVersion != 8 || c.obj == nil || obj == nil || c.obj.GetClassName() != owner || (c.obj.MajorVersion != 51 && c.obj.MajorVersion != 52) || obj.MajorVersion != c.obj.MajorVersion || c.obj.MinorVersion != 0 || obj.MinorVersion != 0 {
		return nil
	}
	originalOwner, originalMethod, known := originalAnonymousOwner(obj)
	if !known || originalOwner != owner || originalMethod != method {
		return nil
	}
	static, known := nativeAnonymousOriginalContext(c.obj, obj, method, c.Work)
	if !known {
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

// javac intentionally retains enclosing fields for Serializable descendants.
// Missing ancestry cannot establish that the selected compiler will omit one.
func nativeAnonymousNonserializable(obj *ClassObject, metadata callbinding.Provider, work *workbudget.Budget) bool {
	serializable, known := nativeAnonymousSerialization(obj, metadata, work)
	return known && !serializable
}

func nativeAnonymousSerialization(obj *ClassObject, metadata callbinding.Provider, work *workbudget.Budget) (bool, bool) {
	if obj == nil || metadata == nil {
		return false, false
	}
	active, done := map[string]bool{}, map[string]bool{}
	var visit func(string) (bool, bool)
	visit = func(name string) (bool, bool) {
		if active[name] || !nativeProofWork(work, 1) {
			return false, false
		}
		if name == "java/io/Serializable" {
			return true, true
		}
		if value, known := done[name]; known {
			return value, true
		}
		if work != nil {
			if work.Enter(workbudget.CounterASTDepth) != nil {
				return false, false
			}
			defer work.Leave(workbudget.CounterASTDepth)
		}
		declaration, known := metadata(name)
		if !known || declaration.Name != name || !declaration.ParentsComplete {
			return false, false
		}
		active[name] = true
		defer delete(active, name)
		serializable := false
		for _, parent := range declaration.Parents {
			value, known := visit(parent)
			if !known {
				return false, false
			}
			serializable = serializable || value
		}
		done[name] = serializable
		return serializable, true
	}
	return visit(obj.GetClassName())
}

// A stored enclosing capture must actually be read if a later source compiler
// is expected to retain it without the serialization exception. Merely seeing
// the constructor's synthetic store does not establish a source-level use.
func nativeAnonymousEnclosingFieldRead(obj *ClassObject, field, owner string, work *workbudget.Budget) bool {
	if obj == nil {
		return false
	}
	found := false
	for _, method := range obj.Methods {
		if method == nil || !nativeProofWork(work, 1) {
			return false
		}
		bodies := 0
		for _, attribute := range method.Attributes {
			if !nativeProofWork(work, 1) {
				return false
			}
			code, ok := attribute.(*CodeAttribute)
			if !ok {
				continue
			}
			bodies++
			if code == nil || bodies != 1 || !nativeProofWork(work, int64(len(code.Code))) {
				return false
			}
			decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
			decoder.Work = work
			if decoder.ParseOpcode() != nil {
				return false
			}
			for _, op := range constructorMotionOps(decoder) {
				if !nativeProofWork(work, 1) {
					return false
				}
				if ref := constructorMotionMember(obj, op, core.OP_GETFIELD); ref != nil && ref.Name == obj.GetClassName() && ref.Member == field && ref.Description == "L"+owner+";" {
					found = true
				}
			}
		}
	}
	return found
}

// EnclosingMethod.method_index == 0 distinguishes an initializer from a
// constructor body, but does not specify whether that initializer is static.
// Recover that fact from the original allocation and matching constructor call.
// Multiple physical copies require a separate common-initializer proof.
func nativeAnonymousOriginalContext(owner, child *ClassObject, lexical string, work *workbudget.Budget) (bool, bool) {
	if owner == nil || child == nil {
		return false, false
	}
	enclosing, method, known := originalAnonymousOwner(child)
	if !known || enclosing != owner.GetClassName() || method != lexical {
		return false, false
	}
	constructor, constructors := "", 0
	for _, declaration := range child.Methods {
		if declaration == nil || !nativeProofWork(work, 1) {
			return false, false
		}
		name, nk := sourceBridgeUTF8(child, declaration.NameIndex)
		desc, dk := sourceBridgeUTF8(child, declaration.DescriptorIndex)
		if !nk || !dk {
			return false, false
		}
		if name == "<init>" {
			constructor = desc
			constructors++
		}
	}
	if constructors != 1 {
		return false, false
	}
	static, matches := false, 0
	for _, declaration := range owner.Methods {
		if declaration == nil || !nativeProofWork(work, 1) {
			return false, false
		}
		name, nk := sourceBridgeUTF8(owner, declaration.NameIndex)
		desc, dk := sourceBridgeUTF8(owner, declaration.DescriptorIndex)
		if !nk || !dk {
			return false, false
		}
		if lexical != "" && name+desc == lexical {
			static = declaration.AccessFlags&8 != 0
			matches++
		}
		if lexical != "" {
			continue
		}
		for _, attribute := range declaration.Attributes {
			code, ok := attribute.(*CodeAttribute)
			if !ok {
				continue
			}
			if code == nil || !nativeProofWork(work, int64(len(code.Code))) {
				return false, false
			}
			decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(owner.ConstantPool, i) })
			decoder.Work = work
			if decoder.ParseOpcode() != nil {
				return false, false
			}
			news, calls := 0, 0
			for _, op := range constructorMotionOps(decoder) {
				if !nativeProofWork(work, 1) {
					return false, false
				}
				if op.Instr.OpCode == core.OP_NEW {
					if len(op.Data) != 2 {
						return false, false
					}
					n, known := sourceBridgeClassName(owner, core.Convert2bytesToInt(op.Data))
					if !known {
						return false, false
					}
					if n == child.GetClassName() {
						news++
					}
				}
				if ref := constructorMotionMember(owner, op, core.OP_INVOKESPECIAL); ref != nil && ref.Name == child.GetClassName() && ref.Member == "<init>" {
					if ref.Description != constructor {
						return false, false
					}
					calls++
				}
			}
			if news == 0 && calls == 0 {
				continue
			}
			isStatic := declaration.AccessFlags&StaticFlag != 0
			if news != 1 || calls != 1 || name != "<init>" && name != "<clinit>" || isStatic != (name == "<clinit>") || name == "<clinit>" && desc != "()V" {
				return false, false
			}
			static = isStatic
			matches++
		}
	}
	return static, matches == 1
}

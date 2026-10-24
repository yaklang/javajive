package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// constructorReceiverRuntimeClass models the VM's final Object.getClass query,
// rather than treating every receiver call as a field observation. Its mirror
// predates this fresh ordinary receiver and cannot alias it; self-storage is
// therefore not a reference-field read. The call remains at its original PC,
// and the enclosing interpreter retains initialization/finalizer/failure guards.
// Object API: https://docs.oracle.com/en/java/javase/21/docs/api/java.base/java/lang/Object.html#getClass()
// Bind the complete original ancestry and exact public final native declaration;
// same-named user methods, shadows and unverifiable ancestors are not primitives.
func (c *ClassObjectDumper) constructorReceiverRuntimeClass(obj *ClassObject, member *values.JavaClassMember, opcode int, remaining *int, aliases *constructorSelfStorageProof) bool {
	if c == nil || c.obj == nil || obj == nil || member == nil || remaining == nil || aliases == nil || !aliases.freshReceiver || c.obj.GetClassName() == "java/lang/Class" || !c.constructorReceiverFinalizerSilent || opcode != core.OP_INVOKEVIRTUAL || member.Member != "getClass" || member.Description != "()Ljava/lang/Class;" {
		return false
	}
	// Bootstrap native declarations are runtime-specific evidence. Replay this
	// path on every supported runtime rather than extending body certificates.
	// Preserve eligibility for owned parsing and local body evidence; only the
	// cross-runtime shortcut is unavailable to a VM primitive declaration.
	if c.constructorProfileEvidence != nil {
		c.constructorProfileEvidence.runtimeSpecific = true
	}
	rootName, rootSuper, rootFlags := c.obj.GetClassName(), c.obj.GetSupperClassName(), c.obj.AccessFlags
	current := c.obj
	seen := map[string]bool{}
	callerSeen, ownerSeen := false, false
	for depth := 0; depth <= 16; depth++ {
		*remaining--
		name := current.GetClassName()
		if *remaining < 0 || name == "" || name == "java/lang/Class" || seen[name] || current.AccessFlags&(0x0200|0x8000) != 0 || depth > 0 && current.AccessFlags&0x0010 != 0 || !nativeProofWork(c.Work, 1) || c.Work != nil && c.Work.CheckAlloc(int64(len(seen)+1)*64) != nil {
			return false
		}
		seen[name] = true
		callerSeen = callerSeen || name == obj.GetClassName()
		ownerSeen = ownerSeen || name == member.Name
		target, known := c.constructorReceiverOwnMethod(current, "getClass", "()Ljava/lang/Class;", remaining)
		if !known {
			return false
		}
		if name == "java/lang/Object" {
			if !callerSeen || !ownerSeen || target == nil || target.AccessFlags != 0x0111 {
				return false
			}
			for _, a := range target.Attributes {
				if _, ok := a.(*CodeAttribute); ok {
					return false
				}
			}
			// Providers can mutate the caller-owned root while ancestors are
			// resolved. Repeat its mutable absence query after the last read.
			return current.SuperClass == 0 && c.obj.GetClassName() == rootName && c.obj.GetSupperClassName() == rootSuper && c.obj.AccessFlags == rootFlags && constructorReceiverRootMethodAbsent(c.obj, member.Member, member.Description, remaining, c.Work)
		}
		if target != nil {
			return false
		}
		var ok bool
		current, ok = c.constructorMotionClass(current.GetSupperClassName())
		if !ok {
			return false
		}
	}
	return false
}

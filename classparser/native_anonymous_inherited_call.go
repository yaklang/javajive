package javaclassparser

import (
	"encoding/binary"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
	"slices"
	"strings"
)

// A javac method reference may name the anonymous allocation class even when
// the called method is inherited. Source regenerates the anonymous receiver;
// no spelling of its binary class is needed for a virtual call. Match the
// original parameter family at each ancestor, rather than treating arity as
// evidence for source ownership. Argument conversion/binding is still proved
// separately by the source invocation machinery.
// Prove the original class-method lookup, rather than allowing arbitrary calls
// into suppressed classes. Unknown/interface/generic lookup remains refused.
func nativeAnonymousInheritedCall(forest *nativeAnonymousForest, caller *ClassObject, op *core.OpCode, work *workbudget.Budget, resolvers ...func(string) (*ClassObject, bool)) bool {
	if forest == nil || caller == nil || op == nil || op.Instr == nil || op.Instr.OpCode != core.OP_INVOKEVIRTUAL || len(op.Data) != 2 {
		return false
	}
	index := int(binary.BigEndian.Uint16(op.Data))
	if index < 1 || index > len(caller.ConstantPool) {
		return false
	}
	ref, ok := caller.ConstantPool[index-1].(*ConstantMethodrefInfo)
	if !ok || ref == nil {
		return false
	}
	symbol := constructorMotionMember(caller, op, core.OP_INVOKEVIRTUAL)
	if symbol == nil || symbol.Member == "<init>" || symbol.Member == "<clinit>" {
		return false
	}
	child := forest.units[symbol.Name]
	if child == nil || child.object == nil {
		return false
	}
	if !nativeProofWork(work, int64(len(symbol.Description))+1) || work != nil && work.CheckAlloc(int64(len(symbol.Description))*64+128) != nil {
		return false
	}
	params, _, err := callbinding.Descriptor(symbol.Description)
	if err != nil {
		return false
	}
	seen := map[string]bool{}
	current := child.object
	for depth := 0; current != nil && depth < 64; depth++ {
		name := current.GetClassName()
		if seen[name] || current.AccessFlags&0x0200 != 0 || len(current.Interfaces) != 0 || !nativeProofWork(work, int64(len(current.Methods)+1)) {
			return false
		}
		if work != nil && work.CheckAlloc(128) != nil {
			return false
		}
		seen[name] = true
		var target *MemberInfo
		for _, m := range current.Methods {
			if m == nil {
				return false
			}
			mn, known := sourceBridgeUTF8(current, m.NameIndex)
			if !known {
				return false
			}
			if mn != symbol.Member {
				continue
			}
			md, known := sourceBridgeUTF8(current, m.DescriptorIndex)
			if !known {
				return false
			}
			if !nativeProofWork(work, int64(len(md))+1) || work != nil && work.CheckAlloc(int64(len(md))*64+128) != nil {
				return false
			}
			ps, _, err := callbinding.Descriptor(md)
			if err != nil {
				return false
			}
			if !slices.Equal(ps, params) {
				continue
			}
			// Return-only alternatives/bridges cannot establish the source binding.
			if target != nil || md != symbol.Description {
				return false
			}
			target = m
		}
		if target != nil {
			if current == child.object || target.AccessFlags&(0x0002|0x0008|0x1000|0x0040|0x0400) != 0 {
				return false
			}
			// ACC_VARARGS is a source declaration property, not a different
			// JVM lookup or permission to expand an argument. The exact physical
			// descriptor above still selects this inherited class declaration.
			// Java also admits an ellipsis method in fixed-arity phases
			// (JLS 15.12.2.2/3). Keep its final array slot and let the existing
			// invocation binder independently preserve arguments/overloads;
			// this ownership proof never grants array spreading permission.
			if target.AccessFlags&0x0080 != 0 && (len(params) == 0 || !strings.HasPrefix(params[len(params)-1], "[")) {
				return false
			}
			if target.AccessFlags&0x0001 == 0 && (target.AccessFlags&0x0004 != 0 || nativeAnonymousCallPackage(name) != nativeAnonymousCallPackage(caller.GetClassName())) {
				return false
			}
			if target.AccessFlags&0x0001 == 0 {
				// Package access is lost at the first foreign-package subclass;
				// returning to the declaration's package cannot inherit it again.
				// JVM resolution alone does not establish source method lookup.
				if !nativeProofWork(work, int64(len(seen))) {
					return false
				}
				for receiver := range seen {
					if nativeAnonymousCallPackage(receiver) != nativeAnonymousCallPackage(name) {
						return false
					}
				}
			}
			for _, a := range target.Attributes {
				if _, ok := a.(*SignatureAttribute); ok {
					return false
				}
			}
			return true
		}
		parent := current.GetSupperClassName()
		current = forest.objects[parent]
		if current == nil && len(resolvers) == 1 && resolvers[0] != nil {
			// An original superclass declaration need not be one of the source
			// scopes emitted by this forest. Resolve its declaration separately;
			// never promote it into the forest's owned object namespace.
			var known bool
			current, known = resolvers[0](parent)
			if !known || current == nil || current.GetClassName() != parent {
				return false
			}
		}
	}
	return false
}
func nativeAnonymousCallPackage(name string) string {
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		return name[:i]
	}
	return ""
}

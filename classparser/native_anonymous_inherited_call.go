package javaclassparser

import (
	"encoding/binary"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
)

// A javac method reference may name the anonymous allocation class even when
// the called method is inherited. Source regenerates the anonymous receiver;
// no spelling of its binary class is needed for a zero-argument virtual call.
// Prove the original class-method lookup, rather than allowing arbitrary calls
// into suppressed classes. Unknown/interface/generic lookup remains refused.
func nativeAnonymousInheritedCall(forest *nativeAnonymousForest, caller *ClassObject, op *core.OpCode, work *workbudget.Budget) bool {
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
	params, _, err := callbinding.Descriptor(symbol.Description)
	if err != nil || len(params) != 0 {
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
			ps, _, err := callbinding.Descriptor(md)
			if err != nil {
				return false
			}
			if len(ps) != 0 {
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
	}
	return false
}
func nativeAnonymousCallPackage(name string) string {
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		return name[:i]
	}
	return ""
}

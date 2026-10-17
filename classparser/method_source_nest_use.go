package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

func (c *ClassObjectDumper) lambdaBodyOriginalUseClosed(method *MemberInfo, work *workbudget.Budget) bool {
	if c == nil || c.obj == nil {
		return false
	}
	if hasOriginalNestAttribute(c.obj, "NestHost") || hasOriginalNestAttribute(c.obj, "NestMembers") {
		var objects map[string]*ClassObject
		if family := c.nativeMemberRoot; family != nil && !family.failed && family.modernNestObjects[c.obj.GetClassName()] == c.obj {
			objects = family.modernNestObjects
		} else {
			var closed bool
			objects, closed = c.nativeModernNestOriginalScope()
			if !closed {
				return false
			}
		}
		if !nativeLambdaBodyNestUsesClosed(c.obj, method, objects, work) {
			return false
		}
	}
	return nativeLambdaBodyOriginalUseClosed(c.obj, method, work)
}

// A private implementation is visible to its entire original modern nest.
// Reuse the existing reciprocal original scope, then close every peer's CP and
// Code uses. This grants no lexical membership or source publication permission.
// As in nativeMemberLambdaArchiveClosed, even an unused foreign symbolic
// reference cannot be borrowed as an unproved declaration-consumption receipt.
func nativeLambdaBodyNestUsesClosed(object *ClassObject, method *MemberInfo, objects map[string]*ClassObject, work *workbudget.Budget) bool {
	if object == nil || method == nil || objects[object.GetClassName()] != object || len(objects) < 2 || len(objects) > 129 || !nativeModernNestSourceScopeClosed(objects, objects, work) {
		return false
	}
	owner := object.GetClassName()
	name, named := sourceBridgeUTF8(object, method.NameIndex)
	descriptor, typed := sourceBridgeUTF8(object, method.DescriptorIndex)
	if !named || !typed {
		return false
	}
	constants, methods, bytes, textBytes := 0, 0, 0, 0
	for identity, peer := range objects {
		if peer == nil || identity != peer.GetClassName() || !nativeModernSourceNestVersion(peer, work) || len(peer.ConstantPool) > 65535-constants || len(peer.Methods) > 4096-methods {
			return false
		}
		constants += len(peer.ConstantPool)
		methods += len(peer.Methods)
		for _, item := range peer.ConstantPool {
			if !nativeProofWork(work, 1) {
				return false
			}
			if text, ok := item.(*ConstantUtf8Info); ok {
				if text == nil || len(text.Value) > (8<<20)-textBytes {
					return false
				}
				textBytes += len(text.Value)
				if !nativeProofWork(work, int64(len(text.Value))) {
					return false
				}
			}
			if peer == object {
				continue
			}
			ref := nativeConstantMember(item)
			if ref == nil {
				continue
			}
			callee, ok := sourceBridgeClassName(peer, ref.ClassIndex)
			if !ok {
				return false
			}
			if callee != owner {
				continue
			}
			if ref.NameAndTypeIndex == 0 || int(ref.NameAndTypeIndex) > len(peer.ConstantPool) {
				return false
			}
			nt, ok := peer.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
			if !ok || nt == nil {
				return false
			}
			n, nk := sourceBridgeUTF8(peer, nt.NameIndex)
			d, dk := sourceBridgeUTF8(peer, nt.DescriptorIndex)
			if !nk || !dk || n == name && d == descriptor {
				return false
			}
		}
		for _, member := range peer.Methods {
			if member == nil || !nativeProofWork(work, 1) {
				return false
			}
			codes := 0
			for _, attribute := range member.Attributes {
				code, ok := attribute.(*CodeAttribute)
				if !ok {
					continue
				}
				if code == nil || len(code.Code) > (1<<20)-bytes {
					return false
				}
				bytes += len(code.Code)
				codes++
				if codes > 1 || !nativeProofWork(work, int64(len(code.Code))) || work != nil && work.CheckAlloc(int64(len(code.Code))*32) != nil {
					return false
				}
				if peer == object {
					continue
				} // The own-use query scans this Code.
				decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(peer.ConstantPool, i) })
				decoder.Work = work
				if decoder.ParseOpcode() != nil {
					return false
				}
				for _, op := range constructorMotionOps(decoder) {
					if !nativeProofWork(work, 1) || !nativeLambdaExclusiveOpcodeUse(peer, owner, name, descriptor, nil, op) {
						return false
					}
				}
			}
		}
	}
	return true
}

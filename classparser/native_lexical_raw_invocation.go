package javaclassparser

import (
	"encoding/binary"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
)

// Qualified-this is implicitly parameterized in Java. An original enclosing
// field is raw in the JVM; that view must survive member selection, or the
// outer class's formals can incorrectly flow into independent inner binders.
// Only an actual virtual call and a closed class-bound method erasure qualify.
// Method formals, nonpublic access and interface lookup need different proofs.
func nativeLexicalRawInvocation(caller *ClassObject, methodName, methodDesc string, call *values.FunctionCallExpression, read *nativeMemberLexicalRead, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) bool {
	if caller == nil || call == nil || read == nil || resolve == nil || !call.HasOriginPC || call.Kind != values.InvokeVirtual || call.IsStatic || call.IsSpecialInvoke || call.FunctionName == "<init>" || read.descriptor != "L"+strings.ReplaceAll(call.ClassName, ".", "/")+";" {
		return false
	}
	var code *CodeAttribute
	for _, m := range caller.Methods {
		if m == nil || !nativeProofWork(work, 1) {
			return false
		}
		n, nk := sourceBridgeUTF8(caller, m.NameIndex)
		d, dk := sourceBridgeUTF8(caller, m.DescriptorIndex)
		if !nk || !dk {
			return false
		}
		if n != methodName || d != methodDesc {
			continue
		}
		for _, a := range m.Attributes {
			if c, ok := a.(*CodeAttribute); ok {
				if c == nil || code != nil {
					return false
				}
				code = c
			}
		}
	}
	if code == nil || !nativeProofWork(work, int64(len(code.Code))) {
		return false
	}
	decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(caller.ConstantPool, i) })
	decoder.Work = work
	if decoder.ParseOpcode() != nil {
		return false
	}
	witnessed := false
	for _, op := range decoder.Opcodes() {
		if read.parameterOwner != "" && (methodName != "<init>" || core.GetStoreIdx(op) == 1) {
			return false
		}
		if int(op.CurrentOffset) != call.OriginPC {
			continue
		}
		if op.Instr == nil || op.Instr.OpCode != core.OP_INVOKEVIRTUAL || len(op.Data) != 2 {
			return false
		}
		idx := int(binary.BigEndian.Uint16(op.Data))
		if idx < 1 || idx > len(caller.ConstantPool) {
			return false
		}
		if c, ok := caller.ConstantPool[idx-1].(*ConstantMethodrefInfo); !ok || c == nil {
			return false
		}
		symbol := constructorMotionMember(caller, op, core.OP_INVOKEVIRTUAL)
		if symbol == nil || symbol.Name != strings.ReplaceAll(call.ClassName, ".", "/") || symbol.Member != call.FunctionName || symbol.Description != call.Descriptor {
			return false
		}
		witnessed = true
	}
	if !witnessed {
		return false
	}
	seen := map[string]bool{}
	name := strings.ReplaceAll(call.ClassName, ".", "/")
	for depth := 0; depth < 64; depth++ {
		if seen[name] || !nativeProofWork(work, 1) || work != nil && work.CheckAlloc(int64(len(seen)+1)*128) != nil {
			return false
		}
		seen[name] = true
		obj, known := resolve(name)
		if !known || obj == nil || obj.GetClassName() != name || obj.AccessFlags&0x0200 != 0 {
			return false
		}
		var target *MemberInfo
		for _, m := range obj.Methods {
			if m == nil || !nativeProofWork(work, 1) {
				return false
			}
			n, nk := sourceBridgeUTF8(obj, m.NameIndex)
			d, dk := sourceBridgeUTF8(obj, m.DescriptorIndex)
			if !nk || !dk {
				return false
			}
			if n == call.FunctionName && d == call.Descriptor {
				if target != nil {
					return false
				}
				target = m
			}
		}
		if target != nil {
			if target.AccessFlags&1 == 0 || target.AccessFlags&(0x0008|0x1000|0x0040) != 0 {
				return false
			}
			classSig, ck := nativeLexicalOriginalSignature(obj, obj.Attributes, work)
			methodSig, mk := nativeLexicalOriginalSignature(obj, target.Attributes, work)
			if !ck || !mk || classSig == "" || methodSig == "" || !nativeProofWork(work, int64(len(classSig)+len(methodSig))*130) || work != nil && work.CheckAlloc(int64(len(classSig)+len(methodSig))*256) != nil {
				return false
			}
			erased, throws, known := types.EraseClassBoundMethodSignatureWithThrows(classSig, methodSig)
			return known && erased == call.Descriptor && nativeOriginalSignatureThrows(obj, target, throws, work)
		}
		name = obj.GetSupperClassName()
		if name == "" {
			return false
		}
	}
	return false
}
func nativeLexicalOriginalSignature(obj *ClassObject, attrs []AttributeInfo, work *workbudget.Budget) (string, bool) {
	result := ""
	for _, a := range attrs {
		if !nativeProofWork(work, 1) {
			return "", false
		}
		if s, ok := a.(*SignatureAttribute); ok {
			if s == nil || result != "" {
				return "", false
			}
			text, known := sourceBridgeUTF8(obj, s.SignatureIndex)
			if !known || text == "" || len(text) > 4096 {
				return "", false
			}
			result = text
		}
	}
	return result, true
}

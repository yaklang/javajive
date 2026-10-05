package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
)

func nativeOriginalClassSignature(obj *ClassObject, work *workbudget.Budget) (string, bool) {
	if obj == nil {
		return "", false
	}
	signature := ""
	for _, a := range obj.Attributes {
		if !nativeProofWork(work, 1) {
			return "", false
		}
		if s, ok := a.(*SignatureAttribute); ok {
			if s == nil || signature != "" {
				return "", false
			}
			raw, known := sourceBridgeUTF8(obj, s.SignatureIndex)
			if !known || raw == "" || !nativeProofWork(work, int64(len(raw))) {
				return "", false
			}
			signature = raw
		}
	}
	return signature, signature != ""
}

// The generated receiver is an explicit raw cast of the original accessor
// owner. A generic raw class erases inherited class-generic methods. A fixed,
// nongeneric subclass or an implicitly parameterized enclosing owner cannot
// use that rule. Method formals need their own inference/binding proof.
func nativeMemberRawInheritedCall(obj, targetOwner *ClassObject, targetSignature, descriptor string, target *MemberInfo, work *workbudget.Budget) bool {
	if obj == nil || targetOwner == nil || obj == targetOwner || !nativeProofWork(work, int64(len(targetSignature))+1) {
		return false
	}
	for _, a := range obj.Attributes {
		if !nativeProofWork(work, 1) {
			return false
		}
		switch a := a.(type) {
		case *UnparsedAttribute:
			if a != nil && a.Name == "EnclosingMethod" {
				return false
			}
		case *InnerClassesAttribute:
			if a == nil {
				return false
			}
			for _, row := range a.Classes {
				if row == nil || !nativeProofWork(work, 1) {
					return false
				}
				name, known := sourceBridgeClassName(obj, row.InnerClassInfoIndex)
				if !known {
					return false
				}
				if name == obj.GetClassName() && row.InnerClassAccessFlags&8 == 0 {
					return false
				}
			}
		}
	}
	rootSig, known := nativeOriginalClassSignature(obj, work)
	if !known {
		return false
	}
	own, refs, valid := types.SignatureTypeVariableReferences(rootSig)
	_, closedRootBounds := types.EraseClassBoundMethodSignature(rootSig, "()V")
	if !valid || !closedRootBounds || len(own) == 0 || !strings.HasPrefix(rootSig, "<") {
		return false
	}
	scope := map[string]bool{}
	for _, name := range own {
		scope[name] = true
	}
	for _, name := range refs {
		if !scope[name] {
			return false
		}
	}
	super, interfaces := types.ParseClassSignatureSupers(rootSig)
	raw, valid := types.RawClassFQN(super)
	if !valid || strings.ReplaceAll(raw, ".", "/") != obj.GetSupperClassName() || len(interfaces) != len(obj.Interfaces) {
		return false
	}
	for i, it := range interfaces {
		raw, known := types.RawClassFQN(it)
		name, valid := sourceBridgeClassName(obj, obj.Interfaces[i])
		if !known || !valid || strings.ReplaceAll(raw, ".", "/") != name {
			return false
		}
	}
	declaringSig, known := nativeOriginalClassSignature(targetOwner, work)
	if !known {
		return false
	}
	erased, signatureThrows, known := types.EraseClassBoundMethodSignatureWithThrows(declaringSig, targetSignature)
	if !known || erased != descriptor || target == nil {
		return false
	}
	if len(signatureThrows) != 0 {
		var exceptions *ExceptionsAttribute
		for _, a := range target.Attributes {
			if !nativeProofWork(work, 1) {
				return false
			}
			if ex, ok := a.(*ExceptionsAttribute); ok {
				if ex == nil || exceptions != nil {
					return false
				}
				exceptions = ex
			}
		}
		if exceptions == nil || len(exceptions.ExceptionIndexTable) != len(signatureThrows) {
			return false
		}
		for i, index := range exceptions.ExceptionIndexTable {
			name, known := sourceBridgeClassName(targetOwner, index)
			if !known || signatureThrows[i] != "L"+name+";" || !nativeProofWork(work, 1) {
				return false
			}
		}
	}
	return true
}

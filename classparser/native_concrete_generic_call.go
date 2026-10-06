package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A closed parameterized signature does not change the bridge's JVM reference
// contract. Source arguments retain the exact descriptor casts, so neither a
// subtype overload nor method-formal inference may replace the original target.
// Class/method variables remain outside this proof; they need a raw receiver or
// an explicit substitution certificate, not guesses from printed type names.
func nativeMemberConcretePrivateCall(owner *ClassObject, signature, descriptor string, target *MemberInfo, work *workbudget.Budget) bool {
	// The parser has a depth ceiling of 128. Charge its worst-case nested
	// argument traversals and retained metadata before doing that work.
	if owner == nil || target == nil || len(signature) > 4096 || !strings.Contains(signature, "<") || !nativeProofWork(work, int64(len(signature))*130+1) || work != nil && work.CheckAlloc(int64(len(signature))*256) != nil {
		return false
	}
	erased, throws, known := types.EraseConcreteMethodSignatureWithThrows(signature)
	if !known || erased != descriptor {
		return false
	}
	return nativeOriginalSignatureThrows(owner, target, throws, work)
}

// A private instance call on an explicit raw declaring receiver erases its
// class-owned formals to their original first bounds. This is independent of
// the caller's (possibly shadowing) lexical variables. Complete original class
// and method signatures must certify that erasure; method-owned or free outer
// variables cannot borrow this raw-class rule. No virtual target or source
// overload is selected by the erasure certificate itself.
func nativeMemberRawOwnPrivateCall(owner *ClassObject, signature, descriptor string, target *MemberInfo, work *workbudget.Budget) bool {
	if owner == nil || target == nil || target.AccessFlags&0x000a != 2 || len(signature) > 4096 {
		return false
	}
	classSignature, known := nativeOriginalClassSignature(owner, work)
	if !known || len(classSignature) > 4096 || !nativeProofWork(work, int64(len(classSignature)+len(signature))*130+1) || work != nil && work.CheckAlloc(int64(len(classSignature)+len(signature))*256) != nil {
		return false
	}
	erased, throws, known := types.EraseClassBoundMethodSignatureWithThrows(classSignature, signature)
	if !known || erased != descriptor {
		return false
	}
	// A class Signature is evidence only for this original declaration. Its
	// class/interface supers must agree with the physical constant-pool owners.
	super, interfaces := types.ParseClassSignatureSupers(classSignature)
	raw, known := types.RawClassFQN(super)
	if !known || strings.ReplaceAll(raw, ".", "/") != owner.GetSupperClassName() || len(interfaces) != len(owner.Interfaces) {
		return false
	}
	for i, it := range interfaces {
		raw, named := types.RawClassFQN(it)
		name, physical := sourceBridgeClassName(owner, owner.Interfaces[i])
		if !named || !physical || strings.ReplaceAll(raw, ".", "/") != name || !nativeProofWork(work, 1) {
			return false
		}
	}
	return nativeOriginalSignatureThrows(owner, target, throws, work)
}

func nativeOriginalSignatureThrows(owner *ClassObject, target *MemberInfo, throws []string, work *workbudget.Budget) bool {
	if len(throws) == 0 {
		return true
	}
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
	if exceptions == nil || len(exceptions.ExceptionIndexTable) != len(throws) {
		return false
	}
	for i, index := range exceptions.ExceptionIndexTable {
		name, known := sourceBridgeClassName(owner, index)
		if !known || throws[i] != "L"+name+";" || !nativeProofWork(work, 1) {
			return false
		}
	}
	return true
}

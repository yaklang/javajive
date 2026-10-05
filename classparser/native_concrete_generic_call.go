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

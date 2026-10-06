package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// An enclosing instance constructed inline has an initialized verifier type
// and retains the original NEW identity. Nominal owner equality alone cannot
// distinguish it from a different allocation, a nullable parameter or a phi.
type nativeMemberFreshEnclosing struct {
	newPC, invokePC   int
	owner, descriptor string
}

// The source operand must independently preserve both original instruction
// sites. A mutable JavaRef.Val, field, factory or arbitrary cast is not the
// actual inline NEW, even if its inferred type and printed text match.
func nativeMemberFreshEnclosingOperand(value any, proof *nativeMemberFreshEnclosing, work *workbudget.Budget) bool {
	if proof == nil || proof.newPC < 0 || proof.invokePC <= proof.newPC || !nativeProofWork(work, 4) {
		return false
	}
	v, ok := value.(values.JavaValue)
	if !ok || sourceProofNil(v) {
		return false
	}
	v, ok = nativeMemberEnclosingUnpack(v, work)
	if !ok {
		return false
	}
	n, ok := v.(*values.NewExpression)
	if !ok || n == nil || n.Type() == nil || n.IsArray() || !n.HasOriginPC || n.OriginPC != proof.newPC || n.ConstructorCall == nil {
		return false
	}
	owner, typed := types.RawClassFQN(n.Type())
	if !typed || strings.ReplaceAll(owner, ".", "/") != proof.owner {
		return false
	}
	call := n.ConstructorCall
	return call.HasOriginPC && call.OriginPC == proof.invokePC && call.FunctionName == "<init>" && call.Descriptor == proof.descriptor && strings.ReplaceAll(call.ClassName, ".", "/") == proof.owner && call.Kind == values.InvokeSpecial && call.IsSpecialInvoke && call.Object == n
}

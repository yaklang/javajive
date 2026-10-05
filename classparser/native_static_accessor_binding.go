package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
)

// A class qualifier can itself bind to a formal type in the current scope.
// An unqualified field is an alternative only inside the original lexical
// ownership chain, with every nearer class/hierarchy proved free of that name.
// Parameters and locals are reserved separately over their declaration IDs.
func nativeStaticAccessorQualifierShadowed(owner string, ctx *class_context.ClassContext) bool {
	if ctx == nil {
		return false
	}
	head := strings.Split(owner, ".")[0]
	for _, formal := range ctx.TypeParams {
		if head == formal {
			return true
		}
	}
	return false
}
func nativeStaticAccessorLexicalField(getter *nativeMemberPrivateGetter, current string, p *nativeMemberFamily, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) bool {
	if getter == nil || !getter.staticField || p == nil || resolve == nil || p.getters[nativeMemberGetterKey(getter.owner, getter.name, getter.descriptor)] != getter {
		return false
	}
	seen := map[string]bool{}
	for depth := 0; depth < 64; depth++ {
		if current == "" || seen[current] || !nativeProofWork(work, 1) {
			return false
		}
		seen[current] = true
		object := p.lexicalObjects[current]
		if object == nil || object.GetClassName() != current {
			return false
		}
		if current == getter.owner {
			return true
		}
		names, known := nativeAnonymousInitializerFieldNames(object, resolve, work)
		if !known || names[getter.field] {
			return false
		}
		child := p.children[current]
		if child == nil || child.object != object {
			return false
		}
		current = child.owner
	}
	return false
}

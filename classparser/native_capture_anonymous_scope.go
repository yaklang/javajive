package javaclassparser

import (
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// A producer Signature may refer to variables declared by its owning method
// or class. Reopen those physical declarations; a renderer's list of same-name
// formals or an erased Object bound cannot establish their binding. The existing
// ownership walk enforces both EnclosingMethod tables, static cuts and nearest
// declaration scope. A use of an existing variable does not redeclare its
// bounds: a method's K cannot change the old bound of an outer E extends K.
func (c *ClassObjectDumper) nativeCapturedAnonymousLexicalContext(producer *nativeAnonymousClass, signature string, references []string) (*class_context.ClassContext, bool) {
	if c == nil || c.obj == nil || c.FuncCtx == nil || producer == nil || producer.object == nil || len(references) == 0 || len(references) > 512 {
		return nil, false
	}
	owner, methodKey, original := originalAnonymousOwner(producer.object)
	group := c.nativeAnonymousRoot
	own, actualReferences, valid := types.SignatureTypeVariableReferences(signature)
	if !original || owner != c.obj.GetClassName() || producer.method != methodKey || methodKey != c.FuncCtx.FunctionName+c.FuncCtx.CurrentMethodDesc ||
		group == nil || group.failed || group.owner != owner || group.children[producer.object.GetClassName()] != producer ||
		!valid || len(own) != 0 || !slices.Equal(actualReferences, references) {
		return nil, false
	}
	reader := NewClassObjectDumper(producer.object)
	reader.Work = c.Work
	resolve := c.nativeAnnotationDeclarationResolver()
	scopes, method, known := reader.flattenedMethodLexicalScopesResolved(func(name string) (*ClassObject, bool) {
		if !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		if name == c.obj.GetClassName() {
			return c.obj, true
		}
		return resolve(name)
	})
	if !known || !method || len(scopes) > 64 || len(scopes) < 2 || scopes[len(scopes)-1].Signature != signature {
		return nil, false
	}
	signatures := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		signatures = append(signatures, scope.Signature)
	}
	if !nativeMethodLocalBindingBudget(signatures, signature, c.Work) {
		return nil, false
	}
	// These bindings are proof-local. Do not alter the live renderer, retain
	// borrowed injected erasures, or depend on a rendering policy's method hint.
	binding := class_context.ClassContext{ClassName: c.FuncCtx.ClassName, PackageName: c.FuncCtx.PackageName, Work: c.Work}
	for i := len(scopes) - 1; i >= 0; i-- {
		if scopes[i].Signature != "" {
			binding.LexicalTypeParamSignatures = append(binding.LexicalTypeParamSignatures, scopes[i].Signature)
		}
	}
	if _, valid := types.LexicalTypeParameterErasures(scopes, references); !valid {
		return nil, false
	}
	for _, name := range references {
		if !class_context.PreservesJava8Identifier(name) || !c.FuncCtx.IsTypeParam(name) || c.FuncCtx.RawEraseTypeVar(name) {
			return nil, false
		}
	}
	binding.TypeParams = slices.Clone(references)
	binding.ClassTypeParams = nil
	return &binding, nativeProofWork(c.Work, 1)
}

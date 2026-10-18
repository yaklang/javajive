package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// MethodParameters fixes these source binders. Other declarations may be
// renamed by the existing identity allocator; the fixed formals may not.
// Own static field selects already consult Arguments and retain their exact
// owner when a formal shadows the field. Other lexical capture reservations
// still require their existing proof and are not waived by parameter metadata.
func (c *ClassObjectDumper) prepareOrdinaryMethodParameterNamespace(binding *ordinaryMethodParameterNameBinding, body []statements.Statement, params []values.JavaValue) bool {
	if binding == nil {
		c.prepareNativeLocalShadowing(body, params)
		return true
	}
	if !c.ordinaryMethodParameterNamesClosed(binding, params) || !nativeProofWork(c.Work, int64(len(binding.names))+1) || c.Work != nil && c.Work.CheckAlloc(int64(len(binding.names))*128) != nil {
		return false
	}
	nodes := len(c.obj.Fields) + len(c.nativeAnonymousBindings) + len(c.nativeCaptureFields) + len(c.nativeLambdaFactoryParameterNames[c.CurrentMethod]) + len(binding.names)
	if c.nativeMemberRoot != nil {
		nodes += len(c.nativeMemberRoot.getters)
	}
	if c.nativeAnonymousForest != nil {
		for _, method := range c.nativeAnonymousForest.consumers[c.obj.GetClassName()] {
			nodes += len(method) + 1
		}
	}
	if nodes > 65536 || !nativeProofWork(c.Work, int64(nodes)*2+1) || c.Work != nil && c.Work.CheckAlloc(int64(nodes)*512+128) != nil {
		return false
	}
	reserved := c.nativeLexicalCaptures()
	lexical := c.nativeLexicalCaptureNames(false)
	protected := map[*coreutils.VariableId]bool{}
	for i, name := range binding.names {
		// An existing capture binder may already bind this exact declaration
		// ID to its original name. Parameter metadata creates no such binder.
		if lexical[name] && c.FuncCtx.LocalNames[binding.identities[i]] != name {
			return false
		}
		reserved[name] = true
		protected[binding.identities[i]] = true
	}
	return c.prepareNativeSourceNames(body, params, reserved, protected) && c.ordinaryMethodParameterNamesClosed(binding, params)
}

// Arrow parameters and local declarations cannot shadow a source formal in
// their enclosing method. Arguments already contains the enclosing method's
// original names before its bytecode is parsed. Reserve them for the existing
// identity allocator; this does not bind the implementation's physical names
// to the SAM declaration.
func (c *ClassObjectDumper) prepareLambdaEnclosingParameterNamespace(body []statements.Statement, params []values.JavaValue) bool {
	if c == nil || c.FuncCtx == nil {
		return false
	}
	names := c.FuncCtx.Arguments
	if len(names) == 0 {
		return true
	}
	if len(names) > 65536 || !nativeProofWork(c.Work, int64(len(names))+1) || c.Work != nil && c.Work.CheckAlloc(int64(len(names))*192+128) != nil {
		return false
	}
	reserved := make(map[string]bool, len(names))
	for _, name := range names {
		reserved[name] = true
	}
	return c.prepareNativeSourceNames(body, params, reserved, nil)
}

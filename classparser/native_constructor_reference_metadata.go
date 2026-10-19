package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
)

func (c *ClassObjectDumper) nativeConstructorOperandMetadata(declarations callbinding.Provider) callbinding.Provider {
	return callbinding.WithReferenceAncestry(declarations, c.buildSiblingSuperTypes(), c.Work)
}

// Declaration and operand providers are deliberately separate: newly known
// ancestry cannot make an opaque declaration look like a nongeneric class.
func (c *ClassObjectDumper) proveNativeMemberDeclaration(obj, enclosing *ClassObject, bridges map[string]*nativeConstructorAccessBridge, lexical map[string]*ClassObject, resolve func(string) (*ClassObject, bool)) *nativeMemberClass {
	declarations := c.buildInvocationMetadata()
	return nativeMemberProofWithMetadata(obj, enclosing, c.Work, bridges, lexical, resolve, declarations, c.nativeConstructorOperandMetadata(declarations), nativeEnumParameterCompiler{profile: c.options.SourceCompiler, source: c.options.TargetSourceVersion})
}

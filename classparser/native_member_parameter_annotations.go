package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

// The annotation table can use JVM descriptor arity or javac source arity.
// Only a proved non-static member constructor loses one enclosing parameter;
// ordinary method/static constructor parameters keep their original indices.
func nativeMemberParameterAnnotationsClosed(table *RuntimeVisibleParameterAnnotationsAttribute, name, descriptor string, enclosing bool, work *workbudget.Budget) bool {
	if table == nil || int(table.NumParameters) != len(table.ParameterAnnotations) {
		return false
	}
	parameters, result, err := callbinding.Descriptor(descriptor)
	if err != nil || name == "<init>" && result != "V" {
		return false
	}
	size := len(table.ParameterAnnotations)
	if name == "<init>" && enclosing && size == len(parameters) && size > 0 && len(table.ParameterAnnotations[0]) != 0 {
		// An annotation on the omitted JVM-only operand cannot be emitted as
		// a source parameter annotation on the lexical enclosing instance.
		return false
	}
	if size != len(parameters) && !(name == "<init>" && enclosing && size == len(parameters)-1) {
		return false
	}
	return nativeAnnotationDependencies([]AttributeInfo{table}, work, func(string) {})
}

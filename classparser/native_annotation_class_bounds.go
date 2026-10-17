package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Annotation elements cannot introduce type variables. A Class element may
// nevertheless constrain its literal argument by identity or a wildcard bound.
// Keep that constraint separate from the erased Class descriptor: accepting the
// latter alone neither proves a default's bound nor reproduces its generic API.
// This certificate covers closed, unparameterized reference/array bounds. A
// parameterized bound needs its own generic hierarchy proof and remains refused.
func nativeAnnotationClassSignatureMatches(desc, signature string, value *ElementValuePairAttribute, work *workbudget.Budget, resolve func(string) (*ClassObject, bool), metadata ...callbinding.Provider) bool {
	if !nativeProofWork(work, int64(len(signature))) || resolve == nil || len(metadata) > 1 {
		return false
	}
	array := desc == "[Ljava/lang/Class;"
	if desc != "Ljava/lang/Class;" && !array {
		return false
	}
	prefix := "()"
	if array {
		prefix += "["
	}
	prefix += "Ljava/lang/Class<"
	if !strings.HasPrefix(signature, prefix) || !strings.HasSuffix(signature, ">;") {
		return false
	}
	argument := signature[len(prefix) : len(signature)-2]
	variant := byte('=')
	if argument == "*" {
		variant = '*'
	} else {
		if argument != "" && (argument[0] == '+' || argument[0] == '-') {
			variant = argument[0]
			argument = argument[1:]
		}
		if !nativeAnnotationClosedReference(argument, work, resolve, metadata...) {
			return false
		}
	}
	if value == nil {
		return true
	}
	nodes := 0
	var literal func(*ElementValuePairAttribute) bool
	literal = func(v *ElementValuePairAttribute) bool {
		nodes++
		if v == nil || nodes > 4096 || !nativeProofWork(work, 1) || v.Tag != 'c' {
			return false
		}
		actual, ok := v.Value.(string)
		if !ok || !nativeAnnotationDependencies([]AttributeInfo{&AnnotationDefaultAttribute{DefaultValue: v}}, work, func(string) {}) {
			return false
		}
		if variant == '*' {
			return true
		}
		// JLS15.8.2: primitive and void literals have Class<boxed type>, while
		// primitive array literals keep their original array component type.
		if len(actual) == 1 {
			boxed := map[byte]string{'B': "Byte", 'C': "Character", 'D': "Double", 'F': "Float", 'I': "Integer", 'J': "Long", 'S': "Short", 'Z': "Boolean", 'V': "Void"}
			name := boxed[actual[0]]
			if name == "" {
				return false
			}
			actual = "Ljava/lang/" + name + ";"
		}
		if !nativeAnnotationClosedReference(actual, work, resolve, metadata...) {
			return false
		}
		if variant == '=' {
			return actual == argument
		}
		formal := argument
		if variant == '-' {
			actual, formal = formal, actual
		}
		visited := 0
		provider := func(name string) (callbinding.Class, bool) {
			visited++
			if visited > 256 || !nativeProofWork(work, 1) {
				return callbinding.Class{}, false
			}
			return nativeAnnotationOriginalDeclaration(name, work, resolve, metadata...)
		}
		return callbinding.Assignable(actual, formal, provider) && visited <= 256 && nativeProofWork(work, 1)
	}
	if !array {
		return literal(value)
	}
	if value.Tag != '[' {
		return false
	}
	elements, ok := value.Value.([]*ElementValuePairAttribute)
	if !ok || len(elements) > 4096 {
		return false
	}
	for _, element := range elements {
		if !literal(element) {
			return false
		}
	}
	return true
}

// Restrict bounds to descriptors with an original declaration identity, rather
// than treating an arbitrary Signature spelling as an erased Java class name.
func nativeAnnotationClosedReference(desc string, work *workbudget.Budget, resolve func(string) (*ClassObject, bool), metadata ...callbinding.Provider) bool {
	if resolve == nil || !nativeProofWork(work, int64(len(desc))) {
		return false
	}
	dimensions := 0
	for dimensions < len(desc) && desc[dimensions] == '[' {
		dimensions++
	}
	if dimensions > 255 || dimensions == len(desc) {
		return false
	}
	element := desc[dimensions:]
	if len(element) == 1 {
		return dimensions > 0 && strings.Contains("BCDFIJSZ", element)
	}
	if len(element) < 3 || element[0] != 'L' || element[len(element)-1] != ';' || !nativeSourceBinaryName(element[1:len(element)-1]) {
		return false
	}
	_, known := nativeAnnotationOriginalDeclaration(element[1:len(element)-1], work, resolve, metadata...)
	return known
}

// Physical declarations and an explicitly supplied metadata view must agree.
// Acquiring more original platform code must not bypass a caller's incomplete
// or contradictory evidence. Metadata-only platform views retain their bounded
// fallback; two available sources corroborate rather than replace each other.
func nativeAnnotationOriginalDeclaration(name string, work *workbudget.Budget, resolve func(string) (*ClassObject, bool), metadata ...callbinding.Provider) (callbinding.Class, bool) {
	if resolve == nil || len(metadata) > 1 || !nativeProofWork(work, 1) {
		return callbinding.Class{}, false
	}
	var declaration callbinding.Class
	if len(metadata) == 1 {
		if metadata[0] == nil {
			return callbinding.Class{}, false
		}
		var known bool
		declaration, known = metadata[0](name)
		if !known || declaration.Name != name || !declaration.ParentsComplete || len(declaration.Parents) > 256 {
			return callbinding.Class{}, false
		}
		seen := map[string]bool{}
		for _, parent := range declaration.Parents {
			if !nativeSourceBinaryName(parent) || seen[parent] || !nativeProofWork(work, 1) {
				return callbinding.Class{}, false
			}
			seen[parent] = true
		}
	}
	obj, known := resolve(name)
	if !known {
		return declaration, len(metadata) == 1
	}
	if obj == nil || obj.GetClassName() != name || len(obj.Interfaces) > 256 {
		return callbinding.Class{}, false
	}
	parents := []string{}
	if parent := obj.GetSupperClassName(); parent != "" && !(obj.AccessFlags&0x0200 != 0 && parent == "java/lang/Object") {
		if !nativeSourceBinaryName(parent) {
			return callbinding.Class{}, false
		}
		parents = append(parents, parent)
	}
	seen := map[string]bool{}
	for _, parent := range parents {
		seen[parent] = true
	}
	for _, index := range obj.Interfaces {
		parent, known := sourceBridgeClassName(obj, index)
		if !known || !nativeSourceBinaryName(parent) || seen[parent] || !nativeProofWork(work, 1) {
			return callbinding.Class{}, false
		}
		parents = append(parents, parent)
		seen[parent] = true
	}
	if len(metadata) == 1 {
		count := 0
		for _, parent := range declaration.Parents {
			// Class-file interfaces name Object as their superclass. Providers
			// may omit that implicit edge and list only superinterfaces.
			if obj.AccessFlags&0x0200 != 0 && parent == "java/lang/Object" {
				continue
			}
			count++
			if !seen[parent] {
				return callbinding.Class{}, false
			}
		}
		if count != len(parents) {
			return callbinding.Class{}, false
		}
	}
	return callbinding.Class{Name: name, Parents: parents, ParentsComplete: true}, true
}

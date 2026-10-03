package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
)

// Annotation descriptors generally live in UTF8 entries, not CONSTANT_Class.
// Include their type, enum, class-literal and nested-value edges in the original
// source-access closure. The traversal uses original parsed metadata only.
func nativeAnnotationDependencies(attrs []AttributeInfo, work *workbudget.Budget, add func(string)) bool {
	nodes := 0
	visiting := map[*AnnotationAttribute]bool{}
	step := func(depth int) bool { nodes++; return depth <= 32 && nodes <= 4096 && nativeProofWork(work, 1) }
	descriptor := func(text string, reference bool) bool {
		if !nativeProofWork(work, int64(len(text))) {
			return false
		}
		if reference && (len(text) < 3 || text[0] != 'L' || text[len(text)-1] != ';' || !nativeSourceBinaryName(text[1:len(text)-1])) {
			return false
		}
		// A class literal carries a descriptor, never a generic Signature.
		// void.class is legal, whereas void arrays and type variables are not.
		dimensions := 0
		for dimensions < len(text) && text[dimensions] == '[' {
			dimensions++
		}
		if dimensions > 255 || dimensions == len(text) {
			return false
		}
		element := text[dimensions:]
		if len(element) == 1 {
			if !strings.Contains("BCDFIJSZ", element) && (element != "V" || dimensions != 0) {
				return false
			}
		} else if len(element) < 3 || element[0] != 'L' || element[len(element)-1] != ';' || !nativeSourceBinaryName(element[1:len(element)-1]) {
			return false
		}
		refs, ok := types.SignatureClassReferences(text)
		if !ok {
			return false
		}
		for _, name := range refs {
			add(strings.ReplaceAll(name, ".", "/"))
		}
		return true
	}
	var annotation func(*AnnotationAttribute, int) bool
	var value func(*ElementValuePairAttribute, int) bool
	annotation = func(a *AnnotationAttribute, depth int) bool {
		if a == nil || visiting[a] || !step(depth) || !descriptor(a.TypeName, true) {
			return false
		}
		visiting[a] = true
		defer delete(visiting, a)
		names := map[string]bool{}
		for _, element := range a.ElementValuePairs {
			if element == nil || element.Name == "" || names[element.Name] || !value(element, depth+1) {
				return false
			}
			names[element.Name] = true
		}
		return true
	}
	value = func(v *ElementValuePairAttribute, depth int) bool {
		if v == nil || !step(depth) {
			return false
		}
		switch v.Tag {
		case 'B', 'C', 'I', 'S', 'Z':
			c, ok := v.Value.(*ConstantIntegerInfo)
			return ok && c != nil
		case 'J':
			c, ok := v.Value.(*ConstantLongInfo)
			return ok && c != nil
		case 'F':
			c, ok := v.Value.(*ConstantFloatInfo)
			return ok && c != nil
		case 'D':
			c, ok := v.Value.(*ConstantDoubleInfo)
			return ok && c != nil
		case 's':
			return annotationStringUnits(v.Value) != nil
		case 'c':
			d, ok := v.Value.(string)
			return ok && descriptor(d, false)
		case 'e':
			e, ok := v.Value.(*EnumConstValue)
			return ok && e != nil && e.ConstName != "" && descriptor(e.TypeName, true)
		case '@':
			a, ok := v.Value.(*AnnotationAttribute)
			return ok && annotation(a, depth+1)
		case '[':
			elements, ok := v.Value.([]*ElementValuePairAttribute)
			if !ok {
				return false
			}
			for _, e := range elements {
				if !value(e, depth+1) {
					return false
				}
			}
			return true
		}
		return false
	}
	var attributes func([]AttributeInfo, int) bool
	attributes = func(list []AttributeInfo, depth int) bool {
		for _, attr := range list {
			if !step(depth) {
				return false
			}
			switch a := attr.(type) {
			case *RuntimeVisibleAnnotationsAttribute:
				if a == nil {
					return false
				}
				for _, entry := range a.Annotations {
					if !annotation(entry, depth+1) {
						return false
					}
				}
			case *RuntimeVisibleParameterAnnotationsAttribute:
				if a == nil {
					return false
				}
				for _, parameter := range a.ParameterAnnotations {
					for _, entry := range parameter {
						if !annotation(entry, depth+1) {
							return false
						}
					}
				}
			case *AnnotationDefaultAttribute:
				if a == nil || !value(a.DefaultValue, depth+1) {
					return false
				}
			case *TypeAnnotationsAttribute:
				if a == nil {
					return false
				}
				for _, entry := range a.Annotations {
					if entry == nil || !annotation(entry.Annotation, depth+1) {
						return false
					}
				}
			case *RuntimeVisibleTypeAnnotationsAttribute:
				if a == nil {
					return false
				}
				for _, entry := range a.Annotations {
					if !annotation(entry, depth+1) {
						return false
					}
				}
			case *CodeAttribute:
				if a == nil || !attributes(a.Attributes, depth+1) {
					return false
				}
			case *UnparsedAttribute:
				if a == nil || strings.Contains(a.Name, "Annotation") {
					return false
				}
			}
		}
		return true
	}
	return attributes(attrs, 0)
}

package javaclassparser

import (
	"github.com/yaklang/javajive/internal/workbudget"
	"math"
	"slices"
)

// Both graphs have already passed the bounded, acyclic annotation traversal.
func sourceAnnotationGraphEqual(a, b *AnnotationAttribute, work *workbudget.Budget) bool {
	if a == nil || b == nil || a.TypeName != b.TypeName || len(a.ElementValuePairs) != len(b.ElementValuePairs) || !nativeProofWork(work, int64(len(a.ElementValuePairs))+1) {
		return false
	}
	right := map[string]*ElementValuePairAttribute{}
	for _, e := range b.ElementValuePairs {
		right[e.Name] = e
	}
	for _, e := range a.ElementValuePairs {
		if !sourceAnnotationElementEqual(e, right[e.Name], work) {
			return false
		}
	}
	return true
}

func sourceAnnotationElementEqual(a, b *ElementValuePairAttribute, work *workbudget.Budget) bool {
	if a == nil || b == nil || a.Tag != b.Tag || !nativeProofWork(work, 1) {
		return false
	}
	switch a.Tag {
	case 'B', 'C', 'I', 'S', 'Z':
		return a.Value.(*ConstantIntegerInfo).Value == b.Value.(*ConstantIntegerInfo).Value
	case 'J':
		return a.Value.(*ConstantLongInfo).Value == b.Value.(*ConstantLongInfo).Value
	case 'F':
		return math.Float32bits(a.Value.(*ConstantFloatInfo).Value) == math.Float32bits(b.Value.(*ConstantFloatInfo).Value)
	case 'D':
		return math.Float64bits(a.Value.(*ConstantDoubleInfo).Value) == math.Float64bits(b.Value.(*ConstantDoubleInfo).Value)
	case 's':
		x, y := annotationStringUnits(a.Value), annotationStringUnits(b.Value)
		return nativeProofWork(work, int64(len(x)+len(y))+1) && slices.Equal(x, y)
	case 'c':
		return a.Value.(string) == b.Value.(string)
	case 'e':
		return *a.Value.(*EnumConstValue) == *b.Value.(*EnumConstValue)
	case '@':
		return sourceAnnotationGraphEqual(a.Value.(*AnnotationAttribute), b.Value.(*AnnotationAttribute), work)
	case '[':
		x, y := a.Value.([]*ElementValuePairAttribute), b.Value.([]*ElementValuePairAttribute)
		if len(x) != len(y) {
			return false
		}
		for i := range x {
			if !sourceAnnotationElementEqual(x[i], y[i], work) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

type inheritedBridgeAnnotationSite struct {
	parameter int // -1 is the declaration; other positions are descriptor arguments.
	invisible bool
}

// Annotation tables describe the forwarding declaration, not its executable
// packet. Keep their retention, argument positions and complete value graphs
// as a separate obligation; an instruction proof cannot authorize their loss.
func inheritedBridgeAnnotationTables(method *MemberInfo, descriptor string, work *workbudget.Budget) (map[inheritedBridgeAnnotationSite]map[string]*AnnotationAttribute, bool) {
	if method == nil || len(method.Attributes) > 128 {
		return nil, false
	}
	parameters, _, err := callbinding.Descriptor(descriptor)
	if err != nil {
		return nil, false
	}
	tables := map[inheritedBridgeAnnotationSite]map[string]*AnnotationAttribute{}
	seenTable := map[inheritedBridgeAnnotationSite]bool{}
	seenType := map[int]map[string]bool{}
	add := func(position int, invisible bool, annotations []*AnnotationAttribute) bool {
		site := inheritedBridgeAnnotationSite{position, invisible}
		if seenType[position] == nil {
			seenType[position] = map[string]bool{}
		}
		for _, annotation := range annotations {
			if annotation == nil || seenType[position][annotation.TypeName] || !nativeProofWork(work, 1) {
				return false
			}
			seenType[position][annotation.TypeName] = true
			if tables[site] == nil {
				tables[site] = map[string]*AnnotationAttribute{}
			}
			tables[site][annotation.TypeName] = annotation
		}
		return true
	}
	for _, attribute := range method.Attributes {
		if !nativeProofWork(work, 1) {
			return nil, false
		}
		switch a := attribute.(type) {
		case *RuntimeVisibleAnnotationsAttribute:
			if a == nil || !nativeAnnotationDependencies([]AttributeInfo{a}, work, func(string) {}) {
				return nil, false
			}
			site := inheritedBridgeAnnotationSite{-1, a.IsInvisible}
			if seenTable[site] || !add(-1, a.IsInvisible, a.Annotations) {
				return nil, false
			}
			seenTable[site] = true
		case *RuntimeVisibleParameterAnnotationsAttribute:
			if a == nil || int(a.NumParameters) != len(parameters) || !nativeMemberParameterAnnotationsClosed(a, "bridge", descriptor, false, work) {
				return nil, false
			}
			// -2 identifies the complete parameter table, including zero arity.
			site := inheritedBridgeAnnotationSite{-2, a.IsInvisible}
			if seenTable[site] {
				return nil, false
			}
			seenTable[site] = true
			for i, annotations := range a.ParameterAnnotations {
				if !add(i, a.IsInvisible, annotations) {
					return nil, false
				}
			}
		}
	}
	return tables, true
}

func inheritedBridgeAnnotationsEqual(a *MemberInfo, ad string, b *MemberInfo, bd string, work *workbudget.Budget) bool {
	left, lok := inheritedBridgeAnnotationTables(a, ad, work)
	right, rok := inheritedBridgeAnnotationTables(b, bd, work)
	if !lok || !rok || len(left) != len(right) {
		return false
	}
	for site, annotations := range left {
		other := right[site]
		if len(annotations) != len(other) {
			return false
		}
		for name, annotation := range annotations {
			if !sourceAnnotationGraphEqual(annotation, other[name], work) {
				return false
			}
		}
	}
	return true
}

func (c *ClassObjectDumper) inheritedBridgeAnnotationsRepresentable(method *MemberInfo, descriptor string) bool {
	tables, valid := inheritedBridgeAnnotationTables(method, descriptor, c.Work)
	if !valid {
		return false
	}
	resolve := c.nativeAnnotationDeclarationResolver()
	for site, annotations := range tables {
		target := "METHOD"
		if site.parameter >= 0 {
			target = "PARAMETER"
		}
		for _, annotation := range annotations {
			name := annotation.TypeName[1 : len(annotation.TypeName)-1]
			definition, known := resolve(name)
			if !known || definition == nil || definition.GetClassName() != name || definition.AccessFlags&0x2200 != 0x2200 {
				return false
			}
			retention, allowed, valid := nativeAnnotationDeclarationPolicyForTargets(definition, c, target)
			if !valid || !allowed || retention == "SOURCE" || site.invisible != (retention == "CLASS") {
				return false
			}
		}
	}
	return true
}

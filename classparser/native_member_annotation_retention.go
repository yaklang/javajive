package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Java source regenerates a declaration annotation according to the annotation
// type's original Retention/Target, not according to the use site's table name.
// A syntactically printable value graph alone is therefore insufficient.
func (c *ClassObjectDumper) nativeMemberAnnotationTablesRepresentable() bool {
	if !nativeMemberDeprecatedDeclarationRepresentable(c.obj, c.Work) {
		return false
	}
	seen := map[string]bool{}
	for _, attribute := range c.obj.Attributes {
		if !nativeProofWork(c.Work, 1) {
			return false
		}
		table, ok := attribute.(*RuntimeVisibleAnnotationsAttribute)
		if !ok {
			continue
		}
		if table == nil || !nativeAnnotationDependencies([]AttributeInfo{table}, c.Work, func(string) {}) {
			return false
		}
		for _, annotation := range table.Annotations {
			if seen[annotation.TypeName] {
				return false
			}
			seen[annotation.TypeName] = true
			name := annotation.TypeName[1 : len(annotation.TypeName)-1]
			var raw []byte
			known := false
			if c.foldSiblingResolver != nil {
				raw, known = c.foldSiblingResolver(name)
			}
			if !known && c.declarationResolver != nil {
				raw, known = c.declarationResolver(name)
			}
			if !known {
				target := c.options.TargetSourceVersion
				if target == 0 {
					target = core.ClassMajorToSourceVersion(c.obj.MajorVersion)
				}
				raw, known = jdkConstructorClassBytes(name, target)
			}
			if !known {
				return false
			}
			definition, err := c.parseResolved(raw)
			if err != nil || definition.GetClassName() != name || definition.AccessFlags&0x2200 != 0x2200 {
				return false
			}
			retention, target, valid := nativeAnnotationDeclarationPolicy(definition, c)
			if !valid || !target || retention == "SOURCE" || table.IsInvisible != (retention == "CLASS") {
				return false
			}
		}
	}
	return true
}

func nativeAnnotationDeclarationPolicy(definition *ClassObject, c *ClassObjectDumper) (string, bool, bool) {
	if definition == nil || c == nil {
		return "", false, false
	}
	retention := "CLASS"
	target := true
	seenRetention, seenTarget := false, false
	for _, attribute := range definition.Attributes {
		if !nativeProofWork(c.Work, 1) {
			return "", false, false
		}
		table, ok := attribute.(*RuntimeVisibleAnnotationsAttribute)
		if !ok {
			continue
		}
		if table == nil || !nativeAnnotationDependencies([]AttributeInfo{table}, c.Work, func(string) {}) {
			return "", false, false
		}
		for _, annotation := range table.Annotations {
			if annotation.TypeName != "Ljava/lang/annotation/Retention;" && annotation.TypeName != "Ljava/lang/annotation/Target;" {
				continue
			}
			if table.IsInvisible || len(annotation.ElementValuePairs) != 1 || annotation.ElementValuePairs[0].Name != "value" {
				return "", false, false
			}
			value := annotation.ElementValuePairs[0]
			if annotation.TypeName == "Ljava/lang/annotation/Retention;" {
				if seenRetention || value.Tag != 'e' {
					return "", false, false
				}
				seenRetention = true
				enum, ok := value.Value.(*EnumConstValue)
				if !ok || enum == nil || enum.TypeName != "Ljava/lang/annotation/RetentionPolicy;" || enum.ConstName != "CLASS" && enum.ConstName != "RUNTIME" && enum.ConstName != "SOURCE" {
					return "", false, false
				}
				retention = enum.ConstName
			} else {
				if seenTarget || value.Tag != '[' {
					return "", false, false
				}
				seenTarget = true
				target = false
				values, ok := value.Value.([]*ElementValuePairAttribute)
				if !ok {
					return "", false, false
				}
				for _, v := range values {
					if v == nil || v.Tag != 'e' {
						return "", false, false
					}
					enum, ok := v.Value.(*EnumConstValue)
					if !ok || enum == nil || enum.TypeName != "Ljava/lang/annotation/ElementType;" {
						return "", false, false
					}
					if enum.ConstName == "TYPE" || enum.ConstName == "TYPE_USE" || enum.ConstName == "ANNOTATION_TYPE" && c.obj.AccessFlags&0x2000 != 0 {
						target = true
					}
				}
			}
		}
	}
	return retention, target, true
}

// An annotation projection needs the exact paired encoding. This deliberately
// remains distinct from the documentation projection below: adding @Deprecated
// for an unpaired marker would invent observable runtime annotation metadata.
func nativeMemberDeprecatedMarkerRepresentable(obj *ClassObject, work *workbudget.Budget) bool {
	markers, annotations, known := nativeMemberDeprecatedEncoding(obj, work)
	return known && markers <= 1 && annotations == markers
}

// A declaration's Javadoc @deprecated tag regenerates only the JVM marker.
// This is a second exact source encoding, not permission to drop the marker or
// borrow the runtime annotation's policy. Both root and nested declarations
// use the same original-attribute proof and retain their lexical source scope.
func nativeMemberDocumentationDeprecated(obj *ClassObject, work *workbudget.Budget) bool {
	markers, annotations, known := nativeMemberDeprecatedEncoding(obj, work)
	return known && markers == 1 && annotations == 0
}

func nativeMemberDeprecatedDeclarationRepresentable(obj *ClassObject, work *workbudget.Budget) bool {
	markers, annotations, known := nativeMemberDeprecatedEncoding(obj, work)
	return known && markers <= 1 && annotations <= markers
}

func nativeMemberDeprecatedEncoding(obj *ClassObject, work *workbudget.Budget) (markers, annotations int, known bool) {
	if obj == nil {
		return 0, 0, false
	}
	for _, attribute := range obj.Attributes {
		if !nativeProofWork(work, 1) {
			return 0, 0, false
		}
		switch a := attribute.(type) {
		case *DeprecatedAttribute:
			if a == nil || a.AttrLen != 0 {
				return 0, 0, false
			}
			markers++
		case *RuntimeVisibleAnnotationsAttribute:
			if a == nil {
				return 0, 0, false
			}
			for _, annotation := range a.Annotations {
				if annotation == nil || !nativeProofWork(work, 1) {
					return 0, 0, false
				}
				if annotation.TypeName == "Ljava/lang/Deprecated;" {
					if a.IsInvisible {
						return 0, 0, false
					}
					annotations++
				}
			}
		}
	}
	return markers, annotations, true
}

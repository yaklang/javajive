package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Java source regenerates a declaration annotation according to the annotation
// type's original Retention/Target, not according to the use site's table name.
// A syntactically printable value graph alone is therefore insufficient.
func (c *ClassObjectDumper) nativeMemberAnnotationTablesRepresentable() bool {
	if !nativeMemberDeprecatedMarkerRepresentable(c.obj, c.Work) {
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
					if enum.ConstName == "TYPE" || enum.ConstName == "TYPE_USE" {
						target = true
					}
				}
			}
		}
	}
	return retention, target, true
}

// javac regenerates the JVM Deprecated marker from the declaration annotation.
// An unpaired legacy documentation marker has no equivalent source annotation:
// introducing one would add runtime metadata. Preserve only the exact paired
// original encoding; never silently drop, duplicate or invent either member.
func nativeMemberDeprecatedMarkerRepresentable(obj *ClassObject, work *workbudget.Budget) bool {
	if obj == nil {
		return false
	}
	markers, annotations := 0, 0
	for _, attribute := range obj.Attributes {
		if !nativeProofWork(work, 1) {
			return false
		}
		switch a := attribute.(type) {
		case *DeprecatedAttribute:
			if a == nil || a.AttrLen != 0 {
				return false
			}
			markers++
		case *RuntimeVisibleAnnotationsAttribute:
			if a == nil {
				return false
			}
			for _, annotation := range a.Annotations {
				if annotation == nil || !nativeProofWork(work, 1) {
					return false
				}
				if annotation.TypeName == "Ljava/lang/Deprecated;" {
					if a.IsInvisible {
						return false
					}
					annotations++
				}
			}
		}
	}
	return markers <= 1 && annotations == markers
}

package javaclassparser

import (
	"math"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Resolve original declarations without making a claim about their emitted
// source ownership. A per-family cache bounds parsing and records misses too.
func (c *ClassObjectDumper) nativeAnnotationDeclarationResolver() func(string) (*ClassObject, bool) {
	cache := map[string]*ClassObject{c.obj.GetClassName(): c.obj}
	bytes := 0
	return func(name string) (*ClassObject, bool) {
		if !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		if obj, seen := cache[name]; seen {
			return obj, obj != nil
		}
		if len(cache) >= 256 {
			return nil, false
		}
		cache[name] = nil
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
		if !known || len(raw) > 2<<20 || bytes+len(raw) > 16<<20 {
			return nil, false
		}
		bytes += len(raw)
		obj, err := c.parseResolved(raw)
		if err != nil || obj == nil || obj.GetClassName() != name {
			return nil, false
		}
		cache[name] = obj
		return obj, true
	}
}

// JLS 9.6.1 gives annotations a narrower method domain than interfaces.
// In particular an element dependency graph must be acyclic, and element
// names cannot override Object/Annotation public or protected declarations.
// Use original metadata for both facts rather than a list of familiar names.
func nativeMemberAnnotationDeclaration(obj *ClassObject, flags uint16, work *workbudget.Budget, resolve func(string) (*ClassObject, bool)) bool {
	if resolve == nil || flags&0x2608 != 0x2608 || flags & ^uint16(0x260f) != 0 {
		return false
	}
	forbidden := map[string]bool{}
	for _, name := range []string{"java/lang/Object", "java/lang/annotation/Annotation"} {
		base, known := resolve(name)
		if !known {
			return false
		}
		for _, method := range base.Methods {
			if method == nil || !nativeProofWork(work, 1) {
				return false
			}
			name, nk := sourceBridgeUTF8(base, method.NameIndex)
			desc, dk := sourceBridgeUTF8(base, method.DescriptorIndex)
			if !nk || !dk {
				return false
			}
			if method.AccessFlags&5 != 0 && strings.HasPrefix(desc, "()") {
				forbidden[name] = true
			}
		}
	}
	visiting, complete := map[string]bool{}, map[string]bool{}
	nodes := 0
	var declaration func(*ClassObject, int) bool
	var domain func(string, int) bool
	domain = func(desc string, depth int) bool {
		if !nativeProofWork(work, int64(len(desc))) {
			return false
		}
		if strings.HasPrefix(desc, "[") {
			desc = desc[1:]
			if strings.HasPrefix(desc, "[") {
				return false
			}
		}
		if len(desc) == 1 {
			return strings.Contains("BCDFIJSZ", desc)
		}
		if len(desc) < 3 || desc[0] != 'L' || desc[len(desc)-1] != ';' || !nativeSourceBinaryName(desc[1:len(desc)-1]) {
			return false
		}
		name := desc[1 : len(desc)-1]
		if name == "java/lang/String" || name == "java/lang/Class" {
			return true
		}
		target, known := resolve(name)
		if !known || target.GetClassName() != name {
			return false
		}
		if target.AccessFlags&0x4000 != 0 {
			return target.GetSupperClassName() == "java/lang/Enum" && target.AccessFlags&0x2200 == 0
		}
		return declaration(target, depth+1)
	}
	declaration = func(def *ClassObject, depth int) bool {
		if def == nil || depth > 32 || !nativeProofWork(work, 1) {
			return false
		}
		name := def.GetClassName()
		if visiting[name] {
			return false
		}
		if complete[name] {
			return true
		}
		nodes++
		if nodes > 256 || def.AccessFlags&0x2600 != 0x2600 || def.AccessFlags & ^uint16(0x2601) != 0 || def.GetSupperClassName() != "java/lang/Object" || len(def.Interfaces) != 1 {
			return false
		}
		iface, known := sourceBridgeClassName(def, def.Interfaces[0])
		if !known || iface != "java/lang/annotation/Annotation" {
			return false
		}
		visiting[name] = true
		defer delete(visiting, name)
		for _, a := range def.Attributes {
			if _, generic := a.(*SignatureAttribute); generic {
				return false
			}
		}
		for _, field := range def.Fields {
			if field == nil || !nativeProofWork(work, 1) || field.AccessFlags != 0x19 {
				return false
			}
		}
		seen := map[string]bool{}
		for _, method := range def.Methods {
			if method == nil || !nativeProofWork(work, 1) {
				return false
			}
			n, nk := sourceBridgeUTF8(def, method.NameIndex)
			desc, dk := sourceBridgeUTF8(def, method.DescriptorIndex)
			if !nk || !dk || seen[n] || forbidden[n] || n == "<init>" || n != "<clinit>" && class_context.SafeIdentifier(n) != n {
				return false
			}
			seen[n] = true
			if n == "<clinit>" {
				if method.AccessFlags != 8 || desc != "()V" {
					return false
				}
				codes := 0
				for _, a := range method.Attributes {
					if code, ok := a.(*CodeAttribute); ok && code != nil {
						codes++
					}
				}
				if codes != 1 {
					return false
				}
				continue
			}
			if method.AccessFlags != 0x401 || !strings.HasPrefix(desc, "()") || !domain(desc[2:], depth) || !nativeAnnotationDependencies(method.Attributes, work, func(string) {}) {
				return false
			}
			defaults, signatures := 0, 0
			for _, a := range method.Attributes {
				if !nativeProofWork(work, 1) {
					return false
				}
				switch a := a.(type) {
				case *CodeAttribute, *ExceptionsAttribute:
					return false
				case *AnnotationDefaultAttribute:
					defaults++
					if defaults > 1 || !nativeAnnotationDefaultMatches(desc[2:], a.DefaultValue, work, resolve) {
						return false
					}
				case *SignatureAttribute:
					signatures++
					if signatures > 1 || a == nil {
						return false
					}
					sig, ok := sourceBridgeUTF8(def, a.SignatureIndex)
					// Wildcard Class elements need no lexical type variables. More
					// constrained class-literal bounds require a separate proof.
					if !ok || (desc != "()Ljava/lang/Class;" || sig != "()Ljava/lang/Class<*>;") && (desc != "()[Ljava/lang/Class;" || sig != "()[Ljava/lang/Class<*>;") {
						return false
					}
				}
			}
		}
		complete[name] = true
		return true
	}
	return declaration(obj, 0)
}

// A printable tag is not proof that a default belongs to its element's type.
// Validate the original typed value graph before javac recreates its metadata.
func nativeAnnotationDefaultMatches(desc string, value *ElementValuePairAttribute, work *workbudget.Budget, resolve func(string) (*ClassObject, bool)) bool {
	nodes := 0
	var matches func(string, *ElementValuePairAttribute, int) bool
	matches = func(desc string, v *ElementValuePairAttribute, depth int) bool {
		nodes++
		if v == nil || depth > 32 || nodes > 4096 || !nativeProofWork(work, 1) {
			return false
		}
		if strings.HasPrefix(desc, "[") {
			if v.Tag != '[' || strings.HasPrefix(desc[1:], "[") {
				return false
			}
			array, ok := v.Value.([]*ElementValuePairAttribute)
			if !ok {
				return false
			}
			for _, item := range array {
				if !matches(desc[1:], item, depth+1) {
					return false
				}
			}
			return true
		}
		if len(desc) == 1 {
			if byte(desc[0]) != v.Tag {
				return false
			}
			switch v.Tag {
			case 'B', 'C', 'I', 'S', 'Z':
				c, ok := v.Value.(*ConstantIntegerInfo)
				if !ok || c == nil {
					return false
				}
				switch v.Tag {
				case 'B':
					return c.Value >= -128 && c.Value <= 127
				case 'C':
					return c.Value >= 0 && c.Value <= 65535
				case 'S':
					return c.Value >= -32768 && c.Value <= 32767
				case 'Z':
					return c.Value == 0 || c.Value == 1
				default:
					return true
				}
			case 'J':
				c, ok := v.Value.(*ConstantLongInfo)
				return ok && c != nil
			case 'F':
				c, ok := v.Value.(*ConstantFloatInfo)
				return ok && c != nil && (!math.IsNaN(float64(c.Value)) || math.Float32bits(c.Value) == 0x7fc00000)
			case 'D':
				c, ok := v.Value.(*ConstantDoubleInfo)
				return ok && c != nil && (!math.IsNaN(c.Value) || math.Float64bits(c.Value) == 0x7ff8000000000000)
			}
			return false
		}
		if desc == "Ljava/lang/String;" {
			return v.Tag == 's' && annotationStringUnits(v.Value) != nil
		}
		if desc == "Ljava/lang/Class;" {
			_, ok := v.Value.(string)
			return v.Tag == 'c' && ok
		}
		if len(desc) < 3 || desc[0] != 'L' || desc[len(desc)-1] != ';' || resolve == nil {
			return false
		}
		definition, known := resolve(desc[1 : len(desc)-1])
		if !known || definition.GetClassName() != desc[1:len(desc)-1] {
			return false
		}
		if definition.AccessFlags&0x4000 != 0 {
			e, ok := v.Value.(*EnumConstValue)
			if v.Tag != 'e' || !ok || e == nil || e.TypeName != desc || class_context.SafeIdentifier(e.ConstName) != e.ConstName {
				return false
			}
			for _, field := range definition.Fields {
				if field == nil || !nativeProofWork(work, 1) {
					return false
				}
				name, nk := sourceBridgeUTF8(definition, field.NameIndex)
				typ, tk := sourceBridgeUTF8(definition, field.DescriptorIndex)
				if nk && tk && name == e.ConstName && typ == desc && field.AccessFlags == 0x4019 {
					return true
				}
			}
			return false
		}
		annotation, ok := v.Value.(*AnnotationAttribute)
		if v.Tag != '@' || !ok || annotation == nil || annotation.TypeName != desc || definition.AccessFlags&0x2000 == 0 {
			return false
		}
		members := map[string]string{}
		required := map[string]bool{}
		for _, method := range definition.Methods {
			if method == nil || !nativeProofWork(work, 1) {
				return false
			}
			name, nk := sourceBridgeUTF8(definition, method.NameIndex)
			typ, tk := sourceBridgeUTF8(definition, method.DescriptorIndex)
			if name == "<clinit>" {
				continue
			}
			if !nk || !tk || !strings.HasPrefix(typ, "()") || members[name] != "" {
				return false
			}
			members[name], required[name] = typ[2:], true
			for _, attr := range method.Attributes {
				if a, ok := attr.(*AnnotationDefaultAttribute); ok && a != nil {
					required[name] = false
				}
			}
		}
		seen := map[string]bool{}
		for _, pair := range annotation.ElementValuePairs {
			if pair == nil || seen[pair.Name] || members[pair.Name] == "" || !matches(members[pair.Name], pair, depth+1) {
				return false
			}
			seen[pair.Name] = true
		}
		for name, necessary := range required {
			if necessary && !seen[name] {
				return false
			}
		}
		return true
	}
	return matches(desc, value, 0)
}

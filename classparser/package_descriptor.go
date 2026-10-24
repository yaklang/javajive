package javaclassparser

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yaklang/javajive/internal/workbudget"
)

// package-info is a JVM package declaration, not a Java type identifier. Keep
// this namespace exception local to descriptor handling; ordinary class and
// member ownership proofs still require legal Java identifiers.
func packageDescriptorName(name string) bool {
	const suffix = "/package-info"
	return strings.HasSuffix(name, suffix) && nativeSourceBinaryName(strings.TrimSuffix(name, suffix))
}

func packageDescriptorAnnotations(obj *ClassObject, work *workbudget.Budget) (map[bool]map[string]*AnnotationAttribute, bool) {
	if obj == nil || !packageDescriptorName(obj.GetClassName()) || obj.AccessFlags != 0x1600 || obj.GetSupperClassName() != "java/lang/Object" || len(obj.Interfaces) != 0 || len(obj.Fields) != 0 || len(obj.Methods) != 0 {
		return nil, false
	}
	result := map[bool]map[string]*AnnotationAttribute{false: {}, true: {}}
	seenTable := map[bool]bool{}
	seenType := map[string]bool{}
	seenSource := false
	for _, attr := range obj.Attributes {
		if !nativeProofWork(work, 1) {
			return nil, false
		}
		switch a := attr.(type) {
		case *SourceFileAttribute:
			if a == nil || seenSource {
				return nil, false
			}
			seenSource = true
		case *RuntimeVisibleAnnotationsAttribute:
			if a == nil || seenTable[a.IsInvisible] || !nativeAnnotationDependencies([]AttributeInfo{a}, work, func(string) {}) {
				return nil, false
			}
			seenTable[a.IsInvisible] = true
			for _, annotation := range a.Annotations {
				if seenType[annotation.TypeName] {
					return nil, false
				}
				seenType[annotation.TypeName] = true
				result[a.IsInvisible][annotation.TypeName] = annotation
			}
		default:
			// Do not silently discard executable or unsupported declaration
			// metadata from a class merely because of its binary suffix.
			return nil, false
		}
	}
	return result, true
}

func (c *ClassObjectDumper) dumpPackageDescriptor() (string, error) {
	if _, valid := packageDescriptorAnnotations(c.obj, c.Work); !valid {
		return "", fmt.Errorf("package descriptor has unsupported declaration metadata")
	}
	var out strings.Builder
	for _, attr := range c.obj.Attributes {
		if table, ok := attr.(*RuntimeVisibleAnnotationsAttribute); ok {
			for _, annotation := range table.Annotations {
				text, err := c.DumpAnnotation(annotation)
				if err != nil {
					return "", err
				}
				out.WriteString(text)
				out.WriteByte('\n')
			}
		}
	}
	out.WriteString("package " + c.PackageName + ";\n")
	// Imports legally follow the annotated package declaration and resolve
	// annotation types, enum constants, nested annotations and class literals.
	for _, name := range c.FuncCtx.GetAllImported() {
		out.WriteString("import " + name + ";\n")
	}
	if err := c.ensureOutput(int64(out.Len())); err != nil {
		return "", err
	}
	return out.String(), nil
}

// No original bytes or executable programs are transplanted. A package
// descriptor has no members; publication accepts the fresh compiler bytes only
// when its identity, flags and annotation value graphs match the original.
// Annotation order and constant-pool indices are not semantic identity.
func finishPackageDescriptor(original, compiled []byte, work *workbudget.Budget) ([]byte, error) {
	old, err := ParseWithBudget(original, work)
	if err != nil {
		return nil, err
	}
	fresh, err := ParseWithBudget(compiled, work)
	if err != nil {
		return nil, err
	}
	left, lok := packageDescriptorAnnotations(old, work)
	right, rok := packageDescriptorAnnotations(fresh, work)
	if !lok || !rok || old.GetClassName() != fresh.GetClassName() {
		return nil, fmt.Errorf("package descriptor identity or declaration shape changed")
	}
	for _, invisible := range []bool{false, true} {
		if len(left[invisible]) != len(right[invisible]) {
			return nil, fmt.Errorf("package descriptor annotation retention changed")
		}
		for name, annotation := range left[invisible] {
			if !sourceAnnotationGraphEqual(annotation, right[invisible][name], work) {
				return nil, fmt.Errorf("package descriptor annotation value changed")
			}
		}
	}
	if work != nil {
		if err := work.CheckAlloc(int64(len(compiled))); err != nil {
			return nil, err
		}
	}
	return bytes.Clone(compiled), nil
}

package javaclassparser

import (
	"bytes"
	"encoding/binary"
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
	if obj == nil || !packageDescriptorName(obj.GetClassName()) || !packageDescriptorFlags(obj) || obj.GetSupperClassName() != "java/lang/Object" || len(obj.Interfaces) != 0 || len(obj.Fields) != 0 || len(obj.Methods) != 0 {
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
		case *InnerClassesAttribute:
			// Nested annotation, enum and class-literal dependencies have their
			// own declaration rows. They do not make package-info a Java type.
			// Validate the whole table below, then compare it at publication.
		default:
			// Do not silently discard executable or unsupported declaration
			// metadata from a class merely because of its binary suffix.
			return nil, false
		}
	}
	rows, err := sourceAnonymousInnerRows(obj, work)
	if err != nil {
		return nil, false
	}
	for inner, row := range rows {
		if !nativeProofWork(work, 1) || inner == obj.GetClassName() || row.outer == obj.GetClassName() || row.outer == "" || row.name == "" || row.anonymous {
			return nil, false
		}
	}
	return result, true
}

// JVMS 3.15 uses a synthetic interface for modern package descriptors. Older
// compilers emitted a non-synthetic interface. For pre-50 interfaces the JVM
// adds ACC_ABSTRACT while loading (ClassFileParser's compatibility rule).
// Preserve that loaded declaration at the new target version, never the
// obsolete missing-ABSTRACT encoding, and never relax ordinary class flags.
func packageDescriptorFlags(obj *ClassObject) bool {
	if obj.MajorVersion < 49 || obj.MajorVersion > 65 || obj.MinorVersion != 0 {
		return false
	}
	return obj.AccessFlags == 0x0600 || obj.AccessFlags == 0x1600 || (obj.MajorVersion < 50 && obj.AccessFlags == 0x0200)
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
// when its identity, loaded flags and annotation value graphs match the original.
// Annotation order and constant-pool indices are not semantic identity.
func finishPackageDescriptor(original, compiled []byte, work *workbudget.Budget) ([]byte, error) {
	old, err := ParseWithBudget(original, work)
	if err != nil {
		return nil, err
	}
	reader, err := NewClassReaderWithBudget(compiled, work)
	if err != nil {
		return nil, err
	}
	layout := &compiledMetadataLayout{}
	fresh, err := parseWithMetadataLayout(reader, nil, layout)
	if err != nil {
		return nil, err
	}
	left, lok := packageDescriptorAnnotations(old, work)
	right, rok := packageDescriptorAnnotations(fresh, work)
	if !lok || !rok || old.GetClassName() != fresh.GetClassName() {
		return nil, fmt.Errorf("package descriptor identity or declaration shape changed")
	}
	flags := old.AccessFlags | 0x0400
	if fresh.AccessFlags != 0x1600 && fresh.AccessFlags != flags {
		return nil, fmt.Errorf("package descriptor compiler declaration changed")
	}
	oldRows, err := sourceAnonymousInnerRows(old, work)
	if err != nil {
		return nil, err
	}
	newRows, err := sourceAnonymousInnerRows(fresh, work)
	if err != nil || len(oldRows) != len(newRows) {
		return nil, fmt.Errorf("package descriptor dependency declarations changed")
	}
	for name, left := range oldRows {
		right, exists := newRows[name]
		if !nativeProofWork(work, 1) || !exists || left.outer != right.outer || left.name != right.name || left.flags != right.flags || left.anonymous != right.anonymous {
			return nil, fmt.Errorf("package descriptor nested dependency changed")
		}
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
	if layout.poolEnd < 10 || layout.poolEnd+2 > len(compiled) || binary.BigEndian.Uint16(compiled[layout.poolEnd:]) != fresh.AccessFlags {
		return nil, fmt.Errorf("package descriptor flag span is invalid")
	}
	result := bytes.Clone(compiled)
	binary.BigEndian.PutUint16(result[layout.poolEnd:], flags)
	return result, nil
}

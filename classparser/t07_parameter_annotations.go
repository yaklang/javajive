package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

func javaAnnotationStringLiteral(v any) string {
	units := annotationStringUnits(v)
	if units != nil {
		return values.JavaUnitsToStringLiteral(units)
	}
	return values.JavaStringToLiteral(v)
}

func utf16UnitsEqual(a, b []uint16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func javaUtf8IndexToStringLiteral(obj *ClassObject, index uint16) string {
	if obj == nil {
		return values.JavaStringToLiteral("")
	}
	units, err := obj.getUtf8Units(index)
	if err == nil && units != nil {
		return values.JavaUnitsToStringLiteral(units)
	}
	s, _ := obj.getUtf8(index)
	return values.JavaStringToLiteral(s)
}

func captureReaderWindow(r *ClassReader) []byte {
	if r == nil || r.Remaining() == 0 {
		return nil
	}
	if !r.reserve(int64(r.Remaining()), 1) {
		return nil
	}
	return append([]byte{}, r.orig[r.pos:r.end]...)
}

func (a *RuntimeVisibleParameterAnnotationsAttribute) readInfo(cp *ClassParser) {
	a.Info = captureReaderWindow(cp.reader)
	a.NumParameters = cp.reader.readUint8()
	if cp.reader.Err() != nil {
		return
	}
	if !cp.reader.reserve(int64(a.NumParameters), 2) {
		return
	}
	a.ParameterAnnotations = make([][]*AnnotationAttribute, a.NumParameters)
	for i := range a.ParameterAnnotations {
		n := cp.reader.readUint16()
		if cp.reader.Err() != nil {
			return
		}
		if !cp.reader.reserve(int64(n), 4) {
			return
		}
		list := make([]*AnnotationAttribute, n)
		for j := range list {
			list[j] = ParseAnnotation(cp)
			if cp.reader.Err() != nil {
				return
			}
		}
		a.ParameterAnnotations[i] = list
	}
}

func paramAnnosFromMethod(method *MemberInfo, invisible bool) *RuntimeVisibleParameterAnnotationsAttribute {
	if method == nil {
		return nil
	}
	for _, attr := range method.Attributes {
		pa, ok := attr.(*RuntimeVisibleParameterAnnotationsAttribute)
		if !ok {
			continue
		}
		if pa.IsInvisible == invisible {
			return pa
		}
	}
	return nil
}

func descriptorSyntheticPrefix(c *ClassObjectDumper, methodName string) int {
	if methodName != "<init>" || c == nil {
		return 0
	}
	// Enum constructors are dumped without the synthetic String/int prefix.
	if c.isGenuineEnum() {
		return 2
	}
	return 0
}

func dumpedParamAnnoIndex(c *ClassObjectDumper, methodName, descriptor string, dumpedCount, tableLen, dumpedIndex int) int {
	descCount := len(methodParamFieldDescriptors(descriptor))
	prefix := descriptorSyntheticPrefix(c, methodName)
	userStart := 0
	if methodName == "<init>" && c != nil && c.hasOuterThisField() && !c.isGenuineEnum() {
		// Outer this is still present in the dumped parameter list.
		userStart = 1
	}
	if tableLen == descCount {
		di := prefix + dumpedIndex
		if di >= 0 && di < tableLen {
			return di
		}
		return -1
	}
	// javac often stores a source-arity table (not JVMS descriptor arity) on
	// constructors with mandated/synthetic parameters. Map user formals in order.
	if dumpedIndex < userStart {
		return -1
	}
	src := dumpedIndex - userStart
	if src >= 0 && src < tableLen {
		return src
	}
	return -1
}

func (c *ClassObjectDumper) formatAnnotationList(annos []*AnnotationAttribute) string {
	if len(annos) == 0 {
		return ""
	}
	parts := make([]string, 0, len(annos))
	for _, a := range annos {
		if a == nil {
			continue
		}
		s, err := c.DumpAnnotation(a)
		if err != nil || s == "" {
			continue
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, " ")
}

func (c *ClassObjectDumper) applyParameterAnnotations(method *MemberInfo, methodName, descriptor string, decls []string) []string {
	if method == nil || len(decls) == 0 {
		return decls
	}
	vis := paramAnnosFromMethod(method, false)
	inv := paramAnnosFromMethod(method, true)
	if vis == nil && inv == nil {
		return decls
	}
	visSlots, invSlots := [][]*AnnotationAttribute(nil), [][]*AnnotationAttribute(nil)
	if vis != nil {
		visSlots = vis.ParameterAnnotations
	}
	if inv != nil {
		invSlots = inv.ParameterAnnotations
	}
	out := append([]string{}, decls...)
	for i := range out {
		var parts []string
		if vi := dumpedParamAnnoIndex(c, methodName, descriptor, len(out), len(visSlots), i); vi >= 0 && vi < len(visSlots) {
			if s := c.formatAnnotationList(visSlots[vi]); s != "" {
				parts = append(parts, s)
			}
		}
		if ii := dumpedParamAnnoIndex(c, methodName, descriptor, len(out), len(invSlots), i); ii >= 0 && ii < len(invSlots) {
			if s := c.formatAnnotationList(invSlots[ii]); s != "" {
				parts = append(parts, s)
			}
		}
		if len(parts) > 0 {
			out[i] = strings.Join(parts, " ") + " " + out[i]
		}
	}
	return out
}

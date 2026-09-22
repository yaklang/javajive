package javaclassparser

import (
	"fmt"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

const (
	typePathKindArray    uint8 = 0
	typePathKindInner    uint8 = 1
	typePathKindWildcard uint8 = 2
	typePathKindTypeArg  uint8 = 3
)

type TypePathEntry struct {
	Kind          uint8
	ArgumentIndex uint8
}

type LocalVarTarget struct {
	StartPC uint16
	Length  uint16
	Index   uint16
}

// TypeAnnotation is one type_annotation (JVMS 4.7.20). Declaration targets
// can be rendered; code-offset/local targets are stored and diagnosed.
type TypeAnnotation struct {
	TargetType           uint8
	TypeParameterIndex   uint8
	SupertypeIndex       uint16
	BoundIndex           uint8
	FormalParameterIndex uint8
	ThrowsTypeIndex      uint16
	LocalVarTable        []LocalVarTarget
	ExceptionTableIndex  uint16
	Offset               uint16
	TypeArgumentIndex    uint8
	TypePath             []TypePathEntry
	Annotation           *AnnotationAttribute
	CodeOffsetTarget     bool
}

type TypeAnnotationsAttribute struct {
	Name        string
	AttrLen     uint32
	IsInvisible bool
	Annotations []*TypeAnnotation
	Info        []byte
}

func (a *TypeAnnotationsAttribute) readInfo(cp *ClassParser) {
	a.Info = captureReaderWindow(cp.reader)
	n := cp.reader.readUint16()
	if cp.reader.Err() != nil {
		return
	}
	if !cp.reader.reserve(int64(n), 6) {
		return
	}
	a.Annotations = make([]*TypeAnnotation, n)
	for i := range a.Annotations {
		ta := parseTypeAnnotation(cp)
		a.Annotations[i] = ta
		if cp.reader.Err() != nil {
			return
		}
	}
}

func parseTypeAnnotation(cp *ClassParser) *TypeAnnotation {
	ta := &TypeAnnotation{}
	ta.TargetType = cp.reader.readUint8()
	if cp.reader.Err() != nil {
		return ta
	}
	switch ta.TargetType {
	case 0x00, 0x01:
		ta.TypeParameterIndex = cp.reader.readUint8()
	case 0x10:
		ta.SupertypeIndex = cp.reader.readUint16()
	case 0x11, 0x12:
		ta.TypeParameterIndex = cp.reader.readUint8()
		ta.BoundIndex = cp.reader.readUint8()
	case 0x13, 0x14, 0x15:
		// empty_target
	case 0x16:
		ta.FormalParameterIndex = cp.reader.readUint8()
	case 0x17:
		ta.ThrowsTypeIndex = cp.reader.readUint16()
	case 0x40, 0x41:
		ta.CodeOffsetTarget = true
		tl := cp.reader.readUint16()
		if !cp.reader.reserve(int64(tl), 6) {
			return ta
		}
		ta.LocalVarTable = make([]LocalVarTarget, tl)
		for i := range ta.LocalVarTable {
			ta.LocalVarTable[i] = LocalVarTarget{
				StartPC: cp.reader.readUint16(),
				Length:  cp.reader.readUint16(),
				Index:   cp.reader.readUint16(),
			}
		}
	case 0x42:
		ta.CodeOffsetTarget = true
		ta.ExceptionTableIndex = cp.reader.readUint16()
	case 0x43, 0x44, 0x45, 0x46:
		ta.CodeOffsetTarget = true
		ta.Offset = cp.reader.readUint16()
	case 0x47, 0x48, 0x49, 0x4A, 0x4B:
		ta.CodeOffsetTarget = true
		ta.Offset = cp.reader.readUint16()
		ta.TypeArgumentIndex = cp.reader.readUint8()
	default:
		cp.reader.fail(ParseCodeInvalidInput, fmt.Sprintf("illegal type annotation target_type 0x%02x", ta.TargetType))
		return ta
	}
	if cp.reader.Err() != nil {
		return ta
	}
	pathLen := cp.reader.readUint8()
	if cp.reader.Err() != nil {
		return ta
	}
	if !cp.reader.reserve(int64(pathLen), 2) {
		return ta
	}
	ta.TypePath = make([]TypePathEntry, pathLen)
	for i := range ta.TypePath {
		kind := cp.reader.readUint8()
		arg := cp.reader.readUint8()
		if kind > typePathKindTypeArg {
			cp.reader.fail(ParseCodeInvalidInput, fmt.Sprintf("illegal type_path_kind %d", kind))
			return ta
		}
		ta.TypePath[i] = TypePathEntry{Kind: kind, ArgumentIndex: arg}
	}
	if cp.reader.Err() != nil {
		return ta
	}
	ta.Annotation = ParseAnnotation(cp)
	return ta
}

func typeAnnosFrom(attrs []AttributeInfo) []*TypeAnnotation {
	var out []*TypeAnnotation
	for _, attr := range attrs {
		ta, ok := attr.(*TypeAnnotationsAttribute)
		if !ok {
			continue
		}
		out = append(out, ta.Annotations...)
	}
	return out
}

func methodAndCodeTypeAnnos(method *MemberInfo) []*TypeAnnotation {
	if method == nil {
		return nil
	}
	all := typeAnnosFrom(method.Attributes)
	for _, attr := range method.Attributes {
		code, ok := attr.(*CodeAttribute)
		if !ok {
			continue
		}
		all = append(all, typeAnnosFrom(code.Attributes)...)
	}
	return all
}

func (c *ClassObjectDumper) diagnoseUnplacedTypeAnnos(method *MemberInfo) {
	for _, ta := range methodAndCodeTypeAnnos(method) {
		if ta != nil && ta.CodeOffsetTarget {
			c.noteTypeAnnoCapability(ta, fmt.Sprintf("code-offset type annotation target 0x%02x preserved, not rendered", ta.TargetType))
		}
	}
}

func filterTypeAnnos(all []*TypeAnnotation, target uint8, formal uint8, useFormal bool) []*TypeAnnotation {
	var out []*TypeAnnotation
	for _, a := range all {
		if a == nil || a.TargetType != target {
			continue
		}
		if useFormal && a.FormalParameterIndex != formal {
			continue
		}
		out = append(out, a)
	}
	return out
}

func pathEqual(a, b []TypePathEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Kind != b[i].Kind || a[i].ArgumentIndex != b[i].ArgumentIndex {
			return false
		}
	}
	return true
}

func collectAnnosAt(all []*TypeAnnotation, prefix []TypePathEntry) []*AnnotationAttribute {
	var out []*AnnotationAttribute
	for _, ta := range all {
		if ta == nil || ta.Annotation == nil {
			continue
		}
		if pathEqual(ta.TypePath, prefix) {
			out = append(out, ta.Annotation)
		}
	}
	return out
}

func appendPath(prefix []TypePathEntry, kind, arg uint8) []TypePathEntry {
	out := make([]TypePathEntry, len(prefix)+1)
	copy(out, prefix)
	out[len(prefix)] = TypePathEntry{Kind: kind, ArgumentIndex: arg}
	return out
}

func (c *ClassObjectDumper) noteTypeAnnoCapability(ta *TypeAnnotation, msg string) {
	if c == nil {
		return
	}
	c.typeAnnosUnsupported = true
	if c.report == nil {
		return
	}
	c.report.Diagnostics = append(c.report.Diagnostics, DecompileDiagnostic{
		Code:    "unsupported",
		Message: msg,
	})
}

func (c *ClassObjectDumper) renderTypeWithAnnos(t types.JavaType, annos []*TypeAnnotation) string {
	for _, ta := range annos {
		if ta == nil {
			continue
		}
		if ta.CodeOffsetTarget {
			c.noteTypeAnnoCapability(ta, fmt.Sprintf("type annotation target 0x%02x is not restored to declarations", ta.TargetType))
			continue
		}
		for _, p := range ta.TypePath {
			if p.Kind == typePathKindInner {
				c.noteTypeAnnoCapability(ta, "inner-type type_path is not fully placed")
			}
		}
	}
	if t == nil {
		return c.renderTypePath(types.NewJavaClass("java.lang.Object"), annos, nil)
	}
	return c.renderTypePath(t, annos, nil)
}

func (c *ClassObjectDumper) renderTypePath(t types.JavaType, all []*TypeAnnotation, prefix []TypePathEntry) string {
	here := c.formatAnnotationList(collectAnnosAt(all, prefix))
	if wildcard, ok := t.(*types.JavaWildcardType); ok && wildcard != nil {
		mark := "?"
		if here != "" {
			mark = here + " ?"
		}
		if wildcard.Bound == nil {
			return mark
		}
		bound := c.renderTypePath(wildcard.Bound, all, appendPath(prefix, typePathKindWildcard, 0))
		return mark + " " + wildcard.Variant + " " + bound
	}
	if t != nil && t.IsArray() {
		// javac treats the leftmost `[]` after the element type as the OUTERMOST
		// dimension (`@A String @B [] @C []` → outer @B, inner @C). Emit outer-to-inner.
		n := 0
		elemT := t
		for elemT != nil && elemT.IsArray() {
			n++
			elemT = elemT.ElementType()
		}
		p := prefix
		dimAnnos := make([]string, n)
		for i := 0; i < n; i++ {
			dimAnnos[i] = c.formatAnnotationList(collectAnnosAt(all, p))
			p = appendPath(p, typePathKindArray, 0)
		}
		elemStr := c.renderTypePath(elemT, all, p)
		var b strings.Builder
		b.WriteString(elemStr)
		for i := 0; i < n; i++ {
			if dimAnnos[i] != "" {
				b.WriteByte(' ')
				b.WriteString(dimAnnos[i])
				b.WriteString(" []")
			} else {
				b.WriteString("[]")
			}
		}
		return b.String()
	}
	if t != nil {
		if pt, ok := types.AsParameterizedType(t); ok && pt != nil {
			args := make([]string, len(pt.TypeArgs))
			for i, arg := range pt.TypeArgs {
				if i > 255 {
					c.noteTypeAnnoCapability(nil, "type argument index exceeds representation")
					break
				}
				args[i] = c.renderTypePath(arg, all, appendPath(prefix, typePathKindTypeArg, uint8(i)))
			}
			base := pt.RawClassName
			if c.FuncCtx != nil {
				base = c.FuncCtx.ShortTypeName(pt.RawClassName)
			}
			if here != "" {
				base = placeTypeAnnotation(base, pt.RawClassName, here)
			}
			if len(args) == 0 {
				return base
			}
			return base + "<" + strings.Join(args, ", ") + ">"
		}
	}
	base := "java.lang.Object"
	rawName := ""
	if t != nil && c.FuncCtx != nil {
		base = t.String(c.FuncCtx)
	} else if t != nil {
		base = t.String(nil)
	}
	if t != nil {
		if jc, ok := t.RawType().(*types.JavaClass); ok && jc != nil {
			rawName = jc.Name
		}
	}
	if here != "" {
		return placeTypeAnnotation(base, rawName, here)
	}
	return base
}

// A type-use annotation on a nested member type belongs after the qualifier:
// Map.@Nullable Entry, not @Nullable Map.Entry (which attempts to annotate the
// scoping construct and javac rejects). The binary '$' is authoritative proof
// that the rendered final segment is a member type rather than a package name.
func placeTypeAnnotation(rendered, rawName, annotation string) string {
	if annotation == "" {
		return rendered
	}
	if strings.Contains(rawName, "$") {
		if dot := strings.LastIndex(rendered, "."); dot >= 0 {
			return rendered[:dot+1] + annotation + " " + rendered[dot+1:]
		}
	}
	return annotation + " " + rendered
}

func (c *ClassObjectDumper) applyFieldTypeAnnotations(field *MemberInfo, fieldType types.JavaType, rendered string) string {
	if field == nil {
		return rendered
	}
	annos := filterTypeAnnos(typeAnnosFrom(field.Attributes), 0x13, 0, false)
	if len(annos) == 0 {
		return rendered
	}
	return c.renderTypeWithAnnos(fieldType, annos)
}

func (c *ClassObjectDumper) applyReturnTypeAnnotations(method *MemberInfo, ret types.JavaType, rendered string) string {
	if method == nil {
		return rendered
	}
	all := typeAnnosFrom(method.Attributes)
	for _, ta := range all {
		if ta != nil && ta.CodeOffsetTarget {
			c.noteTypeAnnoCapability(ta, fmt.Sprintf("code-offset type annotation target 0x%02x preserved, not rendered", ta.TargetType))
		}
	}
	annos := filterTypeAnnos(all, 0x14, 0, false)
	if len(annos) == 0 {
		return rendered
	}
	return c.renderTypeWithAnnos(ret, annos)
}

func (c *ClassObjectDumper) applyParamTypeAnnotations(method *MemberInfo, methodName, descriptor string, paramTypes []types.JavaType, decls []string) []string {
	if method == nil || len(decls) == 0 {
		return decls
	}
	all := filterTypeAnnos(typeAnnosFrom(method.Attributes), 0x16, 0, false)
	if len(all) == 0 {
		return decls
	}
	out := append([]string{}, decls...)
	// Data-flow inference may narrow or otherwise mutate the FunctionType used
	// to render a method body. Type annotations describe declaration types, so
	// recover those types from the member descriptor/signature instead. Align
	// from the tail because enum/inner constructors can have synthetic leading
	// descriptor parameters omitted from source declarations.
	declaredParams := paramTypes
	if c != nil && c.obj != nil {
		if _, authoritative := memberSignatureAndDescriptorTypes(c.obj, method); len(authoritative) > 0 {
			declaredParams = authoritative
		}
	}
	offset := len(declaredParams) - len(out)
	if offset < 0 {
		offset = 0
	}
	for i := range out {
		formal := i + offset
		var matching []*TypeAnnotation
		for _, ta := range all {
			if int(ta.FormalParameterIndex) == formal {
				matching = append(matching, ta)
			}
		}
		if len(matching) == 0 {
			continue
		}
		var pt types.JavaType
		if formal < len(declaredParams) {
			pt = declaredParams[formal]
		}
		annotated := c.renderTypeWithAnnos(pt, matching)
		// Replace the leading type token of "Type name" / "Type... name".
		sp := strings.LastIndex(out[i], " ")
		if sp < 0 {
			out[i] = annotated + " " + out[i]
			continue
		}
		out[i] = annotated + out[i][sp:]
	}
	return out
}

func (c *ClassObjectDumper) applyThrowsTypeAnnotations(method *MemberInfo, exceptions string) string {
	if method == nil || exceptions == "" {
		return exceptions
	}
	annos := filterTypeAnnos(typeAnnosFrom(method.Attributes), 0x17, 0, false)
	if len(annos) == 0 {
		return exceptions
	}
	// exceptions is " throws A, B". Prefix each type with matching throws-index annotations.
	body := strings.TrimPrefix(exceptions, " throws ")
	parts := strings.Split(body, ", ")
	for i := range parts {
		var matching []*TypeAnnotation
		for _, ta := range annos {
			if int(ta.ThrowsTypeIndex) == i {
				matching = append(matching, ta)
			}
		}
		if len(matching) == 0 {
			continue
		}
		if s := c.formatAnnotationList(collectAnnosAt(matching, nil)); s != "" {
			parts[i] = s + " " + parts[i]
		}
	}
	return " throws " + strings.Join(parts, ", ")
}

func (c *ClassObjectDumper) applyReceiverTypeAnnotation(method *MemberInfo, className string) string {
	if method == nil {
		return ""
	}
	annos := filterTypeAnnos(typeAnnosFrom(method.Attributes), 0x15, 0, false)
	if len(annos) == 0 {
		return ""
	}
	s := c.formatAnnotationList(collectAnnosAt(annos, nil))
	if s == "" {
		c.noteTypeAnnoCapability(annos[0], "receiver type annotation could not be rendered")
		return ""
	}
	if className == "" {
		className = "this"
	}
	return s + " " + className + " this"
}

func walkTypePath(t types.JavaType, path []TypePathEntry) error {
	cur := t
	for i, p := range path {
		if cur == nil {
			return fmt.Errorf("type_path[%d]: missing type", i)
		}
		switch p.Kind {
		case typePathKindArray:
			if !cur.IsArray() {
				return fmt.Errorf("type_path[%d]: array step on non-array", i)
			}
			cur = cur.ElementType()
		case typePathKindTypeArg:
			pt, ok := types.AsParameterizedType(cur)
			if !ok || pt == nil {
				return fmt.Errorf("type_path[%d]: type argument %d on non-parameterized type", i, p.ArgumentIndex)
			}
			if int(p.ArgumentIndex) >= len(pt.TypeArgs) {
				return fmt.Errorf("type_path[%d]: type argument %d does not exist (arity %d)", i, p.ArgumentIndex, len(pt.TypeArgs))
			}
			cur = pt.TypeArgs[p.ArgumentIndex]
		case typePathKindWildcard:
			wildcard, ok := cur.(*types.JavaWildcardType)
			if !ok || wildcard == nil || wildcard.Bound == nil {
				return fmt.Errorf("type_path[%d]: wildcard step on non-wildcard type", i)
			}
			cur = wildcard.Bound
		case typePathKindInner:
			// The type model flattens a binary member class into one class name.
			// Legality is validated by the parser; placement uses the '$' proof in
			// placeTypeAnnotation when the annotation is at the member segment.
		default:
			return fmt.Errorf("type_path[%d]: illegal kind %d", i, p.Kind)
		}
	}
	return nil
}

func memberSignatureAndDescriptorTypes(obj *ClassObject, m *MemberInfo) (ret types.JavaType, params []types.JavaType) {
	if obj == nil || m == nil {
		return nil, nil
	}
	desc, err := obj.getUtf8(m.DescriptorIndex)
	if err == nil && desc != "" {
		if mt, perr := types.ParseMethodDescriptor(desc); perr == nil && mt != nil && mt.FunctionType() != nil {
			ret = mt.FunctionType().ReturnType
			params = mt.FunctionType().ParamTypes
		}
	}
	for _, attr := range m.Attributes {
		sig, ok := attr.(*SignatureAttribute)
		if !ok {
			continue
		}
		sigStr, serr := obj.getUtf8(sig.SignatureIndex)
		if serr != nil || sigStr == "" {
			continue
		}
		_, sigParams, sigRet, _ := types.ParseMethodSignatureFullWithThrows(sigStr, nil)
		if sigRet != nil {
			ret = sigRet
		}
		if len(sigParams) == len(params) && sigParams != nil {
			params = sigParams
		}
	}
	return ret, params
}

func fieldJavaType(obj *ClassObject, f *MemberInfo) types.JavaType {
	if obj == nil || f == nil {
		return nil
	}
	desc, err := obj.getUtf8(f.DescriptorIndex)
	var t types.JavaType
	if err == nil {
		t, _ = types.ParseDescriptor(desc)
	}
	for _, attr := range f.Attributes {
		sig, ok := attr.(*SignatureAttribute)
		if !ok {
			continue
		}
		sigStr, serr := obj.getUtf8(sig.SignatureIndex)
		if serr != nil || sigStr == "" {
			continue
		}
		if st := types.ParseSignature(sigStr); st != nil {
			return st
		}
	}
	return t
}

func validateTypeAnnotationPaths(obj *ClassObject) error {
	if obj == nil {
		return nil
	}
	check := func(ta *TypeAnnotation, t types.JavaType) error {
		if ta == nil || ta.CodeOffsetTarget || t == nil {
			return nil
		}
		if err := walkTypePath(t, ta.TypePath); err != nil {
			return &ClassParseError{Code: ParseCodeInvalidInput, Stage: "type_annotation", Field: "type_path", Msg: err.Error()}
		}
		return nil
	}
	for _, f := range obj.Fields {
		ft := fieldJavaType(obj, f)
		for _, ta := range filterTypeAnnos(typeAnnosFrom(f.Attributes), 0x13, 0, false) {
			if err := check(ta, ft); err != nil {
				return err
			}
		}
	}
	for _, m := range obj.Methods {
		ret, params := memberSignatureAndDescriptorTypes(obj, m)
		for _, ta := range typeAnnosFrom(m.Attributes) {
			if ta == nil || ta.CodeOffsetTarget {
				continue
			}
			switch ta.TargetType {
			case 0x14:
				if err := check(ta, ret); err != nil {
					return err
				}
			case 0x16:
				idx := int(ta.FormalParameterIndex)
				var pt types.JavaType
				if idx >= 0 && idx < len(params) {
					pt = params[idx]
				}
				if pt != nil {
					if err := check(ta, pt); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func (c *ClassObjectDumper) applyTypeParamBoundAnnotations(method *MemberInfo, typeParams string) string {
	if method == nil || typeParams == "" {
		return typeParams
	}
	annos := filterTypeAnnos(typeAnnosFrom(method.Attributes), 0x12, 0, false)
	if len(annos) == 0 {
		return typeParams
	}
	// Conservative: prefix the first bound occurrence when a single bound annotation exists.
	for _, ta := range annos {
		s := c.formatAnnotationList([]*AnnotationAttribute{ta.Annotation})
		if s == "" {
			continue
		}
		if strings.Contains(typeParams, " extends ") && !strings.Contains(typeParams, s) {
			typeParams = strings.Replace(typeParams, " extends ", " extends "+s+" ", 1)
		}
	}
	return typeParams
}

package dexparser

import (
	"encoding/binary"

	"github.com/yaklang/javajive/dex2jar/model"
)

// parseAnnotationDirectory decodes the annotation_directory_item of one class
// and attaches every annotation set to the class model. Structure, ownership
// and index ranges are validated; translation to JVM attributes happens during
// lowering.
func (p *parser) parseAnnotationDirectory(off uint32, c *model.Class) error {
	if err := p.rangeCheck(off, 16, "annotations_directory"); err != nil {
		return err
	}
	pos := int(off)
	classSetOff := binary.LittleEndian.Uint32(p.data[pos : pos+4])
	fieldsSize := binary.LittleEndian.Uint32(p.data[pos+4 : pos+8])
	methodsSize := binary.LittleEndian.Uint32(p.data[pos+8 : pos+12])
	parametersSize := binary.LittleEndian.Uint32(p.data[pos+12 : pos+16])
	total := uint64(fieldsSize) + uint64(methodsSize) + uint64(parametersSize)
	if total*8 > uint64(len(p.data)) {
		return p.fail(pos, "annotations_directory", "entry counts exceed file size")
	}
	if err := p.rangeCheck(off+16, uint32(total*8), "annotations_directory"); err != nil {
		return err
	}
	if classSetOff != 0 {
		annotations, err := p.annotationSet(classSetOff)
		if err != nil {
			return err
		}
		c.Annotations = annotations
	}
	cursor := pos + 16
	seenFields := make(map[uint32]bool, fieldsSize)
	for i := uint32(0); i < fieldsSize; i++ {
		fieldIdx := binary.LittleEndian.Uint32(p.data[cursor : cursor+4])
		setOff := binary.LittleEndian.Uint32(p.data[cursor+4 : cursor+8])
		cursor += 8
		if fieldIdx >= uint32(len(p.file.Fields)) {
			return p.fail(cursor-8, "annotations_directory field", "field_idx=%d out of range", fieldIdx)
		}
		if seenFields[fieldIdx] {
			return p.fail(cursor-8, "annotations_directory field", "duplicate field_idx=%d", fieldIdx)
		}
		seenFields[fieldIdx] = true
		annotations, err := p.annotationSet(setOff)
		if err != nil {
			return err
		}
		c.FieldAnnotations = append(c.FieldAnnotations, model.FieldAnnotation{FieldIndex: fieldIdx, Annotations: annotations})
	}
	seenMethods := make(map[uint32]bool, methodsSize)
	for i := uint32(0); i < methodsSize; i++ {
		methodIdx := binary.LittleEndian.Uint32(p.data[cursor : cursor+4])
		setOff := binary.LittleEndian.Uint32(p.data[cursor+4 : cursor+8])
		cursor += 8
		if methodIdx >= uint32(len(p.file.Methods)) {
			return p.fail(cursor-8, "annotations_directory method", "method_idx=%d out of range", methodIdx)
		}
		if seenMethods[methodIdx] {
			return p.fail(cursor-8, "annotations_directory method", "duplicate method_idx=%d", methodIdx)
		}
		seenMethods[methodIdx] = true
		annotations, err := p.annotationSet(setOff)
		if err != nil {
			return err
		}
		c.MethodAnnotations = append(c.MethodAnnotations, model.MethodAnnotation{MethodIndex: methodIdx, Annotations: annotations})
	}
	seenParameters := make(map[uint32]bool, parametersSize)
	for i := uint32(0); i < parametersSize; i++ {
		methodIdx := binary.LittleEndian.Uint32(p.data[cursor : cursor+4])
		setOff := binary.LittleEndian.Uint32(p.data[cursor+4 : cursor+8])
		cursor += 8
		if methodIdx >= uint32(len(p.file.Methods)) {
			return p.fail(cursor-8, "annotations_directory parameter", "method_idx=%d out of range", methodIdx)
		}
		if seenParameters[methodIdx] {
			return p.fail(cursor-8, "annotations_directory parameter", "duplicate method_idx=%d", methodIdx)
		}
		seenParameters[methodIdx] = true
		parameters, err := p.annotationRefList(setOff)
		if err != nil {
			return err
		}
		c.ParameterAnnotations = append(c.ParameterAnnotations, model.ParameterAnnotation{MethodIndex: methodIdx, Parameters: parameters})
	}
	return nil
}

// annotationRefList decodes one annotation_set_ref_list_item: a list of
// annotation sets, one per declared parameter.
func (p *parser) annotationRefList(off uint32) ([][]model.Annotation, error) {
	if err := p.rangeCheck(off, 4, "annotation_set_ref_list"); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(p.data[off : off+4])
	if uint64(size)*4 > uint64(len(p.data)) {
		return nil, p.fail(int(off), "annotation_set_ref_list", "size=%d exceeds file size", size)
	}
	if err := p.rangeCheck(off+4, size*4, "annotation_set_ref_list"); err != nil {
		return nil, err
	}
	sets := make([][]model.Annotation, size)
	for i := range sets {
		setOff := binary.LittleEndian.Uint32(p.data[int(off)+4+i*4 : int(off)+8+i*4])
		annotations, err := p.annotationSet(setOff)
		if err != nil {
			return nil, err
		}
		sets[i] = annotations
	}
	return sets, nil
}

// annotationSet decodes one annotation_set_item.
func (p *parser) annotationSet(off uint32) ([]model.Annotation, error) {
	if off == 0 {
		return nil, nil
	}
	if err := p.rangeCheck(off, 4, "annotation_set"); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(p.data[off : off+4])
	if uint64(size)*4 > uint64(len(p.data)) {
		return nil, p.fail(int(off), "annotation_set", "size=%d exceeds file size", size)
	}
	if err := p.rangeCheck(off+4, size*4, "annotation_set"); err != nil {
		return nil, err
	}
	annotations := make([]model.Annotation, size)
	for i := range annotations {
		itemOff := binary.LittleEndian.Uint32(p.data[int(off)+4+i*4 : int(off)+8+i*4])
		annotation, err := p.annotationItem(itemOff)
		if err != nil {
			return nil, err
		}
		annotations[i] = annotation
	}
	return annotations, nil
}

// annotationItem decodes one annotation_item.
func (p *parser) annotationItem(off uint32) (model.Annotation, error) {
	if err := p.rangeCheck(off, 1, "annotation_item"); err != nil {
		return model.Annotation{}, err
	}
	visibility := p.data[off]
	if visibility > model.AnnotationSystem {
		return model.Annotation{}, p.fail(int(off), "annotation_item", "invalid visibility=%d", visibility)
	}
	cur := int(off) + 1
	encoded, err := p.encodedAnnotation(&cur, 0)
	if err != nil {
		return model.Annotation{}, err
	}
	return model.Annotation{Visibility: visibility, Type: encoded.Type, Elements: encoded.Elements}, nil
}

// systemAnnotationPrefix marks DEX system annotations.
const systemAnnotationPrefix = "Ldalvik/annotation/"

func isSystemAnnotation(annotation model.Annotation) bool {
	return annotation.Visibility == model.AnnotationSystem || len(annotation.Type) > len(systemAnnotationPrefix) && annotation.Type[:len(systemAnnotationPrefix)] == systemAnnotationPrefix
}

func annotationElement(annotation model.Annotation, name string) (model.EncodedValue, bool) {
	for _, element := range annotation.Elements {
		if element.Name == name {
			return element.Value, true
		}
	}
	return model.EncodedValue{}, false
}

func (p *parser) stringElement(annotation model.Annotation, name, field string) (string, error) {
	value, ok := annotationElement(annotation, name)
	if !ok {
		return "", p.fail(0, "system annotation", "%s is missing element %q", field, name)
	}
	if value.Type == model.ValueNull {
		// Anonymous classes carry a null InnerClass name.
		return "", nil
	}
	if value.Type != model.ValueString {
		return "", p.fail(0, "system annotation", "%s element %q is not a string", field, name)
	}
	index := value.Unsigned
	if index >= uint64(len(p.file.Strings)) {
		return "", p.fail(0, "system annotation", "%s element %q string index %d is out of range", field, name, index)
	}
	return p.file.Strings[index], nil
}

func (p *parser) typeElement(annotation model.Annotation, name, field string) (string, error) {
	value, ok := annotationElement(annotation, name)
	if !ok {
		return "", p.fail(0, "system annotation", "%s is missing element %q", field, name)
	}
	if value.Type != model.ValueTypeRef {
		return "", p.fail(0, "system annotation", "%s element %q is not a type", field, name)
	}
	index := value.Unsigned
	if index >= uint64(len(p.file.Types)) {
		return "", p.fail(0, "system annotation", "%s element %q type index %d is out of range", field, name, index)
	}
	return p.file.Types[index].Descriptor, nil
}

func (p *parser) typeArrayElement(annotation model.Annotation, name, field string) ([]string, error) {
	value, ok := annotationElement(annotation, name)
	if !ok {
		return nil, p.fail(0, "system annotation", "%s is missing element %q", field, name)
	}
	if value.Type != model.ValueArray {
		return nil, p.fail(0, "system annotation", "%s element %q is not an array", field, name)
	}
	descriptors := make([]string, 0, len(value.Array))
	for _, item := range value.Array {
		if item.Type != model.ValueTypeRef {
			return nil, p.fail(0, "system annotation", "%s element %q contains a non-type item", field, name)
		}
		if item.Unsigned >= uint64(len(p.file.Types)) {
			return nil, p.fail(0, "system annotation", "%s element %q type index %d is out of range", field, name, item.Unsigned)
		}
		descriptors = append(descriptors, p.file.Types[item.Unsigned].Descriptor)
	}
	return descriptors, nil
}

func (p *parser) stringArrayElement(annotation model.Annotation, name, field string) ([]string, error) {
	value, ok := annotationElement(annotation, name)
	if !ok {
		return nil, p.fail(0, "system annotation", "%s is missing element %q", field, name)
	}
	if value.Type != model.ValueArray {
		return nil, p.fail(0, "system annotation", "%s element %q is not an array", field, name)
	}
	parts := make([]string, 0, len(value.Array))
	for _, item := range value.Array {
		if item.Type != model.ValueString {
			return nil, p.fail(0, "system annotation", "%s element %q contains a non-string item", field, name)
		}
		if item.Unsigned >= uint64(len(p.file.Strings)) {
			return nil, p.fail(0, "system annotation", "%s element %q string index %d is out of range", field, name, item.Unsigned)
		}
		parts = append(parts, p.file.Strings[item.Unsigned])
	}
	return parts, nil
}

// translateSystemAnnotations folds DEX system annotations into class metadata
// that the classfile can express. Annotations whose JVM form needs a newer
// classfile version than the builder emits are dropped deliberately.
func (p *parser) translateSystemAnnotations(c *model.Class) error {
	kept := c.Annotations[:0]
	for _, annotation := range c.Annotations {
		if !isSystemAnnotation(annotation) {
			kept = append(kept, annotation)
			continue
		}
		switch annotation.Type {
		case "Ldalvik/annotation/Signature;":
			parts, err := p.stringArrayElement(annotation, "value", "Signature")
			if err != nil {
				return err
			}
			for _, part := range parts {
				c.Signature += part
			}
		case "Ldalvik/annotation/Throws;":
			descriptors, err := p.typeArrayElement(annotation, "value", "Throws")
			if err != nil {
				return err
			}
			c.Throws = append(c.Throws, descriptors...)
		case "Ldalvik/annotation/SourceDebugExtension;":
			value, err := p.stringElement(annotation, "value", "SourceDebugExtension")
			if err != nil {
				return err
			}
			c.SourceDebugExtension = value
		case "Ldalvik/annotation/EnclosingClass;":
			descriptor, err := p.typeElement(annotation, "value", "EnclosingClass")
			if err != nil {
				return err
			}
			enclosing := model.EnclosingMethod{Class: descriptor}
			if c.Enclosing != nil {
				enclosing.Method, enclosing.Proto = c.Enclosing.Method, c.Enclosing.Proto
			}
			c.Enclosing = &enclosing
		case "Ldalvik/annotation/EnclosingMethod;":
			if err := p.applyEnclosingMethod(c, annotation); err != nil {
				return err
			}
		case "Ldalvik/annotation/InnerClass;":
			if err := p.applyInnerClass(c, annotation); err != nil {
				return err
			}
		case "Ldalvik/annotation/MemberClasses;":
			descriptors, err := p.typeArrayElement(annotation, "value", "MemberClasses")
			if err != nil {
				return err
			}
			for _, descriptor := range descriptors {
				c.InnerClasses = append(c.InnerClasses, model.InnerClass{Inner: descriptor, Outer: c.Descriptor})
			}
		case "Ldalvik/annotation/AnnotationDefault;":
			if err := p.applyAnnotationDefaults(c, annotation); err != nil {
				return err
			}
		default:
			// MethodParameters, Record, PermittedSubclasses, NestHost and other
			// post-Java-5 system annotations have no representation in a
			// major-version-49 classfile and are dropped here.
		}
	}
	c.Annotations = kept
	return nil
}

func (p *parser) applyAnnotationDefaults(c *model.Class, annotation model.Annotation) error {
	value, ok := annotationElement(annotation, "value")
	if !ok {
		return p.fail(0, "system annotation", "AnnotationDefault is missing element %q", "value")
	}
	if value.Type != model.ValueAnnotation || value.Annotation == nil {
		return p.fail(0, "system annotation", "AnnotationDefault element %q is not an annotation", "value")
	}
	methods := make(map[string]uint32)
	for _, method := range append(append([]model.EncodedMethod(nil), c.DirectMethods...), c.VirtualMethods...) {
		if previous, exists := methods[method.Method.Name]; exists && previous != method.MethodIndex {
			return p.fail(0, "system annotation", "AnnotationDefault method name %q is ambiguous", method.Method.Name)
		}
		methods[method.Method.Name] = method.MethodIndex
	}
	for _, element := range value.Annotation.Elements {
		methodIndex, exists := methods[element.Name]
		if !exists {
			return p.fail(0, "system annotation", "AnnotationDefault element %q has no annotation method", element.Name)
		}
		c.MethodAnnotations = append(c.MethodAnnotations, model.MethodAnnotation{
			MethodIndex: methodIndex,
			Annotations: []model.Annotation{{
				Visibility: model.AnnotationSystem,
				Type:       "Ldalvik/annotation/AnnotationDefault;",
				Elements:   []model.AnnotationElement{{Name: "value", Value: element.Value}},
			}},
		})
	}
	return nil
}

func (p *parser) applyEnclosingMethod(c *model.Class, annotation model.Annotation) error {
	value, ok := annotationElement(annotation, "value")
	if !ok {
		return p.fail(0, "system annotation", "EnclosingMethod is missing element %q", "value")
	}
	if value.Type != model.ValueMethod {
		return p.fail(0, "system annotation", "EnclosingMethod element %q is not a method", "value")
	}
	index := value.Unsigned
	if index >= uint64(len(p.file.Methods)) {
		return p.fail(0, "system annotation", "EnclosingMethod method index %d is out of range", index)
	}
	method := p.file.Methods[index]
	enclosing := model.EnclosingMethod{Class: method.Class}
	if int(method.ProtoIndex) < len(p.file.Protos) {
		proto, err := protoDescriptor(p.file.Protos[method.ProtoIndex])
		if err != nil {
			return p.fail(0, "system annotation", "EnclosingMethod proto: %v", err)
		}
		enclosing.Method, enclosing.Proto = method.Name, proto
	}
	c.Enclosing = &enclosing
	return nil
}

func (p *parser) applyInnerClass(c *model.Class, annotation model.Annotation) error {
	name, err := p.stringElement(annotation, "name", "InnerClass")
	if err != nil {
		return err
	}
	flags := uint16(0)
	if value, ok := annotationElement(annotation, "accessFlags"); ok {
		if value.Type != model.ValueInt {
			return p.fail(0, "system annotation", "InnerClass accessFlags is not an int")
		}
		flags = uint16(value.Signed)
	}
	// The InnerClass annotation carries only the simple name and flags. The
	// declaring class comes from a MemberClasses row on the outer class or an
	// EnclosingClass annotation; fillInnerClassOuters links it afterwards
	// instead of parsing "$" out of the descriptor.
	c.InnerClasses = append(c.InnerClasses, model.InnerClass{
		Inner: c.Descriptor, Name: name, Flags: flags,
	})
	return nil
}

// fillInnerClassOuters links nested classes to their declaring class using only
// evidence present in the file: a MemberClasses row declared by the outer class,
// or an EnclosingClass annotation on the nested class itself.
func (p *parser) fillInnerClassOuters() {
	memberOf := make(map[string]string)
	for index := range p.file.Classes {
		outer := &p.file.Classes[index]
		for _, row := range outer.InnerClasses {
			if row.Outer == outer.Descriptor && row.Inner != "" {
				memberOf[row.Inner] = outer.Descriptor
			}
		}
	}
	for index := range p.file.Classes {
		class := &p.file.Classes[index]
		for row := range class.InnerClasses {
			if class.InnerClasses[row].Outer != "" {
				continue
			}
			if class.Enclosing != nil && class.Enclosing.Class != "" {
				class.InnerClasses[row].Outer = class.Enclosing.Class
				continue
			}
			if outer, ok := memberOf[class.Descriptor]; ok {
				class.InnerClasses[row].Outer = outer
			}
		}
	}
}

func protoDescriptor(proto model.Proto) (string, error) {
	descriptor := "("
	for _, parameter := range proto.Parameters {
		descriptor += parameter
	}
	descriptor += ")" + proto.ReturnType
	return descriptor, nil
}

func (p *parser) checkAnnotationMethodMembership() error {
	for _, class := range p.file.Classes {
		owned := make(map[uint32]bool)
		for _, method := range append(append([]model.EncodedMethod(nil), class.DirectMethods...), class.VirtualMethods...) {
			owned[method.MethodIndex] = true
		}
		for _, annotation := range class.MethodAnnotations {
			if !owned[annotation.MethodIndex] {
				return p.fail(0, "annotations_directory method", "method_idx=%d is not a member of %s", annotation.MethodIndex, class.Descriptor)
			}
		}
		for _, annotation := range class.ParameterAnnotations {
			if !owned[annotation.MethodIndex] {
				return p.fail(0, "annotations_directory parameter", "method_idx=%d is not a member of %s", annotation.MethodIndex, class.Descriptor)
			}
		}
	}
	return nil
}

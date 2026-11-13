package classfilebuilder

import (
	"encoding/binary"
	"fmt"
	"math"
)

// AnnotationValueKind selects the JVM element_value tag and payload layout.
// The tag must match the element type declared by the annotation interface:
// DEX encoded values already carry that information.
type AnnotationValueKind byte

const (
	AnnotationByte   AnnotationValueKind = 'B'
	AnnotationChar   AnnotationValueKind = 'C'
	AnnotationShort  AnnotationValueKind = 'S'
	AnnotationInt    AnnotationValueKind = 'I'
	AnnotationLong   AnnotationValueKind = 'J'
	AnnotationFloat  AnnotationValueKind = 'F'
	AnnotationDouble AnnotationValueKind = 'D'
	AnnotationBool   AnnotationValueKind = 'Z'
	AnnotationString AnnotationValueKind = 's'
	AnnotationEnum   AnnotationValueKind = 'e'
	AnnotationClass  AnnotationValueKind = 'c'
	AnnotationNested AnnotationValueKind = '@'
	AnnotationArray  AnnotationValueKind = '['
)

// AnnotationValue is one element_value.
type AnnotationValue struct {
	Kind   AnnotationValueKind
	Int    int64  // for B/C/S/I/J/Z
	Bits   uint64 // IEEE-754 bits for F (low 32 bits) and D
	Text   string // string constant, enum constant name, or class descriptor
	Type   string // enum type descriptor, for Kind == AnnotationEnum
	Nested *Annotation
	Array  []AnnotationValue
}

// AnnotationEntry binds one element name to a value.
type AnnotationEntry struct {
	Name  string
	Value AnnotationValue
}

// Annotation is one runtime or build annotation.
type Annotation struct {
	Descriptor string
	Elements   []AnnotationEntry
}

// InnerClassSpec is one InnerClasses table row.
type InnerClassSpec struct {
	Inner string
	Outer string
	Name  string
	Flags uint16
}

// EnclosingMethodSpec is the EnclosingMethod attribute payload.
type EnclosingMethodSpec struct {
	Class  string
	Method string
	Proto  string
}

// pendingAttribute is an attribute prepared before the constant pool is
// serialized. Both the name index and the payload must be final by then.
type pendingAttribute struct {
	name      string
	nameIndex uint16
	payload   []byte
	// encode is called during Bytes() so constant-pool growth happens before
	// the pool is serialized.
	encode func(*ConstantPool) ([]byte, error)
}

// annotationTarget holds attributes shared by classes and members.
type annotationTarget struct {
	signature      string
	annotations    []Annotation
	visible        []bool
	pending        []pendingAttribute
	exceptions     []string
	defaultValue   *AnnotationValue
	parameterAnnot [][]Annotation
	parameterVis   [][]bool
}

func (t *annotationTarget) addAnnotation(a Annotation, visible bool) {
	t.annotations = append(t.annotations, a)
	t.visible = append(t.visible, visible)
}

func (t *annotationTarget) addPending(name string, encode func(*ConstantPool) ([]byte, error)) {
	t.pending = append(t.pending, pendingAttribute{name: name, encode: encode})
}

func validateAttributePayloadLength(length uint64) error {
	if length > uint64(math.MaxUint32) {
		return fmt.Errorf("attribute length %d exceeds 4294967295", length)
	}
	if length > uint64(^uint(0)>>1) {
		return fmt.Errorf("attribute length %d exceeds the platform int limit", length)
	}
	return nil
}

func writeAttribute(out *byteWriter, attribute pendingAttribute) error {
	if attribute.encode != nil || attribute.nameIndex == 0 {
		// Payloads and name indices must be resolved before the constant pool
		// is serialized; resolving here would append entries past the written
		// constant_pool_count.
		return fmt.Errorf("%s attribute was not resolved before serialization", attribute.name)
	}
	if err := validateAttributePayloadLength(uint64(len(attribute.payload))); err != nil {
		return fmt.Errorf("%s: %w", attribute.name, err)
	}
	out.u2(attribute.nameIndex)
	out.u4(uint32(len(attribute.payload)))
	out.bytes(attribute.payload)
	return nil
}

// encodeAnnotations writes a RuntimeVisibleAnnotations or
// RuntimeInvisibleAnnotations payload.
func encodeAnnotations(pool *ConstantPool, annotations []Annotation, visible []bool, wantVisible bool) ([]byte, error) {
	if len(visible) != len(annotations) {
		return nil, fmt.Errorf("annotations and visibility differ in length")
	}
	selected := make([]Annotation, 0, len(annotations))
	for index, annotation := range annotations {
		if visible[index] == wantVisible {
			selected = append(selected, annotation)
		}
	}
	if len(selected) == 0 {
		return nil, nil
	}
	if len(selected) > math.MaxUint16 {
		return nil, fmt.Errorf("annotation count %d exceeds 65535", len(selected))
	}
	out := &byteWriter{}
	out.u2(uint16(len(selected)))
	for _, annotation := range selected {
		if err := writeAnnotation(pool, out, annotation); err != nil {
			return nil, err
		}
	}
	return out.data, nil
}

func writeAnnotation(pool *ConstantPool, out *byteWriter, annotation Annotation) error {
	if annotation.Descriptor == "" {
		return fmt.Errorf("annotation descriptor is empty")
	}
	typeIndex, err := pool.UTF8(annotation.Descriptor)
	if err != nil {
		return err
	}
	out.u2(typeIndex)
	if len(annotation.Elements) > math.MaxUint16 {
		return fmt.Errorf("annotation element count %d exceeds 65535", len(annotation.Elements))
	}
	out.u2(uint16(len(annotation.Elements)))
	for _, element := range annotation.Elements {
		if element.Name == "" {
			return fmt.Errorf("annotation element name is empty")
		}
		nameIndex, err := pool.UTF8(element.Name)
		if err != nil {
			return err
		}
		out.u2(nameIndex)
		if err := writeValue(pool, out, element.Value); err != nil {
			return err
		}
	}
	return nil
}

func writeValue(pool *ConstantPool, out *byteWriter, value AnnotationValue) error {
	out.u1(byte(value.Kind))
	switch value.Kind {
	case AnnotationByte, AnnotationChar, AnnotationShort, AnnotationInt, AnnotationBool:
		index, err := pool.Integer(int32(value.Int))
		if err != nil {
			return err
		}
		out.u2(index)
	case AnnotationLong:
		index, err := pool.Long(value.Int)
		if err != nil {
			return err
		}
		out.u2(index)
	case AnnotationFloat:
		index, err := pool.Float(uint32(value.Bits))
		if err != nil {
			return err
		}
		out.u2(index)
	case AnnotationDouble:
		index, err := pool.Double(value.Bits)
		if err != nil {
			return err
		}
		out.u2(index)
	case AnnotationString:
		index, err := pool.UTF8(value.Text)
		if err != nil {
			return err
		}
		out.u2(index)
	case AnnotationEnum:
		if value.Type == "" || value.Text == "" {
			return fmt.Errorf("enum annotation value requires a type descriptor and constant name")
		}
		typeIndex, err := pool.UTF8(value.Type)
		if err != nil {
			return err
		}
		nameIndex, err := pool.UTF8(value.Text)
		if err != nil {
			return err
		}
		out.u2(typeIndex)
		out.u2(nameIndex)
	case AnnotationClass:
		if value.Text == "" {
			return fmt.Errorf("class annotation value requires a descriptor")
		}
		index, err := pool.UTF8(value.Text)
		if err != nil {
			return err
		}
		out.u2(index)
	case AnnotationNested:
		if value.Nested == nil {
			return fmt.Errorf("nested annotation value is nil")
		}
		return writeAnnotation(pool, out, *value.Nested)
	case AnnotationArray:
		if len(value.Array) > math.MaxUint16 {
			return fmt.Errorf("annotation array length %d exceeds 65535", len(value.Array))
		}
		out.u2(uint16(len(value.Array)))
		for _, item := range value.Array {
			if err := writeValue(pool, out, item); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported annotation element kind %#x", value.Kind)
	}
	return nil
}

func encodeSignature(pool *ConstantPool, signature string) ([]byte, error) {
	index, err := pool.UTF8(signature)
	if err != nil {
		return nil, err
	}
	payload := make([]byte, 2)
	binary.BigEndian.PutUint16(payload, index)
	return payload, nil
}

func encodeExceptions(pool *ConstantPool, descriptors []string) ([]byte, error) {
	if len(descriptors) > math.MaxUint16 {
		return nil, fmt.Errorf("Exceptions length %d exceeds 65535", len(descriptors))
	}
	out := &byteWriter{}
	out.u2(uint16(len(descriptors)))
	for _, descriptor := range descriptors {
		name, err := internalNameFromDescriptor(descriptor)
		if err != nil {
			return nil, err
		}
		index, err := pool.Class(name)
		if err != nil {
			return nil, err
		}
		out.u2(index)
	}
	return out.data, nil
}

func encodeInnerClasses(pool *ConstantPool, specs []InnerClassSpec) ([]byte, error) {
	if len(specs) > math.MaxUint16 {
		return nil, fmt.Errorf("InnerClasses count %d exceeds 65535", len(specs))
	}
	out := &byteWriter{}
	out.u2(uint16(len(specs)))
	for _, spec := range specs {
		inner, err := classIndexFor(pool, spec.Inner)
		if err != nil {
			return nil, err
		}
		out.u2(inner)
		outer := uint16(0)
		if spec.Outer != "" {
			outer, err = classIndexFor(pool, spec.Outer)
			if err != nil {
				return nil, err
			}
		}
		out.u2(outer)
		name := uint16(0)
		if spec.Name != "" {
			name, err = pool.UTF8(spec.Name)
			if err != nil {
				return nil, err
			}
		}
		out.u2(name)
		out.u2(spec.Flags)
	}
	return out.data, nil
}

func encodeEnclosingMethod(pool *ConstantPool, spec EnclosingMethodSpec) ([]byte, error) {
	classIndex, err := classIndexFor(pool, spec.Class)
	if err != nil {
		return nil, err
	}
	out := &byteWriter{}
	out.u2(classIndex)
	methodIndex := uint16(0)
	if spec.Method != "" {
		nameType, err := pool.NameAndType(spec.Method, spec.Proto)
		if err != nil {
			return nil, err
		}
		methodIndex = nameType
	}
	out.u2(methodIndex)
	return out.data, nil
}

func encodeAnnotationDefault(pool *ConstantPool, value AnnotationValue) ([]byte, error) {
	out := &byteWriter{}
	if err := writeValue(pool, out, value); err != nil {
		return nil, err
	}
	return out.data, nil
}

func encodeParameterAnnotations(pool *ConstantPool, annotations [][]Annotation, visible [][]bool, wantVisible bool) ([]byte, error) {
	if len(annotations) == 0 {
		return nil, nil
	}
	if len(annotations) > math.MaxUint8 {
		return nil, fmt.Errorf("parameter annotation count %d exceeds 255", len(annotations))
	}
	if len(visible) != len(annotations) {
		return nil, fmt.Errorf("parameter annotation sets and visibility differ in length")
	}
	out := &byteWriter{}
	out.u1(uint8(len(annotations)))
	any := false
	for index, set := range annotations {
		if len(visible[index]) != len(set) {
			return nil, fmt.Errorf("parameter %d annotations and visibility differ in length", index)
		}
		selected := make([]Annotation, 0, len(set))
		for i, annotation := range set {
			if visible[index][i] == wantVisible {
				selected = append(selected, annotation)
			}
		}
		if len(selected) > 0 {
			any = true
		}
		if len(selected) > math.MaxUint16 {
			return nil, fmt.Errorf("parameter %d annotation count %d exceeds 65535", index, len(selected))
		}
		out.u2(uint16(len(selected)))
		for _, annotation := range selected {
			if err := writeAnnotation(pool, out, annotation); err != nil {
				return nil, err
			}
		}
	}
	if !any {
		return nil, nil
	}
	return out.data, nil
}

func classIndexFor(pool *ConstantPool, descriptor string) (uint16, error) {
	name, err := internalNameFromDescriptor(descriptor)
	if err != nil {
		return 0, err
	}
	return pool.Class(name)
}

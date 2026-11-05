package lower

import (
	"fmt"

	"github.com/yaklang/javajive/classparser/classfilebuilder"
	"github.com/yaklang/javajive/dex2jar/model"
)

// translatedAnnotations is the classfile-ready projection of one DEX annotation
// set: user annotations split by visibility plus the metadata that must become
// dedicated attributes.
type translatedAnnotations struct {
	annotations []classfilebuilder.Annotation
	visible     []bool
	signature   string
	throws      []string
	defaultVal  *classfilebuilder.AnnotationValue
}

// translateAnnotations separates user annotations from DEX system annotations
// that the classfile expresses as dedicated attributes.
func translateAnnotations(file *model.File, annotations []model.Annotation) (translatedAnnotations, error) {
	var out translatedAnnotations
	for _, annotation := range annotations {
		if isSystemAnnotation(annotation) {
			switch annotation.Type {
			case "Ldalvik/annotation/Signature;":
				parts, err := stringArrayElement(file, annotation, "value")
				if err != nil {
					return out, err
				}
				for _, part := range parts {
					out.signature += part
				}
			case "Ldalvik/annotation/Throws;":
				descriptors, err := typeArrayElement(file, annotation, "value")
				if err != nil {
					return out, err
				}
				out.throws = append(out.throws, descriptors...)
			case "Ldalvik/annotation/AnnotationDefault;":
				value, ok := annotationElement(annotation, "value")
				if !ok {
					return out, fmt.Errorf("AnnotationDefault is missing element \"value\"")
				}
				converted, err := builderValue(file, value)
				if err != nil {
					return out, err
				}
				out.defaultVal = &converted
			default:
				// InnerClass/EnclosingClass/EnclosingMethod/MemberClasses are
				// class-level; post-Java-5 annotations are dropped deliberately.
			}
			continue
		}
		converted, err := builderAnnotation(file, annotation)
		if err != nil {
			return out, err
		}
		out.annotations = append(out.annotations, converted)
		out.visible = append(out.visible, annotation.Visibility == model.AnnotationRuntime)
	}
	return out, nil
}

func builderAnnotation(file *model.File, annotation model.Annotation) (classfilebuilder.Annotation, error) {
	if annotation.Type == "" {
		return classfilebuilder.Annotation{}, fmt.Errorf("annotation descriptor is empty")
	}
	out := classfilebuilder.Annotation{Descriptor: annotation.Type}
	for _, element := range annotation.Elements {
		value, err := builderValue(file, element.Value)
		if err != nil {
			return classfilebuilder.Annotation{}, fmt.Errorf("annotation %s element %q: %w", annotation.Type, element.Name, err)
		}
		out.Elements = append(out.Elements, classfilebuilder.AnnotationEntry{Name: element.Name, Value: value})
	}
	return out, nil
}

func builderValue(file *model.File, value model.EncodedValue) (classfilebuilder.AnnotationValue, error) {
	switch value.Type {
	case model.ValueBoolean:
		flag := int64(0)
		if value.Boolean {
			flag = 1
		}
		return classfilebuilder.AnnotationValue{Kind: classfilebuilder.AnnotationBool, Int: flag}, nil
	case model.ValueByte:
		return classfilebuilder.AnnotationValue{Kind: classfilebuilder.AnnotationByte, Int: int64(int8(value.Signed))}, nil
	case model.ValueShort:
		return classfilebuilder.AnnotationValue{Kind: classfilebuilder.AnnotationShort, Int: int64(int16(value.Signed))}, nil
	case model.ValueChar:
		return classfilebuilder.AnnotationValue{Kind: classfilebuilder.AnnotationChar, Int: int64(uint16(value.Unsigned))}, nil
	case model.ValueInt:
		return classfilebuilder.AnnotationValue{Kind: classfilebuilder.AnnotationInt, Int: int64(int32(value.Signed))}, nil
	case model.ValueString:
		if value.Unsigned >= uint64(len(file.Strings)) {
			return classfilebuilder.AnnotationValue{}, fmt.Errorf("string index %d is out of range", value.Unsigned)
		}
		return classfilebuilder.AnnotationValue{Kind: classfilebuilder.AnnotationString, Text: file.Strings[value.Unsigned]}, nil
	case model.ValueTypeRef:
		if value.Unsigned >= uint64(len(file.Types)) {
			return classfilebuilder.AnnotationValue{}, fmt.Errorf("type index %d is out of range", value.Unsigned)
		}
		return classfilebuilder.AnnotationValue{Kind: classfilebuilder.AnnotationClass, Text: file.Types[value.Unsigned].Descriptor}, nil
	case model.ValueEnum:
		if value.Unsigned >= uint64(len(file.Fields)) {
			return classfilebuilder.AnnotationValue{}, fmt.Errorf("enum field index %d is out of range", value.Unsigned)
		}
		enum := file.Fields[value.Unsigned]
		return classfilebuilder.AnnotationValue{Kind: classfilebuilder.AnnotationEnum, Type: enum.Class, Text: enum.Name}, nil
	case model.ValueAnnotation:
		if value.Annotation == nil {
			return classfilebuilder.AnnotationValue{}, fmt.Errorf("nested annotation is nil")
		}
		nested, err := builderAnnotation(file, model.Annotation{Type: value.Annotation.Type, Elements: value.Annotation.Elements})
		if err != nil {
			return classfilebuilder.AnnotationValue{}, err
		}
		return classfilebuilder.AnnotationValue{Kind: classfilebuilder.AnnotationNested, Nested: &nested}, nil
	case model.ValueArray:
		array := make([]classfilebuilder.AnnotationValue, 0, len(value.Array))
		for _, item := range value.Array {
			converted, err := builderValue(file, item)
			if err != nil {
				return classfilebuilder.AnnotationValue{}, err
			}
			array = append(array, converted)
		}
		return classfilebuilder.AnnotationValue{Kind: classfilebuilder.AnnotationArray, Array: array}, nil
	default:
		return classfilebuilder.AnnotationValue{}, fmt.Errorf("annotation value kind %#x has no classfile representation", value.Type)
	}
}

func isSystemAnnotation(annotation model.Annotation) bool {
	const prefix = "Ldalvik/annotation/"
	return annotation.Visibility == model.AnnotationSystem || len(annotation.Type) > len(prefix) && annotation.Type[:len(prefix)] == prefix
}

func annotationElement(annotation model.Annotation, name string) (model.EncodedValue, bool) {
	for _, element := range annotation.Elements {
		if element.Name == name {
			return element.Value, true
		}
	}
	return model.EncodedValue{}, false
}

func stringArrayElement(file *model.File, annotation model.Annotation, name string) ([]string, error) {
	value, ok := annotationElement(annotation, name)
	if !ok {
		return nil, fmt.Errorf("system annotation %s is missing element %q", annotation.Type, name)
	}
	if value.Type != model.ValueArray {
		return nil, fmt.Errorf("system annotation %s element %q is not an array", annotation.Type, name)
	}
	parts := make([]string, 0, len(value.Array))
	for _, item := range value.Array {
		if item.Type != model.ValueString {
			return nil, fmt.Errorf("system annotation %s element %q contains a non-string item", annotation.Type, name)
		}
		if item.Unsigned >= uint64(len(file.Strings)) {
			return nil, fmt.Errorf("string index %d is out of range", item.Unsigned)
		}
		parts = append(parts, file.Strings[item.Unsigned])
	}
	return parts, nil
}

func typeArrayElement(file *model.File, annotation model.Annotation, name string) ([]string, error) {
	value, ok := annotationElement(annotation, name)
	if !ok {
		return nil, fmt.Errorf("system annotation %s is missing element %q", annotation.Type, name)
	}
	if value.Type != model.ValueArray {
		return nil, fmt.Errorf("system annotation %s element %q is not an array", annotation.Type, name)
	}
	descriptors := make([]string, 0, len(value.Array))
	for _, item := range value.Array {
		if item.Type != model.ValueTypeRef {
			return nil, fmt.Errorf("system annotation %s element %q contains a non-type item", annotation.Type, name)
		}
		if item.Unsigned >= uint64(len(file.Types)) {
			return nil, fmt.Errorf("type index %d is out of range", item.Unsigned)
		}
		descriptors = append(descriptors, file.Types[item.Unsigned].Descriptor)
	}
	return descriptors, nil
}

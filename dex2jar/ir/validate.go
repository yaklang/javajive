package ir

import (
	"fmt"
	"strings"

	"github.com/yaklang/javajive/dex2jar/model"
)

const maxArrayDimensions = 255

type descriptorKind uint8

const (
	descriptorVoid descriptorKind = iota
	descriptorInt
	descriptorWideOrFloat
	descriptorReference
)

func parseDescriptor(descriptor string, allowVoid bool) (descriptorKind, error) {
	end, kind, err := parseType(descriptor, 0, allowVoid)
	if err != nil {
		return descriptorVoid, fmt.Errorf("invalid descriptor %q: %w", descriptor, err)
	}
	if end != len(descriptor) {
		return descriptorVoid, fmt.Errorf("invalid descriptor %q: trailing data", descriptor)
	}
	return kind, nil
}

func parseType(descriptor string, at int, allowVoid bool) (int, descriptorKind, error) {
	if at >= len(descriptor) {
		return at, descriptorVoid, fmt.Errorf("missing type")
	}
	switch descriptor[at] {
	case 'V':
		if !allowVoid {
			return at, descriptorVoid, fmt.Errorf("void is not permitted here")
		}
		return at + 1, descriptorVoid, nil
	case 'Z', 'B', 'C', 'S', 'I':
		return at + 1, descriptorInt, nil
	case 'J', 'F', 'D':
		return at + 1, descriptorWideOrFloat, nil
	case 'L':
		semicolon := strings.IndexByte(descriptor[at+1:], ';')
		if semicolon < 0 {
			return at, descriptorVoid, fmt.Errorf("unterminated object type")
		}
		semicolon += at + 1
		if err := validateInternalName(descriptor[at+1 : semicolon]); err != nil {
			return at, descriptorVoid, fmt.Errorf("invalid object type: %w", err)
		}
		return semicolon + 1, descriptorReference, nil
	case '[':
		i := at
		for i < len(descriptor) && descriptor[i] == '[' {
			i++
			if i-at > maxArrayDimensions {
				return at, descriptorVoid, fmt.Errorf("array type has more than %d dimensions", maxArrayDimensions)
			}
		}
		if i >= len(descriptor) || descriptor[i] == 'V' {
			return at, descriptorVoid, fmt.Errorf("invalid array component")
		}
		end, _, err := parseType(descriptor, i, false)
		return end, descriptorReference, err
	default:
		return at, descriptorVoid, fmt.Errorf("invalid type tag %q", descriptor[at])
	}
}

func validateInternalName(name string) error {
	if name == "" {
		return fmt.Errorf("internal name is empty")
	}
	start := 0
	for index := 0; index <= len(name); index++ {
		if index != len(name) && name[index] != '/' {
			continue
		}
		if index == start {
			return fmt.Errorf("internal name %q has an empty component", name)
		}
		if err := validateUnqualifiedName(name[start:index], "internal name component", false); err != nil {
			return err
		}
		start = index + 1
	}
	return nil
}

func validateUnqualifiedName(name, kind string, method bool) error {
	if name == "" {
		return fmt.Errorf("%s is empty", kind)
	}
	for _, forbidden := range ".;[/" {
		if strings.ContainsRune(name, forbidden) {
			return fmt.Errorf("%s %q contains forbidden character %q", kind, name, forbidden)
		}
	}
	if method && (strings.ContainsRune(name, '<') || strings.ContainsRune(name, '>')) {
		return fmt.Errorf("%s %q contains forbidden angle bracket", kind, name)
	}
	return nil
}

func validateFieldName(name string) error {
	return validateUnqualifiedName(name, "field name", false)
}

func validateMethodName(name string) error {
	if name == "<init>" || name == "<clinit>" {
		return nil
	}
	return validateUnqualifiedName(name, "method name", true)
}

func validateReferenceDescriptor(descriptor string) error {
	kind, err := parseDescriptor(descriptor, false)
	if err != nil {
		return err
	}
	if kind != descriptorReference {
		return fmt.Errorf("descriptor %q is not a reference type", descriptor)
	}
	return nil
}

func validateObjectDescriptor(descriptor string) error {
	if err := validateReferenceDescriptor(descriptor); err != nil {
		return err
	}
	if descriptor[0] != 'L' {
		return fmt.Errorf("descriptor %q is not an object type", descriptor)
	}
	return nil
}

func validateProto(proto model.Proto) error {
	if _, err := parseDescriptor(proto.ReturnType, true); err != nil {
		return fmt.Errorf("return type: %w", err)
	}
	if len(proto.Parameters) != len(proto.ParameterTypes) {
		return fmt.Errorf("prototype has %d parameter descriptors and %d type indices", len(proto.Parameters), len(proto.ParameterTypes))
	}
	for index, descriptor := range proto.Parameters {
		if _, err := parseDescriptor(descriptor, false); err != nil {
			return fmt.Errorf("parameter %d: %w", index, err)
		}
	}
	return nil
}

func validateFileMetadata(file *model.File) error {
	metadata := file.Header.StringIDsSize != 0 || file.Header.TypeIDsSize != 0 || file.Header.ProtoIDsSize != 0 || file.Header.FieldIDsSize != 0 || file.Header.MethodIDsSize != 0
	if metadata {
		counts := []struct {
			name string
			head uint32
			got  int
		}{
			{"string_ids", file.Header.StringIDsSize, len(file.Strings)},
			{"type_ids", file.Header.TypeIDsSize, len(file.Types)},
			{"proto_ids", file.Header.ProtoIDsSize, len(file.Protos)},
			{"field_ids", file.Header.FieldIDsSize, len(file.Fields)},
			{"method_ids", file.Header.MethodIDsSize, len(file.Methods)},
		}
		for _, count := range counts {
			if uint32(count.got) != count.head {
				return fmt.Errorf("%s size %d does not match resolved table length %d", count.name, count.head, count.got)
			}
		}
	}
	for index, typ := range file.Types {
		if _, err := parseDescriptor(typ.Descriptor, true); err != nil {
			return fmt.Errorf("type %d: %w", index, err)
		}
		if metadata {
			if typ.DescriptorIndex >= uint32(len(file.Strings)) {
				return fmt.Errorf("type %d descriptor index %d is out of range", index, typ.DescriptorIndex)
			}
			if file.Strings[typ.DescriptorIndex] != typ.Descriptor {
				return fmt.Errorf("type %d descriptor does not match string index %d", index, typ.DescriptorIndex)
			}
		}
	}
	for index, proto := range file.Protos {
		if err := validateProto(proto); err != nil {
			return fmt.Errorf("prototype %d: %w", index, err)
		}
		if metadata {
			if proto.ShortyIndex >= uint32(len(file.Strings)) {
				return fmt.Errorf("prototype %d shorty index %d is out of range", index, proto.ShortyIndex)
			}
			if file.Strings[proto.ShortyIndex] != proto.Shorty {
				return fmt.Errorf("prototype %d shorty does not match string index %d", index, proto.ShortyIndex)
			}
			if proto.ReturnTypeIndex >= uint32(len(file.Types)) || file.Types[proto.ReturnTypeIndex].Descriptor != proto.ReturnType {
				return fmt.Errorf("prototype %d return type index %d is invalid or inconsistent", index, proto.ReturnTypeIndex)
			}
			for parameter, typeIndex := range proto.ParameterTypes {
				if typeIndex >= uint32(len(file.Types)) || file.Types[typeIndex].Descriptor != proto.Parameters[parameter] {
					return fmt.Errorf("prototype %d parameter %d type index %d is invalid or inconsistent", index, parameter, typeIndex)
				}
			}
		}
	}
	for index, field := range file.Fields {
		if err := validateObjectDescriptor(field.Class); err != nil {
			return fmt.Errorf("field %d owner: %w", index, err)
		}
		if _, err := parseDescriptor(field.Type, false); err != nil {
			return fmt.Errorf("field %d type: %w", index, err)
		}
		if err := validateFieldName(field.Name); err != nil {
			return fmt.Errorf("field %d: %w", index, err)
		}
		if metadata {
			if uint32(field.ClassIndex) >= uint32(len(file.Types)) || file.Types[field.ClassIndex].Descriptor != field.Class {
				return fmt.Errorf("field %d class index %d is invalid or inconsistent", index, field.ClassIndex)
			}
			if uint32(field.TypeIndex) >= uint32(len(file.Types)) || file.Types[field.TypeIndex].Descriptor != field.Type {
				return fmt.Errorf("field %d type index %d is invalid or inconsistent", index, field.TypeIndex)
			}
			if field.NameIndex >= uint32(len(file.Strings)) || file.Strings[field.NameIndex] != field.Name {
				return fmt.Errorf("field %d name index %d is invalid or inconsistent", index, field.NameIndex)
			}
		}
	}
	for index, method := range file.Methods {
		if err := validateReferenceDescriptor(method.Class); err != nil {
			return fmt.Errorf("method %d owner: %w", index, err)
		}
		if err := validateMethodName(method.Name); err != nil {
			return fmt.Errorf("method %d: %w", index, err)
		}
		if int(method.ProtoIndex) >= len(file.Protos) {
			return fmt.Errorf("method %d proto index %d is out of range", index, method.ProtoIndex)
		}
		proto := file.Protos[method.ProtoIndex]
		if method.Class[0] == '[' && (method.Name != "clone" || len(proto.Parameters) != 0 || proto.ReturnType != "Ljava/lang/Object;") {
			return fmt.Errorf("method %d array owner only permits clone()Ljava/lang/Object;", index)
		}
		if method.Name == "<init>" && proto.ReturnType != "V" {
			return fmt.Errorf("method %d constructor must return void", index)
		}
		if method.Name == "<clinit>" && (len(proto.Parameters) != 0 || proto.ReturnType != "V") {
			return fmt.Errorf("method %d class initializer must have descriptor ()V", index)
		}
		if metadata {
			if uint32(method.ClassIndex) >= uint32(len(file.Types)) || file.Types[method.ClassIndex].Descriptor != method.Class {
				return fmt.Errorf("method %d class index %d is invalid or inconsistent", index, method.ClassIndex)
			}
			if method.NameIndex >= uint32(len(file.Strings)) || file.Strings[method.NameIndex] != method.Name {
				return fmt.Errorf("method %d name index %d is invalid or inconsistent", index, method.NameIndex)
			}
		}
	}
	return nil
}

func validateInputMetadata(in Input) error {
	if err := validateObjectDescriptor(in.Class.Descriptor); err != nil {
		return fmt.Errorf("class descriptor: %w", err)
	}
	if err := validateObjectDescriptor(in.Method.Method.Class); err != nil {
		return fmt.Errorf("method owner: %w", err)
	}
	if err := validateMethodName(in.Method.Method.Name); err != nil {
		return err
	}
	if in.Method.Method.Class != in.Class.Descriptor {
		return fmt.Errorf("method owner %q does not match class descriptor %q", in.Method.Method.Class, in.Class.Descriptor)
	}
	if int(in.Method.Method.ProtoIndex) < len(in.File.Protos) {
		proto := in.File.Protos[in.Method.Method.ProtoIndex]
		if in.Method.Method.Name == "<init>" && proto.ReturnType != "V" {
			return fmt.Errorf("constructor must return void")
		}
		if in.Method.Method.Name == "<clinit>" && (len(proto.Parameters) != 0 || proto.ReturnType != "V") {
			return fmt.Errorf("class initializer must have descriptor ()V")
		}
	}
	metadata := in.File.Header.StringIDsSize != 0 || in.File.Header.TypeIDsSize != 0 || in.File.Header.ProtoIDsSize != 0 || in.File.Header.FieldIDsSize != 0 || in.File.Header.MethodIDsSize != 0
	if !metadata {
		return nil
	}
	if in.Class.ClassIndex >= uint32(len(in.File.Types)) || in.File.Types[in.Class.ClassIndex].Descriptor != in.Class.Descriptor {
		return fmt.Errorf("class index %d is invalid or inconsistent", in.Class.ClassIndex)
	}
	if in.Method.MethodIndex >= uint32(len(in.File.Methods)) {
		return fmt.Errorf("method index %d is out of range", in.Method.MethodIndex)
	}
	if in.File.Methods[in.Method.MethodIndex] != in.Method.Method {
		return fmt.Errorf("method index %d does not match encoded method metadata", in.Method.MethodIndex)
	}
	return nil
}

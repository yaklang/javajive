package javaclassparser

import (
	"fmt"

	"github.com/yaklang/javajive/internal/mutf8"
)

// CheckConstantPoolLinks validates every constant-pool reference, including
// unused constants. Deep source-recovery code must not be the first consumer to
// discover a malformed index or a tag that the declared classfile version could
// not contain.
func (obj *ClassObject) CheckConstantPoolLinks() error {
	if obj == nil {
		return nil
	}
	utf := func(index uint16, field string) (*ConstantUtf8Info, error) {
		if err := obj.checkCPIndex(index, false, field, CONSTANT_Utf8); err != nil {
			return nil, err
		}
		return obj.ConstantPool[index-1].(*ConstantUtf8Info), nil
	}
	nameAndType := func(index uint16, field string) (*ConstantNameAndTypeInfo, error) {
		if err := obj.checkCPIndex(index, false, field, CONSTANT_NameAndType); err != nil {
			return nil, err
		}
		return obj.ConstantPool[index-1].(*ConstantNameAndTypeInfo), nil
	}
	bootstrapCount := -1
	for _, attribute := range obj.Attributes {
		if table, ok := attribute.(*BootstrapMethodsAttribute); ok && table != nil {
			bootstrapCount = len(table.BootstrapMethods)
			break
		}
	}
	for index, constant := range obj.ConstantPool {
		cpIndex := index + 1
		if constant == nil {
			continue
		}
		minimum, err := minimumConstantMajor(constant)
		if err != nil {
			return fmt.Errorf("constant pool[%d]: %w", cpIndex, err)
		}
		if obj.MajorVersion >= MinSupportedClassMajor && obj.MajorVersion < minimum {
			return fmt.Errorf("constant pool[%d]: tag %d requires classfile major %d, got %d", cpIndex, constantTagOf(constant), minimum, obj.MajorVersion)
		}
		switch value := constant.(type) {
		case *ConstantStringInfo:
			if _, err := utf(value.StringIndex, "CONSTANT_String.string_index"); err != nil {
				return fmt.Errorf("constant pool[%d]: %w", cpIndex, err)
			}
		case *ConstantFieldrefInfo:
			if err := obj.checkCPIndex(value.ClassIndex, false, "CONSTANT_Fieldref.class_index", CONSTANT_Class); err != nil {
				return fmt.Errorf("constant pool[%d]: %w", cpIndex, err)
			}
			nat, err := nameAndType(value.NameAndTypeIndex, "CONSTANT_Fieldref.name_and_type_index")
			if err != nil {
				return fmt.Errorf("constant pool[%d]: %w", cpIndex, err)
			}
			descriptor, err := utf(nat.DescriptorIndex, "CONSTANT_Fieldref.descriptor_index")
			if err != nil || mutf8.ValidateFieldDescriptor(descriptor.semanticUnits()) != nil {
				return fmt.Errorf("constant pool[%d]: field reference has invalid descriptor", cpIndex)
			}
		case *ConstantMethodrefInfo:
			if err := obj.checkMemberReference(cpIndex, value.ClassIndex, value.NameAndTypeIndex, false); err != nil {
				return err
			}
		case *ConstantInterfaceMethodrefInfo:
			if err := obj.checkMemberReference(cpIndex, value.ClassIndex, value.NameAndTypeIndex, true); err != nil {
				return err
			}
		case *ConstantMethodTypeInfo:
			descriptor, err := utf(value.DescriptorIndex, "CONSTANT_MethodType.descriptor_index")
			if err != nil {
				return fmt.Errorf("constant pool[%d]: %w", cpIndex, err)
			}
			if err := mutf8.ValidateMethodDescriptor(descriptor.semanticUnits()); err != nil {
				return fmt.Errorf("constant pool[%d]: invalid method type descriptor: %w", cpIndex, err)
			}
		case *ConstantMethodHandleInfo:
			if err := obj.checkMethodHandle(cpIndex, value); err != nil {
				return err
			}
		case *ConstantInvokeDynamicInfo:
			if err := obj.checkDynamicReference(cpIndex, value.BootstrapMethodAttrIndex, value.NameAndTypeIndex, bootstrapCount, true); err != nil {
				return err
			}
		case *ConstantDynamicInfo:
			if err := obj.checkDynamicReference(cpIndex, value.BootstrapMethodAttrIndex, value.NameAndTypeIndex, bootstrapCount, false); err != nil {
				return err
			}
		case *ConstantModuleInfo:
			if _, err := utf(value.NameIndex, "CONSTANT_Module.name_index"); err != nil {
				return fmt.Errorf("constant pool[%d]: %w", cpIndex, err)
			}
		case *ConstantPackageInfo:
			if _, err := utf(value.NameIndex, "CONSTANT_Package.name_index"); err != nil {
				return fmt.Errorf("constant pool[%d]: %w", cpIndex, err)
			}
		}
	}
	return nil
}

func minimumConstantMajor(constant ConstantInfo) (uint16, error) {
	switch constant.(type) {
	case *ConstantMethodHandleInfo, *ConstantMethodTypeInfo, *ConstantInvokeDynamicInfo:
		return 51, nil
	case *ConstantModuleInfo, *ConstantPackageInfo:
		return 53, nil
	case *ConstantDynamicInfo:
		return 55, nil
	case *ConstantUtf8Info, *ConstantIntegerInfo, *ConstantFloatInfo, *ConstantLongInfo, *ConstantDoubleInfo,
		*ConstantClassInfo, *ConstantStringInfo, *ConstantFieldrefInfo, *ConstantMethodrefInfo,
		*ConstantInterfaceMethodrefInfo, *ConstantNameAndTypeInfo:
		return 45, nil
	default:
		return 0, fmt.Errorf("unknown constant type %T", constant)
	}
}

func (obj *ClassObject) checkMemberReference(cpIndex int, classIndex, nameAndTypeIndex uint16, interfaceReference bool) error {
	if err := obj.checkCPIndex(classIndex, false, "memberref.class_index", CONSTANT_Class); err != nil {
		return fmt.Errorf("constant pool[%d]: %w", cpIndex, err)
	}
	if err := obj.checkCPIndex(nameAndTypeIndex, false, "memberref.name_and_type_index", CONSTANT_NameAndType); err != nil {
		return fmt.Errorf("constant pool[%d]: %w", cpIndex, err)
	}
	nat := obj.ConstantPool[nameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
	descriptor, err := obj.getUtf8Units(nat.DescriptorIndex)
	if err != nil {
		return fmt.Errorf("constant pool[%d]: method descriptor: %w", cpIndex, err)
	}
	if err := mutf8.ValidateMethodDescriptor(descriptor); err != nil {
		kind := "method"
		if interfaceReference {
			kind = "interface method"
		}
		return fmt.Errorf("constant pool[%d]: invalid %s descriptor: %w", cpIndex, kind, err)
	}
	return nil
}

func (obj *ClassObject) checkMethodHandle(cpIndex int, handle *ConstantMethodHandleInfo) error {
	if handle.ReferenceKind < 1 || handle.ReferenceKind > 9 {
		return fmt.Errorf("constant pool[%d]: invalid method handle reference kind %d", cpIndex, handle.ReferenceKind)
	}
	var tags []uint8
	switch handle.ReferenceKind {
	case 1, 2, 3, 4:
		tags = []uint8{CONSTANT_Fieldref}
	case 5, 8:
		tags = []uint8{CONSTANT_Methodref}
	case 6, 7:
		tags = []uint8{CONSTANT_Methodref}
		if obj.MajorVersion >= 52 {
			tags = append(tags, CONSTANT_InterfaceMethodref)
		}
	case 9:
		tags = []uint8{CONSTANT_InterfaceMethodref}
	}
	if err := obj.checkCPIndex(handle.ReferenceIndex, false, "CONSTANT_MethodHandle.reference_index", tags...); err != nil {
		return fmt.Errorf("constant pool[%d]: %w", cpIndex, err)
	}
	return nil
}

func (obj *ClassObject) checkDynamicReference(cpIndex int, bootstrapIndex, nameAndTypeIndex uint16, bootstrapCount int, method bool) error {
	if bootstrapCount < 0 || int(bootstrapIndex) >= bootstrapCount {
		return fmt.Errorf("constant pool[%d]: bootstrap method index %d is out of range", cpIndex, bootstrapIndex)
	}
	if err := obj.checkCPIndex(nameAndTypeIndex, false, "dynamic.name_and_type_index", CONSTANT_NameAndType); err != nil {
		return fmt.Errorf("constant pool[%d]: %w", cpIndex, err)
	}
	nat := obj.ConstantPool[nameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
	descriptor, err := obj.getUtf8Units(nat.DescriptorIndex)
	if err != nil {
		return fmt.Errorf("constant pool[%d]: dynamic descriptor: %w", cpIndex, err)
	}
	if method {
		err = mutf8.ValidateMethodDescriptor(descriptor)
	} else {
		err = mutf8.ValidateFieldDescriptor(descriptor)
	}
	if err != nil {
		return fmt.Errorf("constant pool[%d]: invalid dynamic descriptor: %w", cpIndex, err)
	}
	return nil
}

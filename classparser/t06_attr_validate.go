package javaclassparser

import (
	"fmt"
)

type attrContext int

const (
	attrCtxClass attrContext = iota
	attrCtxField
	attrCtxMethod
	attrCtxCode
)

func (c attrContext) String() string {
	switch c {
	case attrCtxClass:
		return "class"
	case attrCtxField:
		return "field"
	case attrCtxMethod:
		return "method"
	case attrCtxCode:
		return "code"
	default:
		return "unknown"
	}
}

// attrMustBeUnique is per-name, per attributes table. LineNumberTable,
// LocalVariableTable, LocalVariableTypeTable and unknown names may repeat.
func attrMustBeUnique(name string) bool {
	switch name {
	case "SourceFile", "InnerClasses", "BootstrapMethods", "Signature",
		"EnclosingMethod", "Module", "NestHost", "Record", "PermittedSubclasses",
		"RuntimeVisibleAnnotations", "RuntimeInvisibleAnnotations",
		"RuntimeVisibleTypeAnnotations", "RuntimeInvisibleTypeAnnotations",
		"ConstantValue", "Code", "Exceptions", "AnnotationDefault",
		"RuntimeVisibleParameterAnnotations", "RuntimeInvisibleParameterAnnotations",
		"MethodParameters", "StackMapTable", "Synthetic", "Deprecated":
		return true
	default:
		return false
	}
}

func attrAllowedIn(name string, ctx attrContext) bool {
	switch name {
	case "Code":
		return ctx == attrCtxMethod
	case "ConstantValue":
		return ctx == attrCtxField
	case "SourceFile", "InnerClasses", "BootstrapMethods":
		return ctx == attrCtxClass
	case "AnnotationDefault":
		return ctx == attrCtxMethod
	case "LineNumberTable":
		return ctx == attrCtxCode
	case "Exceptions", "MethodParameters",
		"RuntimeVisibleParameterAnnotations", "RuntimeInvisibleParameterAnnotations":
		return ctx == attrCtxMethod
	case "EnclosingMethod", "Module", "NestHost", "Record", "PermittedSubclasses",
		"SourceDebugExtension", "ModulePackages", "ModuleMainClass", "NestMembers":
		return ctx == attrCtxClass
	case "StackMapTable", "LocalVariableTable", "LocalVariableTypeTable":
		return ctx == attrCtxCode
	case "Signature", "Synthetic", "Deprecated",
		"RuntimeVisibleAnnotations", "RuntimeInvisibleAnnotations":
		return ctx == attrCtxClass || ctx == attrCtxField || ctx == attrCtxMethod
	case "RuntimeVisibleTypeAnnotations", "RuntimeInvisibleTypeAnnotations":
		return ctx == attrCtxClass || ctx == attrCtxField || ctx == attrCtxMethod || ctx == attrCtxCode
	default:
		return true
	}
}

func constantTagOf(info ConstantInfo) uint8 {
	switch info.(type) {
	case *ConstantUtf8Info:
		return CONSTANT_Utf8
	case *ConstantIntegerInfo:
		return CONSTANT_Integer
	case *ConstantFloatInfo:
		return CONSTANT_Float
	case *ConstantLongInfo:
		return CONSTANT_Long
	case *ConstantDoubleInfo:
		return CONSTANT_Double
	case *ConstantClassInfo:
		return CONSTANT_Class
	case *ConstantStringInfo:
		return CONSTANT_String
	case *ConstantFieldrefInfo:
		return CONSTANT_Fieldref
	case *ConstantMethodrefInfo:
		return CONSTANT_Methodref
	case *ConstantInterfaceMethodrefInfo:
		return CONSTANT_InterfaceMethodref
	case *ConstantNameAndTypeInfo:
		return CONSTANT_NameAndType
	case *ConstantMethodHandleInfo:
		return CONSTANT_MethodHandle
	case *ConstantMethodTypeInfo:
		return CONSTANT_MethodType
	case *ConstantDynamicInfo:
		return CONSTANT_Dynamic
	case *ConstantInvokeDynamicInfo:
		return CONSTANT_InvokeDynamic
	case *ConstantModuleInfo:
		return CONSTANT_Module
	case *ConstantPackageInfo:
		return CONSTANT_Package
	default:
		return 0
	}
}

// checkCPIndex validates a constant-pool reference. index==0 is accepted only
// when allowZero is set (SuperClass of Object, catch_type 0, ...). Long/double
// unusable slots and tag mismatches are distinct from a mere bounds check.
func (this *ClassObject) checkCPIndex(index uint16, allowZero bool, field string, tags ...uint8) error {
	if index == 0 {
		if allowZero {
			return nil
		}
		return &ClassParseError{
			Code:  ParseCodeCPIndex,
			Stage: "constant_pool",
			Field: field,
			Msg:   fmt.Sprintf("CP index 0 is not allowed for %s", field),
		}
	}
	if this == nil {
		return &ClassParseError{
			Code:  ParseCodeCPIndex,
			Stage: "constant_pool",
			Field: field,
			Msg:   fmt.Sprintf("CP index %d: missing constant pool", index),
		}
	}
	info, err := this.getConstantInfo(index)
	if err != nil {
		return &ClassParseError{
			Code:  ParseCodeCPIndex,
			Stage: "constant_pool",
			Field: field,
			Msg:   fmt.Sprintf("CP index %d: %v", index, err),
		}
	}
	if len(tags) == 0 {
		return nil
	}
	got := constantTagOf(info)
	for _, t := range tags {
		if got == t {
			return nil
		}
	}
	return &ClassParseError{
		Code:  ParseCodeCPIndex,
		Stage: "constant_pool",
		Field: field,
		Msg:   fmt.Sprintf("CP index %d: tag %d not in %v", index, got, tags),
	}
}

func validateCodeLength(codeLength int) error {
	if codeLength <= 0 {
		return &ClassParseError{
			Code:  ParseCodeCodeLength,
			Stage: "code",
			Field: "code_length",
			Msg:   "code_length must be greater than 0",
		}
	}
	return nil
}

// validateExceptionTable checks structural PC bounds only (not instruction
// starts). catch_type 0 is allowed here; CP tag checks happen separately.
func validateExceptionTable(codeLength int, table []*ExceptionTableEntry) error {
	for i, e := range table {
		if e == nil {
			return &ClassParseError{
				Code:  ParseCodeHandlerRange,
				Stage: "code",
				Field: "exception_table",
				Msg:   fmt.Sprintf("handler %d is nil", i),
			}
		}
		if int(e.StartPc) >= int(e.EndPc) || int(e.EndPc) > codeLength || int(e.HandlerPc) >= codeLength {
			return &ClassParseError{
				Code:  ParseCodeHandlerRange,
				Stage: "code",
				Field: "exception_table",
				Msg:   fmt.Sprintf("handler %d start=%d end=%d handler_pc=%d code_length=%d", i, e.StartPc, e.EndPc, e.HandlerPc, codeLength),
			}
		}
	}
	return nil
}

func (self *CodeAttribute) validateStatic(cp *ClassParser) {
	if cp == nil || cp.reader == nil || cp.reader.Err() != nil {
		return
	}
	if err := validateCodeLength(len(self.Code)); err != nil {
		cp.reader.fail(ParseCodeCodeLength, err.Error())
		return
	}
	if err := validateExceptionTable(len(self.Code), self.ExceptionTable); err != nil {
		cp.reader.fail(ParseCodeHandlerRange, err.Error())
		return
	}
	for i, e := range self.ExceptionTable {
		if e == nil {
			continue
		}
		if err := cp.classObj.checkCPIndex(e.CatchType, true, fmt.Sprintf("exception_table[%d].catch_type", i), CONSTANT_Class); err != nil {
			cp.reader.fail(ParseCodeCPIndex, err.Error())
			return
		}
	}
}

func (this *ClassObject) checkClassHeadCPRefs() error {
	if err := this.checkCPIndex(this.ThisClass, false, "this_class", CONSTANT_Class); err != nil {
		return err
	}
	if err := this.checkCPIndex(this.SuperClass, true, "super_class", CONSTANT_Class); err != nil {
		return err
	}
	for i, idx := range this.Interfaces {
		if err := this.checkCPIndex(idx, false, fmt.Sprintf("interfaces[%d]", i), CONSTANT_Class); err != nil {
			return err
		}
	}
	return nil
}

// requireKnownAttrExactEnd fails when a dispatched (non-Unparsed) attribute did
// not consume its isolated attribute_length region.
func requireKnownAttrExactEnd(sub *ClassReader, attr AttributeInfo, name string) {
	if sub == nil || sub.Err() != nil {
		return
	}
	if _, ok := attr.(*UnparsedAttribute); ok {
		return
	}
	if sub.Remaining() != 0 {
		sub.SetStage("attribute", name)
		sub.fail(ParseCodeAttrLength, fmt.Sprintf("attribute %s left %d unconsumed bytes (declared length too long)", name, sub.Remaining()))
	}
}

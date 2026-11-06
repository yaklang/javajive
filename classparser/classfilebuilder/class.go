package classfilebuilder

import (
	"fmt"
	"math"
)

const (
	classMagic   = 0xcafebabe
	majorVersion = 49
)

type member struct {
	access                uint16
	nameIndex, descriptor uint16
	name, descriptorText  string
	code                  *Code
	constantValue         uint16
	meta                  annotationTarget
}

// Field is a field_info declaration owned by a Class.
type Field struct{ member }

// Method is a method_info declaration owned by a Class.
type Method struct{ member }

// Class is a deterministic Java 5 classfile builder.
type Class struct {
	pool                  *ConstantPool
	access                uint16
	thisClass, superClass uint16
	interfaces            []uint16
	fields                []*Field
	methods               []*Method
	codeName              uint16
	constantValueName     uint16
	sourceFileName        uint16
	sourceFile            uint16
	meta                  annotationTarget
	innerClasses          []InnerClassSpec
	enclosing             *EnclosingMethodSpec
	sourceDebug           string
	fieldAttrs            [][]pendingAttribute
	methodAttrs           [][]pendingAttribute
	classAttrs            []pendingAttribute
	err                   error
}

// NewClass creates a Java 5 class. Names use JVM internal form. superName may
// be empty only for java/lang/Object.
func NewClass(name, superName string, access uint16) *Class {
	class := &Class{pool: newConstantPool(), access: access}
	if err := validateInternalName(name); err != nil {
		class.fail("class name %q: %v", name, err)
		return class
	}
	var err error
	class.thisClass, err = class.pool.Class(name)
	if err != nil {
		class.fail("this class: %v", err)
		return class
	}
	if superName != "" {
		if err := validateInternalName(superName); err != nil {
			class.fail("super class %q: %v", superName, err)
			return class
		}
		class.superClass, err = class.pool.Class(superName)
		if err != nil {
			class.fail("super class: %v", err)
		}
	}
	return class
}

func (c *Class) fail(format string, args ...any) {
	if c.err == nil {
		c.err = fmt.Errorf(format, args...)
	}
}

// Pool returns the class's deduplicating constant pool. Indices returned from
// another Class must not be used with this Class.
func (c *Class) Pool() *ConstantPool { return c.pool }

// SetSourceFile records the original source file name in a SourceFile attribute.
// Calling it again replaces the previous value.
func (c *Class) SetSourceFile(name string) *Class {
	if name == "" {
		c.fail("source file name is empty")
		return c
	}
	var err error
	c.sourceFileName, err = c.pool.UTF8("SourceFile")
	if err != nil {
		c.fail("SourceFile attribute name: %v", err)
		return c
	}
	c.sourceFile, err = c.pool.UTF8(name)
	if err != nil {
		c.fail("source file %q: %v", name, err)
	}
	return c
}

// SetClassSignature records the generic signature of the class.
func (c *Class) SetClassSignature(signature string) *Class {
	c.meta.signature = signature
	return c
}

// SetFieldSignature records the generic signature of a field.
func (c *Class) SetFieldSignature(field *Field, signature string) *Class {
	if field == nil {
		c.fail("nil field for signature")
		return c
	}
	field.meta.signature = signature
	return c
}

// SetMethodSignature records the generic signature of a method.
func (c *Class) SetMethodSignature(method *Method, signature string) *Class {
	if method == nil {
		c.fail("nil method for signature")
		return c
	}
	method.meta.signature = signature
	return c
}

// SetMethodExceptions records the checked exception types a method declares.
func (c *Class) SetMethodExceptions(method *Method, descriptors []string) *Class {
	if method == nil {
		c.fail("nil method for Exceptions")
		return c
	}
	if len(descriptors) > math.MaxUint16 {
		c.fail("Exceptions length %d exceeds 65535", len(descriptors))
		return c
	}
	method.meta.exceptions = append([]string(nil), descriptors...)
	return c
}

// AddClassAnnotation attaches a runtime or build annotation to the class.
func (c *Class) AddClassAnnotation(annotation Annotation, visible bool) *Class {
	c.meta.addAnnotation(annotation, visible)
	return c
}

// AddFieldAnnotation attaches a runtime or build annotation to a field.
func (c *Class) AddFieldAnnotation(field *Field, annotation Annotation, visible bool) *Class {
	if field == nil {
		c.fail("nil field for annotation")
		return c
	}
	field.meta.addAnnotation(annotation, visible)
	return c
}

// AddMethodAnnotation attaches a runtime or build annotation to a method.
func (c *Class) AddMethodAnnotation(method *Method, annotation Annotation, visible bool) *Class {
	if method == nil {
		c.fail("nil method for annotation")
		return c
	}
	method.meta.addAnnotation(annotation, visible)
	return c
}

// SetMethodParameterAnnotations records one annotation set per declared parameter.
func (c *Class) SetMethodParameterAnnotations(method *Method, annotations [][]Annotation, visible [][]bool) *Class {
	if method == nil {
		c.fail("nil method for parameter annotations")
		return c
	}
	if len(annotations) != len(visible) {
		c.fail("parameter annotation sets and visibility differ in length")
		return c
	}
	if len(annotations) > math.MaxUint8 {
		c.fail("parameter annotation count %d exceeds 255", len(annotations))
		return c
	}
	method.meta.parameterAnnot = annotations
	method.meta.parameterVis = visible
	return c
}

// SetMethodAnnotationDefault records the default value of an annotation
// interface method.
func (c *Class) SetMethodAnnotationDefault(method *Method, value AnnotationValue) *Class {
	if method == nil {
		c.fail("nil method for AnnotationDefault")
		return c
	}
	method.meta.defaultValue = &value
	return c
}

// SetInnerClasses records the InnerClasses table of the class.
func (c *Class) SetInnerClasses(specs []InnerClassSpec) *Class {
	c.innerClasses = specs
	return c
}

// SetEnclosingMethod records the EnclosingMethod attribute of the class.
func (c *Class) SetEnclosingMethod(spec EnclosingMethodSpec) *Class {
	c.enclosing = &spec
	return c
}

// SetSourceDebugExtension records a SourceDebugExtension attribute.
func (c *Class) SetSourceDebugExtension(value string) *Class {
	c.sourceDebug = value
	return c
}

// AddInterface appends an implemented interface in declaration order.
func (c *Class) AddInterface(name string) *Class {
	if len(c.interfaces) >= math.MaxUint16 {
		c.fail("interfaces_count exceeds 65535")
		return c
	}
	if err := validateInternalName(name); err != nil {
		c.fail("interface %q: %v", name, err)
		return c
	}
	index, err := c.pool.Class(name)
	if err != nil {
		c.fail("interface %q: %v", name, err)
		return c
	}
	c.interfaces = append(c.interfaces, index)
	return c
}

// AddField appends a field with no attributes.
func (c *Class) AddField(access uint16, name, descriptor string) *Field {
	field := &Field{member: member{access: access, name: name, descriptorText: descriptor}}
	if len(c.fields) >= math.MaxUint16 {
		c.fail("fields_count exceeds 65535")
		return field
	}
	if err := validateFieldSymbol(name, descriptor); err != nil {
		c.fail("field %q: %v", name, err)
		return field
	}
	var err error
	field.nameIndex, err = c.pool.UTF8(name)
	if err != nil {
		c.fail("field name %q: %v", name, err)
		return field
	}
	field.descriptor, err = c.pool.UTF8(descriptor)
	if err != nil {
		c.fail("field descriptor %q: %v", descriptor, err)
		return field
	}
	c.fields = append(c.fields, field)
	return field
}

// SetConstantInt adds a ConstantValue attribute for an integer-like field.
func (c *Class) SetConstantInt(field *Field, value int32) *Class {
	if field == nil {
		c.fail("nil field for ConstantValue")
		return c
	}
	switch field.descriptorText {
	case "Z", "B", "C", "S", "I":
	default:
		c.fail("integer ConstantValue is incompatible with field %q descriptor %q", field.name, field.descriptorText)
		return c
	}
	index, err := c.pool.Integer(value)
	if err != nil {
		c.fail("field %q ConstantValue: %v", field.name, err)
		return c
	}
	c.setConstantValue(field, index)
	return c
}

// SetConstantString adds a ConstantValue attribute for a String field.
func (c *Class) SetConstantString(field *Field, value string) *Class {
	if field == nil {
		c.fail("nil field for ConstantValue")
		return c
	}
	if field.descriptorText != "Ljava/lang/String;" {
		c.fail("string ConstantValue is incompatible with field %q descriptor %q", field.name, field.descriptorText)
		return c
	}
	index, err := c.pool.String(value)
	if err != nil {
		c.fail("field %q ConstantValue: %v", field.name, err)
		return c
	}
	c.setConstantValue(field, index)
	return c
}

// SetConstantLong adds a ConstantValue attribute for a long field.
func (c *Class) SetConstantLong(field *Field, value int64) *Class {
	if field == nil {
		c.fail("nil field for ConstantValue")
		return c
	}
	if field.descriptorText != "J" {
		c.fail("long ConstantValue is incompatible with field %q descriptor %q", field.name, field.descriptorText)
		return c
	}
	index, err := c.pool.Long(value)
	if err != nil {
		c.fail("field %q ConstantValue: %v", field.name, err)
		return c
	}
	c.setConstantValue(field, index)
	return c
}

// SetConstantFloat adds a ConstantValue attribute for a float field from its
// IEEE-754 bit pattern.
func (c *Class) SetConstantFloat(field *Field, bits uint32) *Class {
	if field == nil {
		c.fail("nil field for ConstantValue")
		return c
	}
	if field.descriptorText != "F" {
		c.fail("float ConstantValue is incompatible with field %q descriptor %q", field.name, field.descriptorText)
		return c
	}
	index, err := c.pool.Float(bits)
	if err != nil {
		c.fail("field %q ConstantValue: %v", field.name, err)
		return c
	}
	c.setConstantValue(field, index)
	return c
}

// SetConstantDouble adds a ConstantValue attribute for a double field from its
// IEEE-754 bit pattern.
func (c *Class) SetConstantDouble(field *Field, bits uint64) *Class {
	if field == nil {
		c.fail("nil field for ConstantValue")
		return c
	}
	if field.descriptorText != "D" {
		c.fail("double ConstantValue is incompatible with field %q descriptor %q", field.name, field.descriptorText)
		return c
	}
	index, err := c.pool.Double(bits)
	if err != nil {
		c.fail("field %q ConstantValue: %v", field.name, err)
		return c
	}
	c.setConstantValue(field, index)
	return c
}

func (c *Class) setConstantValue(field *Field, index uint16) {
	if field.constantValue != 0 {
		c.fail("field %q already has a ConstantValue", field.name)
		return
	}
	var err error
	c.constantValueName, err = c.pool.UTF8("ConstantValue")
	if err != nil {
		c.fail("ConstantValue attribute name: %v", err)
		return
	}
	field.constantValue = index
}

// AddMethod appends a method. Concrete methods receive an empty symbolic Code;
// abstract and native methods have no Code attribute.
func (c *Class) AddMethod(access uint16, name, descriptor string) *Method {
	method := &Method{member: member{access: access, name: name, descriptorText: descriptor}}
	if len(c.methods) >= math.MaxUint16 {
		c.fail("methods_count exceeds 65535")
		return method
	}
	args, _, err := methodDescriptorWords(descriptor)
	if err != nil {
		c.fail("method %q: %v", name, err)
		return method
	}
	if err := validateMethodSymbol(name, descriptor); err != nil {
		c.fail("method %q: %v", name, err)
		return method
	}
	if access&ACC_STATIC == 0 && args == 255 {
		c.fail("method %q: parameter word count plus receiver exceeds 255", name)
		return method
	}
	if name == "<init>" && access&(ACC_STATIC|ACC_FINAL|ACC_SYNCHRONIZED|ACC_BRIDGE|ACC_NATIVE|ACC_ABSTRACT) != 0 {
		c.fail("constructor has incompatible access flags 0x%x", access)
		return method
	}
	if name == "<clinit>" {
		const forbidden = ACC_PUBLIC | ACC_PRIVATE | ACC_PROTECTED | ACC_FINAL | ACC_SYNCHRONIZED | ACC_BRIDGE | ACC_VARARGS | ACC_NATIVE | ACC_ABSTRACT | ACC_STRICT
		if access&ACC_STATIC == 0 || access&forbidden != 0 {
			c.fail("class initializer has incompatible access flags 0x%x", access)
			return method
		}
	}
	method.nameIndex, err = c.pool.UTF8(name)
	if err != nil {
		c.fail("method name %q: %v", name, err)
		return method
	}
	method.descriptor, err = c.pool.UTF8(descriptor)
	if err != nil {
		c.fail("method descriptor %q: %v", descriptor, err)
		return method
	}
	if access&(ACC_ABSTRACT|ACC_NATIVE) == 0 {
		method.code = newCode(c.pool)
		if c.codeName == 0 {
			c.codeName, err = c.pool.UTF8("Code")
			if err != nil {
				c.fail("Code attribute name: %v", err)
			}
		}
	}
	c.methods = append(c.methods, method)
	return method
}

// Code returns the symbolic assembler for a concrete method, or nil for an
// abstract/native method.
func (m *Method) Code() *Code {
	if m == nil {
		return nil
	}
	return m.code
}

// Bytes assembles and serializes the complete classfile.
func (c *Class) Bytes() ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("nil Class")
	}
	if c.err != nil {
		return nil, c.err
	}
	if c.superClass == 0 {
		entry, err := c.pool.entry(c.thisClass)
		if err != nil || entry.text != "java/lang/Object" {
			return nil, fmt.Errorf("super class may be empty only for java/lang/Object")
		}
	}
	assembled := make([]*assembledCode, len(c.methods))
	for i, method := range c.methods {
		if method.code == nil {
			continue
		}
		code, err := method.code.assemble(method.descriptorText, method.access)
		if err != nil {
			return nil, fmt.Errorf("method %s%s: %w", method.name, method.descriptorText, err)
		}
		assembled[i] = code
	}
	// Encode metadata attributes before serializing the pool: encoding adds
	// constant-pool entries, and indices must exist before the pool is written.
	c.fieldAttrs = make([][]pendingAttribute, len(c.fields))
	for index, field := range c.fields {
		if err := c.encodeMemberAttributes(&field.meta, &c.fieldAttrs[index]); err != nil {
			return nil, fmt.Errorf("field %q: %w", field.name, err)
		}
	}
	c.methodAttrs = make([][]pendingAttribute, len(c.methods))
	for index, method := range c.methods {
		if err := c.encodeMemberAttributes(&method.meta, &c.methodAttrs[index]); err != nil {
			return nil, fmt.Errorf("method %s%s: %w", method.name, method.descriptorText, err)
		}
	}
	if err := c.encodeClassAttributes(); err != nil {
		return nil, err
	}
	for index := range c.fieldAttrs {
		if err := c.resolvePending(c.fieldAttrs[index]); err != nil {
			return nil, fmt.Errorf("field %q: %w", c.fields[index].name, err)
		}
	}
	for index := range c.methodAttrs {
		if err := c.resolvePending(c.methodAttrs[index]); err != nil {
			return nil, fmt.Errorf("method %s%s: %w", c.methods[index].name, c.methods[index].descriptorText, err)
		}
	}
	if err := c.resolvePending(c.classAttrs); err != nil {
		return nil, err
	}

	out := &byteWriter{}
	out.u4(classMagic)
	out.u2(0)
	out.u2(majorVersion)
	if err := c.pool.write(out); err != nil {
		return nil, err
	}
	out.u2(c.access)
	out.u2(c.thisClass)
	out.u2(c.superClass)
	out.u2(uint16(len(c.interfaces)))
	for _, index := range c.interfaces {
		out.u2(index)
	}
	out.u2(uint16(len(c.fields)))
	for index, field := range c.fields {
		attributes := uint16(len(c.fieldAttrs[index]))
		if field.constantValue != 0 {
			attributes++
		}
		writeMemberHeader(out, &field.member, attributes)
		if field.constantValue != 0 {
			out.u2(c.constantValueName)
			out.u4(2)
			out.u2(field.constantValue)
		}
		for _, attribute := range c.fieldAttrs[index] {
			if err := writeAttribute(out, attribute); err != nil {
				return nil, err
			}
		}
	}
	out.u2(uint16(len(c.methods)))
	for i, method := range c.methods {
		attributes := uint16(len(c.methodAttrs[i]))
		if assembled[i] != nil {
			attributes++
		}
		writeMemberHeader(out, &method.member, attributes)
		if assembled[i] != nil {
			writeCodeAttribute(out, c.codeName, assembled[i])
		}
		for _, attribute := range c.methodAttrs[i] {
			if err := writeAttribute(out, attribute); err != nil {
				return nil, err
			}
		}
	}
	out.u2(uint16(len(c.classAttrs)))
	for _, attribute := range c.classAttrs {
		if err := writeAttribute(out, attribute); err != nil {
			return nil, err
		}
	}
	return out.data, nil
}

// encodeMemberAttributes prepares the non-Code attributes of one member.
func (c *Class) encodeMemberAttributes(meta *annotationTarget, out *[]pendingAttribute) error {
	if meta.signature != "" {
		signature := meta.signature
		*out = append(*out, pendingAttribute{name: "Signature", encode: func(pool *ConstantPool) ([]byte, error) {
			return encodeSignature(pool, signature)
		}})
	}
	if len(meta.exceptions) > 0 {
		exceptions := append([]string(nil), meta.exceptions...)
		*out = append(*out, pendingAttribute{name: "Exceptions", encode: func(pool *ConstantPool) ([]byte, error) {
			return encodeExceptions(pool, exceptions)
		}})
	}
	if meta.defaultValue != nil {
		value := *meta.defaultValue
		*out = append(*out, pendingAttribute{name: "AnnotationDefault", encode: func(pool *ConstantPool) ([]byte, error) {
			return encodeAnnotationDefault(pool, value)
		}})
	}
	if len(meta.parameterAnnot) > 0 {
		annotations := meta.parameterAnnot
		visible := meta.parameterVis
		for _, wantVisible := range []bool{true, false} {
			name := "RuntimeInvisibleParameterAnnotations"
			if wantVisible {
				name = "RuntimeVisibleParameterAnnotations"
			}
			want := wantVisible
			if !parameterAnnotationsPresent(annotations, visible, want) {
				continue
			}
			*out = append(*out, pendingAttribute{name: name, encode: func(pool *ConstantPool) ([]byte, error) {
				return encodeParameterAnnotations(pool, annotations, visible, want)
			}})
		}
	}
	return c.encodeAnnotationAttributes(meta, out)
}

func parameterAnnotationsPresent(annotations [][]Annotation, visible [][]bool, wantVisible bool) bool {
	for index, set := range annotations {
		for i := range set {
			if index < len(visible) && i < len(visible[index]) && visible[index][i] == wantVisible {
				return true
			}
		}
	}
	return false
}

func (c *Class) encodeAnnotationAttributes(meta *annotationTarget, out *[]pendingAttribute) error {
	if len(meta.annotations) == 0 {
		return nil
	}
	for _, wantVisible := range []bool{true, false} {
		name := "RuntimeInvisibleAnnotations"
		if wantVisible {
			name = "RuntimeVisibleAnnotations"
		}
		present := false
		for index := range meta.annotations {
			if meta.visible[index] == wantVisible {
				present = true
				break
			}
		}
		if !present {
			continue
		}
		annotations := meta.annotations
		visible := meta.visible
		want := wantVisible
		*out = append(*out, pendingAttribute{name: name, encode: func(pool *ConstantPool) ([]byte, error) {
			return encodeAnnotations(pool, annotations, visible, want)
		}})
	}
	return nil
}

// resolvePending encodes every attribute payload before the constant pool is
// serialized. Encoding may add pool entries, so it must never run afterwards.
func (c *Class) resolvePending(attributes []pendingAttribute) error {
	for index := range attributes {
		attribute := &attributes[index]
		nameIndex, err := c.pool.UTF8(attribute.name)
		if err != nil {
			return fmt.Errorf("%s attribute name: %w", attribute.name, err)
		}
		attribute.nameIndex = nameIndex
		if attribute.encode == nil {
			continue
		}
		payload, err := attribute.encode(c.pool)
		if err != nil {
			return fmt.Errorf("%s attribute: %w", attribute.name, err)
		}
		attribute.payload = payload
		attribute.encode = nil
	}
	return nil
}

func (c *Class) encodeClassAttributes() error {
	c.classAttrs = nil
	if c.sourceFile != 0 {
		// The attribute_name_index is written by writeAttribute; the payload is
		// only sourcefile_index.
		fileIndex := c.sourceFile
		c.classAttrs = append(c.classAttrs, pendingAttribute{name: "SourceFile", payload: []byte{
			byte(fileIndex >> 8), byte(fileIndex),
		}})
	}
	if c.sourceDebug != "" {
		value := c.sourceDebug
		c.classAttrs = append(c.classAttrs, pendingAttribute{name: "SourceDebugExtension", payload: []byte(value)})
	}
	if c.meta.signature != "" {
		signature := c.meta.signature
		c.classAttrs = append(c.classAttrs, pendingAttribute{name: "Signature", encode: func(pool *ConstantPool) ([]byte, error) {
			return encodeSignature(pool, signature)
		}})
	}
	if len(c.innerClasses) > 0 {
		specs := c.innerClasses
		c.classAttrs = append(c.classAttrs, pendingAttribute{name: "InnerClasses", encode: func(pool *ConstantPool) ([]byte, error) {
			return encodeInnerClasses(pool, specs)
		}})
	}
	if c.enclosing != nil {
		spec := *c.enclosing
		c.classAttrs = append(c.classAttrs, pendingAttribute{name: "EnclosingMethod", encode: func(pool *ConstantPool) ([]byte, error) {
			return encodeEnclosingMethod(pool, spec)
		}})
	}
	return c.encodeAnnotationAttributes(&c.meta, &c.classAttrs)
}

func writeMemberHeader(out *byteWriter, member *member, attributes uint16) {
	out.u2(member.access)
	out.u2(member.nameIndex)
	out.u2(member.descriptor)
	out.u2(attributes)
}

func writeCodeAttribute(out *byteWriter, nameIndex uint16, code *assembledCode) {
	nestedLength := 0
	if len(code.lines) > 0 {
		nestedLength += 8 + 4*len(code.lines)
	}
	if len(code.locals) > 0 {
		nestedLength += 8 + 10*len(code.locals)
	}
	attributeLength := uint32(12 + len(code.bytes) + 8*len(code.handlers) + nestedLength)
	out.u2(nameIndex)
	out.u4(attributeLength)
	out.u2(code.maxStack)
	out.u2(code.maxLocals)
	out.u4(uint32(len(code.bytes)))
	out.bytes(code.bytes)
	out.u2(uint16(len(code.handlers)))
	for _, handler := range code.handlers {
		out.u2(handler.start)
		out.u2(handler.end)
		out.u2(handler.handler)
		out.u2(handler.catchType)
	}
	nestedCount := uint16(0)
	if len(code.lines) > 0 {
		nestedCount++
	}
	if len(code.locals) > 0 {
		nestedCount++
	}
	out.u2(nestedCount)
	if len(code.lines) > 0 {
		out.u2(code.lineName)
		out.u4(uint32(2 + 4*len(code.lines)))
		out.u2(uint16(len(code.lines)))
		for _, line := range code.lines {
			out.u2(line.start)
			out.u2(line.line)
		}
	}
	if len(code.locals) > 0 {
		out.u2(code.localName)
		out.u4(uint32(2 + 10*len(code.locals)))
		out.u2(uint16(len(code.locals)))
		for _, local := range code.locals {
			out.u2(local.start)
			out.u2(local.length)
			out.u2(local.name)
			out.u2(local.descriptor)
			out.u2(local.index)
		}
	}
}

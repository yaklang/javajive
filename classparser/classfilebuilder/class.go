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
	for _, field := range c.fields {
		writeMemberHeader(out, &field.member, 0)
	}
	out.u2(uint16(len(c.methods)))
	for i, method := range c.methods {
		attributes := uint16(0)
		if assembled[i] != nil {
			attributes = 1
		}
		writeMemberHeader(out, &method.member, attributes)
		if assembled[i] != nil {
			writeCodeAttribute(out, c.codeName, assembled[i])
		}
	}
	out.u2(0)
	return out.data, nil
}

func writeMemberHeader(out *byteWriter, member *member, attributes uint16) {
	out.u2(member.access)
	out.u2(member.nameIndex)
	out.u2(member.descriptor)
	out.u2(attributes)
}

func writeCodeAttribute(out *byteWriter, nameIndex uint16, code *assembledCode) {
	attributeLength := uint32(12 + len(code.bytes) + 8*len(code.handlers))
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
	out.u2(0)
}

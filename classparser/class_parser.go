package javaclassparser

import (
	"fmt"

	"github.com/yaklang/javajive/internal/utils"
)

type ClassParser struct {
	reader    *ClassReader
	classObj  *ClassObject
	attrCtx   attrContext
	annoDepth int
}

func NewClassParser(data []byte) *ClassParser {

	return &ClassParser{
		reader:   NewClassReader(data),
		classObj: NewClassObject(),
	}
}

/*
*
记录方法抛出的异常表

	EXCEPTIONS_ATTRIBUTE {
		u2 attribute_name_index;
		u4 attribute_length;
		u2 number_of_exceptions;
		u2 exception_index_table[number_of_exceptions];
	}
*/
type ExceptionsAttribute struct {
	Type                string
	AttrLen             uint32
	ExceptionIndexTable []uint16
}

func (self *ExceptionsAttribute) readInfo(cp *ClassParser) {
	self.ExceptionIndexTable = cp.reader.readUint16s()
	if cp.reader.Err() != nil {
		return
	}
	for i, idx := range self.ExceptionIndexTable {
		if err := cp.classObj.checkCPIndex(idx, false, fmt.Sprintf("exception_index_table[%d]", i), CONSTANT_Class); err != nil {
			cp.reader.fail(ParseCodeCPIndex, err.Error())
			return
		}
	}
}

func (this *ClassParser) Parse() (*ClassObject, error) {
	this.reader.SetStage("magic", "magic")
	if err := this.parseAndCheckMagic(); err != nil {
		return nil, err
	}
	this.reader.SetStage("version", "minor_major")
	if err := this.readAndCheckVersion(); err != nil {
		return nil, err
	}
	this.reader.SetStage("constant_pool", "count")
	if err := this.readConstantPool(); err != nil {
		return nil, err
	}
	this.reader.SetStage("class", "access_flags")
	this.classObj.AccessFlags = this.reader.readUint16()
	this.classObj.AccessFlagsVerbose, this.classObj.AccessFlagsToCode = getClassAccessFlagsVerbose(this.classObj.AccessFlags)
	this.reader.SetStage("class", "this_class")
	this.classObj.ThisClass = this.reader.readUint16()
	this.reader.SetStage("class", "super_class")
	this.classObj.SuperClass = this.reader.readUint16()
	this.reader.SetStage("class", "interfaces")
	this.classObj.Interfaces = this.reader.readUint16s()
	if err := this.reader.Err(); err != nil {
		return nil, err
	}
	if err := this.classObj.checkClassHeadCPRefs(); err != nil {
		return nil, err
	}
	this.reader.SetStage("fields", "count")
	this.attrCtx = attrCtxField
	fields, err := this.readMembers()
	if err != nil {
		return nil, err
	}
	this.classObj.Fields = fields
	this.reader.SetStage("methods", "count")
	this.attrCtx = attrCtxMethod
	methods, err := this.readMembers()
	if err != nil {
		return nil, err
	}
	this.classObj.Methods = methods
	this.reader.SetStage("class_attributes", "count")
	this.attrCtx = attrCtxClass
	this.classObj.Attributes = this.readAttributes()
	if err := this.reader.Err(); err != nil {
		return nil, err
	}
	if err := validateTypeAnnotationPaths(this.classObj); err != nil {
		return nil, err
	}
	if this.reader.Remaining() > 0 {
		this.reader.SetStage("trailing", "eof")
		return nil, this.reader.fail(ParseCodeTrailingBytes, fmt.Sprintf("%d trailing bytes", this.reader.Remaining()))
	}
	return this.classObj, nil
}
func (this *ClassParser) readMembers() ([]*MemberInfo, error) {
	memberCount := this.reader.readUint16()
	if err := this.reader.Err(); err != nil {
		return nil, err
	}
	if !this.reader.reserve(int64(memberCount), 8) {
		return nil, this.reader.Err()
	}
	members := make([]*MemberInfo, memberCount)
	for i := range members {
		members[i] = this.readMember()
		if err := this.reader.Err(); err != nil {
			return nil, err
		}
	}
	return members, nil
}
func (this *ClassParser) readMember() *MemberInfo {
	return &MemberInfo{
		AccessFlags:     this.reader.readUint16(),
		NameIndex:       this.reader.readUint16(),
		DescriptorIndex: this.reader.readUint16(),
		Attributes:      this.readAttributes(),
	}
}
func (this *ClassParser) readAttributes() []AttributeInfo {
	attributesCount := this.reader.readUint16()
	if err := this.reader.Err(); err != nil {
		return nil
	}
	seen := map[string]int{}
	if !this.reader.reserve(int64(attributesCount), 6) {
		return nil
	}
	attributes := make([]AttributeInfo, attributesCount)
	for i := range attributes {
		attributes[i] = this.readAttribute(seen)
		if err := this.reader.Err(); err != nil {
			return attributes[:i+1]
		}
	}
	return attributes
}
func (this *ClassParser) readAttribute(seen map[string]int) AttributeInfo {
	this.reader.SetStage("attribute", "name_index")
	attributeNameIndex := this.reader.readUint16()
	if err := this.reader.Err(); err != nil {
		return &UnparsedAttribute{}
	}
	attrName, err := this.classObj.getUtf8(attributeNameIndex)
	if err != nil {
		this.reader.fail(ParseCodeCPIndex, fmt.Sprintf("Parse Attribute error: %v", err))
		return &UnparsedAttribute{}
	}
	this.reader.SetStage("attribute", "length")
	attrLen := this.reader.readUint32()
	if err := this.reader.Err(); err != nil {
		return &UnparsedAttribute{Name: attrName}
	}
	if !attrAllowedIn(attrName, this.attrCtx) {
		this.reader.fail(ParseCodeAttrPlacement, fmt.Sprintf("attribute %s is not valid on %s", attrName, this.attrCtx))
		_ = this.reader.Subreader(attrLen)
		return &UnparsedAttribute{Name: attrName, Length: attrLen}
	}
	if seen != nil {
		seen[attrName]++
		if attrMustBeUnique(attrName) && seen[attrName] > 1 {
			this.reader.fail(ParseCodeAttrDuplicate, fmt.Sprintf("duplicate attribute %s on %s", attrName, this.attrCtx))
			_ = this.reader.Subreader(attrLen)
			return &UnparsedAttribute{Name: attrName, Length: attrLen}
		}
	}
	parent := this.reader
	sub := parent.Subreader(attrLen)
	this.reader = sub
	attrInfo := newAttributeInfo(attrName, attrLen)
	attrInfo.readInfo(this)
	this.reader = parent
	requireKnownAttrExactEnd(sub, attrInfo, attrName)
	return attrInfo
}

func (this *ClassParser) parseAndCheckMagic() (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = utils.Errorf("read magic error: %v", e)
		}
	}()
	magic := this.reader.readUint32()
	if err := this.reader.Err(); err != nil {
		return err
	}
	if magic != 0xCAFEBABE {
		return this.reader.fail(ParseCodeBadMagic, "java.lang.ClassFormatError: Magic error")
	}
	this.classObj.Magic = magic
	return nil
}
func (this *ClassParser) readAndCheckVersion() error {
	this.classObj.MinorVersion = this.reader.readUint16()
	this.classObj.MajorVersion = this.reader.readUint16()
	if err := this.reader.Err(); err != nil {
		return err
	}
	// Unknown / future versions are a capability boundary, not a corrupt
	// classfile. Continue parsing so callers can report unsupported rather
	// than invalid_input. Do not fail Parse for unknown major/minor.
	return nil
}
func (this *ClassParser) readConstantPool() error {
	cpCountU := this.reader.readUint16()
	if err := this.reader.Err(); err != nil {
		return err
	}
	if cpCountU < 1 {
		return this.reader.fail(ParseCodeCPCount, "constant_pool_count must be >= 1")
	}
	cpCount := int(cpCountU)
	if !this.reader.reserve(int64(cpCount-1), 1) {
		return this.reader.Err()
	}
	cp := make([]ConstantInfo, cpCount-1)

	//索引从1开始，这里用了 <cpCount 说明index是从1到cpCount-1 及上文的1 ~ n-1
	for i := 0; i < cpCount-1; i++ {
		constantInfo, err := this.readConstantInfo()
		if err != nil {
			return err
		}
		if err := this.reader.Err(); err != nil {
			return err
		}
		cp[i] = constantInfo
		switch cp[i].(type) {
		case *ConstantLongInfo, *ConstantDoubleInfo:
			//占两个位置
			i++
		}
	}
	if err := this.reader.Err(); err != nil {
		return err
	}
	this.classObj.ConstantPool = cp
	return nil
}
func (this *ClassParser) readConstantInfo() (ConstantInfo, error) {
	this.reader.SetStage("constant_pool", "tag")
	tag := this.reader.readUint8()
	if err := this.reader.Err(); err != nil {
		return nil, err
	}
	c, err := newConstantInfo(tag)
	if err != nil {
		return nil, this.reader.fail(ParseCodeCPTag, err.Error())
	}
	c.readInfo(this)
	if err := this.reader.Err(); err != nil {
		return nil, err
	}
	return c, nil
}

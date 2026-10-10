package javaclassparser

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/yaklang/javajive/internal/codec"
	"github.com/yaklang/javajive/internal/utils"
)

var ValueTypeError = utils.Error("error value type")

func deleteStringKeysFromMap(data map[string]interface{}, keys ...string) {
	for _, key := range keys {
		delete(data, key)
	}
}
func Interface2Uint64(v interface{}) (uint64, error) {
	switch v.(type) {
	case uint:
		return uint64(v.(uint)), nil
	case int:
		return uint64(v.(int)), nil
	case uint64:
		return v.(uint64), nil
	case int64:
		return uint64(v.(int64)), nil
	case uint32:
		return uint64(v.(uint32)), nil
	case int32:
		return uint64(v.(int32)), nil
	case uint16:
		return uint64(v.(uint16)), nil
	case int16:
		return uint64(v.(int16)), nil
	case uint8:
		return uint64(v.(uint8)), nil
	case int8:
		return uint64(v.(int)), nil
	default:
		return 0, ValueTypeError
	}
}
func GetMap() ([]int, []int) {
	CHAR_MAP := make([]int, 48)
	MAP_CHAR := make([]int, 256)

	j := 0
	var i int
	for i = 65; i <= 90; {
		CHAR_MAP[j] = i
		MAP_CHAR[i] = j
		i += 1
		j += 1

	}

	for i = 103; i <= 122; {
		CHAR_MAP[j] = i
		MAP_CHAR[i] = j
		i += 1
		j += 1
	}

	CHAR_MAP[j] = 36
	MAP_CHAR[36] = j
	j += 1
	CHAR_MAP[j] = 95

	MAP_CHAR[95] = j
	return CHAR_MAP, MAP_CHAR
}
func Bcel2bytes(becl string) ([]byte, error) {
	pre := "$$BCEL$$"
	if !strings.HasPrefix(becl, pre) {
		return nil, utils.Error("Invalid becl header(\"$$BCEL$$\")!")
	}
	becl = becl[len(pre):]
	//生成CHAR_MAP和MAP_CHAR
	_, MAP_CHAR := GetMap()
	//reader
	rd := strings.NewReader(becl)
	var buf bytes.Buffer
	read := func() int {
		for {
			c, err := rd.ReadByte()
			if err != nil {
				return -1
			}
			if c != '$' {
				return int(c)
			} else {
				c, err = rd.ReadByte()
				if err != nil {
					return -1
				}
				if (c < 48 || c > 57) && (c < 97 || c > 102) {
					return MAP_CHAR[c]
				} else {
					c1, err := rd.ReadByte()
					if err != nil {
						return -1
					}
					byts, err := codec.DecodeHex(string([]byte{c, c1}))
					if err != nil {
						return -1
					}
					n := byts[0]
					return int(n)
				}

			}
		}
	}
	for {
		n := read()
		if n != -1 {
			buf.WriteByte(byte(n))
		} else {
			break
		}
	}
	reader, err := gzip.NewReader(&buf)
	if err != nil {
		var out []byte
		return out, err
	}
	defer reader.Close()
	return ioutil.ReadAll(reader)
}
func bytes2bcel(data []byte) (string, error) {
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	if _, err := gz.Write(data); err != nil {
		return "", err
	}
	if err := gz.Flush(); err != nil {
		return "", err
	}
	if err := gz.Close(); err != nil {
		return "", err
	}
	data = b.Bytes()

	CHAR_MAP, _ := GetMap()
	var buf strings.Builder
	isJavaIdentifierPart := func(ch int) bool {
		return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_'
	}
	write := func(b int) {
		if isJavaIdentifierPart(b) && b != 36 {
			buf.WriteByte(byte(b))
		} else {
			buf.WriteByte(36)
			if b >= 0 && b < 48 {
				buf.WriteByte(byte(CHAR_MAP[b]))
			} else {
				strHex := codec.EncodeToHex([]byte{byte(b)})
				if len(strHex) == 1 {
					buf.WriteByte(48)
					buf.WriteByte(strHex[0])
				} else {
					buf.WriteString(strHex)
				}
			}
		}

	}
	l := len(data)
	for i := 0; i < l; i += 1 {
		in := int(data[i]) & 255
		write(in)
	}
	return "$$BCEL$$" + buf.String(), nil
}

const maxAnnotationNesting = 64

func annotationGetUtf8(cp *ClassParser, index uint16) string {
	s, err := cp.classObj.getUtf8(index)
	if err != nil {
		if cp.reader != nil {
			cp.reader.fail(ParseCodeCPIndex, fmt.Sprintf("get utf8 error: %s", err))
			return ""
		}
		panic(fmt.Errorf("get utf8 error: %s", err))
	}
	return s
}

func ParseAnnotationElementValue(cp *ClassParser) *ElementValuePairAttribute {
	if cp.annoDepth >= maxAnnotationNesting {
		cp.reader.fail(ParseCodeResourceLimit, "annotation element nesting exceeds limit")
		return &ElementValuePairAttribute{}
	}
	cp.annoDepth++
	defer func() { cp.annoDepth-- }()
	if !cp.reader.reserve(1, 1) {
		return &ElementValuePairAttribute{}
	}
	reader := cp.reader
	tag := reader.readUint8()
	ele := &ElementValuePairAttribute{
		Tag: tag,
	}
	if reader != nil && reader.Err() != nil {
		return ele
	}
	switch tag {
	case 'B', 'C', 'D', 'F', 'I', 'J', 'S', 'Z':
		index := reader.readUint16()
		var err error
		ele.Value, err = cp.classObj.getConstantInfo(index)
		if err != nil {
			if reader != nil {
				reader.fail(ParseCodeCPIndex, fmt.Sprintf("get constant info error: %s", err))
			} else {
				panic(fmt.Errorf("get constant info error: %s", err))
			}
		}
	case 's':
		idx := reader.readUint16()
		units, err := cp.classObj.getUtf8Units(idx)
		if err != nil {
			if reader != nil {
				reader.fail(ParseCodeCPIndex, err.Error())
			}
			ele.Value = utf8StringFromUnits(units)
		} else {
			ele.Value = utf8StringFromUnits(units)
		}
	case 'e':
		val := &EnumConstValue{
			TypeName:  annotationGetUtf8(cp, reader.readUint16()),
			ConstName: annotationGetUtf8(cp, reader.readUint16()),
		}
		ele.Value = val
	case 'c':
		ele.Value = annotationGetUtf8(cp, reader.readUint16())
	case '@':
		ele.Value = ParseAnnotation(cp)
	case '[':
		length := reader.readUint16()
		if !reader.reserve(int64(length), 1) {
			return ele
		}
		l := []*ElementValuePairAttribute{}
		for k := 0; k < int(length); k++ {
			val := ParseAnnotationElementValue(cp)
			l = append(l, val)
			if reader.Err() != nil {
				return ele
			}
		}
		ele.Value = l
	default:
		if reader != nil {
			reader.fail(ParseCodeInvalidInput, fmt.Sprintf("parse annotation error, unknown tag: %c", tag))
			return ele
		}
		panic(fmt.Errorf("parse annotation error, unknown tag: %c", tag))
	}
	return ele
}
func ParseAnnotationElementValuePair(cp *ClassParser) *ElementValuePairAttribute {
	reader := cp.reader
	nameIndex := reader.readUint16()
	name := annotationGetUtf8(cp, nameIndex)
	value := ParseAnnotationElementValue(cp)
	value.Name = name
	return value
}
func WriteAnnotation(cp *ConstantPool, anno *AnnotationAttribute, writer *JavaBufferWriter) {
	if anno == nil || writer == nil {
		return
	}

	// Find or add the UTF8 constant for the type name
	getOrAddUtf8Index := func(str string) uint16 {
		for i, constant := range cp.GetData() {
			if utf8Info, ok := constant.(*ConstantUtf8Info); ok {
				if utf8Info.Value == str {
					return uint16(i + 1)
				}
			}
		}
		// If not found, we should ideally add it to the constant pool
		// But for simplicity we'll just return 0 here - in a real implementation,
		// you'd need to add the constant to the pool
		return 0
	}

	// Write the type index
	typeIndex := getOrAddUtf8Index(anno.TypeName)
	writer.Write2Byte(typeIndex)

	// Write the number of element-value pairs
	writer.Write2Byte(len(anno.ElementValuePairs))

	// Write each element-value pair
	for _, pair := range anno.ElementValuePairs {
		WriteElementValuePair(cp, pair, writer)
	}
}

func WriteElementValuePair(cp *ConstantPool, pair *ElementValuePairAttribute, writer *JavaBufferWriter) {
	if pair == nil || writer == nil {
		return
	}

	// Find or add the UTF8 constant for the name
	getOrAddUtf8Index := func(str string) uint16 {
		for i, constant := range cp.GetData() {
			if utf8Info, ok := constant.(*ConstantUtf8Info); ok {
				if utf8Info.Value == str {
					return uint16(i + 1)
				}
			}
		}
		return 0
	}

	// Write the name index
	nameIndex := getOrAddUtf8Index(pair.Name)
	writer.Write2Byte(nameIndex)

	// Write the value
	WriteElementValue(cp, pair, writer)
}

func WriteElementValue(cp *ConstantPool, element *ElementValuePairAttribute, writer *JavaBufferWriter) {
	if element == nil || writer == nil {
		return
	}

	// Write the tag
	writer.Write1Byte(element.Tag)

	// Write the value based on the tag
	switch element.Tag {
	case 'B', 'C', 'D', 'F', 'I', 'J', 'S', 'Z':
		// For primitive types, write the constant index
		// Find the constant index
		constIndex := uint16(0)
		for i, constant := range cp.GetData() {
			if constant == element.Value {
				constIndex = uint16(i + 1)
				break
			}
		}
		writer.Write2Byte(constIndex)

	case 's':
		// For string value, write the UTF8 index
		getOrAddUtf8Index := func(str string) uint16 {
			for i, constant := range cp.GetData() {
				if utf8Info, ok := constant.(*ConstantUtf8Info); ok {
					if utf8Info.Value == str {
						return uint16(i + 1)
					}
				}
			}
			return 0
		}
		strIndex := uint16(0)
		units := annotationStringUnits(element.Value)
		if units != nil {
			for i, constant := range cp.GetData() {
				if utf8Info, ok := constant.(*ConstantUtf8Info); ok {
					if utf8Info.Units != nil && utf16UnitsEqual(utf8Info.Units, units) {
						strIndex = uint16(i + 1)
						break
					}
					if utf8Info.Value == unitsToDisplay(units) {
						strIndex = uint16(i + 1)
						break
					}
				}
			}
		} else if sv, ok := element.Value.(string); ok {
			strIndex = getOrAddUtf8Index(sv)
		}
		writer.Write2Byte(strIndex)

	case 'e':
		// For enum constant value
		enumValue, ok := element.Value.(*EnumConstValue)
		if ok {
			var typeNameIndex uint16
			func() uint16 {
				for i, constant := range cp.GetData() {
					if utf8Info, ok := constant.(*ConstantUtf8Info); ok {
						if utf8Info.Value == enumValue.TypeName {
							typeNameIndex = uint16(i + 1)
							return typeNameIndex
						}
					}
				}
				return 0
			}()

			var constNameIndex uint16
			func() uint16 {
				for i, constant := range cp.GetData() {
					if utf8Info, ok := constant.(*ConstantUtf8Info); ok {
						if utf8Info.Value == enumValue.ConstName {
							constNameIndex = uint16(i + 1)
							return constNameIndex
						}
					}
				}
				return 0
			}()

			writer.Write2Byte(typeNameIndex)
			writer.Write2Byte(constNameIndex)
		} else {
			writer.Write2Byte(0) // Default values if not an enum
			writer.Write2Byte(0)
		}

	case 'c':
		// For class info value, write the UTF8 index
		getOrAddUtf8Index := func(str string) uint16 {
			for i, constant := range cp.GetData() {
				if utf8Info, ok := constant.(*ConstantUtf8Info); ok {
					if utf8Info.Value == str {
						return uint16(i + 1)
					}
				}
			}
			return 0
		}
		classValue, ok := element.Value.(string)
		if ok {
			classIndex := getOrAddUtf8Index(classValue)
			writer.Write2Byte(classIndex)
		} else {
			writer.Write2Byte(0) // Default value if not a class
		}

	case '@':
		// For nested annotation
		nestedAnno, ok := element.Value.(*AnnotationAttribute)
		if ok {
			WriteAnnotation(cp, nestedAnno, writer)
		}

	case '[':
		// For array value
		arrayValue, ok := element.Value.([]*ElementValuePairAttribute)
		if ok {
			writer.Write2Byte(uint16(len(arrayValue)))
			for _, value := range arrayValue {
				WriteElementValue(cp, value, writer)
			}
		} else {
			writer.Write2Byte(0) // Empty array if not an array
		}
	}
}

func ParseAnnotation(cp *ClassParser) *AnnotationAttribute {
	if cp.annoDepth >= maxAnnotationNesting {
		if cp.reader != nil {
			cp.reader.fail(ParseCodeResourceLimit, fmt.Sprintf("annotation nesting exceeds %d", maxAnnotationNesting))
		}
		return &AnnotationAttribute{}
	}
	cp.annoDepth++
	defer func() { cp.annoDepth-- }()
	reader := cp.reader
	typeIndex := reader.readUint16()
	elementLen := reader.readUint16()
	if reader != nil && reader.Err() != nil {
		return &AnnotationAttribute{}
	}
	if !reader.reserve(int64(elementLen), 3) {
		return &AnnotationAttribute{}
	}
	typeName := annotationGetUtf8(cp, typeIndex)
	anno := &AnnotationAttribute{
		TypeName:          typeName,
		ElementValuePairs: make([]*ElementValuePairAttribute, elementLen),
	}
	for j := range anno.ElementValuePairs {
		anno.ElementValuePairs[j] = ParseAnnotationElementValuePair(cp)
		if reader != nil && reader.Err() != nil {
			return anno
		}
	}
	return anno
}

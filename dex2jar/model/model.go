// Package model defines the parser-neutral representation of DEX input.
package model

const NoIndex = ^uint32(0)

// File is one DEX file, either standalone or embedded in an archive.
type File struct {
	Entry   string
	Header  Header
	Strings []string
	Types   []Type
	Protos  []Proto
	Fields  []FieldID
	Methods []MethodID
	Classes []Class
}

// Application is an ordered collection of DEX files.
type Application struct {
	Files []*File
}

type Header struct {
	Version       string
	Checksum      uint32
	Signature     [20]byte
	FileSize      uint32
	HeaderSize    uint32
	EndianTag     uint32
	MapOffset     uint32
	StringIDsSize uint32
	StringIDsOff  uint32
	TypeIDsSize   uint32
	TypeIDsOff    uint32
	ProtoIDsSize  uint32
	ProtoIDsOff   uint32
	FieldIDsSize  uint32
	FieldIDsOff   uint32
	MethodIDsSize uint32
	MethodIDsOff  uint32
	ClassDefsSize uint32
	ClassDefsOff  uint32
	DataSize      uint32
	DataOff       uint32
}

type Type struct {
	DescriptorIndex uint32
	Descriptor      string
}

type Proto struct {
	ShortyIndex     uint32
	Shorty          string
	ReturnTypeIndex uint32
	ReturnType      string
	ParameterTypes  []uint32
	Parameters      []string
}

type FieldID struct {
	ClassIndex uint16
	TypeIndex  uint16
	NameIndex  uint32
	Class      string
	Type       string
	Name       string
}

type MethodID struct {
	ClassIndex uint16
	ProtoIndex uint16
	NameIndex  uint32
	Class      string
	Name       string
}

type Class struct {
	ClassIndex       uint32
	Descriptor       string
	AccessFlags      uint32
	SuperclassIndex  uint32
	Superclass       string
	InterfaceIndices []uint32
	Interfaces       []string
	SourceFileIndex  uint32
	SourceFile       string
	StaticFields     []EncodedField
	InstanceFields   []EncodedField
	DirectMethods    []EncodedMethod
	VirtualMethods   []EncodedMethod
	StaticValues     []EncodedValue

	Annotations          []Annotation
	FieldAnnotations     []FieldAnnotation
	MethodAnnotations    []MethodAnnotation
	ParameterAnnotations []ParameterAnnotation
	Signature            string
	Throws               []string
	InnerClasses         []InnerClass
	Enclosing            *EnclosingMethod
	SourceDebugExtension string
}

type EncodedField struct {
	FieldIndex  uint32
	AccessFlags uint32
	Field       FieldID
}

type EncodedMethod struct {
	MethodIndex uint32
	AccessFlags uint32
	Method      MethodID
	Code        *Code
}

type Code struct {
	RegistersSize   uint16
	InsSize         uint16
	OutsSize        uint16
	DebugInfoOffset uint32
	Debug           *DebugInfo
	Instructions    []uint16
	Tries           []Try
}

// DebugInfo is the decoded debug_info_item of one code item. Addresses are DEX
// code-unit offsets; End is exclusive.
type DebugInfo struct {
	LineStart      uint32
	ParameterNames []string
	Positions      []DebugPosition
	Locals         []DebugLocal
}

type DebugPosition struct {
	Address uint32
	Line    uint32
	File    string
}

type DebugLocal struct {
	Register  uint16
	Name      string
	Type      string
	Signature string
	Start     uint32
	End       uint32
}

type Try struct {
	StartAddress     uint32
	InstructionCount uint16
	HandlerOffset    uint16
	Handler          CatchHandler
}

type CatchHandler struct {
	Handlers        []TypedHandler
	CatchAllAddress *uint32
}

type TypedHandler struct {
	TypeIndex uint32
	Type      string
	Address   uint32
}

type ValueType uint8

const (
	ValueByte         ValueType = 0x00
	ValueShort        ValueType = 0x02
	ValueChar         ValueType = 0x03
	ValueInt          ValueType = 0x04
	ValueLong         ValueType = 0x06
	ValueFloat        ValueType = 0x10
	ValueDouble       ValueType = 0x11
	ValueMethodType   ValueType = 0x15
	ValueMethodHandle ValueType = 0x16
	ValueString       ValueType = 0x17
	ValueTypeRef      ValueType = 0x18
	ValueField        ValueType = 0x19
	ValueMethod       ValueType = 0x1a
	ValueEnum         ValueType = 0x1b
	ValueArray        ValueType = 0x1c
	ValueAnnotation   ValueType = 0x1d
	ValueNull         ValueType = 0x1e
	ValueBoolean      ValueType = 0x1f
)

// EncodedValue preserves scalar bits and recursively represents arrays and annotations.
// Signed is populated for signed integer kinds; Unsigned contains raw or reference bits.
type EncodedValue struct {
	Type       ValueType
	Unsigned   uint64
	Signed     int64
	Boolean    bool
	Array      []EncodedValue
	Annotation *EncodedAnnotation
}

type EncodedAnnotation struct {
	TypeIndex uint32
	Type      string
	Elements  []AnnotationElement
}

// Annotation visibility values follow the DEX annotation_item encoding.
const (
	AnnotationBuild   = 0x00
	AnnotationRuntime = 0x01
	AnnotationSystem  = 0x02
)

// Annotation is one decoded annotation_item.
type Annotation struct {
	Visibility uint8
	Type       string
	Elements   []AnnotationElement
}

// FieldAnnotation and MethodAnnotation bind an annotation set to a member.
type FieldAnnotation struct {
	FieldIndex  uint32
	Annotations []Annotation
}

type MethodAnnotation struct {
	MethodIndex uint32
	Annotations []Annotation
}

// ParameterAnnotation holds one annotation set per declared parameter.
type ParameterAnnotation struct {
	MethodIndex uint32
	Parameters  [][]Annotation
}

// InnerClass mirrors one InnerClasses table row reconstructed from DEX system
// annotations.
type InnerClass struct {
	Inner string
	Outer string
	Name  string
	Flags uint16
}

// EnclosingMethod identifies the enclosing class and method of a local or
// anonymous class.
type EnclosingMethod struct {
	Class  string
	Method string
	Proto  string
}

type AnnotationElement struct {
	NameIndex uint32
	Name      string
	Value     EncodedValue
}

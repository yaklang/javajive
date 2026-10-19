package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"slices"
	"strconv"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A physical implementation is not the source SAM declaration. This narrow
// projection covers an original static, uncaptured javac8 body with one actual
// factory. Its parameter words must remain unwritten. Captured operands, shared
// factories, other metadata and other compiler profiles need separate proofs.
type lambdaParameterMetadata struct {
	object        *ClassObject
	method        *MemberInfo
	table         *UnparsedAttribute
	info          []byte
	name          string
	descriptor    string
	names         []string
	flags         []uint16
	parameters    []string
	parentNames   []string
	words         int
	captureTypes  []string
	captures      []*nativeEnumSelectorProducer
	samDescriptor string
	factory       *lambdaSourceFactory
}

// The actual callback supplies this origin. A matching descriptor or capture
// count cannot substitute a different method, Code packet or instruction PC.
type lambdaSourceFactory struct {
	object                            *ClassObject
	method                            *MemberInfo
	code                              *CodeAttribute
	pc                                int
	sealed                            bool
	flags, nameIndex, descriptorIndex uint16
	codeBytes                         []byte
}

func (f *lambdaSourceFactory) same(other *lambdaSourceFactory) bool {
	return f.closed() && other.closed() && f.object == other.object && f.method == other.method && f.code == other.code && f.pc == other.pc
}

func (f *lambdaSourceFactory) closed() bool {
	if f == nil || f.object == nil || f.method == nil || f.code == nil || f.pc < 0 || f.pc >= len(f.code.Code) {
		return false
	}
	return !f.sealed || f.method.AccessFlags == f.flags && f.method.NameIndex == f.nameIndex && f.method.DescriptorIndex == f.descriptorIndex && bytes.Equal(f.codeBytes, f.code.Code)
}

func readLambdaParameterMetadata(object *ClassObject, method *MemberInfo, work *workbudget.Budget) (*lambdaParameterMetadata, bool) {
	if object == nil || method == nil || len(object.Methods) > 4096 || len(object.ConstantPool) > 65535 || len(method.Attributes) > 65535 || !nativeProofWork(work, int64(len(method.Attributes))+1) {
		return nil, false
	}
	var table *UnparsedAttribute
	for _, attribute := range method.Attributes {
		if raw, ok := attribute.(*UnparsedAttribute); ok {
			if raw == nil {
				return nil, false
			}
			if raw.Name == "MethodParameters" {
				if table != nil {
					return nil, false
				}
				table = raw
			}
		}
	}
	if table == nil {
		return nil, true
	}
	// ClassWriter adds ACC_SYNTHETIC to each implementation parameter. The
	// source formal controls only its name and FINAL bit (OpenJDK javac8).
	if object.MajorVersion != 52 || method.AccessFlags != 0x100a {
		return nil, false
	}
	name, named := sourceBridgeUTF8(object, method.NameIndex)
	descriptor, typed := sourceBridgeUTF8(object, method.DescriptorIndex)
	parameters, _, err := callbinding.Descriptor(descriptor)
	if !named || !typed || name == "<init>" || name == "<clinit>" || err != nil || len(parameters) == 0 || len(parameters) > 255 || table.Length != uint32(len(table.Info)) || len(table.Info) != 1+4*len(parameters) || int(table.Info[0]) != len(parameters) || !nativeProofWork(work, int64(len(table.Info))) || work != nil && work.CheckAlloc(int64(len(parameters))*192+int64(len(table.Info))) != nil {
		return nil, false
	}
	projection := &lambdaParameterMetadata{object: object, method: method, table: table, info: bytes.Clone(table.Info), name: name, descriptor: descriptor, parameters: parameters}
	seen := map[string]bool{}
	nameBytes := 0
	for i, parameter := range parameters {
		entry := table.Info[1+4*i : 5+4*i]
		text, known := sourceBridgeUTF8(object, binary.BigEndian.Uint16(entry[:2]))
		flags := binary.BigEndian.Uint16(entry[2:])
		nameBytes += len(text)
		if !known || text == "_" || !class_context.PreservesJava8Identifier(text) || seen[text] || flags&^uint16(0x1010) != 0 || flags&0x1000 == 0 || nameBytes > methodParameterNamePoolPayloadLimit || !nativeProofWork(work, int64(len(text))+1) {
			return nil, false
		}
		projection.names = append(projection.names, text)
		projection.flags = append(projection.flags, flags)
		seen[text] = true
		projection.words++
		if parameter == "J" || parameter == "D" {
			projection.words++
		}
	}
	return projection, projection.words <= 255
}

func (p *lambdaParameterMetadata) schemaMatches(object *ClassObject, method *MemberInfo, work *workbudget.Budget) bool {
	if p == nil || p.object != object || p.method != method || p.factory != nil {
		return false
	}
	fresh, known := readLambdaParameterMetadata(object, method, work)
	return known && fresh != nil && fresh.table == p.table && bytes.Equal(fresh.info, p.info) && fresh.name == p.name && fresh.descriptor == p.descriptor && fresh.words == p.words && slices.Equal(fresh.names, p.names) && slices.Equal(fresh.flags, p.flags) && slices.Equal(fresh.parameters, p.parameters)
}

// Metadata is admitted only as a regeneration requirement on the actual body
// request. It grants no capture, access, lexical membership or publication role.
func (c *ClassObjectDumper) originalLambdaParameterMetadata(method *MemberInfo, work *workbudget.Budget) (*lambdaParameterMetadata, bool) {
	if c == nil {
		return nil, false
	}
	projection, known := readLambdaParameterMetadata(c.obj, method, work)
	if !known {
		return projection, known
	}
	if projection == nil {
		if c.options.SourceCompiler == NativeJavac8 && (c.options.TargetSourceVersion != 8 || !native8UnmarkedLambdaParametersClosed(c.obj, method, work)) {
			return nil, false
		}
		return nil, true
	}
	if c.options.SourceCompiler != NativeJavac8 || c.options.TargetSourceVersion != 8 || !native8LambdaParameterCompilerFlagClosed(c.obj, work) || !nativeLambdaBodyOriginalUseClosed(c.obj, method, work, projection) || projection.factory == nil {
		return nil, false
	}
	factoryName, named := sourceBridgeUTF8(c.obj, projection.factory.method.NameIndex)
	// This is a compiler-name regeneration constraint after the actual factory
	// proof. A name prefix alone never identifies a lambda implementation.
	prefix := "lambda$" + factoryName + "$"
	if !named || !class_context.PreservesJava8Identifier(factoryName) || !strings.HasPrefix(projection.name, prefix) {
		return nil, false
	}
	ordinal := strings.TrimPrefix(projection.name, prefix)
	index, err := strconv.ParseUint(ordinal, 10, 32)
	if err != nil || ordinal != strconv.FormatUint(index, 10) || index >= uint64(len(c.obj.Methods)) {
		return nil, false
	}
	parentDescriptor, typed := sourceBridgeUTF8(c.obj, projection.factory.method.DescriptorIndex)
	// Reuse the ordinary declaration reader without borrowing another proof
	// caller's budget. This query mutates neither the dumper nor its input.
	reader := ClassObjectDumper{obj: c.obj, Work: work}
	parentNames, known := reader.originalOrdinaryMethodParameterNames(projection.factory.method, parentDescriptor)
	if !typed || !known {
		return nil, false
	}
	if !c.native8LambdaParameterCaptures(projection, parentDescriptor, parentNames, work) {
		return nil, false
	}
	for _, name := range projection.names[len(projection.captures):] {
		if slices.Contains(parentNames, name) {
			return nil, false
		}
	}
	projection.parentNames = parentNames
	return projection, true
}

// Absence constrains regeneration too. Any nonempty original parameter table
// requires native javac8's class-wide -parameters flag. Under that flag an
// emitted lambda implementation with physical parameters receives a table,
// even when the input implementation had none. Zero-parameter implementations
// remain table-free, but other ordinary headers must still agree on the flag.
func native8UnmarkedLambdaParametersClosed(object *ClassObject, method *MemberInfo, work *workbudget.Budget) bool {
	if object == nil || method == nil || len(object.Methods) > 4096 || !nativeProofWork(work, int64(len(object.Methods))+1) {
		return false
	}
	descriptor, typed := sourceBridgeUTF8(object, method.DescriptorIndex)
	if !typed || len(descriptor) > 65535 || !nativeProofWork(work, int64(len(descriptor))+1) || work != nil && work.CheckAlloc(int64(len(descriptor))*32+256) != nil {
		return false
	}
	parameters, _, err := callbinding.Descriptor(descriptor)
	if err != nil || len(parameters) > 255 {
		return false
	}
	requiresNames := false
	for _, original := range object.Methods {
		if original == nil || len(original.Attributes) > 65535 || !nativeProofWork(work, int64(len(original.Attributes))+1) {
			return false
		}
		for _, attribute := range original.Attributes {
			if table, ok := attribute.(*UnparsedAttribute); ok {
				if table == nil {
					return false
				}
				if table.Name != "MethodParameters" {
					continue
				}
				if len(table.Info) == 0 || table.Length != uint32(len(table.Info)) || len(table.Info) != 1+4*int(table.Info[0]) || !nativeProofWork(work, int64(len(table.Info))) {
					return false
				}
				requiresNames = requiresNames || table.Info[0] != 0
			}
		}
	}
	return !requiresNames || len(parameters) == 0 && native8LambdaParameterCompilerFlagClosed(object, work)
}

// javac8's -parameters option applies to every source method declaration.
// Regenerating an implementation table must not also invent observable names
// on another original ordinary header that had no table or unnamed entries.
// This necessary class-wide condition grants no binding or declaration role;
// compiler-owned headers retain their separate regeneration proofs.
func native8LambdaParameterCompilerFlagClosed(object *ClassObject, work *workbudget.Budget) bool {
	if object == nil || len(object.Methods) > 4096 || !nativeProofWork(work, int64(len(object.Methods))+1) {
		return false
	}
	for _, method := range object.Methods {
		if method == nil || len(method.Attributes) > 65535 || !nativeProofWork(work, 1) {
			return false
		}
		if method.AccessFlags&(0x1000|0x0040) != 0 {
			continue
		}
		descriptor, typed := sourceBridgeUTF8(object, method.DescriptorIndex)
		if !typed || len(descriptor) > 65535 || !nativeProofWork(work, int64(len(descriptor)+len(method.Attributes))+1) || work != nil && work.CheckAlloc(int64(len(descriptor))*32+256) != nil {
			return false
		}
		parameters, _, err := callbinding.Descriptor(descriptor)
		if err != nil || len(parameters) > 255 {
			return false
		}
		if len(parameters) == 0 {
			continue
		}
		var table *UnparsedAttribute
		for _, attribute := range method.Attributes {
			if raw, ok := attribute.(*UnparsedAttribute); ok {
				if raw == nil {
					return false
				}
				if raw.Name == "MethodParameters" {
					if table != nil {
						return false
					}
					table = raw
				}
			}
		}
		if table == nil || table.Length != uint32(len(table.Info)) || len(table.Info) != 1+4*len(parameters) || int(table.Info[0]) != len(parameters) || !nativeProofWork(work, int64(len(table.Info))) {
			return false
		}
		for i := range parameters {
			name, known := sourceBridgeUTF8(object, binary.BigEndian.Uint16(table.Info[1+4*i:3+4*i]))
			if !known || name == "" || !nativeProofWork(work, int64(len(name))+1) {
				return false
			}
		}
	}
	return true
}

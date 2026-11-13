package dexparser

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/yaklang/javajive/dex2jar/model"
)

const (
	accessPrivate     = uint32(0x0002)
	accessStatic      = uint32(0x0008)
	accessNative      = uint32(0x0100)
	accessAbstract    = uint32(0x0400)
	accessConstructor = uint32(0x10000)
)

func (p *parser) parseClasses() error {
	h := p.file.Header
	if err := p.tableRange(h.ClassDefsOff, h.ClassDefsSize, 32, "class_defs"); err != nil {
		return err
	}
	p.file.Classes = make([]model.Class, h.ClassDefsSize)
	for i := range p.file.Classes {
		off := int(h.ClassDefsOff) + i*32
		classIdx := binary.LittleEndian.Uint32(p.data[off : off+4])
		accessFlags := binary.LittleEndian.Uint32(p.data[off+4 : off+8])
		superIdx := binary.LittleEndian.Uint32(p.data[off+8 : off+12])
		interfacesOff := binary.LittleEndian.Uint32(p.data[off+12 : off+16])
		sourceIdx := binary.LittleEndian.Uint32(p.data[off+16 : off+20])
		annotationsOff := binary.LittleEndian.Uint32(p.data[off+20 : off+24])
		classDataOff := binary.LittleEndian.Uint32(p.data[off+24 : off+28])
		staticValuesOff := binary.LittleEndian.Uint32(p.data[off+28 : off+32])
		if classIdx >= uint32(len(p.file.Types)) {
			return p.fail(off, "class_defs", "class_idx=%d out of range", classIdx)
		}
		if superIdx != model.NoIndex && superIdx >= uint32(len(p.file.Types)) {
			return p.fail(off+8, "class_defs", "superclass_idx=%d out of range", superIdx)
		}
		if sourceIdx != model.NoIndex && sourceIdx >= uint32(len(p.file.Strings)) {
			return p.fail(off+16, "class_defs", "source_file_idx=%d out of range", sourceIdx)
		}
		c := model.Class{ClassIndex: classIdx, Descriptor: p.file.Types[classIdx].Descriptor, AccessFlags: accessFlags, SuperclassIndex: superIdx, SourceFileIndex: sourceIdx}
		if superIdx != model.NoIndex {
			c.Superclass = p.file.Types[superIdx].Descriptor
		}
		if sourceIdx != model.NoIndex {
			c.SourceFile = p.file.Strings[sourceIdx]
		}
		p.class = c.Descriptor
		if annotationsOff != 0 {
			if err := p.parseAnnotationDirectory(annotationsOff, &c); err != nil {
				return err
			}
		}
		indices, names, err := p.typeList(interfacesOff)
		if err != nil {
			return err
		}
		c.InterfaceIndices, c.Interfaces = indices, names
		if err := p.classData(classDataOff, &c); err != nil {
			return err
		}
		if annotationsOff != 0 {
			if err := p.translateSystemAnnotations(&c); err != nil {
				return err
			}
		}
		if staticValuesOff != 0 {
			cur := int(staticValuesOff)
			values, err := p.encodedArray(&cur, 0)
			if err != nil {
				return err
			}
			if len(values) > len(c.StaticFields) {
				return p.fail(int(staticValuesOff), "static_values", "%d values for %d static fields", len(values), len(c.StaticFields))
			}
			c.StaticValues = values
		}
		p.file.Classes[i] = c
		p.class = ""
	}
	return nil
}

func (p *parser) classData(off uint32, c *model.Class) error {
	if off == 0 {
		return nil
	}
	cur := int(off)
	staticCount, err := p.uleb(&cur)
	if err != nil {
		return p.fail(cur, "class_data", "static_fields_size: %v", err)
	}
	instanceCount, err := p.uleb(&cur)
	if err != nil {
		return p.fail(cur, "class_data", "instance_fields_size: %v", err)
	}
	directCount, err := p.uleb(&cur)
	if err != nil {
		return p.fail(cur, "class_data", "direct_methods_size: %v", err)
	}
	virtualCount, err := p.uleb(&cur)
	if err != nil {
		return p.fail(cur, "class_data", "virtual_methods_size: %v", err)
	}
	if staticCount > uint32(len(p.file.Fields)) || instanceCount > uint32(len(p.file.Fields)) {
		return p.fail(int(off), "class_data", "field count exceeds field_ids")
	}
	if directCount > uint32(len(p.file.Methods)) || virtualCount > uint32(len(p.file.Methods)) {
		return p.fail(int(off), "class_data", "method count exceeds method_ids")
	}
	c.StaticFields, err = p.encodedFields(&cur, staticCount)
	if err != nil {
		return err
	}
	c.InstanceFields, err = p.encodedFields(&cur, instanceCount)
	if err != nil {
		return err
	}
	c.DirectMethods, err = p.encodedMethods(&cur, directCount, true)
	if err != nil {
		return err
	}
	c.VirtualMethods, err = p.encodedMethods(&cur, virtualCount, false)
	return err
}

func (p *parser) encodedFields(cur *int, count uint32) ([]model.EncodedField, error) {
	out := make([]model.EncodedField, count)
	var index uint32
	for i := range out {
		delta, err := p.uleb(cur)
		if err != nil {
			return nil, p.fail(*cur, "class_data field", "%v", err)
		}
		if i > 0 && delta == 0 {
			return nil, p.fail(*cur, "class_data field", "field indices are not strictly increasing")
		}
		if delta > math.MaxUint32-index {
			return nil, p.fail(*cur, "class_data field", "field index overflows")
		}
		index += delta
		flags, err := p.uleb(cur)
		if err != nil {
			return nil, p.fail(*cur, "class_data field", "%v", err)
		}
		if index >= uint32(len(p.file.Fields)) {
			return nil, p.fail(*cur, "class_data field", "field_idx=%d out of range", index)
		}
		out[i] = model.EncodedField{FieldIndex: index, AccessFlags: flags, Field: p.file.Fields[index]}
	}
	return out, nil
}

func (p *parser) encodedMethods(cur *int, count uint32, direct bool) ([]model.EncodedMethod, error) {
	out := make([]model.EncodedMethod, count)
	var index uint32
	for i := range out {
		delta, err := p.uleb(cur)
		if err != nil {
			return nil, p.fail(*cur, "class_data method", "%v", err)
		}
		if i > 0 && delta == 0 {
			return nil, p.fail(*cur, "class_data method", "method indices are not strictly increasing")
		}
		if delta > math.MaxUint32-index {
			return nil, p.fail(*cur, "class_data method", "method index overflows")
		}
		index += delta
		flags, err := p.uleb(cur)
		if err != nil {
			return nil, p.fail(*cur, "class_data method", "%v", err)
		}
		codeOff, err := p.uleb(cur)
		if err != nil {
			return nil, p.fail(*cur, "class_data method", "%v", err)
		}
		if index >= uint32(len(p.file.Methods)) {
			return nil, p.fail(*cur, "class_data method", "method_idx=%d out of range", index)
		}
		methodID := p.file.Methods[index]
		p.method = methodID.Name
		isDirect := flags&(accessStatic|accessPrivate|accessConstructor) != 0
		if direct != isDirect {
			list := "virtual_methods"
			if direct {
				list = "direct_methods"
			}
			return nil, p.fail(*cur, "class_data method", "%s membership disagrees with access flags for %q", list, methodID.Name)
		}
		if methodID.Name == "<init>" && flags&accessConstructor == 0 {
			return nil, p.fail(*cur, "class_data method", "<init> is missing ACC_CONSTRUCTOR")
		}
		if methodID.Name == "<clinit>" && flags&(accessStatic|accessConstructor) != accessStatic|accessConstructor {
			return nil, p.fail(*cur, "class_data method", "<clinit> must be static and carry ACC_CONSTRUCTOR")
		}
		if flags&accessConstructor != 0 && methodID.Name != "<init>" && methodID.Name != "<clinit>" {
			return nil, p.fail(*cur, "class_data method", "ACC_CONSTRUCTOR is set on non-initializer %q", methodID.Name)
		}
		declarationOnly := flags&(accessNative|accessAbstract) != 0
		if declarationOnly != (codeOff == 0) {
			return nil, p.fail(*cur, "class_data method", "code_off and abstract/native access flags disagree for %q", methodID.Name)
		}
		method := model.EncodedMethod{MethodIndex: index, AccessFlags: flags, Method: methodID}
		if codeOff != 0 {
			declaredParameters := uint32(len(p.file.Protos[methodID.ProtoIndex].Parameters))
			method.Code, err = p.codeItem(codeOff, declaredParameters)
			if err != nil {
				return nil, err
			}
		}
		p.method = ""
		out[i] = method
	}
	return out, nil
}

func (p *parser) codeItem(off uint32, declaredParameters uint32) (*model.Code, error) {
	if off%4 != 0 {
		return nil, p.fail(int(off), "code_item", "offset is not 4-byte aligned")
	}
	if err := p.rangeCheck(off, 16, "code_item"); err != nil {
		return nil, err
	}
	pos := int(off)
	registers := binary.LittleEndian.Uint16(p.data[pos : pos+2])
	ins := binary.LittleEndian.Uint16(p.data[pos+2 : pos+4])
	outs := binary.LittleEndian.Uint16(p.data[pos+4 : pos+6])
	triesSize := binary.LittleEndian.Uint16(p.data[pos+6 : pos+8])
	debugOff := binary.LittleEndian.Uint32(p.data[pos+8 : pos+12])
	insnsSize := binary.LittleEndian.Uint32(p.data[pos+12 : pos+16])
	insnsOff := uint64(off) + 16
	insnsBytes := uint64(insnsSize) * 2
	if insnsOff+insnsBytes > uint64(len(p.data)) {
		return nil, p.fail(int(off)+12, "code_item", "instructions exceed file bounds")
	}
	code := &model.Code{RegistersSize: registers, InsSize: ins, OutsSize: outs, DebugInfoOffset: debugOff, Instructions: make([]uint16, insnsSize)}
	for i := range code.Instructions {
		at := int(insnsOff) + i*2
		code.Instructions[i] = binary.LittleEndian.Uint16(p.data[at : at+2])
	}
	if debugOff != 0 {
		debug, err := p.parseDebugInfo(debugOff, insnsSize, registers, declaredParameters)
		if err != nil {
			return nil, err
		}
		code.Debug = debug
	}
	if triesSize == 0 {
		return code, nil
	}
	triesOff := insnsOff + insnsBytes
	if insnsSize&1 != 0 {
		triesOff += 2
	}
	triesBytes := uint64(triesSize) * 8
	if triesOff+triesBytes > uint64(len(p.data)) {
		return nil, p.fail(int(triesOff), "code_item tries", "try table exceeds file bounds")
	}
	handlersOff := int(triesOff + triesBytes)
	handlers, err := p.catchHandlerList(handlersOff, insnsSize)
	if err != nil {
		return nil, err
	}
	code.Tries = make([]model.Try, triesSize)
	for i := range code.Tries {
		at := int(triesOff) + i*8
		start := binary.LittleEndian.Uint32(p.data[at : at+4])
		count := binary.LittleEndian.Uint16(p.data[at+4 : at+6])
		handlerOff := binary.LittleEndian.Uint16(p.data[at+6 : at+8])
		if uint64(start)+uint64(count) > uint64(insnsSize) {
			return nil, p.fail(at, "code_item try", "range [%d,%d) exceeds %d instruction units", start, uint64(start)+uint64(count), insnsSize)
		}
		handler, ok := handlers[handlerOff]
		if !ok {
			return nil, p.fail(at+6, "code_item try", "handler_off=%d does not identify a handler", handlerOff)
		}
		code.Tries[i] = model.Try{StartAddress: start, InstructionCount: count, HandlerOffset: handlerOff, Handler: handler}
	}
	return code, nil
}

func (p *parser) catchHandlerList(start int, insnsSize uint32) (map[uint16]model.CatchHandler, error) {
	cur := start
	count, err := p.uleb(&cur)
	if err != nil {
		return nil, p.fail(start, "catch_handler_list", "%v", err)
	}
	if count > uint32(len(p.data)-cur) {
		return nil, p.fail(start, "catch_handler_list", "handler count %d exceeds remaining bytes", count)
	}
	out := make(map[uint16]model.CatchHandler, count)
	for i := uint32(0); i < count; i++ {
		rel := cur - start
		if rel > math.MaxUint16 {
			return nil, p.fail(cur, "catch_handler_list", "handler offset exceeds uint16")
		}
		size, err := p.sleb(&cur)
		if err != nil {
			return nil, p.fail(cur, "catch_handler_list", "%v", err)
		}
		if size == math.MinInt32 {
			return nil, p.fail(cur, "catch_handler_list", "handler size overflows")
		}
		typedCount := size
		if typedCount < 0 {
			typedCount = -typedCount
		}
		if uint32(typedCount) > uint32(len(p.data)-cur) {
			return nil, p.fail(cur, "catch_handler_list", "typed handler count exceeds remaining bytes")
		}
		handler := model.CatchHandler{Handlers: make([]model.TypedHandler, typedCount)}
		for j := range handler.Handlers {
			typeIdx, err := p.uleb(&cur)
			if err != nil {
				return nil, p.fail(cur, "catch_handler", "%v", err)
			}
			address, err := p.uleb(&cur)
			if err != nil {
				return nil, p.fail(cur, "catch_handler", "%v", err)
			}
			if typeIdx >= uint32(len(p.file.Types)) || address >= insnsSize {
				return nil, p.fail(cur, "catch_handler", "invalid type_idx=%d or address=%d", typeIdx, address)
			}
			handler.Handlers[j] = model.TypedHandler{TypeIndex: typeIdx, Type: p.file.Types[typeIdx].Descriptor, Address: address}
		}
		if size <= 0 {
			address, err := p.uleb(&cur)
			if err != nil {
				return nil, p.fail(cur, "catch_handler", "%v", err)
			}
			if address >= insnsSize {
				return nil, p.fail(cur, "catch_handler", "catch-all address=%d out of range", address)
			}
			handler.CatchAllAddress = &address
		}
		out[uint16(rel)] = handler
	}
	return out, nil
}

func (p *parser) encodedArray(cur *int, depth int) ([]model.EncodedValue, error) {
	if depth >= maxEncodedDepth {
		return nil, p.fail(*cur, "encoded_array", "nesting exceeds %d", maxEncodedDepth)
	}
	count, err := p.uleb(cur)
	if err != nil {
		return nil, p.fail(*cur, "encoded_array", "%v", err)
	}
	if count > uint32(len(p.data)-*cur) {
		return nil, p.fail(*cur, "encoded_array", "value count %d exceeds remaining bytes", count)
	}
	out := make([]model.EncodedValue, count)
	for i := range out {
		out[i], err = p.encodedValue(cur, depth+1)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (p *parser) encodedValue(cur *int, depth int) (model.EncodedValue, error) {
	if depth >= maxEncodedDepth {
		return model.EncodedValue{}, p.fail(*cur, "encoded_value", "nesting exceeds %d", maxEncodedDepth)
	}
	if *cur < 0 || *cur >= len(p.data) {
		return model.EncodedValue{}, p.fail(*cur, "encoded_value", "truncated header")
	}
	header := p.data[*cur]
	(*cur)++
	kind := model.ValueType(header & 0x1f)
	arg := int(header >> 5)
	value := model.EncodedValue{Type: kind}
	maxArg := -1
	signed := false
	switch kind {
	case model.ValueByte:
		maxArg, signed = 0, true
	case model.ValueShort:
		maxArg, signed = 1, true
	case model.ValueChar:
		maxArg = 1
	case model.ValueInt:
		maxArg, signed = 3, true
	case model.ValueLong:
		maxArg, signed = 7, true
	case model.ValueFloat:
		maxArg = 3
	case model.ValueDouble:
		maxArg = 7
	case model.ValueMethodType, model.ValueMethodHandle, model.ValueString, model.ValueTypeRef, model.ValueField, model.ValueMethod, model.ValueEnum:
		maxArg = 3
	case model.ValueArray:
		if arg != 0 {
			return value, p.fail(*cur-1, "encoded_value", "array value_arg=%d", arg)
		}
		var err error
		value.Array, err = p.encodedArray(cur, depth)
		return value, err
	case model.ValueAnnotation:
		if arg != 0 {
			return value, p.fail(*cur-1, "encoded_value", "annotation value_arg=%d", arg)
		}
		annotation, err := p.encodedAnnotation(cur, depth)
		value.Annotation = annotation
		return value, err
	case model.ValueNull:
		if arg != 0 {
			return value, p.fail(*cur-1, "encoded_value", "null value_arg=%d", arg)
		}
		return value, nil
	case model.ValueBoolean:
		if arg > 1 {
			return value, p.fail(*cur-1, "encoded_value", "boolean value_arg=%d", arg)
		}
		value.Boolean = arg == 1
		return value, nil
	default:
		return value, p.fail(*cur-1, "encoded_value", "unsupported value_type=%#x", kind)
	}
	if arg > maxArg {
		return value, p.fail(*cur-1, "encoded_value", "value_arg=%d exceeds %d for type %#x", arg, maxArg, kind)
	}
	n := arg + 1
	if *cur < 0 || n > len(p.data)-*cur {
		return value, p.fail(*cur, "encoded_value", "truncated %d-byte scalar", n)
	}
	var raw uint64
	for i := 0; i < n; i++ {
		raw |= uint64(p.data[*cur+i]) << (8 * i)
	}
	*cur += n
	if kind == model.ValueFloat {
		raw <<= uint((4 - n) * 8)
	} else if kind == model.ValueDouble {
		raw <<= uint((8 - n) * 8)
	}
	value.Unsigned = raw
	if signed {
		shift := uint(64 - n*8)
		value.Signed = int64(raw<<shift) >> shift
	}
	if err := p.validateEncodedReference(kind, raw, *cur-n); err != nil {
		return value, err
	}
	return value, nil
}

func (p *parser) validateEncodedReference(kind model.ValueType, index uint64, off int) error {
	var limit int
	switch kind {
	case model.ValueString:
		limit = len(p.file.Strings)
	case model.ValueTypeRef:
		limit = len(p.file.Types)
	case model.ValueField, model.ValueEnum:
		limit = len(p.file.Fields)
	case model.ValueMethod:
		limit = len(p.file.Methods)
	case model.ValueMethodType:
		limit = len(p.file.Protos)
	case model.ValueMethodHandle:
		// DEX 038+ method_handle_ids are intentionally not modeled yet.
		return nil
	default:
		return nil
	}
	if index >= uint64(limit) {
		return p.fail(off, "encoded_value", "reference index=%d out of range for type %#x", index, kind)
	}
	return nil
}

func (p *parser) encodedAnnotation(cur *int, depth int) (*model.EncodedAnnotation, error) {
	typeIdx, err := p.uleb(cur)
	if err != nil {
		return nil, p.fail(*cur, "encoded_annotation", "%v", err)
	}
	count, err := p.uleb(cur)
	if err != nil {
		return nil, p.fail(*cur, "encoded_annotation", "%v", err)
	}
	if typeIdx >= uint32(len(p.file.Types)) || count > uint32(len(p.data)-*cur) {
		return nil, p.fail(*cur, "encoded_annotation", "invalid type_idx=%d or element count=%d", typeIdx, count)
	}
	annotation := &model.EncodedAnnotation{TypeIndex: typeIdx, Type: p.file.Types[typeIdx].Descriptor, Elements: make([]model.AnnotationElement, count)}
	for i := range annotation.Elements {
		nameIdx, err := p.uleb(cur)
		if err != nil {
			return nil, p.fail(*cur, "encoded_annotation", "%v", err)
		}
		if nameIdx >= uint32(len(p.file.Strings)) {
			return nil, p.fail(*cur, "encoded_annotation", "name_idx=%d out of range", nameIdx)
		}
		value, err := p.encodedValue(cur, depth+1)
		if err != nil {
			return nil, err
		}
		annotation.Elements[i] = model.AnnotationElement{NameIndex: nameIdx, Name: p.file.Strings[nameIdx], Value: value}
	}
	return annotation, nil
}

func (p *parser) uleb(cur *int) (uint32, error) {
	if *cur < 0 {
		return 0, fmt.Errorf("negative offset")
	}
	var out uint32
	for i := 0; i < maxVariableBytes; i++ {
		if *cur >= len(p.data) {
			return 0, fmt.Errorf("truncated uleb128")
		}
		b := p.data[*cur]
		(*cur)++
		if i == 4 && b&0xf0 != 0 {
			return 0, fmt.Errorf("uleb128 overflow")
		}
		out |= uint32(b&0x7f) << (7 * i)
		if b&0x80 == 0 {
			return out, nil
		}
	}
	return 0, fmt.Errorf("uleb128 too long")
}

func (p *parser) sleb(cur *int) (int32, error) {
	if *cur < 0 {
		return 0, fmt.Errorf("negative offset")
	}
	var out uint32
	var b byte
	for i := 0; i < maxVariableBytes; i++ {
		if *cur >= len(p.data) {
			return 0, fmt.Errorf("truncated sleb128")
		}
		b = p.data[*cur]
		*cur++
		if i == 4 && b&0xf0 != 0 && b&0xf0 != 0x70 {
			return 0, fmt.Errorf("sleb128 overflow")
		}
		out |= uint32(b&0x7f) << (7 * i)
		if b&0x80 == 0 {
			shift := uint((i + 1) * 7)
			if shift < 32 && b&0x40 != 0 {
				out |= ^uint32(0) << shift
			}
			return int32(out), nil
		}
	}
	return 0, fmt.Errorf("sleb128 too long")
}

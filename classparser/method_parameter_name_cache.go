package javaclassparser

import (
	"bytes"
	"slices"
)

// This stamp only protects packets that consumed ordinary MethodParameters
// names. It is not a general cache authority or an input-immutability claim.
type methodParameterNameCacheStamp struct {
	lambdaParameters                         bool
	lambdaFactory                            *lambdaSourceFactory
	lambdaParentNames                        []string
	object                                   *ClassObject
	method                                   *MemberInfo
	major, flags, nameIndex, descriptorIndex uint16
	name, descriptor                         string
	names                                    []string
	table                                    *UnparsedAttribute
	tableLength                              uint32
	tableInfo                                []byte
	code                                     *CodeAttribute
	codeLength                               uint32
	maxStack, maxLocals                      uint16
	codeBytes                                []byte
	exceptions                               []ExceptionTableEntry
	poolStamp                                [32]byte
	signature                                *SignatureAttribute
	signatureLength                          uint32
	signatureIndex                           uint16
	throws                                   *ExceptionsAttribute
	throwsLength                             uint32
	throwsIndices                            []uint16
}

func (c *ClassObjectDumper) newMethodParameterNameCacheStamp(method *MemberInfo, descriptor string, names []string) (*methodParameterNameCacheStamp, bool) {
	if names == nil {
		return nil, true
	}
	if c == nil || c.obj == nil || method == nil {
		return nil, false
	}
	name, named := sourceBridgeUTF8(c.obj, method.NameIndex)
	if !named {
		return nil, false
	}
	s := &methodParameterNameCacheStamp{name: name, object: c.obj, method: method, major: c.obj.MajorVersion, flags: method.AccessFlags, nameIndex: method.NameIndex, descriptorIndex: method.DescriptorIndex, descriptor: descriptor, names: append([]string(nil), names...)}
	for _, attribute := range method.Attributes {
		if !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		switch a := attribute.(type) {
		case *UnparsedAttribute:
			if a == nil {
				return nil, false
			}
			if a.Name == "MethodParameters" {
				if s.table != nil {
					return nil, false
				}
				s.table = a
			}
		case *SignatureAttribute:
			if a == nil || s.signature != nil {
				return nil, false
			}
			s.signature = a
		case *ExceptionsAttribute:
			if a == nil || s.throws != nil || len(a.ExceptionIndexTable) > 65535 {
				return nil, false
			}
			s.throws = a
		case *CodeAttribute:
			if a == nil || s.code != nil {
				return nil, false
			}
			s.code = a
		}
	}
	if s.table == nil || !nativeProofWork(c.Work, int64(len(s.table.Info))+1) {
		return nil, false
	}
	n := int64(len(s.table.Info))
	if s.code != nil {
		if len(s.code.Code) > 65535 || len(s.code.ExceptionTable) > 65535 {
			return nil, false
		}
		n += int64(len(s.code.Code)) + int64(len(s.code.ExceptionTable))*8
	}
	if s.throws != nil {
		n += int64(len(s.throws.ExceptionIndexTable)) * 2
	}
	if !nativeProofWork(c.Work, n) || c.Work != nil && c.Work.CheckAlloc(n+int64(len(names))*32+256) != nil {
		return nil, false
	}
	var poolKnown bool
	s.poolStamp, poolKnown = c.methodParameterNamePoolStamp()
	if !poolKnown {
		return nil, false
	}
	if s.signature != nil {
		s.signatureLength = s.signature.AttrLen
		s.signatureIndex = s.signature.SignatureIndex
	}
	if s.throws != nil {
		s.throwsLength = s.throws.AttrLen
		s.throwsIndices = slices.Clone(s.throws.ExceptionIndexTable)
	}
	s.tableLength = s.table.Length
	s.tableInfo = append([]byte(nil), s.table.Info...)
	if s.code != nil {
		s.codeLength = s.code.AttrLen
		s.maxStack = s.code.MaxStack
		s.maxLocals = s.code.MaxLocals
		s.codeBytes = append([]byte(nil), s.code.Code...)
		for _, entry := range s.code.ExceptionTable {
			if entry == nil {
				return nil, false
			}
			s.exceptions = append(s.exceptions, *entry)
		}
	}
	return s, true
}

func (c *ClassObjectDumper) methodParameterNameCacheClosed(s *methodParameterNameCacheStamp) bool {
	if s == nil {
		return true
	}
	if c == nil || c.obj != s.object || s.method == nil || c.obj.MajorVersion != s.major || s.method.AccessFlags != s.flags || s.method.NameIndex != s.nameIndex || s.method.DescriptorIndex != s.descriptorIndex {
		return false
	}
	name, named := sourceBridgeUTF8(c.obj, s.method.NameIndex)
	if !named || name != s.name {
		return false
	}
	var names []string
	var known bool
	if s.lambdaParameters {
		projection, closed := c.originalLambdaParameterMetadata(s.method, c.Work)
		if !closed || projection == nil || !s.lambdaFactory.same(projection.factory) || !slices.Equal(s.lambdaParentNames, projection.parentNames) {
			return false
		}
		names, known = projection.names, true
	} else {
		names, known = c.originalOrdinaryMethodParameterNames(s.method, s.descriptor)
	}
	if !known || len(names) != len(s.names) {
		return false
	}
	for i, name := range names {
		if name != s.names[i] {
			return false
		}
	}
	var table *UnparsedAttribute
	var code *CodeAttribute
	var signature *SignatureAttribute
	var throws *ExceptionsAttribute
	for _, attribute := range s.method.Attributes {
		if !nativeProofWork(c.Work, 1) {
			return false
		}
		switch a := attribute.(type) {
		case *UnparsedAttribute:
			if a == nil {
				return false
			}
			if a.Name == "MethodParameters" {
				if table != nil {
					return false
				}
				table = a
			}
		case *SignatureAttribute:
			if a == nil || signature != nil {
				return false
			}
			signature = a
		case *ExceptionsAttribute:
			if a == nil || throws != nil {
				return false
			}
			throws = a
		case *CodeAttribute:
			if a == nil || code != nil {
				return false
			}
			code = a
		}
	}
	if table != s.table || table == nil || table.Length != s.tableLength || !nativeProofWork(c.Work, int64(len(table.Info))) || !bytes.Equal(table.Info, s.tableInfo) || code != s.code {
		return false
	}
	if signature != s.signature || throws != s.throws {
		return false
	}
	if signature != nil && (signature.AttrLen != s.signatureLength || signature.SignatureIndex != s.signatureIndex) {
		return false
	}
	if throws != nil && (throws.AttrLen != s.throwsLength || !nativeProofWork(c.Work, int64(len(throws.ExceptionIndexTable))) || !slices.Equal(throws.ExceptionIndexTable, s.throwsIndices)) {
		return false
	}
	pool, poolKnown := c.methodParameterNamePoolStamp()
	if !poolKnown || pool != s.poolStamp {
		return false
	}
	if code == nil {
		return true
	}
	if code.AttrLen != s.codeLength || code.MaxStack != s.maxStack || code.MaxLocals != s.maxLocals || len(code.ExceptionTable) != len(s.exceptions) || !nativeProofWork(c.Work, int64(len(code.Code))+int64(len(code.ExceptionTable))) || !bytes.Equal(code.Code, s.codeBytes) {
		return false
	}
	for i, entry := range code.ExceptionTable {
		if entry == nil || *entry != s.exceptions[i] {
			return false
		}
	}
	return true
}

package javaclassparser

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
)

const methodParameterNamePoolPayloadLimit = 8 << 20

// Seal only the CP dependency of an ordinary named source packet. The method
// reader uses obj.ConstantPool while bytecode getters use c.ConstantPool, so
// both actual inputs matter. This is not a general cache or classfile proof.
// Stream typed payloads; never marshal the original class or allocate a pool
// sized byte buffer. Nil Work still has fixed entry and aggregate payload caps.
func (c *ClassObjectDumper) methodParameterNamePoolStamp() ([32]byte, bool) {
	var empty [32]byte
	if c == nil || c.obj == nil {
		return empty, false
	}
	h := sha256.New()
	aggregate := sha256.New()
	var first [32]byte
	var firstPayload int64
	remaining := int64(methodParameterNamePoolPayloadLimit)
	var word [8]byte
	var chunk [1024]byte
	charge := func(n int64) bool {
		if n < 0 || n > remaining || !nativeProofWork(c.Work, n+1) {
			return false
		}
		remaining -= n
		return true
	}
	put := func(value uint64) { binary.BigEndian.PutUint64(word[:], value); h.Write(word[:]) }
	if c.Work != nil && c.Work.CheckAlloc(2048) != nil {
		return empty, false
	}
	for ordinal, pool := range [][]ConstantInfo{c.obj.ConstantPool, c.ConstantPool} {
		// The declaration and bytecode views normally share the exact same
		// slice. No external callback runs in this typed payload scan. Hash it
		// once, retaining both ordered view digests and the original aggregate
		// payload bound. A detached view still gets its own complete scan;
		// neither equality of names nor an earlier request is a reuse proof.
		if ordinal == 1 && len(pool) != 0 && len(pool) == len(c.obj.ConstantPool) && &pool[0] == &c.obj.ConstantPool[0] {
			if firstPayload > remaining || !nativeProofWork(c.Work, 1) {
				return empty, false
			}
			remaining -= firstPayload
			aggregate.Write(first[:])
			continue
		}
		h.Reset()
		before := remaining
		if len(pool) == 0 || len(pool) > 65534 || !charge(int64(len(pool))*8) {
			return empty, false
		}
		put(uint64(len(pool)))
		for i, constant := range pool {
			if !charge(16) {
				return empty, false
			}
			switch v := constant.(type) {
			case nil:
				if i == 0 {
					return empty, false
				}
				switch prior := pool[i-1].(type) {
				case *ConstantLongInfo:
					if prior == nil {
						return empty, false
					}
				case *ConstantDoubleInfo:
					if prior == nil {
						return empty, false
					}
				default:
					return empty, false
				}
				put(0)
			case *ConstantUtf8Info:
				if v == nil || v.DecodeErr != nil || len(v.Value) > 65535*6 || len(v.Units) > 65535 || !charge(int64(len(v.Value))+int64(len(v.Units))*2) {
					return empty, false
				}
				put(CONSTANT_Utf8)
				put(uint64(len(v.Value)))
				for text := v.Value; len(text) > 0; {
					n := copy(chunk[:], text)
					h.Write(chunk[:n])
					text = text[n:]
				}
				put(uint64(len(v.Units)))
				for units := v.Units; len(units) > 0; {
					n := len(units)
					if n > len(chunk)/2 {
						n = len(chunk) / 2
					}
					for j := 0; j < n; j++ {
						binary.BigEndian.PutUint16(chunk[j*2:j*2+2], units[j])
					}
					h.Write(chunk[:n*2])
					units = units[n:]
				}
			case *ConstantIntegerInfo:
				if v == nil {
					return empty, false
				}
				put(CONSTANT_Integer)
				put(uint64(uint32(v.Value)))
			case *ConstantFloatInfo:
				if v == nil {
					return empty, false
				}
				put(CONSTANT_Float)
				put(uint64(math.Float32bits(v.Value)))
			case *ConstantLongInfo:
				if v == nil || i+1 >= len(pool) || pool[i+1] != nil {
					return empty, false
				}
				put(CONSTANT_Long)
				put(uint64(v.Value))
			case *ConstantDoubleInfo:
				if v == nil || i+1 >= len(pool) || pool[i+1] != nil {
					return empty, false
				}
				put(CONSTANT_Double)
				put(math.Float64bits(v.Value))
			case *ConstantClassInfo:
				if v == nil {
					return empty, false
				}
				put(CONSTANT_Class)
				put(uint64(v.NameIndex))
			case *ConstantStringInfo:
				if v == nil {
					return empty, false
				}
				put(CONSTANT_String)
				put(uint64(v.StringIndex))
			case *ConstantFieldrefInfo:
				if v == nil {
					return empty, false
				}
				put(CONSTANT_Fieldref)
				put(uint64(v.ClassIndex))
				put(uint64(v.NameAndTypeIndex))
			case *ConstantMethodrefInfo:
				if v == nil {
					return empty, false
				}
				put(CONSTANT_Methodref)
				put(uint64(v.ClassIndex))
				put(uint64(v.NameAndTypeIndex))
			case *ConstantInterfaceMethodrefInfo:
				if v == nil {
					return empty, false
				}
				put(CONSTANT_InterfaceMethodref)
				put(uint64(v.ClassIndex))
				put(uint64(v.NameAndTypeIndex))
			case *ConstantNameAndTypeInfo:
				if v == nil {
					return empty, false
				}
				put(CONSTANT_NameAndType)
				put(uint64(v.NameIndex))
				put(uint64(v.DescriptorIndex))
			case *ConstantMethodHandleInfo:
				if v == nil {
					return empty, false
				}
				put(CONSTANT_MethodHandle)
				put(uint64(v.ReferenceKind))
				put(uint64(v.ReferenceIndex))
			case *ConstantMethodTypeInfo:
				if v == nil {
					return empty, false
				}
				put(CONSTANT_MethodType)
				put(uint64(v.DescriptorIndex))
			case *ConstantDynamicInfo:
				if v == nil {
					return empty, false
				}
				put(CONSTANT_Dynamic)
				put(uint64(v.BootstrapMethodAttrIndex))
				put(uint64(v.NameAndTypeIndex))
			case *ConstantInvokeDynamicInfo:
				if v == nil {
					return empty, false
				}
				put(CONSTANT_InvokeDynamic)
				put(uint64(v.BootstrapMethodAttrIndex))
				put(uint64(v.NameAndTypeIndex))
			case *ConstantModuleInfo:
				if v == nil {
					return empty, false
				}
				put(CONSTANT_Module)
				put(uint64(v.NameIndex))
			case *ConstantPackageInfo:
				if v == nil {
					return empty, false
				}
				put(CONSTANT_Package)
				put(uint64(v.NameIndex))
			default:
				return empty, false
			}
		}
		var digest [32]byte
		copy(digest[:], h.Sum(nil))
		aggregate.Write(digest[:])
		if ordinal == 0 {
			first, firstPayload = digest, before-remaining
		}
	}
	var result [32]byte
	copy(result[:], aggregate.Sum(nil))
	return result, true
}

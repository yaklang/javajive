package classfilebuilder

import (
	"encoding/binary"
	"fmt"
	"math"
	"unicode/utf16"
)

const (
	cpUTF8               = 1
	cpInteger            = 3
	cpFloat              = 4
	cpLong               = 5
	cpDouble             = 6
	cpClass              = 7
	cpString             = 8
	cpFieldref           = 9
	cpMethodref          = 10
	cpInterfaceMethodref = 11
	cpNameAndType        = 12
	// cpPadding marks the unusable second slot a long or double constant
	// occupies. It is never written to the class file.
	cpPadding = 0
)

type cpEntry struct {
	tag   byte
	a, b  uint16
	i32   int32
	i64   int64
	bytes []byte
	key   string
	text  string
}

// ConstantPool is an append-only, deduplicating JVM constant pool. Index zero
// is never returned. Its methods are deterministic for a deterministic call
// order.
type ConstantPool struct {
	entries []cpEntry
	index   map[string]uint16
}

func newConstantPool() *ConstantPool {
	return &ConstantPool{index: make(map[string]uint16)}
}

// Count returns constant_pool_count, including the unusable zero entry.
func (p *ConstantPool) Count() int { return len(p.entries) + 1 }

func (p *ConstantPool) add(e cpEntry) (uint16, error) {
	if index, ok := p.index[e.key]; ok {
		return index, nil
	}
	if len(p.entries) >= math.MaxUint16-1 {
		return 0, fmt.Errorf("constant pool limit exceeded: constant_pool_count would exceed 65535")
	}
	p.entries = append(p.entries, e)
	index := uint16(len(p.entries))
	p.index[e.key] = index
	return index, nil
}

// addWide adds a two-slot constant. A long or double occupies index n and the
// unusable index n+1, so the pool keeps a padding entry for the second slot.
func (p *ConstantPool) addWide(e cpEntry) (uint16, error) {
	if index, ok := p.index[e.key]; ok {
		return index, nil
	}
	if len(p.entries)+2 > math.MaxUint16-1 {
		return 0, fmt.Errorf("constant pool limit exceeded: constant_pool_count would exceed 65535")
	}
	p.entries = append(p.entries, e)
	index := uint16(len(p.entries))
	p.entries = append(p.entries, cpEntry{tag: cpPadding})
	p.index[e.key] = index
	return index, nil
}

// UTF8 adds a CONSTANT_Utf8 encoded as JVM modified UTF-8.
func (p *ConstantPool) UTF8(value string) (uint16, error) {
	return p.UTF16(utf16.Encode([]rune(value)))
}

// UTF16 adds a lossless sequence of UTF-16 code units as CONSTANT_Utf8. This
// form can represent isolated surrogate code units that a Go string cannot.
func (p *ConstantPool) UTF16(units []uint16) (uint16, error) {
	payload := modifiedUTF8(units)
	if len(payload) > math.MaxUint16 {
		return 0, fmt.Errorf("modified UTF-8 payload length %d exceeds 65535", len(payload))
	}
	key := "u:" + string(payload)
	return p.add(cpEntry{tag: cpUTF8, bytes: payload, key: key})
}

// Class adds a CONSTANT_Class. Names use JVM internal form, such as
// java/lang/Object; array descriptors are also accepted by the classfile.
func (p *ConstantPool) Class(name string) (uint16, error) {
	if err := validateClassConstantName(name); err != nil {
		return 0, err
	}
	nameIndex, err := p.UTF8(name)
	if err != nil {
		return 0, err
	}
	return p.add(cpEntry{tag: cpClass, a: nameIndex, key: "c:" + name, text: name})
}

// String adds a CONSTANT_String.
func (p *ConstantPool) String(value string) (uint16, error) {
	utf8Index, err := p.UTF8(value)
	if err != nil {
		return 0, err
	}
	return p.add(cpEntry{tag: cpString, a: utf8Index, key: "s:" + value, text: value})
}

// Integer adds a CONSTANT_Integer.
func (p *ConstantPool) Integer(value int32) (uint16, error) {
	return p.add(cpEntry{tag: cpInteger, i32: value, key: fmt.Sprintf("i:%d", value)})
}

// Long adds a CONSTANT_Long.
func (p *ConstantPool) Long(value int64) (uint16, error) {
	return p.addWide(cpEntry{tag: cpLong, i64: value, key: fmt.Sprintf("j:%d", value)})
}

// Float adds a CONSTANT_Float from its IEEE-754 bit pattern.
func (p *ConstantPool) Float(bits uint32) (uint16, error) {
	return p.add(cpEntry{tag: cpFloat, i32: int32(bits), key: fmt.Sprintf("f:%08x", bits)})
}

// Double adds a CONSTANT_Double from its IEEE-754 bit pattern.
func (p *ConstantPool) Double(bits uint64) (uint16, error) {
	return p.addWide(cpEntry{tag: cpDouble, i64: int64(bits), key: fmt.Sprintf("d:%016x", bits)})
}

// NameAndType adds a CONSTANT_NameAndType.
func (p *ConstantPool) NameAndType(name, descriptor string) (uint16, error) {
	if len(descriptor) > 0 && descriptor[0] == '(' {
		if err := validateMethodSymbol(name, descriptor); err != nil {
			return 0, err
		}
	} else if err := validateFieldSymbol(name, descriptor); err != nil {
		return 0, err
	}
	nameIndex, err := p.UTF8(name)
	if err != nil {
		return 0, err
	}
	descriptorIndex, err := p.UTF8(descriptor)
	if err != nil {
		return 0, err
	}
	return p.add(cpEntry{tag: cpNameAndType, a: nameIndex, b: descriptorIndex, key: "n:" + name + "\x00" + descriptor})
}

func (p *ConstantPool) member(tag byte, kind, owner, name, descriptor string) (uint16, error) {
	// Method owners may be array descriptors ("[I" for array clone); fields
	// never take an array owner.
	if tag == cpFieldref {
		if err := validateInternalName(owner); err != nil {
			return 0, fmt.Errorf("owner %q: %w", owner, err)
		}
	} else if err := validateClassConstantName(owner); err != nil {
		return 0, fmt.Errorf("owner %q: %w", owner, err)
	}
	switch tag {
	case cpFieldref:
		if err := validateFieldSymbol(name, descriptor); err != nil {
			return 0, err
		}
	case cpMethodref:
		if err := validateMethodSymbol(name, descriptor); err != nil {
			return 0, err
		}
		if name == "<clinit>" {
			return 0, fmt.Errorf("method reference name must not be <clinit>")
		}
	case cpInterfaceMethodref:
		if err := validateMethodSymbol(name, descriptor); err != nil {
			return 0, err
		}
		if name == "<init>" || name == "<clinit>" {
			return 0, fmt.Errorf("interface method reference has forbidden name %q", name)
		}
	}
	classIndex, err := p.Class(owner)
	if err != nil {
		return 0, err
	}
	nameTypeIndex, err := p.NameAndType(name, descriptor)
	if err != nil {
		return 0, err
	}
	return p.add(cpEntry{tag: tag, a: classIndex, b: nameTypeIndex, key: kind + ":" + owner + "\x00" + name + "\x00" + descriptor, text: descriptor})
}

// Fieldref adds a CONSTANT_Fieldref.
func (p *ConstantPool) Fieldref(owner, name, descriptor string) (uint16, error) {
	return p.member(cpFieldref, "f", owner, name, descriptor)
}

// Methodref adds a CONSTANT_Methodref.
func (p *ConstantPool) Methodref(owner, name, descriptor string) (uint16, error) {
	return p.member(cpMethodref, "m", owner, name, descriptor)
}

// InterfaceMethodref adds a CONSTANT_InterfaceMethodref.
func (p *ConstantPool) InterfaceMethodref(owner, name, descriptor string) (uint16, error) {
	return p.member(cpInterfaceMethodref, "im", owner, name, descriptor)
}

func (p *ConstantPool) entry(index uint16) (cpEntry, error) {
	if index == 0 || int(index) > len(p.entries) {
		return cpEntry{}, fmt.Errorf("constant pool index %d is out of range", index)
	}
	entry := p.entries[index-1]
	if entry.tag == cpPadding {
		return cpEntry{}, fmt.Errorf("constant pool index %d is the unusable half of a long/double constant", index)
	}
	return entry, nil
}

func (p *ConstantPool) write(dst *byteWriter) error {
	if p.Count() > math.MaxUint16 {
		return fmt.Errorf("constant_pool_count %d exceeds 65535", p.Count())
	}
	dst.u2(uint16(p.Count()))
	for _, e := range p.entries {
		if e.tag == cpPadding {
			continue
		}
		dst.u1(e.tag)
		switch e.tag {
		case cpUTF8:
			dst.u2(uint16(len(e.bytes)))
			dst.bytes(e.bytes)
		case cpInteger, cpFloat:
			dst.u4(uint32(e.i32))
		case cpLong, cpDouble:
			dst.u4(uint32(uint64(e.i64) >> 32))
			dst.u4(uint32(e.i64))
		case cpClass, cpString:
			dst.u2(e.a)
		case cpFieldref, cpMethodref, cpInterfaceMethodref, cpNameAndType:
			dst.u2(e.a)
			dst.u2(e.b)
		default:
			return fmt.Errorf("unsupported internal constant pool tag %d", e.tag)
		}
	}
	return nil
}

func modifiedUTF8(units []uint16) []byte {
	out := make([]byte, 0, len(units)*3)
	for _, unit := range units {
		switch {
		case unit == 0:
			out = append(out, 0xc0, 0x80)
		case unit <= 0x7f:
			out = append(out, byte(unit))
		case unit <= 0x7ff:
			out = append(out, byte(0xc0|(unit>>6)), byte(0x80|(unit&0x3f)))
		default:
			out = append(out, byte(0xe0|(unit>>12)), byte(0x80|((unit>>6)&0x3f)), byte(0x80|(unit&0x3f)))
		}
	}
	return out
}

type byteWriter struct{ data []byte }

func (w *byteWriter) u1(v byte)      { w.data = append(w.data, v) }
func (w *byteWriter) bytes(v []byte) { w.data = append(w.data, v...) }
func (w *byteWriter) u2(v uint16)    { w.data = binary.BigEndian.AppendUint16(w.data, v) }
func (w *byteWriter) u4(v uint32)    { w.data = binary.BigEndian.AppendUint32(w.data, v) }
func (w *byteWriter) i2(v int16)     { w.u2(uint16(v)) }
func (w *byteWriter) i4(v int32)     { w.u4(uint32(v)) }

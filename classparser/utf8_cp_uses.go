package javaclassparser

import (
	"fmt"
	"unicode/utf16"

	"github.com/yaklang/javajive/internal/mutf8"
)

// Utf8String is the lossless representation for annotation tag 's' and
// bootstrap recipe constants. Parser T07 should store this instead of a Go
// string so unpaired surrogates survive. Value is display/compat only.
type Utf8String struct {
	Units []uint16
	Value string
}

func Utf8StringFromInfo(u *ConstantUtf8Info) Utf8String {
	if u == nil {
		return Utf8String{}
	}
	return Utf8String{Units: u.CodeUnits(), Value: u.Value}
}

func annotationStringUnits(v any) []uint16 {
	switch x := v.(type) {
	case Utf8String:
		if x.Units != nil {
			return x.Units
		}
		return utf16.Encode([]rune(x.Value))
	case *Utf8String:
		if x == nil {
			return nil
		}
		return annotationStringUnits(*x)
	case *ConstantUtf8Info:
		if x == nil {
			return nil
		}
		return x.semanticUnits()
	case string:
		return utf16.Encode([]rune(x))
	default:
		return nil
	}
}

// CheckUtf8UseSites applies JVMS name/descriptor rules to actual CP references.
// Illegal descriptors/names are invalid class bytes. JVM-legal but
// Java-unrepresentable names (NUL, '-') are accepted here; source rendering
// uses mutf8.JavaSourceIdentifierOK separately.
func (obj *ClassObject) CheckUtf8UseSites() error {
	if obj == nil {
		return nil
	}
	utf := func(index uint16) (*ConstantUtf8Info, error) {
		if int(index) == 0 || int(index) > len(obj.ConstantPool) {
			return nil, fmt.Errorf("utf8 use: bad CP index %d", index)
		}
		u, ok := obj.ConstantPool[index-1].(*ConstantUtf8Info)
		if !ok || u == nil {
			return nil, fmt.Errorf("utf8 use: index %d is not CONSTANT_Utf8", index)
		}
		return u, nil
	}
	for i, c := range obj.ConstantPool {
		switch t := c.(type) {
		case *ConstantClassInfo:
			u, err := utf(t.NameIndex)
			if err != nil {
				return err
			}
			if err := mutf8.ValidateUse(u.semanticUnits(), mutf8.UseInternalName); err != nil {
				return fmt.Errorf("CONSTANT_Class[%d]: %w", i+1, err)
			}
		case *ConstantNameAndTypeInfo:
			name, err := utf(t.NameIndex)
			if err != nil {
				return err
			}
			desc, err := utf(t.DescriptorIndex)
			if err != nil {
				return err
			}
			du := desc.semanticUnits()
			if len(du) > 0 && du[0] == '(' {
				if err := mutf8.ValidateMethodName(name.semanticUnits()); err != nil {
					return fmt.Errorf("NameAndType[%d] method name: %w", i+1, err)
				}
				if err := mutf8.ValidateMethodDescriptor(du); err != nil {
					return fmt.Errorf("NameAndType[%d] method desc: %w", i+1, err)
				}
			} else {
				if err := mutf8.ValidateUnqualifiedName(name.semanticUnits()); err != nil {
					return fmt.Errorf("NameAndType[%d] field name: %w", i+1, err)
				}
				if err := mutf8.ValidateFieldDescriptor(du); err != nil {
					return fmt.Errorf("NameAndType[%d] field desc: %w", i+1, err)
				}
			}
		}
	}
	for _, m := range obj.Methods {
		if m == nil {
			continue
		}
		name, err := utf(m.NameIndex)
		if err != nil {
			return err
		}
		desc, err := utf(m.DescriptorIndex)
		if err != nil {
			return err
		}
		if err := mutf8.ValidateMethodName(name.semanticUnits()); err != nil {
			return fmt.Errorf("method name: %w", err)
		}
		du := desc.semanticUnits()
		if err := mutf8.ValidateMethodDescriptor(du); err != nil {
			return fmt.Errorf("method %s descriptor: %w", name.Value, err)
		}
		instance := m.AccessFlags&StaticFlag == 0
		if _, err := mutf8.InvokeSlots(du, instance); err != nil {
			return fmt.Errorf("method %s slots: %w", name.Value, err)
		}
	}
	for _, f := range obj.Fields {
		if f == nil {
			continue
		}
		name, err := utf(f.NameIndex)
		if err != nil {
			return err
		}
		desc, err := utf(f.DescriptorIndex)
		if err != nil {
			return err
		}
		if err := mutf8.ValidateUnqualifiedName(name.semanticUnits()); err != nil {
			return fmt.Errorf("field name: %w", err)
		}
		if err := mutf8.ValidateFieldDescriptor(desc.semanticUnits()); err != nil {
			return fmt.Errorf("field %s descriptor: %w", name.Value, err)
		}
	}
	return nil
}

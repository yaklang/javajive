package core

import (
	"fmt"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

type cpOperandKind uint8

const (
	cpLiteral cpOperandKind = iota
	cpClass
	cpMember
)

// validateConstantPoolReferences rejects malformed bytecode that points at a
// missing or wrong-kind constant before stack simulation indexes the pool or
// asserts a member/class type. A malformed classfile then produces a
// structured method error instead of a recovered runtime panic. Verifier-valid
// classes always pass: the same resolved constant is consumed by the normal
// stack simulation immediately afterward.
func (d *Decompiler) validateConstantPoolReferences() error {
	if d == nil || d.constantPoolGetter == nil {
		return nil
	}
	for _, op := range d.opCodes {
		index, kind, ok := constantPoolOperand(op)
		if !ok {
			continue
		}
		getter := d.constantPoolGetter
		if kind == cpLiteral {
			getter = d.ConstantPoolLiteralGetter
		}
		if getter == nil {
			continue
		}
		value, err := safeConstantPoolLookup(getter, index)
		if err != nil {
			return fmt.Errorf("invalid_constant_pool_reference: %s at PC %d index %d: %w",
				opcodeName(op), op.CurrentOffset, index, err)
		}
		switch kind {
		case cpMember:
			if _, ok := value.(*values.JavaClassMember); !ok {
				return fmt.Errorf("invalid_constant_pool_reference: %s at PC %d index %d resolved to %T, want member",
					opcodeName(op), op.CurrentOffset, index, value)
			}
		case cpClass:
			if _, ok := value.(*values.JavaClassValue); !ok {
				return fmt.Errorf("invalid_constant_pool_reference: %s at PC %d index %d resolved to %T, want class",
					opcodeName(op), op.CurrentOffset, index, value)
			}
		case cpLiteral:
			if value == nil {
				return fmt.Errorf("invalid_constant_pool_reference: %s at PC %d index %d resolved to nil",
					opcodeName(op), op.CurrentOffset, index)
			}
		}
	}
	return nil
}

func opcodeName(op *OpCode) string {
	if op == nil || op.Instr == nil || op.Instr.Name == "" {
		return "opcode"
	}
	return op.Instr.Name
}

// constantPoolOperand returns the constant-pool index and the value family the
// opcode requires. OP_INVOKEDYNAMIC is handled through a separate call-site
// resolver and is intentionally omitted.
func constantPoolOperand(op *OpCode) (int, cpOperandKind, bool) {
	if op == nil || op.Instr == nil {
		return 0, cpLiteral, false
	}
	switch op.Instr.OpCode {
	case OP_LDC:
		if len(op.Data) < 1 {
			return 0, cpLiteral, false
		}
		return int(op.Data[0]), cpLiteral, true
	case OP_LDC_W, OP_LDC2_W:
		if len(op.Data) < 2 {
			return 0, cpLiteral, false
		}
		return int(Convert2bytesToInt(op.Data)), cpLiteral, true
	case OP_GETSTATIC, OP_PUTSTATIC, OP_GETFIELD, OP_PUTFIELD,
		OP_INVOKEVIRTUAL, OP_INVOKESPECIAL, OP_INVOKESTATIC, OP_INVOKEINTERFACE:
		if len(op.Data) < 2 {
			return 0, cpLiteral, false
		}
		return int(Convert2bytesToInt(op.Data)), cpMember, true
	case OP_NEW, OP_ANEWARRAY, OP_CHECKCAST, OP_INSTANCEOF, OP_MULTIANEWARRAY:
		if len(op.Data) < 2 {
			return 0, cpLiteral, false
		}
		return int(Convert2bytesToInt(op.Data)), cpClass, true
	}
	return 0, cpLiteral, false
}

func safeConstantPoolLookup(getter func(int) values.JavaValue, index int) (value values.JavaValue, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			value = nil
			err = fmt.Errorf("%v", recovered)
		}
	}()
	if index <= 0 {
		return nil, fmt.Errorf("non-positive constant pool index %d", index)
	}
	value = getter(index)
	if value == nil {
		return nil, fmt.Errorf("nil constant")
	}
	return value, nil
}

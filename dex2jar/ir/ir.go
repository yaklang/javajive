// Package ir decodes and validates the strict DEX 035 subset accepted by the
// native translator. Offsets and branch targets are measured in 16-bit DEX
// code units.
package ir

import (
	"fmt"

	"github.com/yaklang/javajive/dex2jar/model"
	"github.com/yaklang/javajive/dex2jar/report"
)

// Context supplies source identity for diagnostics.
type Context struct {
	Entry  string
	Class  string
	Method string
}

// Input identifies one encoded method and the tables needed to resolve its
// symbolic operands.
type Input struct {
	File   *model.File
	Class  *model.Class
	Method *model.EncodedMethod
}

// CategoryKind is the verifier-relevant category of a DEX register value.
type CategoryKind uint8

const (
	Unknown CategoryKind = iota
	Int
	Reference
	Null
	Uninitialized
	// Long and Double are category-2 values: they occupy the register named by
	// the instruction plus the following register.
	Long
	Double
	Float
	// Bits32 and Bits64 are constant definitions whose width is known but whose
	// interpretation is not: DEX constants are raw bit patterns, so the same
	// const-wide can feed a long or a double operation. The first typed use
	// resolves the kind.
	Bits32
	Bits64
	// WideUpper marks the second register of a category-2 value. Reading that
	// register through a narrow instruction is invalid DEX, and the category
	// join rejects it instead of silently reading the high half.
	WideUpper
	Conflict
)

// IntLike is retained as an internal compatibility alias for Int.
const IntLike = Int

func (k CategoryKind) String() string {
	switch k {
	case Unknown:
		return "unknown"
	case Int:
		return "int"
	case Reference:
		return "reference"
	case Null:
		return "null"
	case Uninitialized:
		return "uninitialized"
	case Long:
		return "long"
	case Double:
		return "double"
	case Float:
		return "float"
	case Bits32:
		return "32-bit constant"
	case Bits64:
		return "64-bit constant"
	case WideUpper:
		return "wide-high"
	case Conflict:
		return "conflict"
	default:
		return fmt.Sprintf("category(%d)", k)
	}
}

// Words reports how many 32-bit registers the category occupies.
func (k CategoryKind) Words() int {
	if k == Long || k == Double || k == Bits64 {
		return 2
	}
	return 1
}

// isUntyped reports whether the category is a constant whose interpretation is
// still open.
func (k CategoryKind) isUntyped() bool {
	return k == Null || k == Bits32 || k == Bits64
}

// Category records a register category at a program point. Allocation is the
// code-unit offset of new-instance plus one; zero means no allocation identity.
type Category struct {
	Kind       CategoryKind
	Descriptor string
	Allocation uint32
}

// Instruction is one decoded DEX instruction.
type Instruction struct {
	Offset  uint32
	Opcode  byte
	Name    string
	Format  string
	Width   uint16
	Regs    []uint16
	Index   uint32
	Literal int64
	Target  int64
}

func (i Instruction) End() uint32 { return i.Offset + uint32(i.Width) }

// Block is a basic block. Instruction indices refer to Method.Instructions.
type Block struct {
	ID           int
	Start        uint32
	End          uint32
	Instructions []int
	Successors   []int
	Predecessors []int
	Entry        []Category
	// Handler marks a block entered by the JVM exception dispatcher. Its first
	// instruction must be move-exception and the exception is on the stack.
	Handler bool
}

// Allocation describes a verifier-safe new-instance/constructor pair.
type Allocation struct {
	ID             uint32
	Descriptor     string
	NewInstruction int
	Constructor    int
	AliasRegisters []uint16
	// Fused reports that only alias moves separate allocation from
	// initialization, so the translator may emit one fused new/dup/invokespecial
	// sequence instead of keeping the uninitialized value in a local.
	Fused bool
}

// Method is the validated intermediate representation of one DEX method.
type Method struct {
	Code         *model.Code
	Instructions []Instruction
	Blocks       []Block
	// RegisterTypes holds the method-entry category of each register (parameters
	// and the receiver). Per-instruction categories live in Categories.
	RegisterTypes []Category
	// Categories is a flat table of value categories, indexed by
	// instruction*CategoryStride + register. It is the category of that register
	// immediately before the instruction executes.
	Categories     []Category
	CategoryStride int
	// Definitions records the category each instruction writes, if any.
	Definitions []Definition
	Allocations []Allocation
	Context     Context
	Proto       model.Proto
	AccessFlags uint32
	// constantKinds records the interpretation the backward refinement derived
	// for constant definitions whose literal is a raw bit pattern.
	constantKinds map[int]Category
	// demands records the categories each instruction requires of its operands.
	demands [][]demand
}

func contextFor(in Input) Context {
	ctx := Context{}
	if in.File != nil {
		ctx.Entry = in.File.Entry
	}
	if in.Class != nil {
		ctx.Class = in.Class.Descriptor
	}
	if in.Method != nil {
		ctx.Method = in.Method.Method.Name
	}
	return ctx
}

func diagnostic(ctx Context, ins *Instruction, op string, err error) error {
	offset := int64(-1)
	if ins != nil {
		offset = int64(ins.Offset)
		if op == "" {
			op = fmt.Sprintf("%s(0x%02x)", ins.Name, ins.Opcode)
		}
	}
	return report.New(ctx.Entry, ctx.Class, ctx.Method, offset, op, err)
}

// DescriptorCategory returns the register category for a field or method
// descriptor. Category-2 descriptors (long and double) report their own kind so
// callers can reserve the second register slot.
func DescriptorCategory(descriptor string) (CategoryKind, error) {
	if _, err := parseDescriptor(descriptor, true); err != nil {
		return Unknown, err
	}
	switch descriptor[0] {
	case 'V':
		return Unknown, nil
	case 'Z', 'B', 'C', 'S', 'I':
		return IntLike, nil
	case 'J':
		return Long, nil
	case 'D':
		return Double, nil
	case 'F':
		return Float, nil
	case 'L', '[':
		return Reference, nil
	default:
		return Unknown, fmt.Errorf("invalid descriptor %q", descriptor)
	}
}

// InternalName converts an object descriptor to a JVM internal name. Array
// descriptors are already valid CONSTANT_Class names and are returned intact.
func InternalName(descriptor string) (string, error) {
	kind, err := parseDescriptor(descriptor, false)
	if err != nil {
		return "", err
	}
	if kind != descriptorReference {
		return "", fmt.Errorf("descriptor %q is not a reference type", descriptor)
	}
	if descriptor[0] == '[' {
		return descriptor, nil
	}
	return descriptor[1 : len(descriptor)-1], nil
}

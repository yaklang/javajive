package frametransfer

import (
	"fmt"
	"math"
)

type Kind uint8

const (
	Top Kind = iota
	Int
	Float
	Long
	Double
	LongTail
	DoubleTail
	Ref
	Null
	UninitThis
	UninitNew
)

func (k Kind) String() string {
	switch k {
	case Top:
		return "top"
	case Int:
		return "int"
	case Float:
		return "float"
	case Long:
		return "long"
	case Double:
		return "double"
	case LongTail:
		return "long_tail"
	case DoubleTail:
		return "double_tail"
	case Ref:
		return "ref"
	case Null:
		return "null"
	case UninitThis:
		return "uninit_this"
	case UninitNew:
		return "uninit_new"
	default:
		return fmt.Sprintf("kind%d", k)
	}
}

func (k Kind) IsTail() bool { return k == LongTail || k == DoubleTail }

func (k Kind) IsCat2Head() bool { return k == Long || k == Double }

func (k Kind) Category() int {
	switch k {
	case Long, Double:
		return 2
	case LongTail, DoubleTail, Top:
		return 0
	default:
		return 1
	}
}

type Type struct {
	Kind    Kind
	Class   string
	NewPC   uint16
	HasBits bool
	Bits    uint64
	HasInt  bool
	Int     int32
	HasLong bool
	Long    int64
}

func T(k Kind) Type { return Type{Kind: k} }

func RefOf(class string) Type { return Type{Kind: Ref, Class: class} }

func UninitAt(pc uint16) Type { return Type{Kind: UninitNew, NewPC: pc} }

func IntConst(v int32) Type { return Type{Kind: Int, HasInt: true, Int: v} }

func LongConst(v int64) Type { return Type{Kind: Long, HasLong: true, Long: v} }

func FloatBits(bits uint32) Type {
	return Type{Kind: Float, HasBits: true, Bits: uint64(bits)}
}

func DoubleBits(bits uint64) Type {
	return Type{Kind: Double, HasBits: true, Bits: bits}
}

func TailOf(h Type) Type {
	switch h.Kind {
	case Long:
		return Type{Kind: LongTail}
	case Double:
		return Type{Kind: DoubleTail}
	default:
		return Type{Kind: Top}
	}
}

func (t Type) Equal(o Type) bool {
	if t.Kind != o.Kind || t.Class != o.Class || t.NewPC != o.NewPC {
		return false
	}
	if t.HasBits != o.HasBits || (t.HasBits && t.Bits != o.Bits) {
		return false
	}
	if t.HasInt != o.HasInt || (t.HasInt && t.Int != o.Int) {
		return false
	}
	if t.HasLong != o.HasLong || (t.HasLong && t.Long != o.Long) {
		return false
	}
	return true
}

func (t Type) SameLattice(o Type) bool {
	if t.Kind != o.Kind {
		return false
	}
	if t.Kind == UninitNew {
		return t.NewPC == o.NewPC
	}
	if t.Kind == Ref {
		return t.Class == o.Class
	}
	return true
}

func (t Type) Width() int {
	if t.Kind.IsCat2Head() {
		return 2
	}
	if t.Kind.IsTail() {
		return 0
	}
	if t.Kind == Top {
		return 1
	}
	return 1
}

func (t Type) Computational() bool {
	return t.Kind >= Int && t.Kind <= UninitNew && !t.Kind.IsTail()
}

func (t Type) String() string {
	switch t.Kind {
	case Ref:
		if t.Class == "" {
			return "ref"
		}
		return "ref:" + t.Class
	case UninitNew:
		return fmt.Sprintf("uninit@%d", t.NewPC)
	case Float:
		if t.HasBits {
			return fmt.Sprintf("float:%#x", uint32(t.Bits))
		}
	case Double:
		if t.HasBits {
			return fmt.Sprintf("double:%#x", t.Bits)
		}
	case Int:
		if t.HasInt {
			return fmt.Sprintf("int:%d", t.Int)
		}
	case Long:
		if t.HasLong {
			return fmt.Sprintf("long:%d", t.Long)
		}
	}
	return t.Kind.String()
}

func (t Type) DropValue() Type {
	t.HasBits, t.Bits, t.HasInt, t.Int, t.HasLong, t.Long = false, 0, false, 0, false, 0
	return t
}

type Frame struct {
	Locals []Type
	Stack  []Type
	// Both locals and stack use expanded category-2 head/tail slots.
	// Locals never grow during transfer. A zero-value frame has the legacy
	// stack bound; NewFrameWithLimits also represents max_stack == 0.
	maxStack          int
	hasLimits         bool
	ThisUninitialized bool
	ThisClass         string
	DirectSuperClass  string
}

func NewFrame(nLocals int) Frame {
	if nLocals < 0 {
		nLocals = 0
	}
	locals := make([]Type, nLocals)
	for i := range locals {
		locals[i] = T(Top)
	}
	return Frame{Locals: locals, Stack: nil}
}

// NewFrameWithLimits retains the declared Code attribute limits.
func NewFrameWithLimits(maxLocals, maxStack int) (Frame, error) {
	if maxLocals < 0 || maxLocals > maxSlots || maxStack < 0 || maxStack > maxSlots {
		return Frame{}, invalidf("invalid frame limits %d/%d", maxLocals, maxStack)
	}
	f := NewFrame(maxLocals)
	f.maxStack, f.hasLimits = maxStack, true
	return f, nil
}

func (f Frame) stackLimit() int {
	if f.hasLimits {
		return f.maxStack
	}
	return maxSlots
}

func (f Frame) Clone() Frame {
	f.Locals = append([]Type(nil), f.Locals...)
	f.Stack = append([]Type(nil), f.Stack...)
	return f
}

func (f Frame) Equal(o Frame) bool {
	if len(f.Locals) != len(o.Locals) || len(f.Stack) != len(o.Stack) || f.stackLimit() != o.stackLimit() || f.ThisUninitialized != o.ThisUninitialized || f.ThisClass != o.ThisClass || f.DirectSuperClass != o.DirectSuperClass {
		return false
	}
	for i := range f.Locals {
		if !f.Locals[i].Equal(o.Locals[i]) {
			return false
		}
	}
	for i := range f.Stack {
		if !f.Stack[i].Equal(o.Stack[i]) {
			return false
		}
	}
	return true
}

func (f Frame) Canonical() string {
	s := "locals["
	for i, t := range f.Locals {
		if i > 0 {
			s += ","
		}
		s += t.String()
	}
	s += "] stack["
	for i, t := range f.Stack {
		if i > 0 {
			s += ","
		}
		s += t.String()
	}
	return s + "]"
}

const maxSlots = 65535

func (f *Frame) ensureLocal(idx int) error {
	if idx < 0 || idx >= len(f.Locals) || idx >= maxSlots {
		return invalidf("local index %d exceeds max_locals %d", idx, len(f.Locals))
	}
	return nil
}

func (f *Frame) invalidatePairAt(idx int) {
	if idx < 0 {
		return
	}
	if idx < len(f.Locals) {
		if f.Locals[idx].Kind.IsCat2Head() && idx+1 < len(f.Locals) {
			f.Locals[idx] = T(Top)
			f.Locals[idx+1] = T(Top)
		}
		if f.Locals[idx].Kind.IsTail() && idx-1 >= 0 {
			f.Locals[idx-1] = T(Top)
			f.Locals[idx] = T(Top)
		}
	}
	if idx-1 >= 0 && idx-1 < len(f.Locals) && f.Locals[idx-1].Kind.IsCat2Head() {
		f.Locals[idx-1] = T(Top)
		if idx < len(f.Locals) {
			f.Locals[idx] = T(Top)
		}
	}
}

func (f *Frame) StoreLocal(idx int, t Type) error { return f.storeLocal(idx, t) }

func (f *Frame) storeLocal(idx int, t Type) error {
	if !t.Computational() {
		return invalidf("invalid local value %s", t)
	}
	if idx < 0 || idx >= len(f.Locals) {
		return invalidf("local index %d", idx)
	}
	width := 1
	if t.Kind.IsCat2Head() {
		width = 2
	}
	if err := f.ensureLocal(idx + width - 1); err != nil {
		return err
	}
	f.invalidatePairAt(idx)
	if width == 2 {
		f.invalidatePairAt(idx + 1)
	} else {
		if idx+1 < len(f.Locals) && f.Locals[idx+1].Kind.IsTail() {
			f.Locals[idx+1] = T(Top)
		}
	}
	f.Locals[idx] = t
	if t.Kind == UninitThis {
		f.ThisUninitialized = true
	}
	if width == 2 {
		f.Locals[idx+1] = TailOf(t)
	}
	return nil
}

func (f *Frame) loadLocal(idx int, want Kind) (Type, error) {
	if err := f.ensureLocal(idx); err != nil {
		return Type{}, err
	}
	t := f.Locals[idx]
	if !t.Computational() {
		return Type{}, invalidf("local %d is %s", idx, t)
	}
	if want == Ref {
		if !referenceLike(t, true) {
			return Type{}, invalidf("local %d is %s, want ref", idx, t)
		}
	} else if t.Kind != want {
		return Type{}, invalidf("local %d is %s, want %s", idx, t, want)
	}
	if t.Kind.IsCat2Head() && (idx+1 >= len(f.Locals) || !f.Locals[idx+1].Equal(TailOf(t))) {
		return Type{}, invalidf("local %d has mismatched category-2 tail", idx)
	}
	return t, nil
}

func referenceLike(t Type, allowUninit bool) bool {
	return t.Kind == Ref || t.Kind == Null || allowUninit && (t.Kind == UninitThis || t.Kind == UninitNew)
}

func (f *Frame) push(t Type) error {
	if !t.Computational() {
		return invalidf("push of non-computational value %s", t)
	}
	if len(f.Stack)+t.Width() > f.stackLimit() {
		return invalidf("max_stack exceeded")
	}
	f.Stack = append(f.Stack, t)
	if t.Kind.IsCat2Head() {
		f.Stack = append(f.Stack, TailOf(t))
	}
	return nil
}

// Validate checks the representation and available bounds, not JVM class
// hierarchy, member accessibility, or return-descriptor assignability.
func (f Frame) Validate() error {
	if len(f.Locals) > maxSlots || len(f.Stack) > f.stackLimit() {
		return invalidf("frame bounds exceeded")
	}
	for _, part := range []struct {
		values []Type
		locals bool
	}{{f.Locals, true}, {f.Stack, false}} {
		for i := 0; i < len(part.values); i++ {
			t := part.values[i]
			if part.locals && t.Kind == Top {
				continue
			}
			if !t.Computational() {
				return invalidf("non-computational slot %d: %s", i, t)
			}
			if t.Kind.IsCat2Head() {
				if i+1 >= len(part.values) || !part.values[i+1].Equal(TailOf(t)) {
					return invalidf("mismatched category-2 slot %d", i)
				}
				i++
			}
		}
	}
	return nil
}

func (f *Frame) popValue() (Type, error) {
	if len(f.Stack) == 0 {
		return Type{}, invalidf("stack underflow")
	}
	top := f.Stack[len(f.Stack)-1]
	if top.Kind.IsTail() {
		if len(f.Stack) < 2 {
			return Type{}, invalidf("truncated category-2")
		}
		head := f.Stack[len(f.Stack)-2]
		if (top.Kind == LongTail && head.Kind != Long) || (top.Kind == DoubleTail && head.Kind != Double) {
			return Type{}, invalidf("mismatched category-2 tail")
		}
		f.Stack = f.Stack[:len(f.Stack)-2]
		return head, nil
	}
	if !top.Computational() || top.Kind.IsCat2Head() {
		return Type{}, invalidf("invalid computational stack top")
	}
	f.Stack = f.Stack[:len(f.Stack)-1]
	return top, nil
}

func (f *Frame) popKind(want Kind) (Type, error) {
	t, err := f.popValue()
	if err != nil {
		return Type{}, err
	}
	if want == Ref {
		switch t.Kind {
		case Ref, Null:
			return t, nil
		}
		return Type{}, invalidf("pop %s, want ref", t.Kind)
	}
	if t.Kind != want {
		return Type{}, invalidf("pop %s, want %s", t.Kind, want)
	}
	return t, nil
}

func (f *Frame) peek(n int) (Type, error) {
	if n < 0 || n >= len(f.Stack) {
		return Type{}, invalidf("stack underflow")
	}
	return f.Stack[len(f.Stack)-1-n], nil
}

func sameUninitialized(t, site Type) bool {
	return site.Kind == UninitThis && t.Kind == UninitThis || site.Kind == UninitNew && t.Kind == UninitNew && t.NewPC == site.NewPC
}

func (f *Frame) initialize(site Type, to Type) {
	for i, t := range f.Locals {
		if sameUninitialized(t, site) {
			f.Locals[i] = to
		}
	}
	for i, t := range f.Stack {
		if sameUninitialized(t, site) {
			f.Stack[i] = to
		}
	}
}

func JoinTypes(a, b Type) (Type, bool) {
	if a.Kind == Top {
		return T(Top), true
	}
	if b.Kind == Top {
		return T(Top), true
	}
	if a.Kind.IsTail() || b.Kind.IsTail() {
		if a.Kind == b.Kind {
			return a.DropValue(), true
		}
		return Type{}, false
	}
	if a.Width() != b.Width() {
		return Type{}, false
	}
	if a.Kind == Null && (b.Kind == Ref || b.Kind == Null) {
		return b.DropValue(), true
	}
	if b.Kind == Null && a.Kind == Ref {
		return a.DropValue(), true
	}
	if a.Kind == UninitNew && b.Kind == UninitNew && a.NewPC == b.NewPC {
		if a.Class != b.Class {
			return Type{}, false
		}
		return a.DropValue(), true
	}
	if a.Kind == UninitThis && b.Kind == UninitThis {
		return T(UninitThis), true
	}
	if a.Kind == Ref && b.Kind == Ref {
		if a.Class == b.Class {
			return RefOf(a.Class), true
		}
		return RefOf("java/lang/Object"), true
	}
	if a.Kind == b.Kind && a.Kind != UninitNew && a.Kind != UninitThis {
		out := a.DropValue()
		if a.HasBits && b.HasBits && a.Bits == b.Bits {
			out.HasBits, out.Bits = true, a.Bits
		}
		if a.HasInt && b.HasInt && a.Int == b.Int {
			out.HasInt, out.Int = true, a.Int
		}
		if a.HasLong && b.HasLong && a.Long == b.Long {
			out.HasLong, out.Long = true, a.Long
		}
		return out, true
	}
	if a.Width() == b.Width() {
		return T(Top), true
	}
	return Type{}, false
}

// JoinFrames forgets incompatible locals but never turns a computational
// stack value into TOP. Unknown predecessors must be omitted by the solver.
func JoinFrames(a, b Frame) (Frame, error) {
	if len(a.Stack) != len(b.Stack) {
		return Frame{}, invalidf("stack height mismatch %d vs %d", len(a.Stack), len(b.Stack))
	}
	if len(a.Locals) != len(b.Locals) || a.stackLimit() != b.stackLimit() || a.ThisClass != b.ThisClass || a.DirectSuperClass != b.DirectSuperClass {
		return Frame{}, invalidf("frame shape/context mismatch")
	}
	if err := a.Validate(); err != nil {
		return Frame{}, err
	}
	if err := b.Validate(); err != nil {
		return Frame{}, err
	}
	out := a.Clone()
	out.ThisUninitialized = a.ThisUninitialized || b.ThisUninitialized
	for i := range out.Locals {
		j, ok := JoinTypes(a.Locals[i], b.Locals[i])
		if !ok {
			j = T(Top)
		}
		out.Locals[i] = j
	}
	// A merge may kill one half of a pair; its other half is also unusable.
	for i, t := range out.Locals {
		if t.Kind.IsCat2Head() && (i+1 >= len(out.Locals) || !out.Locals[i+1].Equal(TailOf(t))) {
			out.Locals[i] = T(Top)
		}
		if t.Kind.IsTail() && (i == 0 || !t.Equal(TailOf(out.Locals[i-1]))) {
			out.Locals[i] = T(Top)
		}
	}
	for i := 0; i < len(a.Stack); i++ {
		j, ok := JoinTypes(a.Stack[i], b.Stack[i])
		if !ok || !j.Computational() {
			return Frame{}, invalidf("incompatible computational stack at %d: %s vs %s", i, a.Stack[i], b.Stack[i])
		}
		out.Stack[i] = j
		if j.Kind.IsCat2Head() {
			i++
			out.Stack[i] = TailOf(j)
		}
	}
	return out, nil
}

func Float64FromType(t Type) (float64, bool) {
	if t.Kind != Double || !t.HasBits {
		return 0, false
	}
	return math.Float64frombits(t.Bits), true
}

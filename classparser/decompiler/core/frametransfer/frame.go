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
	return !t.Kind.IsTail() && t.Kind != Top
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

func (f Frame) Clone() Frame {
	return Frame{
		Locals: append([]Type(nil), f.Locals...),
		Stack:  append([]Type(nil), f.Stack...),
	}
}

func (f Frame) Equal(o Frame) bool {
	if len(f.Locals) != len(o.Locals) || len(f.Stack) != len(o.Stack) {
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

const maxSlots = 65536

func (f *Frame) ensureLocal(idx int) error {
	if idx < 0 || idx >= maxSlots {
		return invalidf("local index %d", idx)
	}
	if idx >= len(f.Locals) {
		n := make([]Type, idx+1)
		copy(n, f.Locals)
		for i := len(f.Locals); i < len(n); i++ {
			n[i] = T(Top)
		}
		f.Locals = n
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
	if want == Long || want == Double {
		if err := f.ensureLocal(idx + 1); err != nil {
			return Type{}, err
		}
		if t.Kind != want || !f.Locals[idx+1].Kind.IsTail() {
			return Type{}, invalidf("local %d is %s, want %s pair", idx, t.Kind, want)
		}
		return t, nil
	}
	if t.Kind.IsCat2Head() || t.Kind.IsTail() {
		return Type{}, invalidf("local %d category-2 overlap load", idx)
	}
	if want != Top && t.Kind != want && !(want == Ref && (t.Kind == Ref || t.Kind == Null || t.Kind == UninitThis || t.Kind == UninitNew)) {
		if t.Kind == Top {
			return Type{}, invalidf("local %d is top", idx)
		}
		if want != Ref {
			return Type{}, invalidf("local %d is %s, want %s", idx, t.Kind, want)
		}
	}
	return t, nil
}

func (f *Frame) push(t Type) error {
	if t.Kind.IsCat2Head() {
		if len(f.Stack)+2 > maxSlots {
			return invalidf("stack overflow")
		}
		f.Stack = append(f.Stack, t, TailOf(t))
		return nil
	}
	if t.Kind.IsTail() {
		return invalidf("push of tail")
	}
	if len(f.Stack)+1 > maxSlots {
		return invalidf("stack overflow")
	}
	f.Stack = append(f.Stack, t)
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
	if top.Kind.IsCat2Head() {
		return Type{}, invalidf("category-2 head on top without tail")
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
		case Ref, Null, UninitThis, UninitNew:
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

func (f *Frame) initialize(site Type, to Type) {
	match := func(t Type) bool {
		if site.Kind == UninitThis {
			return t.Kind == UninitThis
		}
		return t.Kind == UninitNew && t.NewPC == site.NewPC
	}
	for i, t := range f.Locals {
		if match(t) {
			f.Locals[i] = to
		}
	}
	for i, t := range f.Stack {
		if match(t) {
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
		return UninitAt(a.NewPC), true
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

func JoinFrames(a, b Frame) (Frame, error) {
	if len(a.Stack) != len(b.Stack) {
		return Frame{}, invalidf("stack height mismatch %d vs %d", len(a.Stack), len(b.Stack))
	}
	nloc := len(a.Locals)
	if len(b.Locals) > nloc {
		nloc = len(b.Locals)
	}
	out := NewFrame(nloc)
	out.Stack = make([]Type, len(a.Stack))
	for i := 0; i < nloc; i++ {
		var ta, tb Type
		if i < len(a.Locals) {
			ta = a.Locals[i]
		} else {
			ta = T(Top)
		}
		if i < len(b.Locals) {
			tb = b.Locals[i]
		} else {
			tb = T(Top)
		}
		j, ok := JoinTypes(ta, tb)
		if !ok {
			return Frame{}, invalidf("local %d width/kind mismatch %s vs %s", i, ta, tb)
		}
		out.Locals[i] = j
	}
	for i := range a.Stack {
		j, ok := JoinTypes(a.Stack[i], b.Stack[i])
		if !ok {
			return Frame{}, invalidf("stack %d width/kind mismatch %s vs %s", i, a.Stack[i], b.Stack[i])
		}
		out.Stack[i] = j
	}
	return out, nil
}

func Float64FromType(t Type) (float64, bool) {
	if t.Kind != Double || !t.HasBits {
		return 0, false
	}
	return math.Float64frombits(t.Bits), true
}

package frametransfer

import (
	"encoding/binary"
	"fmt"
)

type SMTFrame struct {
	Offset uint16
	Locals []Type
	Stack  []Type
}

const (
	itemTop               = 0
	itemInteger           = 1
	itemFloat             = 2
	itemDouble            = 3
	itemLong              = 4
	itemNull              = 5
	itemUninitializedThis = 6
	itemObject            = 7
	itemUninitialized     = 8
)

func ParseStackMapTable(info []byte, resolveClass func(uint16) string) ([]SMTFrame, error) {
	if len(info) < 2 {
		return nil, smtf("truncated StackMapTable")
	}
	n := int(binary.BigEndian.Uint16(info[:2]))
	p := 2
	var frames []SMTFrame
	var prev SMTFrame
	offset := -1
	readU1 := func() (byte, error) {
		if p >= len(info) {
			return 0, smtf("truncated")
		}
		b := info[p]
		p++
		return b, nil
	}
	readU2 := func() (uint16, error) {
		if p+1 >= len(info) {
			return 0, smtf("truncated")
		}
		v := binary.BigEndian.Uint16(info[p:])
		p += 2
		return v, nil
	}
	readType := func() (Type, error) {
		tag, err := readU1()
		if err != nil {
			return Type{}, err
		}
		switch tag {
		case itemTop:
			return T(Top), nil
		case itemInteger:
			return T(Int), nil
		case itemFloat:
			return T(Float), nil
		case itemDouble:
			return T(Double), nil
		case itemLong:
			return T(Long), nil
		case itemNull:
			return T(Null), nil
		case itemUninitializedThis:
			return T(UninitThis), nil
		case itemObject:
			idx, err := readU2()
			if err != nil {
				return Type{}, err
			}
			name := "java/lang/Object"
			if resolveClass != nil {
				if s := resolveClass(idx); s != "" {
					name = s
				}
			}
			return RefOf(name), nil
		case itemUninitialized:
			pc, err := readU2()
			if err != nil {
				return Type{}, err
			}
			return UninitAt(pc), nil
		default:
			return Type{}, smtf("unknown verification type %d", tag)
		}
	}
	expand := func(types []Type) []Type {
		var out []Type
		for _, t := range types {
			out = append(out, t)
			if t.Kind.IsCat2Head() {
				out = append(out, TailOf(t))
			}
		}
		return out
	}
	for i := 0; i < n; i++ {
		ft, err := readU1()
		if err != nil {
			return nil, err
		}
		var delta int
		cur := SMTFrame{}
		switch {
		case ft <= 63:
			delta = int(ft)
			cur.Locals = append([]Type(nil), prev.Locals...)
			cur.Stack = nil
		case ft <= 127:
			delta = int(ft - 64)
			t, err := readType()
			if err != nil {
				return nil, err
			}
			cur.Locals = append([]Type(nil), prev.Locals...)
			cur.Stack = expand([]Type{t})
		case ft == 247:
			d, err := readU2()
			if err != nil {
				return nil, err
			}
			delta = int(d)
			t, err := readType()
			if err != nil {
				return nil, err
			}
			cur.Locals = append([]Type(nil), prev.Locals...)
			cur.Stack = expand([]Type{t})
		case ft >= 248 && ft <= 250:
			d, err := readU2()
			if err != nil {
				return nil, err
			}
			delta = int(d)
			chop := int(251 - ft)
			loc := append([]Type(nil), prev.Locals...)
			for c := 0; c < chop && len(loc) > 0; c++ {
				if loc[len(loc)-1].Kind.IsTail() && len(loc) >= 2 {
					loc = loc[:len(loc)-2]
				} else {
					loc = loc[:len(loc)-1]
				}
			}
			cur.Locals = loc
			cur.Stack = nil
		case ft == 251:
			d, err := readU2()
			if err != nil {
				return nil, err
			}
			delta = int(d)
			cur.Locals = append([]Type(nil), prev.Locals...)
			cur.Stack = nil
		case ft >= 252 && ft <= 254:
			d, err := readU2()
			if err != nil {
				return nil, err
			}
			delta = int(d)
			add := int(ft - 251)
			var extra []Type
			for a := 0; a < add; a++ {
				t, err := readType()
				if err != nil {
					return nil, err
				}
				extra = append(extra, t)
			}
			cur.Locals = append(append([]Type(nil), prev.Locals...), expand(extra)...)
			cur.Stack = nil
		case ft == 255:
			d, err := readU2()
			if err != nil {
				return nil, err
			}
			delta = int(d)
			nl, err := readU2()
			if err != nil {
				return nil, err
			}
			var loc []Type
			for j := 0; j < int(nl); j++ {
				t, err := readType()
				if err != nil {
					return nil, err
				}
				loc = append(loc, t)
			}
			ns, err := readU2()
			if err != nil {
				return nil, err
			}
			var st []Type
			for j := 0; j < int(ns); j++ {
				t, err := readType()
				if err != nil {
					return nil, err
				}
				st = append(st, t)
			}
			cur.Locals = expand(loc)
			cur.Stack = expand(st)
		default:
			return nil, smtf("reserved frame type %d", ft)
		}
		if i == 0 {
			offset = delta
		} else {
			offset += delta + 1
		}
		if offset < 0 || offset > 65535 {
			return nil, smtf("bad frame offset")
		}
		cur.Offset = uint16(offset)
		frames = append(frames, cur)
		prev = cur
	}
	return frames, nil
}

func CheckStackMapTable(major uint16, smt []byte, computed map[uint16]Frame, resolveClass func(uint16) string) error {
	if len(smt) == 0 {
		if major < 50 {
			return nil
		}
		return nil
	}
	frames, err := ParseStackMapTable(smt, resolveClass)
	if err != nil {
		return err
	}
	for _, sm := range frames {
		got, ok := computed[sm.Offset]
		if !ok {
			return smtf("no transfer frame at SMT offset %d", sm.Offset)
		}
		if err := framesCompatible(sm, got); err != nil {
			return fmt.Errorf("%w at offset %d", err, sm.Offset)
		}
	}
	return nil
}

func framesCompatible(smt SMTFrame, got Frame) error {
	if len(smt.Stack) != len(got.Stack) {
		return smtf("stack height SMT %d transfer %d", len(smt.Stack), len(got.Stack))
	}
	for i := range smt.Stack {
		if !smtTypeOK(smt.Stack[i], got.Stack[i]) {
			return smtf("stack[%d] SMT %s transfer %s", i, smt.Stack[i], got.Stack[i])
		}
	}
	n := len(smt.Locals)
	if len(got.Locals) < n {
		return smtf("locals shorter in transfer")
	}
	for i := 0; i < n; i++ {
		if !smtTypeOK(smt.Locals[i], got.Locals[i]) {
			return smtf("local[%d] SMT %s transfer %s", i, smt.Locals[i], got.Locals[i])
		}
	}
	return nil
}

func smtTypeOK(smt, got Type) bool {
	if smt.Kind == Top {
		return true
	}
	if smt.Kind == got.Kind {
		if smt.Kind == UninitNew {
			return smt.NewPC == got.NewPC
		}
		return true
	}
	if smt.Kind == Ref && (got.Kind == Ref || got.Kind == Null) {
		return true
	}
	if smt.Kind == Null && (got.Kind == Null || got.Kind == Ref) {
		return true
	}
	return false
}

func EncodeFullFrame(offsetDelta uint16, locals, stack []Type) []byte {
	var body []byte
	u2 := func(v uint16) { body = append(body, byte(v>>8), byte(v)) }
	u1 := func(v byte) { body = append(body, v) }
	write := func(t Type) {
		switch t.Kind {
		case Top:
			u1(itemTop)
		case Int:
			u1(itemInteger)
		case Float:
			u1(itemFloat)
		case Long:
			u1(itemLong)
		case Double:
			u1(itemDouble)
		case Null:
			u1(itemNull)
		case UninitThis:
			u1(itemUninitializedThis)
		case UninitNew:
			u1(itemUninitialized)
			u2(t.NewPC)
		case Ref:
			u1(itemObject)
			u2(1)
		default:
			u1(itemTop)
		}
	}
	compact := func(ts []Type) []Type {
		var out []Type
		for i := 0; i < len(ts); i++ {
			if ts[i].Kind.IsTail() {
				continue
			}
			out = append(out, ts[i])
		}
		return out
	}
	cl, cs := compact(locals), compact(stack)
	u1(255)
	u2(offsetDelta)
	u2(uint16(len(cl)))
	for _, t := range cl {
		write(t)
	}
	u2(uint16(len(cs)))
	for _, t := range cs {
		write(t)
	}
	n := uint16(1)
	out := []byte{byte(n >> 8), byte(n)}
	return append(out, body...)
}

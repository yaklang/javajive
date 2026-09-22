package methodir

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

func CombineHashes(prev, next string) string {
	if prev == "" {
		return next
	}
	if next == "" {
		return prev
	}
	sum := sha256.Sum256([]byte(prev + "\n" + next))
	return hex.EncodeToString(sum[:])
}

func (m *MethodIR) Canonical() string {
	if m == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "version %d\n", m.Version)
	fmt.Fprintf(&b, "method %s\n", m.ID)
	fmt.Fprintf(&b, "static %t\n", m.IsStatic)
	fmt.Fprintf(&b, "limits %t %d %d %q\n", m.Limits.Present, m.Limits.MaxLocals, m.Limits.MaxStack, m.Limits.DirectSuperClass)
	blocks := append([]Block(nil), m.Blocks...)
	sort.Slice(blocks, func(i, j int) bool { return blocks[i].FirstPC < blocks[j].FirstPC })
	for _, bl := range blocks {
		fmt.Fprintf(&b, "block %d first=%d", bl.ID, bl.FirstPC)
		for _, id := range bl.InstrIDs {
			fmt.Fprintf(&b, " %d", id)
		}
		b.WriteByte('\n')
	}
	instrs := append([]Instr(nil), m.Instrs...)
	sort.Slice(instrs, func(i, j int) bool { return instrs[i].PC < instrs[j].PC })
	for _, in := range instrs {
		fmt.Fprintf(&b, "instr pc=%d op=%d name=%s wide=%t local=%d iinc=%d cp=%d class=%s member=%s desc=%s maythrow=%t result=%d data=%s\n",
			in.PC, in.Opcode, in.Name, in.Wide, in.Local, in.IincConst, in.CPIndex, in.Class, in.Member, in.Desc, in.MayThrow, in.Result, hex.EncodeToString(in.Data))
		if in.Const.Kind != ConstNone {
			fmt.Fprintf(&b, "  const kind=%d i=%d l=%d f=%d d=%d s=%q c=%s\n",
				in.Const.Kind, in.Const.Int, in.Const.Long, in.Const.FloatBits, in.Const.DoubleBits, in.Const.String, in.Const.Class)
		}
		cases := append([]SwitchCase(nil), in.Cases...)
		sort.Slice(cases, func(i, j int) bool { return cases[i].Key < cases[j].Key })
		for _, c := range cases {
			fmt.Fprintf(&b, "  case %d -> %d\n", c.Key, c.TargetPC)
		}
		if in.DefaultPC != 0 || len(in.Cases) > 0 {
			fmt.Fprintf(&b, "  default %d\n", in.DefaultPC)
		}
	}
	edges := append([]Edge(nil), m.Edges...)
	sort.Slice(edges, func(i, j int) bool {
		a, b2 := edges[i], edges[j]
		if a.From != b2.From {
			return a.From < b2.From
		}
		if a.To != b2.To {
			return a.To < b2.To
		}
		if a.Kind != b2.Kind {
			return a.Kind < b2.Kind
		}
		if a.CaseValue != b2.CaseValue {
			return a.CaseValue < b2.CaseValue
		}
		return a.HandlerOrder < b2.HandlerOrder
	})
	for _, e := range edges {
		fmt.Fprintf(&b, "edge %s catch=%d\n", e.ID.String(), e.CatchType)
	}
	handlers := append([]Handler(nil), m.Handlers...)
	sort.Slice(handlers, func(i, j int) bool { return handlers[i].Order < handlers[j].Order })
	for _, h := range handlers {
		fmt.Fprintf(&b, "handler %d [%d,%d)->%d type=%d\n", h.Order, h.StartPC, h.EndPC, h.HandlerPC, h.CatchType)
	}
	vals := append([]Value(nil), m.Values...)
	sort.Slice(vals, func(i, j int) bool { return vals[i].ID < vals[j].ID })
	for _, v := range vals {
		fmt.Fprintf(&b, "value %d kind=%s pc=%d slot=%d param=%d\n", v.ID, v.Kind, v.PC, v.Slot, v.Param)
	}
	return b.String()
}

func (m *MethodIR) Hash() string {
	sum := sha256.Sum256([]byte(m.Canonical()))
	return hex.EncodeToString(sum[:])
}

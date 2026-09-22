package methodir

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sort"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

func BuildFromRequest(req core.ShadowIRRequest) (string, uint64, error) {
	meta := MethodMeta{
		Limits:     req.Limits,
		ClassName:  req.ClassName,
		Name:       req.MethodName,
		Descriptor: req.Descriptor,
		Bytecode:   append([]byte(nil), req.Bytecode...),
		IsStatic:   req.IsStatic,
	}
	ir, err := BuildFromCFG(req.CFG, meta, req.D)
	if err != nil {
		return "", 0, err
	}
	return ir.Hash(), ir.Version, nil
}

// BuildFromCFG copies an immutable MethodIR out of a SemanticCFG snapshot.
// d may be nil; when present it is used only to resolve CP names and exception tables.
func BuildFromCFG(cfg *core.SemanticCFG, meta MethodMeta, d ...*core.Decompiler) (*MethodIR, error) {
	if cfg == nil {
		return nil, errInvalid(0, 0, "nil semantic CFG")
	}
	var dec *core.Decompiler
	if len(d) > 0 {
		dec = d[0]
	}
	for _, n := range cfg.Nodes {
		if n == nil || n.Instr == nil {
			return nil, errInvalid(0, 0, "nil opcode in CFG")
		}
		switch n.Instr.OpCode {
		case core.OP_JSR, core.OP_JSR_W, core.OP_RET:
			return nil, errUnsupported(n.CurrentOffset, n.Instr.OpCode, "jsr/ret not inlined")
		}
		if _, ok := core.InstrInfos[n.Instr.OpCode]; !ok && n.Instr.OpCode != core.OP_START && n.Instr.OpCode != core.OP_END {
			return nil, errInvalid(n.CurrentOffset, n.Instr.OpCode, "illegal or unknown instruction")
		}
	}

	bc := append([]byte(nil), meta.Bytecode...)
	sum := sha256.Sum256(bc)
	id := MethodID(meta.ClassName + "\x1f" + meta.Name + "\x1f" + meta.Descriptor + "\x1f" + hex.EncodeToString(sum[:]))

	ir := &MethodIR{
		Limits:     meta.Limits,
		ID:         id,
		Version:    SnapshotVersion,
		ClassName:  meta.ClassName,
		Name:       meta.Name,
		Descriptor: meta.Descriptor,
		IsStatic:   meta.IsStatic,
		Bytecode:   bc,
		instrIndex: map[InstrID]int{},
	}

	if dec != nil {
		for i, h := range dec.ExceptionTable {
			if h == nil {
				continue
			}
			ir.Handlers = append(ir.Handlers, Handler{
				Order: i, StartPC: h.StartPc, EndPC: h.EndPc, HandlerPC: h.HandlerPc, CatchType: h.CatchType,
			})
		}
	}

	nodes := append([]*core.OpCode(nil), cfg.Nodes...)
	sort.SliceStable(nodes, func(i, j int) bool { return nodes[i].CurrentOffset < nodes[j].CurrentOffset })

	leaders := map[uint16]bool{}
	if len(nodes) > 0 {
		leaders[nodes[0].CurrentOffset] = true
		ir.EntryPC = nodes[0].CurrentOffset
	}
	for _, e := range cfg.Edges {
		if e.To != nil {
			leaders[e.To.CurrentOffset] = true
		}
	}
	for i, n := range nodes {
		if !fallsThrough(n) && i+1 < len(nodes) {
			leaders[nodes[i+1].CurrentOffset] = true
		}
	}

	type pendingBlock struct {
		first uint16
		pcs   []InstrID
	}
	var pending []pendingBlock
	var cur pendingBlock
	for i, n := range nodes {
		pc := n.CurrentOffset
		if i == 0 || leaders[pc] {
			if i != 0 {
				pending = append(pending, cur)
			}
			cur = pendingBlock{first: pc}
		}
		cur.pcs = append(cur.pcs, InstrID(pc))
	}
	if len(nodes) > 0 {
		pending = append(pending, cur)
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].first < pending[j].first })
	for i, p := range pending {
		ids := append([]InstrID(nil), p.pcs...)
		ir.Blocks = append(ir.Blocks, Block{ID: BlockID(i), FirstPC: p.first, InstrIDs: ids})
	}

	nextVal := ValueID(1)
	if !meta.IsStatic {
		ir.Values = append(ir.Values, Value{ID: nextVal, Kind: "param", Param: 0, Slot: 0})
		nextVal++
	}
	paramSlots := descriptorParamSlots(meta.Descriptor)
	slot := 0
	if !meta.IsStatic {
		slot = 1
	}
	for i, width := range paramSlots {
		ir.Values = append(ir.Values, Value{ID: nextVal, Kind: "param", Param: i + 1, Slot: slot})
		nextVal++
		slot += width
	}

	for _, n := range nodes {
		in, err := copyInstr(n, dec)
		if err != nil {
			return nil, err
		}
		in.Result = nextVal
		ir.Values = append(ir.Values, Value{ID: nextVal, Kind: "instr", PC: in.PC})
		nextVal++
		ir.instrIndex[in.ID] = len(ir.Instrs)
		ir.Instrs = append(ir.Instrs, in)
	}

	seen := map[EdgeID]int{}
	for _, e := range cfg.Edges {
		if e.From == nil || e.To == nil {
			continue
		}
		id := EdgeID{
			From:         InstrID(e.From.CurrentOffset),
			To:           InstrID(e.To.CurrentOffset),
			Kind:         e.Kind,
			CaseValue:    e.CaseValue,
			HandlerOrder: e.HandlerOrder,
		}
		catch := uint16(0)
		if e.Kind == core.EdgeException && e.HandlerOrder >= 0 && e.HandlerOrder < len(ir.Handlers) {
			catch = ir.Handlers[e.HandlerOrder].CatchType
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = len(ir.Edges)
		ir.Edges = append(ir.Edges, Edge{
			ID: id, From: id.From, To: id.To, Kind: e.Kind,
			CaseValue: e.CaseValue, HandlerOrder: e.HandlerOrder, CatchType: catch,
		})
	}
	sort.SliceStable(ir.Edges, func(i, j int) bool {
		a, b := ir.Edges[i], ir.Edges[j]
		if a.From != b.From {
			return a.From < b.From
		}
		if a.To != b.To {
			return a.To < b.To
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.CaseValue != b.CaseValue {
			return a.CaseValue < b.CaseValue
		}
		return a.HandlerOrder < b.HandlerOrder
	})
	return ir, nil
}

func fallsThrough(n *core.OpCode) bool {
	if n == nil || n.Instr == nil {
		return false
	}
	switch n.Instr.OpCode {
	case core.OP_RETURN, core.OP_IRETURN, core.OP_LRETURN, core.OP_FRETURN, core.OP_DRETURN, core.OP_ARETURN, core.OP_ATHROW,
		core.OP_GOTO, core.OP_GOTO_W, core.OP_LOOKUPSWITCH, core.OP_TABLESWITCH, core.OP_JSR, core.OP_JSR_W, core.OP_RET:
		return false
	}
	return true
}

func copyInstr(n *core.OpCode, d *core.Decompiler) (Instr, error) {
	in := Instr{
		ID:       InstrID(n.CurrentOffset),
		PC:       n.CurrentOffset,
		Opcode:   n.Instr.OpCode,
		Name:     n.Instr.Name,
		Wide:     n.IsWide,
		Data:     append([]byte(nil), n.Data...),
		Local:    -1,
		MayThrow: core.MayThrowOpcode(n.Instr.OpCode),
	}
	acc := core.LocalAccessOf(n.Instr.OpCode)
	if acc.Read || acc.Write {
		if acc.Write {
			in.Local = core.GetStoreIdx(n)
		} else {
			in.Local = core.GetRetrieveIdx(n)
		}
	}
	if n.Instr.OpCode == core.OP_IINC {
		in.Local = core.GetStoreIdx(n)
		in.IincConst = iincConst(n)
	}
	switch n.Instr.OpCode {
	case core.OP_LDC, core.OP_LDC_W, core.OP_LDC2_W,
		core.OP_GETFIELD, core.OP_PUTFIELD, core.OP_GETSTATIC, core.OP_PUTSTATIC,
		core.OP_INVOKEVIRTUAL, core.OP_INVOKESPECIAL, core.OP_INVOKESTATIC, core.OP_INVOKEINTERFACE, core.OP_INVOKEDYNAMIC,
		core.OP_NEW, core.OP_ANEWARRAY, core.OP_CHECKCAST, core.OP_INSTANCEOF, core.OP_MULTIANEWARRAY:
		in.CPIndex = cpIndex(n)
		class, member, desc, cnst := decodeCP(d, int(in.CPIndex))
		in.Class, in.Member, in.Desc, in.Const = class, member, desc, cnst
		if n.Instr.OpCode == core.OP_NEW || n.Instr.OpCode == core.OP_ANEWARRAY || n.Instr.OpCode == core.OP_CHECKCAST || n.Instr.OpCode == core.OP_INSTANCEOF {
			if in.Class == "" && cnst.Class != "" {
				in.Class = cnst.Class
			}
		}
	}
	if n.Instr.OpCode == core.OP_LOOKUPSWITCH || n.Instr.OpCode == core.OP_TABLESWITCH {
		if n.SwitchJmpCase != nil {
			n.SwitchJmpCase.ForEach(func(k int, pc int32) bool {
				in.Cases = append(in.Cases, SwitchCase{Key: int32(k), TargetPC: uint16(pc)})
				return true
			})
			sort.Slice(in.Cases, func(i, j int) bool { return in.Cases[i].Key < in.Cases[j].Key })
		}
		in.DefaultPC = n.SwitchDefaultOffset
	}
	return in, nil
}

func iincConst(op *core.OpCode) int32 {
	if op.IsWide {
		if len(op.Data) >= 4 {
			return int32(int16(binary.BigEndian.Uint16(op.Data[2:])))
		}
		return 0
	}
	if len(op.Data) >= 2 {
		return int32(int8(op.Data[1]))
	}
	return 0
}

func descriptorParamSlots(desc string) []int {
	if desc == "" || desc[0] != '(' {
		return nil
	}
	end := -1
	for i := 1; i < len(desc); i++ {
		if desc[i] == ')' {
			end = i
			break
		}
	}
	if end < 0 {
		return nil
	}
	var widths []int
	for i := 1; i < end; {
		switch desc[i] {
		case 'J', 'D':
			widths = append(widths, 2)
			i++
		case 'B', 'C', 'F', 'I', 'S', 'Z':
			widths = append(widths, 1)
			i++
		case '[':
			for i < end && desc[i] == '[' {
				i++
			}
			if i < end && desc[i] == 'L' {
				for i < end && desc[i] != ';' {
					i++
				}
				if i < end {
					i++
				}
			} else if i < end {
				i++
			}
			widths = append(widths, 1)
		case 'L':
			for i < end && desc[i] != ';' {
				i++
			}
			if i < end {
				i++
			}
			widths = append(widths, 1)
		default:
			i++
		}
	}
	return widths
}

// BuildFromBytes parses opcodes, snapshots the CFG, and builds MethodIR.
func BuildFromBytes(code []byte, exceptions []*core.ExceptionTableEntry, meta MethodMeta, d *core.Decompiler) (*MethodIR, error) {
	dec := d
	if dec == nil {
		dec = core.NewDecompiler(code, nil)
	}
	if exceptions != nil {
		dec.ExceptionTable = exceptions
	}
	if err := dec.ParseOpcode(); err != nil {
		msg := err.Error()
		if containsFold(msg, "unknow op") || containsFold(msg, "unknown") || containsFold(msg, "illegal") || containsFold(msg, "invalid") {
			return nil, errInvalid(0, 0, msg)
		}
		return nil, err
	}
	cfg, err := core.SnapshotSemanticCFG(dec)
	if err != nil {
		if isJSR(err) {
			return nil, errUnsupported(0, core.OP_JSR, err.Error())
		}
		return nil, err
	}
	if meta.Bytecode == nil {
		meta.Bytecode = append([]byte(nil), code...)
	}
	return BuildFromCFG(cfg, meta, dec)
}

func isJSR(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return containsFold(s, "jsr")
}

func containsFold(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(s) > 0 && (indexFold(s, sub) >= 0)))
}

func indexFold(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		ok := true
		for j := 0; j < len(sub); j++ {
			a, b := s[i+j], sub[j]
			if a >= 'A' && a <= 'Z' {
				a += 'a' - 'A'
			}
			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
			}
			if a != b {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

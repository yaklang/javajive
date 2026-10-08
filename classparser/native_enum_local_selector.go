package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/internal/workbudget"
)

type nativeEnumLocalRead struct {
	pc, opcode, slot, storePC int
	descriptor                string
}

// Walk backward from this LOAD, stopping at the first overlapping normal
// write on each reachable path. Exception edges carry their source's input
// state, so they do not consume that source's write. A reachable entry without
// a write, or two distinct frontier writes, cannot certify one declaration.
// Unlike a physical prefix scan, this handles disjoint exits and backedges.
func (f *nativeEnumParameterFlow) reachingStore(read *nativeEnumSelectorProducer, work *workbudget.Budget) (int, bool) {
	if f == nil || f.entry == nil || len(f.byPC) > 8192 || read == nil || read.owner != "" || read.slot < 0 || read.slot > 65535 || !nativeProofWork(work, 1) || work != nil && work.CheckAlloc(int64(len(f.byPC))*192) != nil {
		return 0, false
	}
	at := f.byPC[read.pc]
	if at == nil || at.Instr == nil || at.Instr.OpCode != read.opcode || core.GetRetrieveIdx(at) != read.slot || !constructorMotionLoad(at, read.result) {
		return 0, false
	}
	width := 1
	if read.result == "J" || read.result == "D" {
		width = 2
	}
	reachable := map[*core.OpCode]bool{f.entry: true}
	pending := []*core.OpCode{f.entry}
	reverse := map[*core.OpCode][]core.SemanticEdge{}
	edgeCount := int64(0)
	for len(pending) > 0 {
		from := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if from == nil || from.Instr == nil || f.byPC[int(from.CurrentOffset)] != from || !nativeProofWork(work, 1) {
			return 0, false
		}
		for _, edge := range f.edges[from] {
			if edge.From != from || edge.To == nil || edge.To.Instr == nil || f.byPC[int(edge.To.CurrentOffset)] != edge.To || !nativeProofWork(work, 1) {
				return 0, false
			}
			edgeCount++
			if work != nil && work.CheckAlloc(int64(len(f.byPC))*192+edgeCount*64) != nil {
				return 0, false
			}
			reverse[edge.To] = append(reverse[edge.To], edge)
			if !reachable[edge.To] {
				reachable[edge.To] = true
				pending = append(pending, edge.To)
			}
		}
	}
	if !reachable[at] {
		return 0, false
	}
	seen := map[*core.OpCode]bool{at: true}
	pending = []*core.OpCode{at}
	store := -1
	for len(pending) > 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if current == f.entry || !nativeProofWork(work, 1) {
			return 0, false
		}
		for _, edge := range reverse[current] {
			if !nativeProofWork(work, 1) {
				return 0, false
			}
			access := core.LocalAccessOf(edge.From.Instr.OpCode)
			if access.Write && edge.Kind != core.EdgeException {
				slot := core.GetStoreIdx(edge.From)
				if slot < 0 || access.Width < 1 || access.Width > 2 {
					return 0, false
				}
				if slot < read.slot+width && read.slot < slot+access.Width {
					pc := int(edge.From.CurrentOffset)
					if store >= 0 && store != pc {
						return 0, false
					}
					store = pc
					continue
				}
			}
			if !seen[edge.From] {
				seen[edge.From] = true
				pending = append(pending, edge.From)
			}
		}
	}
	return store, store >= 0
}

// Original typed frames give the computational reference and value origin at
// both the STORE and LOAD. They do not infer a type from source spelling, debug
// tables or a later consumer. Source publication separately requires that very
// decoded STORE declaration and its unchanged seed; no producer is inlined.
func (c *ClassObjectDumper) nativeEnumLocalSelectorReads(method *MemberInfo, code *CodeAttribute, flow *nativeEnumParameterFlow, params map[int]string) (map[int]*nativeEnumLocalRead, bool) {
	return c.nativeTypedLocalReads(method, code, flow, params, false)
}

// The same original STORE/LOAD identity proof admits canonical computational
// words for local captures. Verifier int is not a narrow source descriptor:
// Z/B/C/S still require a separate range/declaration certificate.
func (c *ClassObjectDumper) nativeTypedLocalReads(method *MemberInfo, code *CodeAttribute, flow *nativeEnumParameterFlow, params map[int]string, primitiveWords bool) (map[int]*nativeEnumLocalRead, bool) {
	if c == nil || c.obj == nil || method == nil || code == nil || flow == nil || flow.code != code || len(c.obj.Methods) > 65535 || len(method.Attributes) > 65535 {
		return nil, false
	}
	matches, bodies := 0, 0
	for _, original := range c.obj.Methods {
		if !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		if original == method {
			matches++
		}
	}
	for _, attribute := range method.Attributes {
		if !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		if body, ok := attribute.(*CodeAttribute); ok {
			if body != code {
				return nil, false
			}
			bodies++
		}
	}
	if matches != 1 || bodies != 1 {
		return nil, false
	}
	desc, known := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
	arguments, _, err := callbinding.Descriptor(desc)
	if !known || err != nil || !nativeProofWork(c.Work, int64(len(arguments))+1) || c.Work != nil && c.Work.CheckAlloc(int64(len(arguments)+1)*64) != nil {
		return nil, false
	}
	expected := map[int]string{}
	slot := 0
	if method.AccessFlags&8 == 0 {
		expected[0] = "L" + c.obj.GetClassName() + ";"
		slot = 1
	}
	for _, argument := range arguments {
		expected[slot] = argument
		slot++
		if argument == "J" || argument == "D" {
			slot++
		}
	}
	if len(params) != len(expected) {
		return nil, false
	}
	for slot, descriptor := range expected {
		if params[slot] != descriptor {
			return nil, false
		}
	}
	ir, frames, known := c.nativeOriginalMethodSnapshot(method, code)
	if !known || !nativeProofWork(c.Work, int64(len(frames.Instructions))) || c.Work != nil && c.Work.CheckAlloc(int64(len(frames.Instructions))*128) != nil {
		return nil, false
	}
	records := map[int]ssabuild.InstructionValues{}
	for _, record := range frames.Instructions {
		if _, duplicate := records[int(record.PC)]; duplicate {
			return nil, false
		}
		records[int(record.PC)] = record
	}
	result := map[int]*nativeEnumLocalRead{}
	for _, record := range frames.Instructions {
		instruction, found := ir.InstrByID(methodir.InstrID(record.PC))
		if !found || !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		access := core.LocalAccessOf(instruction.Opcode)
		slot := instruction.Local
		if !access.Read || access.Write || access.Width < 1 || access.Width > 2 || slot < 0 || params[slot] != "" || slot >= len(record.Before.Locals) || len(record.BeforeOrigins) != len(record.Before.Locals)+len(record.Before.Stack) {
			continue
		}
		word := record.Before.Locals[slot]
		origin := record.BeforeOrigins[slot]
		if origin.Kind != ssabuild.OriginInstr && origin.Kind != ssabuild.OriginParam {
			continue
		}
		descriptor := ""
		if word.Kind == frametransfer.Ref && word.Class != "" {
			descriptor = word.Class
			if !strings.HasPrefix(descriptor, "[") {
				descriptor = "L" + descriptor + ";"
			}
		} else if primitiveWords {
			switch word.Kind {
			case frametransfer.Int:
				descriptor = "I"
			case frametransfer.Float:
				descriptor = "F"
			case frametransfer.Long:
				descriptor = "J"
			case frametransfer.Double:
				descriptor = "D"
			}
		}
		width := word.Width()
		if descriptor == "" || access.Width != width || slot+width > len(record.Before.Locals) {
			continue
		}
		if width == 2 && (record.Before.Locals[slot+1].Kind != frametransfer.TailOf(word).Kind || record.BeforeOrigins[slot+1] != origin) {
			continue
		}
		_, parsed, err := callbinding.Descriptor("()" + descriptor)
		if err != nil || parsed != descriptor {
			continue
		}
		read := &nativeEnumSelectorProducer{opcode: instruction.Opcode, pc: int(record.PC), slot: slot, result: descriptor}
		storePC, known := flow.reachingStore(read, c.Work)
		if !known {
			continue
		}
		stored, found := records[storePC]
		store, storeKnown := ir.InstrByID(methodir.InstrID(storePC))
		if !found || !storeKnown || !core.LocalAccessOf(store.Opcode).Write || store.Local != slot || len(stored.Uses) != 1 || stored.Uses[0] != origin || len(stored.Before.Stack) < width || len(stored.BeforeOrigins) != len(stored.Before.Locals)+len(stored.Before.Stack) {
			continue
		}
		// The initialized reference or complete computational word reaches
		// this STORE and LOAD unchanged. Both halves of category-2 values
		// must retain their type and identical origin; tails are never values.
		stackIndex := len(stored.Before.Stack) - width
		value := stored.Before.Stack[stackIndex]
		originIndex := len(stored.Before.Locals) + stackIndex
		if value.Kind != word.Kind || value.Class != word.Class || core.LocalAccessOf(store.Opcode).Width != width || stored.BeforeOrigins[originIndex] != origin {
			continue
		}
		if width == 2 && (stored.Before.Stack[stackIndex+1].Kind != frametransfer.TailOf(value).Kind || stored.BeforeOrigins[originIndex+1] != origin) {
			continue
		}
		if c.Work != nil && c.Work.CheckAlloc(int64(len(result)+1)*128) != nil {
			return nil, false
		}
		result[int(record.PC)] = &nativeEnumLocalRead{pc: int(record.PC), opcode: instruction.Opcode, slot: slot, storePC: storePC, descriptor: descriptor}
	}
	return result, true
}

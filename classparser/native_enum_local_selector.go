package javaclassparser

import (
	"sort"
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
	// Multiple original definitions belong only to the lambda capture proof.
	// Enum selector publication still requires the single storePC witness.
	storePCs []int
	// Only the original lambda descriptor may select a reference domain.
	// Every reaching producer must separately prove assignment to it.
	referenceAssignable bool
}

type nativeLocalReferenceDomain struct {
	descriptors map[int]string // original capture LOAD PC -> factory descriptor
	metadata    callbinding.Provider
}

// Walk backward from this LOAD, stopping at the first overlapping normal
// write on each reachable path. Exception edges carry their source's input
// state, so they do not consume that source's write. A reachable entry without
// a write, or two distinct frontier writes, cannot certify one declaration.
// Unlike a physical prefix scan, this handles disjoint exits and backedges.
func (f *nativeEnumParameterFlow) reachingStore(read *nativeEnumSelectorProducer, work *workbudget.Budget) (int, bool) {
	stores, known := f.reachingStores(read, work)
	if !known || len(stores) != 1 {
		return 0, false
	}
	return stores[0], true
}

// Complete last-writer frontier, with no uninitialized entry predecessor.
// This proves original definitions, not their final source declaration.
func (f *nativeEnumParameterFlow) reachingStores(read *nativeEnumSelectorProducer, work *workbudget.Budget) ([]int, bool) {
	return f.reachingLocalStores(read, work, false)
}

func (f *nativeEnumParameterFlow) reachingLocalStores(read *nativeEnumSelectorProducer, work *workbudget.Budget, readWrite bool) ([]int, bool) {
	if f == nil || f.entry == nil || len(f.byPC) > 8192 || read == nil || read.owner != "" || read.slot < 0 || read.slot > 65535 || !nativeProofWork(work, 1) || work != nil && work.CheckAlloc(int64(len(f.byPC))*192) != nil {
		return nil, false
	}
	at := f.byPC[read.pc]
	if at == nil || at.Instr == nil || at.Instr.OpCode != read.opcode {
		return nil, false
	}
	if readWrite {
		access := core.LocalAccessOf(at.Instr.OpCode)
		if !access.Read || !access.Write || access.Width != 1 || at.Instr.OpCode != core.OP_IINC || core.GetStoreIdx(at) != read.slot {
			return nil, false
		}
	} else if core.GetRetrieveIdx(at) != read.slot || !constructorMotionLoad(at, read.result) {
		return nil, false
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
			return nil, false
		}
		for _, edge := range f.edges[from] {
			if edge.From != from || edge.To == nil || edge.To.Instr == nil || f.byPC[int(edge.To.CurrentOffset)] != edge.To || !nativeProofWork(work, 1) {
				return nil, false
			}
			edgeCount++
			if work != nil && work.CheckAlloc(int64(len(f.byPC))*192+edgeCount*64) != nil {
				return nil, false
			}
			reverse[edge.To] = append(reverse[edge.To], edge)
			if !reachable[edge.To] {
				reachable[edge.To] = true
				pending = append(pending, edge.To)
			}
		}
	}
	if !reachable[at] {
		return nil, false
	}
	seen := map[*core.OpCode]bool{at: true}
	pending = []*core.OpCode{at}
	stores := map[int]bool{}
	for len(pending) > 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if current == f.entry || !nativeProofWork(work, 1) {
			return nil, false
		}
		for _, edge := range reverse[current] {
			if !nativeProofWork(work, 1) {
				return nil, false
			}
			access := core.LocalAccessOf(edge.From.Instr.OpCode)
			if access.Write && edge.Kind != core.EdgeException {
				slot := core.GetStoreIdx(edge.From)
				if slot < 0 || access.Width < 1 || access.Width > 2 {
					return nil, false
				}
				if slot < read.slot+width && read.slot < slot+access.Width {
					pc := int(edge.From.CurrentOffset)
					stores[pc] = true
					if len(stores) > 64 {
						return nil, false
					}
					continue
				}
			}
			if !seen[edge.From] {
				seen[edge.From] = true
				pending = append(pending, edge.From)
			}
		}
	}
	result := make([]int, 0, len(stores))
	for pc := range stores {
		result = append(result, pc)
	}
	sort.Ints(result)
	return result, len(result) > 0
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
func (c *ClassObjectDumper) nativeTypedLocalReads(method *MemberInfo, code *CodeAttribute, flow *nativeEnumParameterFlow, params map[int]string, primitiveWords bool, domains ...nativeLocalReferenceDomain) (map[int]*nativeEnumLocalRead, bool) {
	if len(domains) > 1 || len(domains) == 1 && (!primitiveWords || len(domains[0].descriptors) > 64) || c == nil || c.obj == nil || method == nil || code == nil || flow == nil || flow.code != code || len(c.obj.Methods) > 65535 || len(method.Attributes) > 65535 {
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
	var referenceWords *nativeConstructorFrameWords
	var widening *constructorWideningQuery
	if len(domains) == 1 && len(domains[0].descriptors) > 0 {
		widening = newConstructorWideningQuery(domains[0].metadata)
	}
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
		// A lambda word can join stack producers before one STORE, or join
		// several original STOREs. The complete last-writer frontier and every
		// STORE's initialized typed word are checked below. Source publication
		// separately requires the retained definition and path certificate.
		// Enum selector publication keeps its older single-origin boundary.
		if origin.Kind != ssabuild.OriginInstr && origin.Kind != ssabuild.OriginParam && !(primitiveWords && origin.Kind == ssabuild.OriginPhi) {
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
		referenceAssignable := false
		if len(domains) == 1 && word.Kind == frametransfer.Ref && callbinding.Reference(domains[0].descriptors[int(record.PC)]) {
			descriptor = domains[0].descriptors[int(record.PC)]
			referenceAssignable = true
		}
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
		storePCs, known := flow.reachingStores(read, c.Work)
		if !known || !primitiveWords && len(storePCs) != 1 {
			continue
		}
		valid := true
		for _, storePC := range storePCs {
			stored, found := records[storePC]
			store, storeKnown := ir.InstrByID(methodir.InstrID(storePC))
			if !found || !storeKnown || !core.LocalAccessOf(store.Opcode).Write || store.Local != slot || len(stored.Uses) != 1 || len(storePCs) == 1 && stored.Uses[0] != origin || len(stored.Before.Stack) < width || len(stored.BeforeOrigins) != len(stored.Before.Locals)+len(stored.Before.Stack) {
				valid = false
				break
			}
			// Each STORE consumes a complete initialized computational word.
			// Category-2 halves must carry its identical origin; tails are not
			// values. Equal types do not identify different branch producers.
			stackIndex := len(stored.Before.Stack) - width
			value := stored.Before.Stack[stackIndex]
			originIndex := len(stored.Before.Locals) + stackIndex
			storedOrigin := stored.Uses[0]
			typedNull := primitiveWords && word.Kind == frametransfer.Ref && value.Kind == frametransfer.Null && width == 1
			if !typedNull && (value.Kind != word.Kind || !referenceAssignable && value.Class != word.Class) || core.LocalAccessOf(store.Opcode).Width != width || stored.BeforeOrigins[originIndex] != storedOrigin || storedOrigin.Kind != ssabuild.OriginInstr && storedOrigin.Kind != ssabuild.OriginParam && storedOrigin.Kind != ssabuild.OriginPhi {
				valid = false
				break
			}
			// Exact initialized frame descriptors retain the older admission,
			// including producers outside the hierarchy proof's opcode domain.
			// Only a widened reference requires the bounded all-origin graph.
			actual := value.Class
			if value.Kind == frametransfer.Ref && actual != "" && actual[0] != '[' {
				actual = "L" + actual + ";"
			}
			if referenceAssignable && !typedNull && actual != descriptor {
				if referenceWords == nil {
					referenceWords = newNativeMethodFrameWords(c.obj, method, ir, frames, c.Work)
				}
				if referenceWords == nil || !referenceWords.reference(storedOrigin, descriptor, widening, frames) {
					valid = false
					break
				}
			}
			if width == 2 && (stored.Before.Stack[stackIndex+1].Kind != frametransfer.TailOf(value).Kind || stored.BeforeOrigins[originIndex+1] != storedOrigin) {
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		if c.Work != nil && c.Work.CheckAlloc(int64(len(result)+1)*128) != nil {
			return nil, false
		}
		storePC := storePCs[0]
		var multiple []int
		if len(storePCs) > 1 {
			storePC, multiple = -1, storePCs
		}
		result[int(record.PC)] = &nativeEnumLocalRead{pc: int(record.PC), opcode: instruction.Opcode, slot: slot, storePC: storePC, descriptor: descriptor, storePCs: multiple, referenceAssignable: referenceAssignable}
	}
	return result, true
}

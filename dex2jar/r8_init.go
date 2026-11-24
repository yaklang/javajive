package dex2jar

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/javajive/dex2jar/ir"
	"github.com/yaklang/javajive/dex2jar/model"
)

const (
	r8DexPublic      = uint32(0x0001)
	r8DexStatic      = uint32(0x0008)
	r8DexInterface   = uint32(0x0200)
	r8DexSynthetic   = uint32(0x1000)
	r8DexConstructor = uint32(0x10000)
)

const thisAliasSentinel = "<this>"

// synthesizeR8InlinedConstructors restores empty forwarding constructors that
// R8/ProGuard stripped and bypassed by rewriting `new-instance Sub; invoke Object.<init>`
// or `Sub.<init> -> invoke GrandParent.<init>` (FailureRecover only).
func synthesizeR8InlinedConstructors(app *model.Application, hierarchy map[string]*model.Class) []ConversionDiagnostic {
	if app == nil {
		return nil
	}
	fileByClass := make(map[string]*model.File, len(hierarchy))
	fileEntry := make(map[*model.File]string, len(app.Files))
	for _, f := range app.Files {
		if f == nil {
			continue
		}
		fileEntry[f] = f.Entry
		for i := range f.Classes {
			c := &f.Classes[i]
			if _, exists := fileByClass[c.Descriptor]; !exists {
				fileByClass[c.Descriptor] = f
			}
		}
	}

	var diags []ConversionDiagnostic
	synthesized := make(map[string]bool)

	for pass := 0; pass < 4; pass++ {
		changedInPass := false
		for _, f := range app.Files {
			if f == nil {
				continue
			}
			for ci := range f.Classes {
				c := &f.Classes[ci]
				for mi := range c.DirectMethods {
					if r8PatchMethod(f, c, &c.DirectMethods[mi], fileByClass, hierarchy, fileEntry, synthesized, &diags) {
						changedInPass = true
					}
				}
				for mi := range c.VirtualMethods {
					if r8PatchMethod(f, c, &c.VirtualMethods[mi], fileByClass, hierarchy, fileEntry, synthesized, &diags) {
						changedInPass = true
					}
				}
			}
		}
		if !changedInPass {
			break
		}
	}
	return diags
}

type r8PatchSite struct {
	offset        uint32
	desiredTarget string
	callee        model.MethodID
	proto         model.Proto
}

func r8PatchMethod(
	f *model.File,
	c *model.Class,
	m *model.EncodedMethod,
	fileByClass map[string]*model.File,
	hierarchy map[string]*model.Class,
	fileEntry map[*model.File]string,
	synthesized map[string]bool,
	diags *[]ConversionDiagnostic,
) bool {
	if m == nil || m.Code == nil {
		return false
	}
	ctx := ir.Context{Entry: f.Entry, Class: c.Descriptor, Method: m.Method.Name}
	insns, err := ir.Decode(m.Code, ctx)
	if err != nil || len(insns) == 0 {
		return false
	}
	isCtor := m.Method.Name == "<init>" && m.AccessFlags&r8DexStatic == 0 && m.Code.InsSize <= m.Code.RegistersSize
	hasCandidate := false
	for i := range insns {
		op := insns[i].Opcode
		if (op == 0x70 || op == 0x76) && int(insns[i].Index) < len(f.Methods) {
			callee := f.Methods[insns[i].Index]
			if callee.Name == "<init>" {
				hasCandidate = true
				break
			}
		}
	}
	if !hasCandidate {
		return false
	}

	sites := findR8PatchSites(f, c, m, insns, isCtor, hierarchy)
	if len(sites) == 0 {
		return false
	}

	clonedCode := false
	changed := false
	for _, site := range sites {
		if int(site.offset)+1 >= len(m.Code.Instructions) {
			continue
		}
		resolvedProto, extraNulls, ok := ensureForwardingCtorChain(site.desiredTarget, site.callee.Class, site.proto, fileByClass, hierarchy, fileEntry, synthesized, diags)
		if !ok {
			continue
		}
		pIdx, ok := r8EnsureProto(f, resolvedProto)
		if !ok {
			continue
		}
		mIdx, ok := r8EnsureMethod(f, site.desiredTarget, "<init>", pIdx)
		if !ok {
			continue
		}
		if mIdx <= 0xffff {
			if !clonedCode {
				copied := make([]uint16, len(m.Code.Instructions))
				copy(copied, m.Code.Instructions)
				m.Code.Instructions = copied
				clonedCode = true
			}
			m.Code.Instructions[site.offset+1] = uint16(mIdx)
		} else {
			if m.Code.PatchedMethods == nil {
				m.Code.PatchedMethods = make(map[uint32]uint32)
			}
			m.Code.PatchedMethods[site.offset] = mIdx
		}
		if extraNulls > 0 {
			if m.Code.PatchedNullArgs == nil {
				m.Code.PatchedNullArgs = make(map[uint32]int)
			}
			m.Code.PatchedNullArgs[site.offset] += extraNulls
		}
		changed = true
	}
	return changed
}

func findR8PatchSites(
	f *model.File,
	c *model.Class,
	m *model.EncodedMethod,
	insns []ir.Instruction,
	isCtor bool,
	hierarchy map[string]*model.Class,
) []r8PatchSite {
	// Build minimal intraprocedural blocks over decoded instructions to track
	// uninitialized `new-instance` and `this` aliases conservatively.
	offsetToIdx := make(map[uint32]int, len(insns))
	starts := map[uint32]bool{insns[0].Offset: true}
	for i := range insns {
		offsetToIdx[insns[i].Offset] = i
	}
	for i := range insns {
		ins := &insns[i]
		op := ins.Opcode
		if ins.Target >= 0 {
			starts[uint32(ins.Target)] = true
		}
		for _, t := range ins.Targets {
			if t >= 0 {
				starts[uint32(t)] = true
			}
		}
		isCondBranch := (op >= 0x32 && op <= 0x3d) || op == 0x2b || op == 0x2c
		isUncond := (op >= 0x28 && op <= 0x2a) || (op >= 0x0e && op <= 0x11) || op == 0x27
		if (isCondBranch || isUncond) && i+1 < len(insns) {
			starts[insns[i+1].Offset] = true
		}
	}
	for _, try := range m.Code.Tries {
		for _, h := range try.Handler.Handlers {
			starts[h.Address] = true
		}
		if try.Handler.CatchAllAddress != nil {
			starts[*try.Handler.CatchAllAddress] = true
		}
	}

	var blockStarts []uint32
	for off := range starts {
		if _, ok := offsetToIdx[off]; ok {
			blockStarts = append(blockStarts, off)
		}
	}
	sort.Slice(blockStarts, func(i, j int) bool { return blockStarts[i] < blockStarts[j] })
	blockByOffset := make(map[uint32]int, len(blockStarts))
	for bi, off := range blockStarts {
		blockByOffset[off] = bi
	}

	numBlocks := len(blockStarts)
	blockInsns := make([][]int, numBlocks)
	succs := make([][]int, numBlocks)
	excSuccs := make([][]int, numBlocks)
	curBlock := -1
	for i := range insns {
		if b, ok := blockByOffset[insns[i].Offset]; ok {
			curBlock = b
		}
		if curBlock >= 0 {
			blockInsns[curBlock] = append(blockInsns[curBlock], i)
		}
	}
	for b := 0; b < numBlocks; b++ {
		if len(blockInsns[b]) == 0 {
			continue
		}
		last := &insns[blockInsns[b][len(blockInsns[b])-1]]
		op := last.Opcode
		if last.Target >= 0 {
			if tb, ok := blockByOffset[uint32(last.Target)]; ok {
				succs[b] = append(succs[b], tb)
			}
		}
		for _, t := range last.Targets {
			if t >= 0 {
				if tb, ok := blockByOffset[uint32(t)]; ok {
					succs[b] = append(succs[b], tb)
				}
			}
		}
		isUncond := (op >= 0x28 && op <= 0x2a) || (op >= 0x0e && op <= 0x11) || op == 0x27
		if !isUncond && b+1 < numBlocks {
			succs[b] = append(succs[b], b+1)
		}
		if len(m.Code.Tries) > 0 {
			seenExc := make(map[int]bool)
			for _, idx := range blockInsns[b] {
				off := insns[idx].Offset
				for ti := range m.Code.Tries {
					tr := &m.Code.Tries[ti]
					if off < tr.StartAddress || off >= tr.StartAddress+uint32(tr.InstructionCount) {
						continue
					}
					for _, h := range tr.Handler.Handlers {
						if hb, ok := blockByOffset[h.Address]; ok && !seenExc[hb] {
							seenExc[hb] = true
							excSuccs[b] = append(excSuccs[b], hb)
						}
					}
					if tr.Handler.CatchAllAddress != nil {
						if hb, ok := blockByOffset[*tr.Handler.CatchAllAddress]; ok && !seenExc[hb] {
							seenExc[hb] = true
							excSuccs[b] = append(excSuccs[b], hb)
						}
					}
				}
			}
		}
	}

	type regState map[uint16]string
	cloneState := func(s regState) regState {
		out := make(regState, len(s))
		for k, v := range s {
			out[k] = v
		}
		return out
	}
	statesEqual := func(a, b regState) bool {
		if len(a) != len(b) {
			return false
		}
		for k, v := range a {
			if bv, ok := b[k]; !ok || bv != v {
				return false
			}
		}
		return true
	}
	intersectState := func(dst, src regState) regState {
		out := make(regState)
		for k, v := range dst {
			if sv, ok := src[k]; ok && sv == v {
				out[k] = v
			}
		}
		return out
	}

	in := make([]regState, numBlocks)
	visited := make([]bool, numBlocks)
	in[0] = make(regState)
	if isCtor {
		in[0][m.Code.RegistersSize-m.Code.InsSize] = thisAliasSentinel
	}
	visited[0] = true

	queue := []int{0}
	inQueue := make([]bool, numBlocks)
	inQueue[0] = true

	transferInsn := func(ins *ir.Instruction, state regState) {
		op := ins.Opcode
		switch {
		case op == 0x22 && len(ins.Regs) > 0 && int(ins.Index) < len(f.Types):
			state[ins.Regs[0]] = f.Types[ins.Index].Descriptor
		case ((op >= 0x07 && op <= 0x09) || (op >= 0x01 && op <= 0x03)) && len(ins.Regs) >= 2:
			if v, ok := state[ins.Regs[1]]; ok && v != "" {
				state[ins.Regs[0]] = v
			} else {
				delete(state, ins.Regs[0])
			}
		case (op == 0x70 || op == 0x76) && len(ins.Regs) > 0 && int(ins.Index) < len(f.Methods) && f.Methods[ins.Index].Name == "<init>":
			recv := ins.Regs[0]
			if state[recv] == thisAliasSentinel {
				for k, v := range state {
					if v == thisAliasSentinel {
						delete(state, k)
					}
				}
			} else {
				delete(state, recv)
			}
		default:
			_, defs := ir.InstructionEffects(*ins)
			for _, d := range defs {
				delete(state, d)
			}
		}
	}

	maxSteps := numBlocks*32 + 256
	for steps := 0; len(queue) > 0 && steps < maxSteps; steps++ {
		b := queue[0]
		queue = queue[1:]
		inQueue[b] = false
		st := cloneState(in[b])
		for _, idx := range blockInsns[b] {
			transferInsn(&insns[idx], st)
		}
		for _, succ := range succs[b] {
			var next regState
			changed := false
			if !visited[succ] {
				visited[succ] = true
				next = cloneState(st)
				changed = true
			} else {
				next = intersectState(in[succ], st)
				changed = !statesEqual(in[succ], next)
			}
			in[succ] = next
			if changed && !inQueue[succ] {
				inQueue[succ] = true
				queue = append(queue, succ)
			}
		}
		// Exception handlers start with no uninitialized local allocations from the
		// try body because any protected instruction in the block may throw.
		for _, exc := range excSuccs[b] {
			if !visited[exc] || len(in[exc]) > 0 {
				visited[exc] = true
				in[exc] = make(regState)
				if !inQueue[exc] {
					inQueue[exc] = true
					queue = append(queue, exc)
				}
			}
		}
	}

	var sites []r8PatchSite
	for b := 0; b < numBlocks; b++ {
		if !visited[b] {
			continue
		}
		st := cloneState(in[b])
		for _, idx := range blockInsns[b] {
			ins := &insns[idx]
			op := ins.Opcode
			if (op == 0x70 || op == 0x76) && len(ins.Regs) > 0 && int(ins.Index) < len(f.Methods) {
				callee := f.Methods[ins.Index]
				if callee.Name == "<init>" && int(callee.ProtoIndex) < len(f.Protos) {
					recv := ins.Regs[0]
					tag := st[recv]
					var desired string
					if isCtor && tag == thisAliasSentinel {
						if callee.Class != c.Descriptor && callee.Class != c.Superclass && isProperAncestor(c.Superclass, callee.Class, hierarchy) {
							desired = c.Superclass
						}
					} else if tag != "" && tag != thisAliasSentinel && tag != callee.Class && isProperAncestor(tag, callee.Class, hierarchy) {
						desired = tag
					}
					if desired != "" {
						sites = append(sites, r8PatchSite{
							offset:        ins.Offset,
							desiredTarget: desired,
							callee:        callee,
							proto:         f.Protos[callee.ProtoIndex],
						})
					}
				}
			}
			transferInsn(ins, st)
		}
	}
	return sites
}

var platformSuperclass = map[string]string{
	"Ljava/lang/Throwable;":                     "Ljava/lang/Object;",
	"Ljava/lang/Error;":                         "Ljava/lang/Throwable;",
	"Ljava/lang/AssertionError;":                "Ljava/lang/Error;",
	"Ljava/lang/LinkageError;":                  "Ljava/lang/Error;",
	"Ljava/lang/VirtualMachineError;":           "Ljava/lang/Error;",
	"Ljava/lang/Exception;":                     "Ljava/lang/Throwable;",
	"Ljava/lang/RuntimeException;":              "Ljava/lang/Exception;",
	"Ljava/lang/IllegalArgumentException;":      "Ljava/lang/RuntimeException;",
	"Ljava/lang/IllegalStateException;":         "Ljava/lang/RuntimeException;",
	"Ljava/lang/UnsupportedOperationException;": "Ljava/lang/RuntimeException;",
	"Ljava/lang/NullPointerException;":          "Ljava/lang/RuntimeException;",
	"Ljava/lang/IndexOutOfBoundsException;":     "Ljava/lang/RuntimeException;",
	"Ljava/lang/ClassCastException;":            "Ljava/lang/RuntimeException;",
	"Ljava/lang/NumberFormatException;":         "Ljava/lang/IllegalArgumentException;",
	"Ljava/lang/SecurityException;":             "Ljava/lang/RuntimeException;",
	"Ljava/util/NoSuchElementException;":        "Ljava/lang/RuntimeException;",
	"Ljava/lang/ArithmeticException;":           "Ljava/lang/RuntimeException;",
	"Ljava/io/IOException;":                     "Ljava/lang/Exception;",
	"Ljava/io/FileNotFoundException;":           "Ljava/io/IOException;",
	"Ljava/io/EOFException;":                    "Ljava/io/IOException;",
	"Ljava/lang/ReflectiveOperationException;":  "Ljava/lang/Exception;",
	"Ljava/lang/InterruptedException;":          "Ljava/lang/Exception;",
	"Ljava/lang/Thread;":                        "Ljava/lang/Object;",
	"Ljava/util/AbstractCollection;":            "Ljava/lang/Object;",
	"Ljava/util/AbstractList;":                  "Ljava/util/AbstractCollection;",
	"Ljava/util/ArrayList;":                     "Ljava/util/AbstractList;",
	"Ljava/util/AbstractMap;":                   "Ljava/lang/Object;",
	"Ljava/util/HashMap;":                       "Ljava/util/AbstractMap;",
	"Ljava/lang/Enum;":                          "Ljava/lang/Object;",
	"Ljava/lang/Number;":                        "Ljava/lang/Object;",
}

func platformClassAcceptsCtor(desc string, proto model.Proto) bool {
	if desc == "Ljava/lang/Object;" {
		return protoSignature(proto) == "()V"
	}
	if _, ok := platformSuperclass[desc]; !ok {
		return false
	}
	sig := protoSignature(proto)
	if sig == "()V" {
		return desc != "Ljava/lang/Enum;"
	}
	switch desc {
	case "Ljava/lang/Throwable;",
		"Ljava/lang/Error;",
		"Ljava/lang/Exception;",
		"Ljava/lang/RuntimeException;",
		"Ljava/lang/IllegalArgumentException;",
		"Ljava/lang/IllegalStateException;",
		"Ljava/lang/UnsupportedOperationException;",
		"Ljava/lang/SecurityException;",
		"Ljava/lang/ReflectiveOperationException;",
		"Ljava/io/IOException;":
		return sig == "(Ljava/lang/String;)V" ||
			sig == "(Ljava/lang/Throwable;)V" ||
			sig == "(Ljava/lang/String;Ljava/lang/Throwable;)V"
	case "Ljava/lang/AssertionError;":
		return sig == "(Ljava/lang/Object;)V" || sig == "(Ljava/lang/String;Ljava/lang/Throwable;)V"
	case "Ljava/lang/LinkageError;":
		return sig == "(Ljava/lang/String;)V" || sig == "(Ljava/lang/String;Ljava/lang/Throwable;)V"
	case "Ljava/lang/Enum;":
		return sig == "(Ljava/lang/String;I)V"
	case "Ljava/lang/NullPointerException;",
		"Ljava/lang/IndexOutOfBoundsException;",
		"Ljava/lang/ClassCastException;",
		"Ljava/lang/NumberFormatException;",
		"Ljava/util/NoSuchElementException;",
		"Ljava/lang/ArithmeticException;",
		"Ljava/lang/InterruptedException;",
		"Ljava/io/FileNotFoundException;",
		"Ljava/io/EOFException;":
		return sig == "(Ljava/lang/String;)V"
	}
	return false
}

func superclassOf(desc string, hierarchy map[string]*model.Class) (string, bool) {
	if cls, ok := hierarchy[desc]; ok && cls != nil && cls.Superclass != "" {
		return cls.Superclass, true
	}
	sup, ok := platformSuperclass[desc]
	return sup, ok
}

func isProperAncestor(subDesc, ancDesc string, hierarchy map[string]*model.Class) bool {
	if subDesc == "" || ancDesc == "" || subDesc == ancDesc {
		return false
	}
	if ancDesc == "Ljava/lang/Object;" {
		return true
	}
	visited := make(map[string]bool, 8)
	cur := subDesc
	for cur != "" && !visited[cur] && len(visited) < 128 {
		visited[cur] = true
		sup, ok := superclassOf(cur, hierarchy)
		if !ok {
			return false
		}
		if sup == ancDesc {
			return true
		}
		cur = sup
	}
	return false
}

func ensureForwardingCtorChain(
	desiredTarget string,
	ancTarget string,
	origProto model.Proto,
	fileByClass map[string]*model.File,
	hierarchy map[string]*model.Class,
	fileEntry map[*model.File]string,
	synthesized map[string]bool,
	diags *[]ConversionDiagnostic,
) (model.Proto, int, bool) {
	origSig := protoSignature(origProto)
	origInsWords := 1 + protoParameterWords(origProto)
	if origInsWords > 255 {
		return model.Proto{}, 0, false
	}

	var chain []*model.Class
	visited := make(map[string]bool, 8)
	cur := desiredTarget
	reachedAnc := false
	for cur != "" && !visited[cur] && len(visited) < 128 {
		if cur == ancTarget {
			reachedAnc = true
			break
		}
		visited[cur] = true
		if cls, ok := hierarchy[cur]; ok && cls != nil {
			if cls.AccessFlags&r8DexInterface != 0 {
				return model.Proto{}, 0, false
			}
			chain = append(chain, cls)
			cur = cls.Superclass
			continue
		}
		if !platformClassAcceptsCtor(cur, origProto) {
			return model.Proto{}, 0, false
		}
		for len(visited) < 128 {
			platSup, ok := platformSuperclass[cur]
			if !ok || platSup == "" {
				break
			}
			cur = platSup
			if cur == ancTarget {
				reachedAnc = true
				break
			}
			if visited[cur] {
				break
			}
			visited[cur] = true
		}
		break
	}
	if !reachedAnc {
		return model.Proto{}, 0, false
	}
	if len(chain) == 0 {
		// Caller already targets desiredTarget directly as a platform class
		// whose direct super constructor accepts origProto.
		return origProto, 0, true
	}

	var chosenProto model.Proto
	chosenExtraNulls := -1
	for extraNulls := 0; extraNulls <= 8; extraNulls++ {
		if origInsWords+extraNulls > 255 {
			break
		}
		candProto := model.Proto{
			ReturnType: origProto.ReturnType,
			Parameters: append([]string(nil), origProto.Parameters...),
		}
		for j := 0; j < extraNulls; j++ {
			candProto.Parameters = append(candProto.Parameters, "Ljava/lang/Void;")
		}
		candProto.Shorty = protoShorty(candProto)
		candSig := protoSignature(candProto)
		collision := false
		for i := len(chain) - 1; i >= 0; i-- {
			cls := chain[i]
			tf := fileByClass[cls.Descriptor]
			if tf == nil {
				return model.Proto{}, 0, false
			}
			wantCalleeSig := candSig
			if i == len(chain)-1 {
				wantCalleeSig = origSig
			}
			found, trivial := isTrivialForwardingCtor(tf, cls, cls.Superclass, candSig, wantCalleeSig)
			if found && !trivial {
				collision = true
				break
			}
		}
		if !collision {
			chosenProto = candProto
			chosenExtraNulls = extraNulls
			break
		}
	}
	if chosenExtraNulls < 0 {
		return model.Proto{}, 0, false
	}

	candSig := protoSignature(chosenProto)
	selfInsWords := origInsWords + chosenExtraNulls
	// Walk top-down (closest to ancTarget down to desiredTarget).
	for i := len(chain) - 1; i >= 0; i-- {
		cls := chain[i]
		tf := fileByClass[cls.Descriptor]
		if tf == nil {
			return model.Proto{}, 0, false
		}
		calleeProto := chosenProto
		calleeSig := candSig
		callWords := selfInsWords
		if i == len(chain)-1 {
			calleeProto = origProto
			calleeSig = origSig
			callWords = origInsWords
		}
		found, trivial := isTrivialForwardingCtor(tf, cls, cls.Superclass, candSig, calleeSig)
		if found {
			if !trivial {
				return model.Proto{}, 0, false
			}
			continue
		}
		pIdx, ok := r8EnsureProto(tf, chosenProto)
		if !ok {
			return model.Proto{}, 0, false
		}
		selfIdx, ok := r8EnsureMethod(tf, cls.Descriptor, "<init>", pIdx)
		if !ok {
			return model.Proto{}, 0, false
		}
		superPIdx, ok := r8EnsureProto(tf, calleeProto)
		if !ok {
			return model.Proto{}, 0, false
		}
		superIdx, ok := r8EnsureMethod(tf, cls.Superclass, "<init>", superPIdx)
		if !ok {
			return model.Proto{}, 0, false
		}
		var words []uint16
		var patchedMethods map[uint32]uint32
		superWord := uint16(superIdx)
		if superIdx > 0xffff {
			superWord = 0
			patchedMethods = map[uint32]uint32{0: superIdx}
		}
		if callWords <= 5 {
			var g uint16
			if callWords == 5 {
				g = 4
			}
			w0 := uint16(callWords<<12) | (g << 8) | 0x70
			var w2 uint16
			for r := 0; r < callWords && r < 4; r++ {
				w2 |= uint16(r) << uint(r*4)
			}
			words = []uint16{w0, superWord, w2, 0x000e}
		} else {
			w0 := uint16(callWords<<8) | 0x76
			words = []uint16{w0, superWord, 0x0000, 0x000e}
		}
		cls.DirectMethods = append(cls.DirectMethods, model.EncodedMethod{
			MethodIndex: selfIdx,
			AccessFlags: r8DexPublic | r8DexSynthetic | r8DexConstructor,
			Method:      tf.Methods[selfIdx],
			Code: &model.Code{
				RegistersSize:  uint16(selfInsWords),
				InsSize:        uint16(selfInsWords),
				OutsSize:       uint16(callWords),
				Instructions:   words,
				PatchedMethods: patchedMethods,
			},
		})
		key := cls.Descriptor + candSig
		if !synthesized[key] {
			synthesized[key] = true
			*diags = append(*diags, ConversionDiagnostic{
				Entry:      fileEntry[tf],
				Class:      cls.Descriptor,
				Method:     "<init>",
				Descriptor: candSig,
				Category:   "recovery",
				Action:     "synthesize_ctor",
				Feature:    "r8-inlined-constructor",
				Code:       "r8.ctor_synthesized",
				Kind:       "recovery",
				Scope:      "method",
				Stage:      "recover",
				Severity:   "warning",
				Offset:     -1,
				Message:    fmt.Sprintf("synthesized forwarding constructor delegating to %s.<init>%s stripped by R8", cls.Superclass, calleeSig),
			})
		}
	}
	return chosenProto, chosenExtraNulls, true
}

func isTrivialForwardingCtor(f *model.File, cls *model.Class, wantSuper, wantProto, wantCalleeProto string) (found bool, trivial bool) {
	for i := range cls.DirectMethods {
		m := &cls.DirectMethods[i]
		if m.Method.Name != "<init>" || int(m.Method.ProtoIndex) >= len(f.Protos) {
			continue
		}
		if protoSignature(f.Protos[m.Method.ProtoIndex]) != wantProto {
			continue
		}
		found = true
		if m.Code == nil || len(m.Code.Tries) > 0 {
			return true, false
		}
		insns, err := ir.Decode(m.Code, ir.Context{Entry: f.Entry, Class: cls.Descriptor, Method: "<init>"})
		if err != nil || len(insns) != 2 {
			return true, false
		}
		if (insns[0].Opcode != 0x70 && insns[0].Opcode != 0x76) || insns[1].Opcode != 0x0e {
			return true, false
		}
		if insns[0].ExtraNullArgs != 0 || int(insns[0].Index) >= len(f.Methods) {
			return true, false
		}
		callee := f.Methods[insns[0].Index]
		if callee.Name != "<init>" || callee.Class != wantSuper || int(callee.ProtoIndex) >= len(f.Protos) {
			return true, false
		}
		calleeProto := f.Protos[callee.ProtoIndex]
		if protoSignature(calleeProto) != wantCalleeProto {
			return true, false
		}
		firstParam := m.Code.RegistersSize - m.Code.InsSize
		wantCallWords := 1 + protoParameterWords(calleeProto)
		if len(insns[0].Regs) != wantCallWords {
			return true, false
		}
		for idx, r := range insns[0].Regs {
			if r != firstParam+uint16(idx) {
				return true, false
			}
		}
		return true, true
	}
	return false, false
}

func protoSignature(p model.Proto) string {
	return "(" + strings.Join(p.Parameters, "") + ")" + p.ReturnType
}

func protoParameterWords(p model.Proto) int {
	w := 0
	for _, d := range p.Parameters {
		if d == "J" || d == "D" {
			w += 2
		} else {
			w++
		}
	}
	return w
}

func protoShorty(p model.Proto) string {
	var b strings.Builder
	shortyOf := func(d string) byte {
		if len(d) == 1 {
			return d[0]
		}
		return 'L'
	}
	b.WriteByte(shortyOf(p.ReturnType))
	for _, param := range p.Parameters {
		b.WriteByte(shortyOf(param))
	}
	return b.String()
}

func hasFileMetadataSizes(f *model.File) bool {
	return f.Header.StringIDsSize != 0 || f.Header.TypeIDsSize != 0 ||
		f.Header.ProtoIDsSize != 0 || f.Header.FieldIDsSize != 0 || f.Header.MethodIDsSize != 0
}

func r8EnsureString(f *model.File, s string) uint32 {
	for i, existing := range f.Strings {
		if existing == s {
			return uint32(i)
		}
	}
	idx := uint32(len(f.Strings))
	f.Strings = append(f.Strings, s)
	if hasFileMetadataSizes(f) {
		f.Header.StringIDsSize = uint32(len(f.Strings))
	}
	return idx
}

func r8EnsureType(f *model.File, desc string) uint32 {
	for i, existing := range f.Types {
		if existing.Descriptor == desc {
			return uint32(i)
		}
	}
	meta := hasFileMetadataSizes(f)
	strIdx := r8EnsureString(f, desc)
	idx := uint32(len(f.Types))
	f.Types = append(f.Types, model.Type{DescriptorIndex: strIdx, Descriptor: desc})
	if meta || f.Header.TypeIDsSize != 0 {
		f.Header.TypeIDsSize = uint32(len(f.Types))
	}
	return idx
}

func r8EnsureProto(f *model.File, proto model.Proto) (uint16, bool) {
	sig := protoSignature(proto)
	for i, existing := range f.Protos {
		if protoSignature(existing) == sig {
			return uint16(i), true
		}
	}
	if len(f.Protos) >= 0xffff {
		return 0, false
	}
	meta := hasFileMetadataSizes(f)
	shorty := protoShorty(proto)
	shortyIdx := r8EnsureString(f, shorty)
	retIdx := r8EnsureType(f, proto.ReturnType)
	paramTypes := make([]uint32, len(proto.Parameters))
	for i, p := range proto.Parameters {
		paramTypes[i] = r8EnsureType(f, p)
	}
	idx := uint16(len(f.Protos))
	f.Protos = append(f.Protos, model.Proto{
		ShortyIndex:     shortyIdx,
		Shorty:          shorty,
		ReturnTypeIndex: retIdx,
		ReturnType:      proto.ReturnType,
		ParameterTypes:  paramTypes,
		Parameters:      append([]string(nil), proto.Parameters...),
	})
	if meta || f.Header.ProtoIDsSize != 0 {
		f.Header.ProtoIDsSize = uint32(len(f.Protos))
	}
	return idx, true
}

func r8EnsureMethod(f *model.File, classDesc, name string, protoIdx uint16) (uint32, bool) {
	for i, existing := range f.Methods {
		if existing.Class == classDesc && existing.Name == name && existing.ProtoIndex == protoIdx {
			return uint32(i), true
		}
	}
	meta := hasFileMetadataSizes(f)
	classIdx := r8EnsureType(f, classDesc)
	if classIdx > 0xffff {
		return 0, false
	}
	nameIdx := r8EnsureString(f, name)
	idx := uint32(len(f.Methods))
	f.Methods = append(f.Methods, model.MethodID{
		ClassIndex: uint16(classIdx),
		Class:      classDesc,
		NameIndex:  nameIdx,
		Name:       name,
		ProtoIndex: protoIdx,
	})
	if meta || f.Header.MethodIDsSize != 0 {
		f.Header.MethodIDsSize = uint32(len(f.Methods))
	}
	return idx, true
}

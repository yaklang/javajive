package javaclassparser

import (
	"slices"
	"sort"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// Propose a declaration order before any whole-source or private registration
// certificate is built. Completed original factory occurrences give contiguous
// implementation-ordinal intervals. Only ordinary method declarations move;
// fields, initializers, constructors and every other packet keep their order.
// This is not a publication proof: the candidate is assembled anew, then the
// existing constructor, lexical membership, accessor registration and final
// lambda-identity protocols must all close against that actual new source.
func (c *ClassObjectDumper) proposeLexicalLambdaMethodOrder(source string, members []string, methods []*dumpedMethods) ([]*dumpedMethods, bool) {
	if c == nil || c.obj == nil || len(methods) > 4096 || c.obj.AccessFlags&0x6200 != 0 || c.nativeMemberCurrent != nil || c.nativeMethodLocalCurrent != nil || c.nativeEnumConstantCurrent != nil || c.nativeCaptureFields != nil {
		return nil, false
	}
	if c.options.SourceCompiler != NativeJavac8 && c.options.SourceCompiler != ModernJavac {
		return nil, false
	}
	// The owning member packets are not inserted into this assembly yet, so
	// only this original source scope contributes to its implementation counter.
	events, known := c.native8LambdaSourceIdentityEvents(source, nil, methods)
	if !known || len(events) == 0 || len(events) > 4096 || nativeLambdaSourceIdentityOrderClosed(events) || !c.lambdaScheduleOwnDeclarationScope() || !nativeProofWork(c.Work, int64(len(methods))+int64(len(events))+1) || c.Work != nil && c.Work.CheckAlloc(int64(len(methods))*64+int64(len(events))*128) != nil {
		return nil, false
	}
	type interval struct {
		body     *dumpedMethods
		first    int
		count    int
		anchored bool
	}
	groups := map[*dumpedMethods]*interval{}
	seen := make([]bool, len(events))
	scanned := 0
	for _, event := range events {
		if event.parent == nil || event.parent.member == nil || event.ordinal < 0 || event.ordinal >= len(events) || seen[event.ordinal] || !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		seen[event.ordinal] = true
		parent := event.parent
		name, named := sourceBridgeUTF8(c.obj, parent.member.NameIndex)
		if !named || name == "<init>" || isSyntheticMethod(parent.member.AccessFlags) || parent.sourceRequest.role != methodSourceDeclaration || parent.sourceDumper != c || parent.sourceObject != c.obj || parent.sourceCacheKey == "" || c.dumpedMethodsSet[parent.sourceCacheKey] != parent || parent.sourceReconstructionFailed() || parent.initializerCode != "" || name == "<clinit>" && parent.member.AccessFlags != 8 {
			return nil, false
		}
		group := groups[parent]
		if group == nil {
			if name != "<clinit>" && !c.lambdaScheduleHasNoLexicalEvents(parent, map[*MemberInfo]bool{}, 0, &scanned) {
				return nil, false
			}
			group = &interval{body: parent, first: event.ordinal, anchored: name == "<clinit>"}
			groups[parent] = group
		}
		if event.ordinal != group.first+group.count {
			// Interleaved source phases need a finer-grained compiler schedule;
			// splitting or reordering an initializer's statements is forbidden.
			return nil, false
		}
		group.count++
	}
	positions := map[*dumpedMethods]int{}
	var slots []int
	var ordered []*interval
	anchored := false
	for i, method := range methods {
		if method == nil || !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		if _, duplicate := positions[method]; duplicate {
			return nil, false
		}
		positions[method] = i
		if group := groups[method]; group != nil {
			slots = append(slots, i)
			ordered = append(ordered, group)
			anchored = anchored || group.anchored
		}
	}
	if len(ordered) != len(groups) {
		return nil, false
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].first < ordered[j].first })
	var result []*dumpedMethods
	if !anchored {
		result = slices.Clone(methods)
		for i, slot := range slots {
			result[slot] = ordered[i].body
		}
	} else {
		var movable []*interval
		for _, group := range ordered {
			if !group.anchored {
				movable = append(movable, group)
			}
		}
		cursor, ordinal := 0, 0
		emit := func(group *interval) bool {
			if group.first != ordinal {
				return false
			}
			result = append(result, group.body)
			ordinal += group.count
			return true
		}
		for _, method := range methods {
			group := groups[method]
			if group == nil {
				result = append(result, method)
				continue
			}
			if !group.anchored {
				continue
			}
			for cursor < len(movable) && movable[cursor].first < group.first {
				if !emit(movable[cursor]) {
					return nil, false
				}
				cursor++
			}
			if !emit(group) {
				return nil, false
			}
		}
		for ; cursor < len(movable); cursor++ {
			if !emit(movable[cursor]) {
				return nil, false
			}
		}
		if ordinal != len(events) {
			return nil, false
		}
	}
	// Pointer conservation preserves each completed packet byte for byte. The
	// fixed subsequence includes the original static initializer, every other
	// initialization phase and all methods which did not need this proposal.
	if len(result) != len(methods) || slices.Equal(result, methods) {
		return nil, false
	}
	var fixedBefore, fixedAfter []*dumpedMethods
	for _, method := range methods {
		if group := groups[method]; group == nil || group.anchored {
			fixedBefore = append(fixedBefore, method)
		}
	}
	for _, method := range result {
		if _, found := positions[method]; !found {
			return nil, false
		}
		delete(positions, method)
		if group := groups[method]; group == nil || group.anchored {
			fixedAfter = append(fixedAfter, method)
		}
	}
	return result, len(positions) == 0 && slices.Equal(fixedBefore, fixedAfter)
}

// A complete lexical family owns its existing registration certificate. A
// plain original top-level class can own its method-declaration order without
// owning a family. Admit that narrower case only when the original table has
// no local/anonymous/owned member scopes whose compiler events could move.
// An absent or incomplete family never becomes a successful family proof.
func (c *ClassObjectDumper) lambdaScheduleOwnDeclarationScope() bool {
	if c == nil || c.obj == nil {
		return false
	}
	if p := c.nativeMemberRoot; p != nil && p.owner != "" {
		return !p.failed && !p.declarationLookupOnly && p.owner == c.obj.GetClassName()
	}
	if c.nativeAnonymousRoot != nil || !nativeMemberTopLevelEvidence(c.obj, c.Work) {
		return false
	}
	owner := c.obj.GetClassName()
	tables := 0
	for _, attribute := range c.obj.Attributes {
		if !nativeProofWork(c.Work, 1) {
			return false
		}
		if raw, ok := attribute.(*UnparsedAttribute); ok && (raw == nil || raw.Name == "InnerClasses") {
			return false
		}
		if table, ok := attribute.(*InnerClassesAttribute); ok {
			if table == nil || tables != 0 || int(table.NumberOfClasses) != len(table.Classes) || table.AttrLen != uint32(2+8*len(table.Classes)) {
				return false
			}
			tables++
			for _, row := range table.Classes {
				if row == nil || !nativeProofWork(c.Work, 1) {
					return false
				}
				inner, innerKnown := sourceBridgeClassName(c.obj, row.InnerClassInfoIndex)
				outer, outerKnown := sourceBridgeClassName(c.obj, row.OuterClassInfoIndex)
				name, named := sourceBridgeUTF8(c.obj, row.InnerNameIndex)
				if !innerKnown || !outerKnown || !named || !nativeProofWork(c.Work, int64(len(inner)+len(outer)+len(name))+1) || !class_context.PreservesJava8Identifier(name) || outer == owner || strings.HasPrefix(inner, owner+"$") || strings.HasPrefix(outer, owner+"$") {
					return false
				}
			}
		}
	}
	return true
}

// An unrelated anonymous scope is not itself an obstacle to proposing an
// ordinary declaration's position. The moving packet and each of its compiler
// implementation bodies must, however, have no object/local-class allocation
// or private-access registration event. This bounded structural query grants
// no right to move a statement or to skip the family's final registrations.
func (c *ClassObjectDumper) lambdaScheduleHasNoLexicalEvents(body *dumpedMethods, active map[*MemberInfo]bool, depth int, scanned *int) bool {
	if c == nil || c.obj == nil || !c.lambdaScheduleOwnDeclarationScope() || body == nil || body.member == nil || body.sourceObject != c.obj || body.sourceDumper != c || c.dumpedMethodsSet[body.sourceCacheKey] != body || body.sourceReconstructionFailed() || depth < 0 || depth >= 64 || active == nil || active[body.member] || scanned == nil || *scanned < 0 || *scanned > 256<<10 || !nativeProofWork(c.Work, 1) {
		return false
	}
	name, named := sourceBridgeUTF8(c.obj, body.member.NameIndex)
	desc, typed := sourceBridgeUTF8(c.obj, body.member.DescriptorIndex)
	if !named || !typed || name == "<init>" || name == "<clinit>" {
		return false
	}
	if c.nativeMemberRoot != nil {
		for _, local := range c.nativeMemberRoot.methodLocals {
			if !nativeProofWork(c.Work, 1) || local == nil || local.owner == nil || local.owner.method == name && local.owner.descriptor == desc {
				return false
			}
		}
	}
	var code *CodeAttribute
	for _, attribute := range body.member.Attributes {
		if !nativeProofWork(c.Work, 1) {
			return false
		}
		if candidate, ok := attribute.(*CodeAttribute); ok {
			if candidate == nil || code != nil {
				return false
			}
			code = candidate
		}
	}
	if code == nil || len(code.Code) > (256<<10)-*scanned || !nativeProofWork(c.Work, int64(len(code.Code))+1) || c.Work != nil && c.Work.CheckAlloc(int64(len(code.Code))*128+int64(len(active)+1)*64) != nil {
		return false
	}
	*scanned += len(code.Code)
	active[body.member] = true
	defer delete(active, body.member)
	decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(c.obj.ConstantPool, i) })
	decoder.Work = c.Work
	if decoder.ParseOpcode() != nil {
		return false
	}
	for _, op := range constructorMotionOps(decoder) {
		if !nativeProofWork(c.Work, 1) || op.Instr.OpCode == core.OP_NEW {
			return false
		}
		if opcode := op.Instr.OpCode; opcode == core.OP_INVOKESTATIC || opcode == core.OP_INVOKEVIRTUAL || opcode == core.OP_INVOKEINTERFACE || opcode == core.OP_INVOKESPECIAL {
			member := constructorMotionMember(c.obj, op, opcode)
			if member == nil {
				return false
			}
			key := nativeMemberGetterKey(member.Name, member.Member, member.Description)
			if c.nativeMemberRoot != nil && (c.nativeMemberRoot.getters[key] != nil || c.nativeMemberRoot.retainedAccessors[key] != nil) {
				return false
			}
		}
		if op.Instr.OpCode != core.OP_INVOKEDYNAMIC {
			continue
		}
		var child *dumpedMethods
		for method, receipt := range c.lambdaMethodSourceBodies {
			if !nativeProofWork(c.Work, 1) {
				return false
			}
			if receipt == nil || receipt.request.factory == nil || receipt.request.factory.method != body.member || receipt.request.factory.code != code || receipt.request.factory.pc != int(op.CurrentOffset) {
				continue
			}
			if child != nil || !receipt.request.factory.sealed || !receipt.request.factory.closed() || !c.lambdaMethodSourcePacketClosed(method, receipt.body, c.Work) {
				return false
			}
			child = receipt.body
		}
		if child == nil || !c.lambdaScheduleHasNoLexicalEvents(child, active, depth+1, scanned) {
			return false
		}
	}
	return true
}

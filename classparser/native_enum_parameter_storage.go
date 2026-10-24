package javaclassparser

import (
	"encoding/base64"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"strconv"
	"strings"
)

// This certificate concerns a stable, typed parameter storage cell, not its
// entry value. Every original access must stay in that cell's reference domain;
// every original writer must later have a retained, sealed source assignment.
// Slot reuse, primitive writes and implicit receiver replacement are outside
// this domain. They cannot borrow an equal descriptor from the method header.
type nativeEnumParameterStorage struct {
	slot       int
	descriptor string
	stores     map[int]bool
	bound      *values.JavaRef
	markers    map[int]string
	validated  bool
}

func (c *ClassObjectDumper) nativeEnumParameterStorageProof(method *MemberInfo, code *CodeAttribute, read *nativeEnumSelectorProducer) *nativeEnumParameterStorage {
	if c == nil || c.obj == nil || method == nil || code == nil || read == nil || read.owner != "" || read.local != nil || !callbinding.Reference(read.result) || len(code.Code) == 0 || len(code.Code) > 65535 || !nativeProofWork(c.Work, 1) {
		return nil
	}
	desc, known := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
	args, _, err := callbinding.Descriptor(desc)
	if !known || err != nil {
		return nil
	}
	parameter, slot := false, 0
	if method.AccessFlags&8 == 0 {
		slot = 1 // THIS is never an assignable Java parameter declaration.
	}
	for _, arg := range args {
		parameter = parameter || slot == read.slot && arg == read.result
		slot++
		if arg == "J" || arg == "D" {
			slot++
		}
	}
	if !parameter {
		return nil
	}
	// A matching descriptor on a foreign method or Code object is not an
	// original instruction witness for this physical class.
	methodCount, codeCount := 0, 0
	for _, original := range c.obj.Methods {
		if !nativeProofWork(c.Work, 1) {
			return nil
		}
		if original == method {
			methodCount++
		}
	}
	for _, attribute := range method.Attributes {
		if original, ok := attribute.(*CodeAttribute); ok {
			codeCount++
			if original != code {
				return nil
			}
		}
	}
	if methodCount != 1 || codeCount != 1 {
		return nil
	}
	ir, frames, known := c.nativeOriginalMethodSnapshot(method, code)
	if !known || len(frames.Instructions) > 8192 || !nativeProofWork(c.Work, int64(len(frames.Instructions))) || c.Work != nil && c.Work.CheckAlloc(int64(len(frames.Instructions))*128) != nil {
		return nil
	}
	wordMatches := func(word frametransfer.Type) bool {
		if word.Kind == frametransfer.Null {
			return true
		}
		if word.Kind != frametransfer.Ref || word.Class == "" {
			return false
		}
		actual := word.Class
		if actual[0] != '[' {
			actual = "L" + actual + ";"
		}
		return actual == read.result
	}
	proof := &nativeEnumParameterStorage{slot: read.slot, descriptor: read.result, stores: map[int]bool{}}
	seen, reached := map[int]bool{}, false
	for _, record := range frames.Instructions {
		pc := int(record.PC)
		instruction, found := ir.InstrByID(methodir.InstrID(record.PC))
		if !found || seen[pc] || !nativeProofWork(c.Work, 1) {
			return nil
		}
		seen[pc] = true
		access := core.LocalAccessOf(instruction.Opcode)
		if !access.Read && !access.Write {
			continue
		}
		if instruction.Local < 0 || access.Width < 1 || access.Width > 2 {
			return nil
		}
		if instruction.Local >= read.slot+1 || read.slot >= instruction.Local+access.Width {
			continue
		}
		if instruction.Local != read.slot || access.Width != 1 || len(record.BeforeOrigins) != len(record.Before.Locals)+len(record.Before.Stack) {
			return nil
		}
		if access.Read {
			if access.Write || instruction.Opcode != core.OP_ALOAD && (instruction.Opcode < core.OP_ALOAD_0 || instruction.Opcode > core.OP_ALOAD_3) || read.slot >= len(record.Before.Locals) || !wordMatches(record.Before.Locals[read.slot]) {
				return nil
			}
			if pc == read.pc {
				reached = instruction.Opcode == read.opcode
			}
		}
		if access.Write {
			if instruction.Opcode != core.OP_ASTORE && (instruction.Opcode < core.OP_ASTORE_0 || instruction.Opcode > core.OP_ASTORE_3) || len(record.Before.Stack) == 0 || len(record.Uses) != 1 || !wordMatches(record.Before.Stack[len(record.Before.Stack)-1]) {
				return nil
			}
			origin := record.Uses[0]
			if origin.Kind != ssabuild.OriginInstr && origin.Kind != ssabuild.OriginParam && origin.Kind != ssabuild.OriginPhi || record.BeforeOrigins[len(record.BeforeOrigins)-1] != origin || len(proof.stores) >= 64 {
				return nil
			}
			proof.stores[pc] = true
		}
	}
	if !reached || len(proof.stores) == 0 {
		return nil
	}
	name, known := sourceBridgeUTF8(c.obj, method.NameIndex)
	markerBytes := int64(len(name)+len(desc)+len(c.obj.GetClassName())+64) * int64(len(proof.stores)) * 2
	if !known || !nativeProofWork(c.Work, markerBytes) || c.Work != nil && c.Work.CheckAlloc(markerBytes) != nil {
		return nil
	}
	proof.markers = map[int]string{}
	for pc := range proof.stores {
		identity := c.obj.GetClassName() + "\x00" + name + desc + "\x00" + strconv.Itoa(proof.slot) + "\x00" + strconv.Itoa(pc)
		proof.markers[pc] = "/*jdec-owned-parameter-store:" + base64.RawURLEncoding.EncodeToString([]byte(identity)) + "*/"
	}
	return proof
}

func (c *ClassObjectDumper) nativeEnumSelectorStorageFlow(method *MemberInfo, code *CodeAttribute, node *nativeEnumSelectorProducer, flow *nativeEnumParameterFlow, depth int) bool {
	if node == nil || flow == nil || depth >= 32 || !nativeProofWork(c.Work, 1) {
		return false
	}
	if node.owner == "" {
		if node.local != nil {
			pc, known := flow.reachingStore(node, c.Work)
			if !known || pc != node.local.storePC {
				return false
			}
		} else if !flow.parameterAt(node, c.Work) {
			node.storage = c.nativeEnumParameterStorageProof(method, code, node)
			if node.storage == nil {
				return false
			}
		}
	}
	for _, child := range node.operands {
		if !c.nativeEnumSelectorStorageFlow(method, code, child, flow, depth+1) {
			return false
		}
	}
	return true
}

func nativeEnumSelectorStorages(node *nativeEnumSelectorProducer, work *workbudget.Budget, depth int, visit func(*nativeEnumParameterStorage) bool) bool {
	if node == nil {
		return true
	}
	if depth >= 32 || !nativeProofWork(work, 1) {
		return false
	}
	if node.storage != nil && !visit(node.storage) {
		return false
	}
	for _, child := range node.operands {
		if !nativeEnumSelectorStorages(child, work, depth+1, visit) {
			return false
		}
	}
	return true
}

func (s *nativeEnumParameterStorage) assignment(a *statements.AssignStatement, ctx *class_context.ClassContext) (int, bool) {
	if s == nil || a == nil {
		return 0, false
	}
	pc, slot, known := a.OriginalParameterStore()
	ref, isRef := a.LeftValue.(*values.JavaRef)
	if !known || slot != s.slot || !s.stores[pc] || !isRef || ref.Id == nil || s.bound != nil && s.bound != ref {
		return 0, false
	}
	erasure, known := values.SourceTypeErasure(ref.Type(), ctx)
	if !known || erasure != s.descriptor {
		return 0, false
	}
	s.bound = ref
	return pc, true
}

// Validate the actual final method tree, not a count of bytecode STOREs or
// source names. An omitted writer, another binder, expression assignment or
// opaque source callback cannot discharge the original storage certificate.
func (c *ClassObjectDumper) nativeEnumParameterStorageBody(s *nativeEnumParameterStorage, ctx *class_context.ClassContext) bool {
	if s == nil {
		return false
	}
	s.validated = false
	if len(s.stores) == 0 || !nativeProofWork(c.Work, 1) || c.Work != nil && c.Work.CheckAlloc(4096*128) != nil {
		return false
	}
	seen := map[int]bool{}
	activeValues := map[values.JavaValue]bool{}
	activeStatements := map[statements.Statement]bool{}
	remaining := 4096
	enter := func(depth int) bool { remaining--; return remaining >= 0 && depth < 32 && nativeProofWork(c.Work, 1) }
	parameter := func(v values.JavaValue) (bool, bool) {
		x, known := nativeMemberEnclosingUnpack(v, c.Work)
		if !known || sourceProofNil(x) {
			return false, false
		}
		r, isRef := x.(*values.JavaRef)
		if !isRef {
			return false, true
		}
		if r.Id == nil {
			return false, false
		}
		slot, sealed := r.OriginalParameterSlot()
		return sealed && slot == s.slot || s.bound != nil && r.Id == s.bound.Id, true
	}
	var value func(values.JavaValue, int) bool
	value = func(v values.JavaValue, depth int) bool {
		if sourceProofNil(v) || activeValues[v] || !enter(depth) {
			return false
		}
		activeValues[v] = true
		defer delete(activeValues, v)
		if r, ok := v.(*values.JavaRef); ok {
			matched, known := parameter(r)
			if !known {
				return false
			}
			if matched {
				return s.bound == r && r.CustomValue == nil && r.StackVar == nil
			}
		}
		switch x := v.(type) {
		case *values.AssignmentExpression:
			if matched, known := parameter(x.Target); !known || matched {
				return false
			}
		case *values.JavaExpression:
			if x.Op == "=" || strings.HasSuffix(string(x.Op), "=") && x.Op != "==" && x.Op != "!=" && x.Op != "<=" && x.Op != ">=" || x.Op == values.INC || x.Op == values.DEC {
				if len(x.Values) == 0 {
					return false
				}
				if matched, known := parameter(x.Values[0]); !known || matched {
					return false
				}
			}
		case *values.FunctionCallExpression:
			if !x.IsStatic && !value(x.Object, depth+1) {
				return false
			}
			for _, arg := range x.Arguments {
				if !value(arg, depth+1) {
					return false
				}
			}
			return true
		}
		children, known := values.Children(v)
		if !known {
			return false
		}
		for _, child := range children {
			if !value(child, depth+1) {
				return false
			}
		}
		return true
	}
	// Bind from the sealed assignments before traversing earlier parameter
	// reads: a loop's first read legitimately precedes its later source writer.
	type operand struct {
		value values.JavaValue
		depth int
	}
	operands := []operand{}
	var bind func([]statements.Statement, int) bool
	bind = func(body []statements.Statement, depth int) bool {
		if !enter(depth) {
			return false
		}
		for _, st := range body {
			if sourceProofNil(st) || activeStatements[st] || !enter(depth) {
				return false
			}
			activeStatements[st] = true
			if a, ok := st.(*statements.AssignStatement); ok && a.ArrayMember == nil {
				if matched, known := parameter(a.LeftValue); !known {
					return false
				} else if matched {
					pc, known := s.assignment(a, ctx)
					if !known || seen[pc] {
						return false
					}
					seen[pc] = true
				}
			}
			roots, children, known := nativeSourceNameChildren(st)
			if custom, ok := st.(*statements.CustomStatement); ok && !custom.SourceTransferOnly() {
				operand, sealed := custom.SourceThrowOperand()
				roots, children, known = []values.JavaValue{operand}, nil, sealed
			}
			if !known || len(roots) > 4096-len(operands) || len(children) > remaining {
				return false
			}
			for _, root := range roots {
				operands = append(operands, operand{root, depth + 1})
			}
			for _, child := range children {
				if !bind(child, depth+1) {
					return false
				}
			}
			delete(activeStatements, st)
		}
		return true
	}
	if !bind(c.nativeMemberBody, 0) || s.bound == nil || len(seen) != len(s.stores) {
		return false
	}
	for _, root := range operands {
		if !value(root.value, root.depth) {
			return false
		}
	}
	s.validated = true
	return true
}

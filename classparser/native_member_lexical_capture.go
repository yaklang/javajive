package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"sort"
	"strings"
)

type nativeMemberLexicalRead struct {
	owner, field, descriptor string
	pc                       int
	parameterOwner           string
	basePC                   int
	prior                    *nativeMemberLexicalRead
}

// Only a consecutive original ALOAD_0 / enclosing-field chain represents
// qualified lexical THIS. A foreign receiver, mutable alias or computed value
// cannot borrow the source spelling. Traversing a nullable intermediate owner
// remains a dereference, including its original NPE before a parent callback.
func nativeMemberLexicalReads(obj *ClassObject, p *nativeMemberFamily, work *workbudget.Budget) (map[string]map[int]*nativeMemberLexicalRead, bool) {
	if obj == nil || p == nil {
		return nil, false
	}
	result := map[string]map[int]*nativeMemberLexicalRead{}
	for _, m := range obj.Methods {
		name, nok := sourceBridgeUTF8(obj, m.NameIndex)
		desc, dok := sourceBridgeUTF8(obj, m.DescriptorIndex)
		if !nok || !dok || !nativeProofWork(work, 1) {
			return nil, false
		}
		reads := map[int]*nativeMemberLexicalRead{}
		result[name+desc] = reads
		for _, a := range m.Attributes {
			code, ok := a.(*CodeAttribute)
			if !ok {
				continue
			}
			if !nativeProofWork(work, int64(len(code.Code))) {
				return nil, false
			}
			decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
			decoder.Work = work
			if decoder.ParseOpcode() != nil {
				return nil, false
			}
			entries, closed := nativeMemberLexicalControlEntries(decoder, code, work)
			if !closed {
				return nil, false
			}
			ops := constructorMotionOps(decoder)
			thisStable := true
			for _, op := range ops {
				if core.GetStoreIdx(op) == 0 {
					thisStable = false
				}
			}
			for i, op := range ops {
				if m.AccessFlags&8 != 0 || core.GetRetrieveIdx(op) != 0 || !constructorMotionLoad(op, "Ljava/lang/Object;") {
					continue
				}
				current := p.children[obj.GetClassName()]
				var prior *nativeMemberLexicalRead
				for j := i + 1; j < len(ops) && current != nil && !current.static; j++ {
					if !nativeProofWork(work, 1) {
						return nil, false
					}
					field := constructorMotionMember(obj, ops[j], core.OP_GETFIELD)
					if field == nil || field.Name != current.object.GetClassName() || field.Member != current.field || field.Description != "L"+current.owner+";" {
						break
					}
					// A branch/handler must not enter partway through this
					// expression, nor may a catch boundary split its dereferences.
					entry := sort.SearchInts(entries, int(ops[j-1].CurrentOffset)+1)
					if entry < len(entries) && entries[entry] <= int(ops[j].CurrentOffset) {
						break
					}
					read := &nativeMemberLexicalRead{owner: field.Name, field: field.Member, descriptor: field.Description, pc: int(ops[j].CurrentOffset), prior: prior}
					if reads[read.pc] != nil {
						return nil, false
					}
					reads[read.pc] = read
					prior = read
					current = p.children[current.owner]
				}
			}
			if len(reads) > 0 && !thisStable {
				return nil, false
			}
			if name == "<init>" {
				if child := p.children[obj.GetClassName()]; child != nil {
					if ctor := child.constructors[desc]; ctor != nil && ctor.enclosingSuperPath != nil {
						for _, op := range ops {
							if core.GetStoreIdx(op) == 1 {
								return nil, false
							}
						}
						for node := ctor.enclosingSuperPath; node != nil; node = node.prior {
							preceding := node.basePC
							if node.prior != nil {
								preceding = node.prior.pc
							}
							entry := sort.SearchInts(entries, preceding+1)
							if entry < len(entries) && entries[entry] <= node.pc {
								return nil, false
							}
							if reads[node.pc] != nil {
								return nil, false
							}
							reads[node.pc] = node
						}
					}
				}
			}
			// Every actual read/store of a committed synthetic capture must close.
			// Merely finding one valid lexical path does not license other receivers.
			for _, op := range ops {
				field := constructorMotionMember(obj, op, core.OP_GETFIELD)
				write := false
				if field == nil {
					field = constructorMotionMember(obj, op, core.OP_PUTFIELD)
					write = field != nil
				}
				if field == nil {
					continue
				}
				target := p.children[field.Name]
				if target == nil || target.static || field.Member != target.field {
					continue
				}
				if field.Description != "L"+target.owner+";" {
					return nil, false
				}
				pc := int(op.CurrentOffset)
				if write {
					ctor := target.constructors[desc]
					if name != "<init>" || obj.GetClassName() != field.Name || ctor == nil || ctor.capturePC != pc {
						return nil, false
					}
				} else if reads[pc] == nil {
					return nil, false
				}
			}
		}
	}
	return result, true
}

func nativeMemberLexicalReadOperand(value any, read *nativeMemberLexicalRead, work *workbudget.Budget, contexts ...*class_context.ClassContext) bool {
	ctx := &class_context.ClassContext{}
	if len(contexts) > 0 {
		ctx = contexts[0]
	}
	v, ok := value.(values.JavaValue)
	if !ok || read == nil {
		return false
	}
	seen := map[values.JavaValue]bool{}
	parameterOwner := ""
	for node := read; node != nil; node = node.prior {
		if !nativeProofWork(work, 1) {
			return false
		}
		v, ok = nativeMemberEnclosingUnpack(v, work)
		if !ok || seen[v] {
			return false
		}
		seen[v] = true
		field, known := v.(*values.RefMember)
		if !known || field == nil || !field.HasOriginPC || field.OriginPC != node.pc || field.Member != node.field {
			return false
		}
		if sourceProofNil(field.Object) {
			return false
		}
		receiver, receiverKnown := values.SourceTypeErasure(field.Object.Type(), ctx)
		result, resultKnown := values.SourceTypeErasure(field.Type(), ctx)
		if !receiverKnown || !resultKnown || receiver != "L"+node.owner+";" || result != node.descriptor {
			return false
		}
		v = field.Object
		if node.prior == nil {
			parameterOwner = node.parameterOwner
		}
	}
	v, ok = nativeMemberEnclosingUnpack(v, work)
	if !ok {
		return false
	}
	if parameterOwner != "" {
		return ctx.FunctionName == "<init>" && nativeMemberSourceEnclosingParameter(v, ctx, parameterOwner)
	}
	ref, known := v.(*values.JavaRef)
	return known && ref != nil && ref.IsThis && ref.CustomValue == nil && ref.StackVar == nil
}

func nativeMemberCaptureIndexKey(owner, field string) string { return owner + "\x00" + field }
func nativeMemberCaptureIndexName(name string) bool {
	digits, ok := strings.CutPrefix(name, "this$")
	if !ok || digits == "" {
		return false
	}
	for _, c := range digits {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// Validate signed branch displacements and instruction boundaries separately
// from opcode parsing. Keep dead edges; no reachability assumption can
// turn a multiple-origin operand into lexical THIS.
func nativeMemberLexicalControlEntries(decoder *core.Decompiler, code *CodeAttribute, work *workbudget.Budget) ([]int, bool) {
	seen := map[int]bool{}
	boundaries := map[int]bool{}
	if work != nil && work.CheckAlloc(int64(len(code.Code))*16) != nil {
		return nil, false
	}
	for _, op := range decoder.Opcodes() {
		if op != nil && op.Instr != nil && op.Instr.OpCode != core.OP_START {
			boundaries[int(op.CurrentOffset)] = true
		}
	}
	for _, op := range decoder.Opcodes() {
		if op == nil || op.Instr == nil || !nativeProofWork(work, 1) {
			return nil, false
		}
		switch op.Instr.OpCode {
		case core.OP_GOTO, core.OP_GOTO_W, core.OP_JSR, core.OP_JSR_W, core.OP_IFEQ, core.OP_IFNE, core.OP_IFLT, core.OP_IFGE, core.OP_IFGT, core.OP_IFLE, core.OP_IF_ICMPEQ, core.OP_IF_ICMPNE, core.OP_IF_ICMPLT, core.OP_IF_ICMPGE, core.OP_IF_ICMPGT, core.OP_IF_ICMPLE, core.OP_IF_ACMPEQ, core.OP_IF_ACMPNE, core.OP_IFNULL, core.OP_IFNONNULL:
			width := 2
			if op.Instr.OpCode == core.OP_GOTO_W || op.Instr.OpCode == core.OP_JSR_W {
				width = 4
			}
			target, err := core.BranchTarget(int(op.CurrentOffset), op.Data, width, len(code.Code))
			if err != nil || !boundaries[target] {
				return nil, false
			}
			seen[target] = true
		case core.OP_TABLESWITCH, core.OP_LOOKUPSWITCH:
			if !boundaries[int(op.SwitchDefaultOffset)] {
				return nil, false
			}
			seen[int(op.SwitchDefaultOffset)] = true
			valid := true
			op.SwitchJmpCase.ForEach(func(_ int, pc int32) bool {
				if !boundaries[int(pc)] || !nativeProofWork(work, 1) {
					valid = false
					return false
				}
				seen[int(pc)] = true
				return true
			})
			if !valid {
				return nil, false
			}
		}
	}
	for _, handler := range code.ExceptionTable {
		if handler == nil || handler.StartPc >= handler.EndPc || !boundaries[int(handler.StartPc)] || int(handler.EndPc) > len(code.Code) || int(handler.EndPc) != len(code.Code) && !boundaries[int(handler.EndPc)] || !boundaries[int(handler.HandlerPc)] || !nativeProofWork(work, 1) {
			return nil, false
		}
		seen[int(handler.StartPc)] = true
		seen[int(handler.EndPc)] = true
		seen[int(handler.HandlerPc)] = true
	}
	out := make([]int, 0, len(seen))
	for pc := range seen {
		out = append(out, pc)
	}
	sort.Ints(out)
	return out, true
}

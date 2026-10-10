package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

type nativeMemberAllocationInvocation struct {
	pc                int
	owner, descriptor string
	enclosing         *nativeMemberFreshEnclosing
}

// Nested NEW expressions can have identical nominal owners. Match a call to
// its original uninitialized NEW site, never to the first same-owner Methodref.
// This immutable typed snapshot changes no evaluation, capture or handler.
// The source renderer must independently match its own NEW/invoke origins.
func (c *ClassObjectDumper) nativeMemberAllocationInvocations(method *MemberInfo, code *CodeAttribute) (map[int]nativeMemberAllocationInvocation, bool) {
	ir, fn, known := c.nativeOriginalMethodSnapshot(method, code)
	if !known {
		return nil, false
	}
	result := map[int]nativeMemberAllocationInvocation{}
	fresh := map[int]nativeMemberFreshEnclosing{}
	for _, record := range fn.Instructions {
		if !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		invoke, found := ir.InstrByID(methodir.InstrID(record.PC))
		if !found || invoke.Opcode != core.OP_INVOKESPECIAL || invoke.Member != "<init>" {
			continue
		}
		params, ret, err := callbinding.Descriptor(invoke.Desc)
		index := len(record.Before.Stack) - nativeMemberParameterWidth(params) - 1
		if err != nil || ret != "V" || index < 0 || len(record.BeforeOrigins) != len(record.Before.Locals)+len(record.Before.Stack) {
			return nil, false
		}
		receiver := record.Before.Stack[index]
		if receiver.Kind == frametransfer.UninitThis {
			continue
		}
		origin := record.BeforeOrigins[len(record.Before.Locals)+index]
		allocation, found := ir.InstrByID(methodir.InstrID(receiver.NewPC))
		if receiver.Kind != frametransfer.UninitNew || receiver.Class != invoke.Class || origin.Kind != ssabuild.OriginInstr || origin.PC != receiver.NewPC || !found || allocation.Opcode != core.OP_NEW || allocation.Class != invoke.Class {
			return nil, false
		}
		pc := int(receiver.NewPC)
		if _, duplicate := result[pc]; duplicate {
			// Alternative initializations of one allocation need a joint source
			// expression proof; nominal type equality cannot select a branch.
			return nil, false
		}
		result[pc] = nativeMemberAllocationInvocation{pc: int(record.PC), owner: invoke.Class, descriptor: invoke.Desc}
		if len(params) > 0 && index+1 < len(record.Before.Stack) {
			arg := record.Before.Stack[index+1]
			origin := record.BeforeOrigins[len(record.Before.Locals)+index+1]
			// Initialization changes verifier types, not the physical NEW's
			// origin. A general Ref, phi, field or factory result is not evidence.
			if arg.Kind == frametransfer.Ref && params[0] == "L"+arg.Class+";" && origin.Kind == ssabuild.OriginInstr {
				allocation, exists := ir.InstrByID(methodir.InstrID(origin.PC))
				if exists && allocation.Opcode == core.OP_NEW && allocation.Class == arg.Class {
					fresh[pc] = nativeMemberFreshEnclosing{newPC: int(origin.PC), owner: arg.Class}
				}
			}
		}
	}
	// Prefix counts make every enclosure query constant time. Re-scanning
	// all control edges for every nested NEW would be quadratic in one method.
	var entries, exits []int
	if len(fresh) != 0 {
		count := len(code.Code) + 2
		if !nativeProofWork(c.Work, int64(count+len(ir.Edges))) || c.Work != nil && c.Work.CheckAlloc(int64(count)*16) != nil {
			return nil, false
		}
		entries, exits = make([]int, count), make([]int, count)
		for _, edge := range ir.Edges {
			from, to := int(edge.From), int(edge.To)
			if from >= len(code.Code) || to >= len(code.Code) {
				return nil, false
			}
			if edge.Kind != core.EdgeFallthrough {
				entries[to+1]++
			}
			if edge.Kind != core.EdgeFallthrough && edge.Kind != core.EdgeException {
				exits[from+1]++
			}
		}
		for i := 1; i < count; i++ {
			entries[i] += entries[i-1]
			exits[i] += exits[i-1]
		}
	}
	for pc, qualifier := range fresh {
		if !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		target := result[pc]
		initialized, exists := result[qualifier.newPC]
		if !exists || initialized.owner != qualifier.owner || qualifier.newPC <= pc || initialized.pc >= target.pc {
			continue
		}
		// No alternate branch or handler entry may bypass the original inline
		// allocation/initialization prefix. Original exception exits remain.
		if entries[initialized.pc+1] != entries[pc+1] || exits[initialized.pc] != exits[pc] {
			continue
		}
		qualifier.invokePC, qualifier.descriptor = initialized.pc, initialized.descriptor
		target.enclosing = &qualifier
		result[pc] = target
	}
	return result, true
}

// Shared immutable typed frames and value origins let source proofs ask about
// operands without mutating the legacy decompiler's locals or inferred types.
// All callers retain the same frame-allocation and analysis-update bounds.
func (c *ClassObjectDumper) nativeOriginalMethodSnapshot(method *MemberInfo, code *CodeAttribute) (*methodir.MethodIR, *ssabuild.Function, bool) {
	if c == nil || c.obj == nil || method == nil || code == nil || len(code.Code) == 0 || len(code.Code) > 65535 {
		return nil, nil, false
	}
	// Bound the frame copies before building a snapshot, in addition to the
	// shared analysis counter. A failed proof leaves the original flat source.
	allocation := int64(len(code.Code)+1) * int64(int(code.MaxLocals)+int(code.MaxStack)+1) * 64
	if allocation > 64<<20 || c.Work != nil && c.Work.CheckAlloc(allocation) != nil {
		return nil, nil, false
	}
	name, nk := sourceBridgeUTF8(c.obj, method.NameIndex)
	desc, dk := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
	if !nk || !dk {
		return nil, nil, false
	}
	d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(c.obj.ConstantPool, i) })
	d.Work = c.Work
	d.ConstantPoolLiteralGetter = func(i int) values.JavaValue { return GetLiteralFromCP(c.obj.ConstantPool, i) }
	d.ConstantPoolInvokeDynamicInfo = func(index int) (uint16, string, string) {
		if index <= 0 || index > len(c.obj.ConstantPool) {
			return 0, "", ""
		}
		dynamic, ok := c.obj.ConstantPool[index-1].(*ConstantInvokeDynamicInfo)
		if !ok || dynamic == nil || dynamic.NameAndTypeIndex == 0 || int(dynamic.NameAndTypeIndex) > len(c.obj.ConstantPool) {
			return 0, "", ""
		}
		table, ok := c.obj.ConstantPool[dynamic.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
		if !ok || table == nil {
			return 0, "", ""
		}
		name, nk := sourceBridgeUTF8(c.obj, table.NameIndex)
		descriptor, dk := sourceBridgeUTF8(c.obj, table.DescriptorIndex)
		if !nk || !dk {
			return 0, "", ""
		}
		return dynamic.BootstrapMethodAttrIndex, name, descriptor
	}
	limits := core.CodeLimits{Present: true, MaxLocals: int(code.MaxLocals), MaxStack: int(code.MaxStack), DirectSuperClass: c.obj.GetSupperClassName()}
	d.CodeLimits = limits
	for _, h := range code.ExceptionTable {
		if h == nil {
			return nil, nil, false
		}
		d.ExceptionTable = append(d.ExceptionTable, &core.ExceptionTableEntry{StartPc: h.StartPc, EndPc: h.EndPc, HandlerPc: h.HandlerPc, CatchType: h.CatchType})
	}
	ir, err := methodir.BuildFromBytes(code.Code, d.ExceptionTable, methodir.MethodMeta{Limits: limits, ClassName: c.obj.GetClassName(), Name: name, Descriptor: desc, Bytecode: code.Code, IsStatic: method.AccessFlags&8 != 0}, d)
	if err != nil {
		return nil, nil, false
	}
	counter := &shadowAnalysisCounter{limit: defaultShadowAnalysisUpdates, budget: c.Work}
	fn, err := ssabuild.Build(ir, ssabuild.Options{MaxUpdates: defaultShadowAnalysisUpdates, Counter: counter})
	if err != nil {
		return nil, nil, false
	}
	return ir, fn, true
}

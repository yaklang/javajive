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
}

// Nested NEW expressions can have identical nominal owners. Match a call to
// its original uninitialized NEW site, never to the first same-owner Methodref.
// This immutable typed snapshot changes no evaluation, capture or handler.
// The source renderer must independently match its own NEW/invoke origins.
func (c *ClassObjectDumper) nativeMemberAllocationInvocations(method *MemberInfo, code *CodeAttribute) (map[int]nativeMemberAllocationInvocation, bool) {
	if c == nil || c.obj == nil || method == nil || code == nil || len(code.Code) == 0 || len(code.Code) > 65535 {
		return nil, false
	}
	// Bound the frame copies before building a snapshot, in addition to the
	// shared analysis counter. A failed proof leaves the original flat source.
	allocation := int64(len(code.Code)+1) * int64(int(code.MaxLocals)+int(code.MaxStack)+1) * 64
	if allocation > 64<<20 || c.Work != nil && c.Work.CheckAlloc(allocation) != nil {
		return nil, false
	}
	name, nk := sourceBridgeUTF8(c.obj, method.NameIndex)
	desc, dk := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
	if !nk || !dk {
		return nil, false
	}
	d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(c.obj.ConstantPool, i) })
	d.Work = c.Work
	d.ConstantPoolLiteralGetter = func(i int) values.JavaValue { return GetLiteralFromCP(c.obj.ConstantPool, i) }
	limits := core.CodeLimits{Present: true, MaxLocals: int(code.MaxLocals), MaxStack: int(code.MaxStack), DirectSuperClass: c.obj.GetSupperClassName()}
	d.CodeLimits = limits
	for _, h := range code.ExceptionTable {
		if h == nil {
			return nil, false
		}
		d.ExceptionTable = append(d.ExceptionTable, &core.ExceptionTableEntry{StartPc: h.StartPc, EndPc: h.EndPc, HandlerPc: h.HandlerPc, CatchType: h.CatchType})
	}
	ir, err := methodir.BuildFromBytes(code.Code, d.ExceptionTable, methodir.MethodMeta{Limits: limits, ClassName: c.obj.GetClassName(), Name: name, Descriptor: desc, Bytecode: code.Code, IsStatic: method.AccessFlags&8 != 0}, d)
	if err != nil {
		return nil, false
	}
	counter := &shadowAnalysisCounter{limit: defaultShadowAnalysisUpdates, budget: c.Work}
	fn, err := ssabuild.Build(ir, ssabuild.Options{MaxUpdates: defaultShadowAnalysisUpdates, Counter: counter})
	if err != nil {
		return nil, false
	}
	result := map[int]nativeMemberAllocationInvocation{}
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
	}
	return result, true
}

package javaclassparser

import (
	"bytes"
	"slices"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// Reconnect a printer-spilled, uncaptured factory to its original field
// expression. The original SSA result must have exactly one consumer, either
// the field store or one argument of its immediately consumed static call.
// No JVM local store, branch, handler, shared value or opaque source operand
// borrows this proof. The source call and earlier argument order stay fixed.
func (c *ClassObjectDumper) interfaceInitializerLambdaExpression(plan *interfaceInitializerSegment, code *CodeAttribute) (values.JavaValue, bool) {
	if c == nil || c.obj == nil || c.CurrentMethod == nil || code == nil || plan == nil || len(plan.prefix) != 1 || len(code.ExceptionTable) != 0 {
		return nil, false
	}
	assignment, ok := plan.prefix[0].(*statements.AssignStatement)
	if !ok || assignment == nil || assignment.ArrayMember != nil {
		return nil, false
	}
	local, ok := values.UnpackSoltValue(assignment.LeftValue).(*values.JavaRef)
	if !ok || local == nil || local.Id == nil || local.IsThis || local.CustomValue != nil || local.StackVar != nil {
		return nil, false
	}
	factory, ok := values.UnpackSoltValue(assignment.JavaValue).(*values.CustomValue)
	if !ok || factory == nil || factory.Flag != "lambda" || !factory.NoOuterCapture {
		return nil, false
	}
	pc, captures, known := factory.OriginalLambdaFactoryEvaluation()
	if !known || len(captures) != 0 || pc < 0 || pc > 65535 || pc >= plan.write.pc || plan.write.pc > 65535 {
		return nil, false
	}
	// A completed original body request, not the CustomValue's text/type
	// callback, owns this factory's source reconstruction.
	closedFactory := false
	for method, receipt := range c.lambdaMethodSourceBodies {
		if !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		if receipt == nil || receipt.request.factory == nil || receipt.request.factory.method != c.CurrentMethod || receipt.request.factory.code != code || receipt.request.factory.pc != pc {
			continue
		}
		if closedFactory || !receipt.request.factory.sealed || !receipt.request.factory.closed() || !c.lambdaMethodSourcePacketClosed(method, receipt.body, c.Work) {
			return nil, false
		}
		closedFactory = true
	}
	var reference *interfaceInitializerMethodReference
	if !closedFactory {
		reference = c.interfaceInitializerOriginalMethodReference(factory, code, pc)
	}
	if !closedFactory && reference == nil {
		return nil, false
	}
	consumerPC := plan.write.pc
	consumerPosition := 0
	var call *values.FunctionCallExpression
	var replacement values.JavaValue
	sameLocal := func(v values.JavaValue) bool {
		ref, ok := values.UnpackSoltValue(v).(*values.JavaRef)
		return ok && ref != nil && values.SameLocal(ref, local)
	}
	if sameLocal(plan.value) {
		replacement = assignment.JavaValue
	} else {
		call, ok = values.UnpackSoltValue(plan.value).(*values.FunctionCallExpression)
		if !ok || call == nil || call.Kind != values.InvokeStatic || !call.HasOriginPC || call.OriginPC <= pc || call.OriginPC >= plan.write.pc {
			return nil, false
		}
		position := -1
		for i, argument := range call.Arguments {
			if sameLocal(argument) {
				if position >= 0 {
					return nil, false
				}
				position = i
				continue
			}
			effects, references := values.InspectValue(argument)
			if effects&values.EffectOpaque != 0 {
				return nil, false
			}
			for ref := range references {
				if values.SameLocal(ref, local) {
					return nil, false
				}
			}
			if position < 0 && effects != 0 {
				return nil, false
			}
		}
		if position < 0 {
			return nil, false
		}
		copy := *call
		copy.Arguments = slices.Clone(call.Arguments)
		copy.Arguments[position] = assignment.JavaValue
		replacement, consumerPC, consumerPosition = &copy, call.OriginPC, position
	}
	ir, frames, known := c.nativeOriginalMethodSnapshot(c.CurrentMethod, code)
	if !known {
		return nil, false
	}
	origin := ssabuild.Origin{Kind: ssabuild.OriginInstr, PC: uint16(pc)}
	uses, resultUses, foundFactory, foundConsumer, foundStore := 0, 0, false, false, false
	for _, record := range frames.Instructions {
		if !nativeProofWork(c.Work, int64(len(record.Uses))+1) {
			return nil, false
		}
		ins, present := ir.InstrByID(methodir.InstrID(record.PC))
		if !present {
			return nil, false
		}
		for _, operand := range record.Uses {
			if call != nil && operand == (ssabuild.Origin{Kind: ssabuild.OriginInstr, PC: uint16(consumerPC)}) {
				resultUses++
				if int(record.PC) != plan.write.pc {
					return nil, false
				}
			}
			if operand == origin {
				uses++
				if int(record.PC) != consumerPC {
					return nil, false
				}
				foundConsumer = true
			}
		}
		if int(record.PC) == pc {
			parameters, _, err := callbinding.Descriptor(ins.Desc)
			if ins.Opcode != core.OP_INVOKEDYNAMIC || err != nil || len(parameters) != 0 || len(record.Uses) != 0 || len(record.Results) != 1 || record.Results[0] != origin {
				return nil, false
			}
			foundFactory = true
		}
		if call != nil && int(record.PC) == consumerPC {
			parameters, result, err := callbinding.Descriptor(ins.Desc)
			if ins.Opcode != core.OP_INVOKESTATIC || ins.Class != normalizedInitializerOwner(call.ClassName) || ins.Member != call.FunctionName || ins.Desc != call.Descriptor || err != nil || result != plan.write.descriptor || len(record.Uses) != len(parameters) || consumerPosition >= len(record.Uses) || record.Uses[consumerPosition] != origin {
				return nil, false
			}
		}
		if int(record.PC) == plan.write.pc {
			want := origin
			if call != nil {
				want = ssabuild.Origin{Kind: ssabuild.OriginInstr, PC: uint16(consumerPC)}
			}
			if ins.Opcode != core.OP_PUTSTATIC || ins.Class != c.obj.GetClassName() || ins.Member != plan.write.name || ins.Desc != plan.write.descriptor || len(record.Uses) != 1 || record.Uses[0] != want {
				return nil, false
			}
			foundStore = true
		}
	}
	for _, edge := range ir.Edges {
		if !nativeProofWork(c.Work, 1) || int(edge.To) > pc && int(edge.To) <= plan.write.pc && edge.Kind != core.EdgeFallthrough || int(edge.From) >= pc && int(edge.From) < plan.write.pc && edge.Kind != core.EdgeFallthrough {
			return nil, false
		}
	}
	closed := uses == 1 && (call == nil || resultUses == 1) && foundFactory && foundConsumer && foundStore
	if closed {
		plan.reference = reference
	}
	return replacement, closed
}

// An interface's actual field writers retain its original clinit occurrence
// owner. This receipt does not invent a source static block or move a factory
// into a helper (which would change its generated implementation identity).
type interfaceLambdaInitializerReceipt struct {
	parent        *dumpedMethods
	code          *CodeAttribute
	codeBytes     []byte
	maxStack      uint16
	maxLocals     uint16
	names         []string
	namesSeal     []string
	expressions   map[string]string
	fields        []string
	fieldSeal     []string
	references    []*interfaceInitializerMethodReference
	referenceSeal []*interfaceInitializerMethodReference
}

func newInterfaceLambdaInitializerReceipt(parent *dumpedMethods, code *CodeAttribute, names []string, expressions map[string]string) *interfaceLambdaInitializerReceipt {
	copy := map[string]string{}
	for name, value := range expressions {
		copy[name] = strings.Clone(value)
	}
	return &interfaceLambdaInitializerReceipt{parent: parent, code: code, codeBytes: bytes.Clone(code.Code), maxStack: code.MaxStack, maxLocals: code.MaxLocals, names: slices.Clone(names), namesSeal: slices.Clone(names), expressions: copy}
}

func (c *ClassObjectDumper) interfaceLambdaInitializerClosed(parent *dumpedMethods) bool {
	if c == nil || c.obj == nil || c.obj.AccessFlags&0x200 == 0 {
		return false
	}
	p := c.interfaceLambdaInitializer
	if p == nil || parent == nil || p.parent != parent || parent.member == nil || parent.sourceDumper != c || parent.sourceObject != c.obj || parent.sourceRequest.role != methodSourceDeclaration || !parent.sourceRequest.valid() || parent.sourceReconstructionFailed() || c.dumpedMethodsSet[parent.sourceCacheKey] != parent || p.code == nil || p.code.MaxStack != p.maxStack || p.code.MaxLocals != p.maxLocals || len(p.code.ExceptionTable) != 0 || !bytes.Equal(p.code.Code, p.codeBytes) || len(p.names) == 0 || len(p.names) > 128 || !slices.Equal(p.names, p.namesSeal) || len(p.names) != len(p.expressions) || len(p.fields) != len(p.names) || !slices.Equal(p.fields, p.fieldSeal) || !parent.lambdaEmission.closed() || len(parent.lambdaEmission.children) == 0 || len(c.interfaceInitializerHelpers) != 0 || len(c.obj.Methods) > 4096 || !nativeProofWork(c.Work, int64(len(p.codeBytes))+int64(len(p.names))+int64(len(c.obj.Methods))+1) {
		return false
	}
	name, named := sourceBridgeUTF8(c.obj, parent.member.NameIndex)
	desc, typed := sourceBridgeUTF8(c.obj, parent.member.DescriptorIndex)
	if !named || !typed || name != "<clinit>" || desc != "()V" || parent.member.AccessFlags != 8 {
		return false
	}
	owned := 0
	for _, method := range c.obj.Methods {
		if method == parent.member {
			owned++
		}
	}
	if owned != 1 || len(parent.member.Attributes) != 1 || parent.member.Attributes[0] != p.code {
		return false
	}
	if len(p.references) > len(p.names) || !slices.Equal(p.references, p.referenceSeal) {
		return false
	}
	for _, reference := range p.references {
		if reference == nil || reference.factory == nil || reference.factory.method != parent.member || reference.factory.code != p.code || !reference.closed(c) {
			return false
		}
	}
	for _, name := range p.names {
		if p.expressions[name] == "" || c.fieldDefaultValue[name] != p.expressions[name] {
			return false
		}
	}
	return true
}

func (c *ClassObjectDumper) bindInterfaceLambdaInitializerFields(fields []dumpedFields) bool {
	p := c.interfaceLambdaInitializer
	if p == nil {
		return false
	}
	var packets []string
	for _, field := range fields {
		if _, present := p.expressions[field.fieldName]; !present {
			continue
		}
		index := len(packets)
		if index >= len(p.names) || field.fieldName != p.names[index] || field.code == "" || !nativeProofWork(c.Work, int64(len(field.code))+1) || c.Work != nil && c.Work.CheckAlloc(int64(len(field.code))*2+int64(len(p.names))*64) != nil {
			return false
		}
		packets = append(packets, strings.Clone(field.code))
	}
	if len(p.fields) != 0 {
		return slices.Equal(packets, p.fields) && c.interfaceLambdaInitializerClosed(p.parent)
	}
	p.fields = packets
	p.fieldSeal = slices.Clone(p.fields)
	return c.interfaceLambdaInitializerClosed(p.parent)
}

func (c *ClassObjectDumper) interfaceLambdaInitializerSourceSpan(source string, parent *dumpedMethods) (string, int, bool) {
	if !c.interfaceLambdaInitializerClosed(parent) || len(source) > 16<<20 || !nativeProofWork(c.Work, int64(len(source))+1) {
		return "", 0, false
	}
	open := javaIndexTopBrace(source)
	end := -1
	if open >= 0 {
		end = javaMatchBrace(source, open)
	}
	if open < 0 || end < 0 || strings.TrimSpace(source[end+1:]) != "" {
		return "", 0, false
	}
	nodes, known := nativeMemberLayoutDeclarations(source[open+1 : end])
	if !known || len(nodes) > 4096 {
		return "", 0, false
	}
	first, last, next, offset := -1, -1, 0, open+1
	scanned := int64(len(source))
	for _, node := range nodes {
		cost := int64(len(node)) * int64(len(c.interfaceLambdaInitializer.fields)) * 2
		if cost > (128<<20)-scanned || !nativeProofWork(c.Work, cost+1) {
			return "", 0, false
		}
		scanned += cost
		for i, field := range c.interfaceLambdaInitializer.fields {
			if !strings.Contains(node, field) {
				continue
			}
			if i != next || strings.Count(node, field) != 1 {
				return "", 0, false
			}
			if first < 0 {
				first = offset
			}
			last, next = offset+len(node), next+1
		}
		offset += len(node)
	}
	if next != len(c.interfaceLambdaInitializer.fields) || first < 0 {
		return "", 0, false
	}
	return source[first:last], first, true
}

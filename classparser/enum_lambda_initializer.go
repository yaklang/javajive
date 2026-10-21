package javaclassparser

import (
	"bytes"
	"slices"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
)

// Enum constant arguments belong to the original <clinit> even when lifting
// the constants leaves no source static block. Retain its completed factory
// frame and the actual constant writers. A helper name or text match alone
// cannot substitute for the original enum allocation/constructor/store proof.
type enumLambdaInitializerReceipt struct {
	parent              *dumpedMethods
	code                *CodeAttribute
	codeBytes           []byte
	maxStack, maxLocals uint16
	names               []string
	expressions         map[string]string
	methods, fields     []*MemberInfo
	packets, packetSeal []string
}

type enumLambdaConstantPacket struct{ name, source string }

func (c *ClassObjectDumper) recordEnumLambdaInitializer(parent *dumpedMethods) bool {
	if c == nil || c.obj == nil || c.obj.AccessFlags&0x4000 == 0 || c.obj.GetSupperClassName() != "java/lang/Enum" || parent == nil || parent.member == nil || !parent.lambdaEmission.closed() || len(parent.lambdaEmission.children) == 0 {
		return false
	}
	name, nk := sourceBridgeUTF8(c.obj, parent.member.NameIndex)
	descriptor, dk := sourceBridgeUTF8(c.obj, parent.member.DescriptorIndex)
	if !nk || !dk || name != "<clinit>" || descriptor != "()V" || parent.member.AccessFlags != 8 {
		return false
	}
	plans, err := c.nativeEnumConstantInitializations()
	if err != nil || len(plans) == 0 || len(plans) > 4096 {
		return false
	}
	names := make([]string, len(plans))
	var code *CodeAttribute
	for _, attribute := range parent.member.Attributes {
		if body, ok := attribute.(*CodeAttribute); ok {
			if body == nil || code != nil {
				return false
			}
			code = body
		}
	}
	if code == nil {
		return false
	}
	cost := int64(len(code.Code)) + int64(len(plans))*256 + int64(len(c.obj.Methods)+len(c.obj.Fields))*16
	for name, plan := range plans {
		if plan.constant != name || plan.ordinal < 0 || plan.ordinal >= len(names) || names[plan.ordinal] != "" || plan.allocatedClass != c.obj.GetClassName() || !class_context.PreservesJava8Identifier(name) {
			return false
		}
		names[plan.ordinal] = name
		cost += int64(len(c.fieldDefaultValue[name])) * 4
	}
	if cost > 16<<20 || !nativeProofWork(c.Work, cost+1) || c.Work != nil && c.Work.CheckAlloc(cost) != nil {
		return false
	}
	for _, child := range parent.lambdaEmission.children {
		f := child.request.factory
		if child.body == nil || !f.closed() || !f.sealed || f.object != c.obj || f.method != parent.member || f.code != code || child.request.captureCount != 0 || child.body.lambdaEmission != nil && len(child.body.lambdaEmission.children) != 0 {
			return false
		}
		owners := 0
		for _, plan := range plans {
			if !nativeProofWork(c.Work, 1) {
				return false
			}
			if f.pc > plan.newPC && f.pc < plan.invokePC {
				owners++
			}
		}
		if owners != 1 {
			return false
		}
	}
	expressions := make(map[string]string, len(names))
	for _, name := range names {
		if c.fieldDefaultValue[name] == "" {
			return false
		}
		expressions[name] = strings.Clone(c.fieldDefaultValue[name])
	}
	parent.enumLambdaInitializer = &enumLambdaInitializerReceipt{parent: parent, code: code, codeBytes: bytes.Clone(code.Code), maxStack: code.MaxStack, maxLocals: code.MaxLocals, names: names, expressions: expressions, methods: slices.Clone(c.obj.Methods), fields: slices.Clone(c.obj.Fields)}
	return true
}

func (c *ClassObjectDumper) bindEnumLambdaInitializerPackets(methods []*dumpedMethods, fields []enumLambdaConstantPacket) bool {
	for _, parent := range methods {
		if parent == nil || parent.enumLambdaInitializer == nil {
			continue
		}
		p := parent.enumLambdaInitializer
		if len(fields) != len(p.names) || !nativeProofWork(c.Work, int64(len(fields))+1) {
			return false
		}
		cost := int64(len(fields)) * 64
		for _, field := range fields {
			cost += int64(len(field.source)) * 2
		}
		if cost > 16<<20 || c.Work != nil && c.Work.CheckAlloc(cost) != nil {
			return false
		}
		packets := make([]string, len(fields))
		for i, field := range fields {
			if field.name != p.names[i] || field.source == "" || !nativeProofWork(c.Work, int64(len(field.source))+1) {
				return false
			}
			packets[i] = strings.Clone(field.source)
		}
		if len(p.packets) != 0 {
			if !slices.Equal(packets, p.packets) {
				return false
			}
		} else {
			p.packets, p.packetSeal = packets, slices.Clone(packets)
		}
		if !c.enumLambdaInitializerClosed(parent) {
			return false
		}
	}
	return true
}

func (c *ClassObjectDumper) enumLambdaInitializerClosed(parent *dumpedMethods) bool {
	if c == nil || c.obj == nil || parent == nil || parent.enumLambdaInitializer == nil || c.obj.AccessFlags&0x4000 == 0 || c.obj.GetSupperClassName() != "java/lang/Enum" {
		return false
	}
	p := parent.enumLambdaInitializer
	if p.parent != parent || parent.member == nil || parent.sourceDumper != c || parent.sourceObject != c.obj || parent.sourceRequest.role != methodSourceDeclaration || !parent.sourceRequest.valid() || parent.sourceReconstructionFailed() || c.dumpedMethodsSet[parent.sourceCacheKey] != parent || !parent.lambdaEmission.closed() || parent.lambdaEmission.body != parent || parent.lambdaEmission.request != parent.sourceRequest || len(parent.lambdaEmission.children) == 0 || p.code == nil || p.code.MaxStack != p.maxStack || p.code.MaxLocals != p.maxLocals || !bytes.Equal(p.code.Code, p.codeBytes) || len(p.names) == 0 || len(p.names) != len(p.expressions) || len(p.names) != len(p.packets) || !slices.Equal(p.packets, p.packetSeal) || !slices.Equal(p.methods, c.obj.Methods) || !slices.Equal(p.fields, c.obj.Fields) || !nativeProofWork(c.Work, int64(len(p.codeBytes)+len(c.obj.Methods)+len(c.obj.Fields))+1) {
		return false
	}
	name, named := sourceBridgeUTF8(c.obj, parent.member.NameIndex)
	descriptor, typed := sourceBridgeUTF8(c.obj, parent.member.DescriptorIndex)
	if !named || !typed || name != "<clinit>" || descriptor != "()V" || parent.member.AccessFlags != 8 {
		return false
	}
	owned, codes, constants := 0, 0, 0
	for _, method := range c.obj.Methods {
		if method == parent.member {
			owned++
		}
	}
	for _, attribute := range parent.member.Attributes {
		if body, ok := attribute.(*CodeAttribute); ok {
			if body != p.code {
				return false
			}
			codes++
		}
	}
	for _, field := range c.obj.Fields {
		if field == nil {
			return false
		}
		if field.AccessFlags&0x4000 == 0 {
			continue
		}
		name, nk := sourceBridgeUTF8(c.obj, field.NameIndex)
		descriptor, dk := sourceBridgeUTF8(c.obj, field.DescriptorIndex)
		if constants >= len(p.names) || !nk || !dk || field.AccessFlags != 0x4019 || descriptor != "L"+c.obj.GetClassName()+";" || name != p.names[constants] || c.fieldDefaultValue[name] != p.expressions[name] {
			return false
		}
		constants++
	}
	return owned == 1 && codes == 1 && constants == len(p.names)
}

func (c *ClassObjectDumper) enumLambdaInitializerSourceSpan(source string, parent *dumpedMethods) (string, int, bool) {
	if !c.enumLambdaInitializerClosed(parent) || len(source) > 16<<20 || !nativeProofWork(c.Work, int64(len(source))+1) || c.Work != nil && c.Work.CheckAlloc(int64(len(source))) != nil {
		return "", 0, false
	}
	start := javaIndexTopBrace(source) + 1
	if start <= 0 {
		return "", 0, false
	}
	for start < len(source) {
		if strings.ContainsRune(" \t\r\n", rune(source[start])) {
			start++
			continue
		}
		if start+1 < len(source) && source[start] == '/' && (source[start+1] == '/' || source[start+1] == '*') {
			next, known := skipJavaNonCode(source, start)
			if !known || next <= start {
				return "", 0, false
			}
			start = next
			continue
		}
		break
	}
	packet := strings.TrimLeft(strings.Join(parent.enumLambdaInitializer.packets, ""), " \t\r\n")
	if !strings.HasPrefix(source[start:], packet) {
		return "", 0, false
	}
	return source[start : start+len(packet)], start, true
}

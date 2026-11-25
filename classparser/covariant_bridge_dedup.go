package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// isStrippedCovariantBridge reports whether an instance method without ACC_BRIDGE
// (e.g., stripped by D8/R8) is a redundant wider-return covariant bridge of another
// instance method in the same class sharing the exact name and parameter descriptor.
func (c *ClassObjectDumper) isStrippedCovariantBridge(method *MemberInfo, name, descriptor string) bool {
	if c == nil || c.obj == nil || method == nil {
		return false
	}
	end := strings.IndexByte(descriptor, ')')
	if end < 0 {
		return false
	}
	paramPart := descriptor[:end+1]
	retPart := descriptor[end+1:]
	if !isReferenceReturnDescriptor(retPart) {
		return false
	}
	for _, other := range c.obj.Methods {
		if other == nil || other == method || other.AccessFlags&StaticFlag != 0 {
			continue
		}
		otherName, nk := sourceBridgeUTF8(c.obj, other.NameIndex)
		if !nk || otherName != name {
			continue
		}
		otherDesc, dk := sourceBridgeUTF8(c.obj, other.DescriptorIndex)
		if !dk {
			continue
		}
		otherEnd := strings.IndexByte(otherDesc, ')')
		if otherEnd < 0 || otherDesc[:otherEnd+1] != paramPart {
			continue
		}
		otherRet := otherDesc[otherEnd+1:]
		if otherRet == retPart || !isReferenceReturnDescriptor(otherRet) {
			continue
		}
		if c.isStrictReferenceSubtype(otherRet, retPart) {
			return true
		}
		if c.isForwardingCovariantBridgeBytecode(method, name, otherDesc) &&
			!c.isForwardingCovariantBridgeBytecode(other, name, descriptor) {
			return true
		}
	}
	return false
}

func isReferenceReturnDescriptor(desc string) bool {
	return len(desc) >= 2 && (desc[0] == 'L' && desc[len(desc)-1] == ';' || desc[0] == '[')
}

func (c *ClassObjectDumper) isStrictReferenceSubtype(subDesc, superDesc string) bool {
	for strings.HasPrefix(subDesc, "[") && strings.HasPrefix(superDesc, "[") {
		subDesc = subDesc[1:]
		superDesc = superDesc[1:]
		if !isReferenceReturnDescriptor(subDesc) || !isReferenceReturnDescriptor(superDesc) {
			return false
		}
	}
	if subDesc == superDesc {
		return false
	}
	if superDesc == "Ljava/lang/Object;" {
		return true
	}
	if strings.HasPrefix(subDesc, "[") {
		return superDesc == "Ljava/lang/Cloneable;" || superDesc == "Ljava/io/Serializable;"
	}
	if !strings.HasPrefix(subDesc, "L") || !strings.HasSuffix(subDesc, ";") ||
		!strings.HasPrefix(superDesc, "L") || !strings.HasSuffix(superDesc, ";") {
		return false
	}
	subName := subDesc[1 : len(subDesc)-1]
	superName := superDesc[1 : len(superDesc)-1]

	type node struct {
		name  string
		depth int
	}
	visited := map[string]bool{subName: true}
	queue := []node{{name: subName, depth: 0}}
	const maxDepth = 16
	const maxVisited = 64

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.depth >= maxDepth || !nativeProofWork(c.Work, 1) {
			continue
		}
		for _, parent := range c.directSupertypesForDedup(cur.name) {
			if parent == "" || parent == cur.name {
				continue
			}
			if parent == superName {
				return true
			}
			if !visited[parent] && len(visited) < maxVisited {
				visited[parent] = true
				queue = append(queue, node{name: parent, depth: cur.depth + 1})
			}
		}
	}
	return false
}

func (c *ClassObjectDumper) directSupertypesForDedup(internalName string) []string {
	if c.obj != nil && internalName == c.obj.GetClassName() {
		parents := make([]string, 0, 1+len(c.obj.Interfaces))
		if sup := c.obj.GetSupperClassName(); sup != "" {
			parents = append(parents, sup)
		}
		for _, iface := range c.obj.GetInterfacesName() {
			if iface != "" {
				parents = append(parents, iface)
			}
		}
		return parents
	}
	if c.FuncCtx != nil && c.FuncCtx.SiblingSuperTypes != nil {
		if parents, ok := c.FuncCtx.SiblingSuperTypes(internalName); ok {
			return parents
		}
	}
	// DEX-derived classfiles often carry MajorVersion 49/50, outside the JDK 8+
	// catalog map, so probe release 8 then 21 explicitly for platform types.
	for _, release := range []int{8, 21} {
		if parents, ok := jdkReferenceSupertypes(internalName, release); ok {
			return parents
		}
	}
	return nil
}

func (c *ClassObjectDumper) isForwardingCovariantBridgeBytecode(method *MemberInfo, name, targetDesc string) bool {
	if c == nil || c.obj == nil || method == nil {
		return false
	}
	var code *CodeAttribute
	for _, attr := range method.Attributes {
		if ca, ok := attr.(*CodeAttribute); ok && ca != nil {
			code = ca
			break
		}
	}
	if code == nil || len(code.ExceptionTable) != 0 || len(code.Code) == 0 || len(code.Code) > 64 {
		return false
	}
	decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(c.obj.ConstantPool, i) })
	decoder.Work = c.Work
	if decoder.ParseOpcode() != nil {
		return false
	}
	ops := constructorMotionOps(decoder)
	if len(ops) < 2 || ops[len(ops)-1].Instr.OpCode != core.OP_ARETURN {
		return false
	}
	selfClass := c.obj.GetClassName()
	selfSuper := c.obj.GetSupperClassName()
	invokeIdx := -1
	for i := 0; i < len(ops)-1; i++ {
		opcode := ops[i].Instr.OpCode
		switch {
		case opcode >= core.OP_ILOAD && opcode <= core.OP_ALOAD_3:
			continue
		case opcode == core.OP_CHECKCAST:
			if invokeIdx < 0 || i != len(ops)-2 {
				return false
			}
			continue
		case opcode == core.OP_INVOKEVIRTUAL, opcode == core.OP_INVOKESPECIAL, opcode == core.OP_INVOKEINTERFACE:
			if invokeIdx >= 0 {
				return false
			}
			ref := constructorMotionMember(c.obj, ops[i], opcode)
			if ref == nil || ref.Member != name || ref.Description != targetDesc {
				return false
			}
			if ref.Name != selfClass && ref.Name != selfSuper {
				return false
			}
			invokeIdx = i
		default:
			return false
		}
	}
	return invokeIdx >= 0 && (invokeIdx == len(ops)-2 || invokeIdx == len(ops)-3)
}

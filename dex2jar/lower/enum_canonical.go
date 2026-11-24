package lower

import (
	"sort"
	"strings"

	"github.com/yaklang/javajive/classparser/classfilebuilder"
	"github.com/yaklang/javajive/dex2jar/ir"
	"github.com/yaklang/javajive/dex2jar/model"
)

const (
	jvmSyntheticBackingFlags = uint16(classfilebuilder.ACC_PRIVATE | classfilebuilder.ACC_STATIC | classfilebuilder.ACC_FINAL | classfilebuilder.ACC_SYNTHETIC)
	jvmSyntheticHelperFlags  = uint16(classfilebuilder.ACC_PRIVATE | classfilebuilder.ACC_STATIC | classfilebuilder.ACC_SYNTHETIC)
	jvmEnumConstantFlags     = uint16(0x4019) // public static final enum
	jvmEnumFactoryFlags      = uint16(classfilebuilder.ACC_PUBLIC | classfilebuilder.ACC_STATIC)
	jvmEnumFactoryFinalFlags = uint16(classfilebuilder.ACC_PUBLIC | classfilebuilder.ACC_STATIC | classfilebuilder.ACC_FINAL)
)

type canonicalClinitPlan struct {
	startIdx     int
	endIdx       int
	skipSet      map[int]bool
	backingField string
	helperName   string
	constants    []string
	entriesField string
}

type pureConstKind uint8

const (
	pureNone pureConstKind = iota
	pure32Bits
	pure64Bits
	pureString
	pureClass
)

type pureConstVal struct {
	kind pureConstKind
	bits int64
	text string
}

type canonicalConstArg struct {
	paramDesc string
	kind      pureConstKind
	bits      int64
	text      string
}

type canonicalEnumConstSpan struct {
	startIdx  int
	allocDesc string
	ctorProto model.Proto
	args      []canonicalConstArg
	fieldName string
}

func enumBackingField(class *model.Class) (string, bool) {
	if class == nil {
		return "", false
	}
	wantType := "[" + class.Descriptor
	backingName := ""
	count := 0
	for i := range class.StaticFields {
		sf := &class.StaticFields[i]
		if sf.Field.Class != class.Descriptor || sf.Field.Type != wantType {
			continue
		}
		acc, err := fieldAccess(sf.AccessFlags)
		if err != nil || acc != jvmSyntheticBackingFlags {
			continue
		}
		if len(fieldAnnotationsOf(class, sf.FieldIndex)) != 0 {
			continue
		}
		backingName = sf.Field.Name
		count++
	}
	if count != 1 || backingName == "" {
		return "", false
	}
	return backingName, true
}

func orderedEnumConstantNames(file *model.File, class *model.Class, hierarchy map[string]*model.Class) ([]string, bool) {
	if file == nil || class == nil {
		return nil, false
	}
	var names []string
	for i := range class.StaticFields {
		sf := &class.StaticFields[i]
		if sf.AccessFlags&dexEnum == 0 {
			continue
		}
		acc, err := fieldAccess(sf.AccessFlags)
		if err != nil || acc != jvmEnumConstantFlags || sf.Field.Type != class.Descriptor || sf.Field.Class != class.Descriptor {
			return nil, false
		}
		names = append(names, sf.Field.Name)
	}
	if len(names) > 1 {
		if ordinals := inferEnumFieldOrdinals(file, class, hierarchy); len(ordinals) == len(names) {
			seen := make([]bool, len(names))
			valid := true
			for _, name := range names {
				ord, ok := ordinals[name]
				if !ok || ord < 0 || ord >= len(names) || seen[ord] {
					valid = false
					break
				}
				seen[ord] = true
			}
			if valid {
				sort.Slice(names, func(i, j int) bool {
					return ordinals[names[i]] < ordinals[names[j]]
				})
			}
		}
	}
	return names, true
}

func isCanonicalEnumClass(class *model.Class) bool {
	return class != nil && class.AccessFlags&dexEnum != 0 && class.Superclass == "Ljava/lang/Enum;"
}

func (l *methodLowerer) tryEmitCanonicalEnumMethod() (bool, error) {
	if !isCanonicalEnumClass(l.class) || len(l.method.Code.Tries) != 0 || len(l.method.Blocks) != 1 {
		return false, nil
	}
	access, err := methodAccess(l.encoded.AccessFlags, l.encoded.Method.Name)
	if err != nil {
		return false, nil
	}
	proto := l.method.Proto
	name := l.encoded.Method.Name
	arrDesc := "[" + l.class.Descriptor

	if name == "values" && len(proto.Parameters) == 0 && proto.ReturnType == arrDesc &&
		(access == jvmEnumFactoryFlags || access == jvmEnumFactoryFinalFlags) {
		return l.tryEmitCanonicalEnumValues()
	}
	if name == "valueOf" && len(proto.Parameters) == 1 && proto.Parameters[0] == "Ljava/lang/String;" &&
		proto.ReturnType == l.class.Descriptor && (access == jvmEnumFactoryFlags || access == jvmEnumFactoryFinalFlags) {
		return l.tryEmitCanonicalEnumValueOf()
	}
	if len(proto.Parameters) == 0 && proto.ReturnType == arrDesc &&
		(access == jvmSyntheticHelperFlags || access == jvmSyntheticBackingFlags) {
		return l.tryEmitCanonicalEnumValuesHelper()
	}
	return false, nil
}

func (l *methodLowerer) tryEmitCanonicalEnumValues() (bool, error) {
	backingName, ok := enumBackingField(l.class)
	if !ok {
		return false, nil
	}
	arrDesc := "[" + l.class.Descriptor
	block := l.method.Blocks[0]
	regBacking := make(map[uint16]bool)
	regCloned := make(map[uint16]bool)
	regCast := make(map[uint16]bool)
	clearReg := func(r uint16) {
		delete(regBacking, r)
		delete(regCloned, r)
		delete(regCast, r)
	}
	sawInvoke, sawCast, sawReturn := false, false, false
	for p := 0; p < len(block.Instructions); p++ {
		idx := block.Instructions[p]
		ins := &l.method.Instructions[idx]
		op := ins.Opcode
		switch {
		case op == 0x00:
			continue
		case (op >= 0x01 && op <= 0x03) || (op >= 0x07 && op <= 0x09):
			dst, src := ins.Regs[0], ins.Regs[1]
			b, cl, ca := regBacking[src], regCloned[src], regCast[src]
			clearReg(dst)
			if b {
				regBacking[dst] = true
			}
			if cl {
				regCloned[dst] = true
			}
			if ca {
				regCast[dst] = true
			}
		case op == 0x62:
			if sawInvoke || sawReturn || int(ins.Index) >= len(l.file.Fields) {
				return false, nil
			}
			f := l.file.Fields[ins.Index]
			if f.Class != l.class.Descriptor || f.Name != backingName || f.Type != arrDesc {
				return false, nil
			}
			clearReg(ins.Regs[0])
			regBacking[ins.Regs[0]] = true
		case op == 0x6e || op == 0x74:
			if sawInvoke || sawReturn || len(ins.Regs) != 1 || !regBacking[ins.Regs[0]] || int(ins.Index) >= len(l.file.Methods) {
				return false, nil
			}
			callee := l.file.Methods[ins.Index]
			if callee.Name != "clone" || (callee.Class != arrDesc && callee.Class != "Ljava/lang/Object;" && callee.Class != "[Ljava/lang/Object;") || int(callee.ProtoIndex) >= len(l.file.Protos) {
				return false, nil
			}
			calleeProto := l.file.Protos[callee.ProtoIndex]
			if len(calleeProto.Parameters) != 0 || calleeProto.ReturnType != "Ljava/lang/Object;" {
				return false, nil
			}
			if p+1 >= len(block.Instructions) {
				return false, nil
			}
			next := &l.method.Instructions[block.Instructions[p+1]]
			if next.Opcode != 0x0c {
				return false, nil
			}
			sawInvoke = true
			clearReg(next.Regs[0])
			regCloned[next.Regs[0]] = true
			p++
		case op == 0x1f:
			if !sawInvoke || sawCast || sawReturn || !regCloned[ins.Regs[0]] || int(ins.Index) >= len(l.file.Types) {
				return false, nil
			}
			if l.file.Types[ins.Index].Descriptor != arrDesc {
				return false, nil
			}
			clearReg(ins.Regs[0])
			regCast[ins.Regs[0]] = true
			sawCast = true
		case op == 0x11:
			if p != len(block.Instructions)-1 || !sawInvoke || !sawCast || !regCast[ins.Regs[0]] {
				return false, nil
			}
			sawReturn = true
		default:
			return false, nil
		}
	}
	if !sawReturn {
		return false, nil
	}
	ownerInternal, err := objectInternalName(l.class.Descriptor)
	if err != nil {
		return false, methodDiagnostic(l.method.Context, nil, "enum values()", err)
	}
	fieldRef, err := l.builder.Pool().Fieldref(ownerInternal, backingName, arrDesc)
	if err != nil {
		return false, methodDiagnostic(l.method.Context, nil, "enum values()", err)
	}
	cloneRef, err := l.builder.Pool().Methodref(arrDesc, "clone", "()Ljava/lang/Object;")
	if err != nil {
		return false, methodDiagnostic(l.method.Context, nil, "enum values()", err)
	}
	castClass, err := l.builder.Pool().Class(arrDesc)
	if err != nil {
		return false, methodDiagnostic(l.method.Context, nil, "enum values()", err)
	}
	l.code.CP(classfilebuilder.GETSTATIC, fieldRef)
	l.code.CP(classfilebuilder.INVOKEVIRTUAL, cloneRef)
	l.code.CP(classfilebuilder.CHECKCAST, castClass)
	l.code.Emit(classfilebuilder.ARETURN)
	l.code.SetMaxLocals(0)
	return true, nil
}

func (l *methodLowerer) tryEmitCanonicalEnumValueOf() (bool, error) {
	if l.method.Code.InsSize != 1 || l.method.Code.RegistersSize < 1 {
		return false, nil
	}
	param0 := l.method.Code.RegistersSize - l.method.Code.InsSize
	block := l.method.Blocks[0]
	regParam := map[uint16]bool{param0: true}
	regClass := make(map[uint16]bool)
	regEnum := make(map[uint16]bool)
	regCast := make(map[uint16]bool)
	clearReg := func(r uint16) {
		delete(regParam, r)
		delete(regClass, r)
		delete(regEnum, r)
		delete(regCast, r)
	}
	sawInvoke, sawCast, sawReturn := false, false, false
	for p := 0; p < len(block.Instructions); p++ {
		idx := block.Instructions[p]
		ins := &l.method.Instructions[idx]
		op := ins.Opcode
		switch {
		case op == 0x00:
			continue
		case (op >= 0x01 && op <= 0x03) || (op >= 0x07 && op <= 0x09):
			dst, src := ins.Regs[0], ins.Regs[1]
			pa, cl, en, ca := regParam[src], regClass[src], regEnum[src], regCast[src]
			clearReg(dst)
			if pa {
				regParam[dst] = true
			}
			if cl {
				regClass[dst] = true
			}
			if en {
				regEnum[dst] = true
			}
			if ca {
				regCast[dst] = true
			}
		case op == 0x1c:
			if sawInvoke || sawReturn || int(ins.Index) >= len(l.file.Types) || l.file.Types[ins.Index].Descriptor != l.class.Descriptor {
				return false, nil
			}
			clearReg(ins.Regs[0])
			regClass[ins.Regs[0]] = true
		case op == 0x71 || op == 0x77:
			if sawInvoke || sawReturn || len(ins.Regs) != 2 || !regClass[ins.Regs[0]] || !regParam[ins.Regs[1]] || int(ins.Index) >= len(l.file.Methods) {
				return false, nil
			}
			callee := l.file.Methods[ins.Index]
			if callee.Class != "Ljava/lang/Enum;" || callee.Name != "valueOf" || int(callee.ProtoIndex) >= len(l.file.Protos) {
				return false, nil
			}
			cp := l.file.Protos[callee.ProtoIndex]
			if len(cp.Parameters) != 2 || cp.Parameters[0] != "Ljava/lang/Class;" || cp.Parameters[1] != "Ljava/lang/String;" || cp.ReturnType != "Ljava/lang/Enum;" {
				return false, nil
			}
			if p+1 >= len(block.Instructions) {
				return false, nil
			}
			next := &l.method.Instructions[block.Instructions[p+1]]
			if next.Opcode != 0x0c {
				return false, nil
			}
			sawInvoke = true
			clearReg(next.Regs[0])
			regEnum[next.Regs[0]] = true
			p++
		case op == 0x1f:
			if !sawInvoke || sawCast || sawReturn || !regEnum[ins.Regs[0]] || int(ins.Index) >= len(l.file.Types) {
				return false, nil
			}
			if l.file.Types[ins.Index].Descriptor != l.class.Descriptor {
				return false, nil
			}
			clearReg(ins.Regs[0])
			regCast[ins.Regs[0]] = true
			sawCast = true
		case op == 0x11:
			if p != len(block.Instructions)-1 || !sawInvoke || !sawCast || !regCast[ins.Regs[0]] {
				return false, nil
			}
			sawReturn = true
		default:
			return false, nil
		}
	}
	if !sawReturn {
		return false, nil
	}
	ownerInternal, err := objectInternalName(l.class.Descriptor)
	if err != nil {
		return false, methodDiagnostic(l.method.Context, nil, "enum valueOf()", err)
	}
	ownerClass, err := l.builder.Pool().Class(ownerInternal)
	if err != nil {
		return false, methodDiagnostic(l.method.Context, nil, "enum valueOf()", err)
	}
	enumValueOf, err := l.builder.Pool().Methodref("java/lang/Enum", "valueOf", "(Ljava/lang/Class;Ljava/lang/String;)Ljava/lang/Enum;")
	if err != nil {
		return false, methodDiagnostic(l.method.Context, nil, "enum valueOf()", err)
	}
	l.code.LDC(ownerClass)
	l.code.Load(classfilebuilder.RefLocal, 0)
	l.code.CP(classfilebuilder.INVOKESTATIC, enumValueOf)
	l.code.CP(classfilebuilder.CHECKCAST, ownerClass)
	l.code.Emit(classfilebuilder.ARETURN)
	l.code.SetMaxLocals(1)
	return true, nil
}

func matchEnumArrayPacket(
	file *model.File,
	ownerDesc string,
	insns []ir.Instruction,
	seq []int,
	constants []string,
	preInt map[uint16]int32,
	preConst map[uint16]string,
) (arrRegs map[uint16]bool, ok bool) {
	arrDesc := "[" + ownerDesc
	constSet := make(map[string]bool, len(constants))
	for _, name := range constants {
		constSet[name] = true
	}
	intVals := make(map[uint16]int32, len(preInt))
	for k, v := range preInt {
		intVals[k] = v
	}
	constInReg := make(map[uint16]string, len(preConst))
	for k, v := range preConst {
		constInReg[k] = v
	}
	arrRegs = make(map[uint16]bool)
	clearReg := func(r uint16) {
		delete(intVals, r)
		delete(constInReg, r)
		delete(arrRegs, r)
	}
	sawArray := false
	filled := make([]bool, len(constants))
	for i := 0; i < len(seq); i++ {
		idx := seq[i]
		ins := &insns[idx]
		op := ins.Opcode
		switch {
		case op == 0x00:
			continue
		case op >= 0x12 && op <= 0x15:
			clearReg(ins.Regs[0])
			intVals[ins.Regs[0]] = int32(ins.Literal)
		case (op >= 0x01 && op <= 0x03) || (op >= 0x07 && op <= 0x09):
			dst, src := ins.Regs[0], ins.Regs[1]
			iv, hasIV := intVals[src]
			cn, hasCN := constInReg[src]
			isArr := arrRegs[src]
			clearReg(dst)
			if hasIV {
				intVals[dst] = iv
			}
			if hasCN {
				constInReg[dst] = cn
			}
			if isArr {
				arrRegs[dst] = true
			}
		case op == 0x23:
			if sawArray || int(ins.Index) >= len(file.Types) || file.Types[ins.Index].Descriptor != arrDesc {
				return nil, false
			}
			n, hasLen := intVals[ins.Regs[1]]
			if !hasLen || int(n) != len(constants) {
				return nil, false
			}
			sawArray = true
			clearReg(ins.Regs[0])
			arrRegs[ins.Regs[0]] = true
		case op == 0x24 || op == 0x25:
			if sawArray || i+1 >= len(seq) || int(ins.Index) >= len(file.Types) || file.Types[ins.Index].Descriptor != arrDesc {
				return nil, false
			}
			next := &insns[seq[i+1]]
			if next.Opcode != 0x0c || len(ins.Regs) != len(constants) {
				return nil, false
			}
			for elemIdx, reg := range ins.Regs {
				cName, hasConst := constInReg[reg]
				if !hasConst || cName != constants[elemIdx] {
					return nil, false
				}
			}
			sawArray = true
			for k := range filled {
				filled[k] = true
			}
			clearReg(next.Regs[0])
			arrRegs[next.Regs[0]] = true
			i++
		case op == 0x62:
			if int(ins.Index) >= len(file.Fields) {
				return nil, false
			}
			f := file.Fields[ins.Index]
			if f.Class != ownerDesc || f.Type != ownerDesc || !constSet[f.Name] {
				return nil, false
			}
			clearReg(ins.Regs[0])
			constInReg[ins.Regs[0]] = f.Name
		case op == 0x4d:
			if !sawArray || !arrRegs[ins.Regs[1]] {
				return nil, false
			}
			elemIdx, hasIdx := intVals[ins.Regs[2]]
			if !hasIdx || elemIdx < 0 || int(elemIdx) >= len(constants) {
				return nil, false
			}
			cName, hasConst := constInReg[ins.Regs[0]]
			if !hasConst || cName != constants[elemIdx] || filled[elemIdx] {
				return nil, false
			}
			filled[elemIdx] = true
		default:
			return nil, false
		}
	}
	if !sawArray {
		return nil, false
	}
	for _, done := range filled {
		if !done {
			return nil, false
		}
	}
	return arrRegs, true
}

func (l *methodLowerer) emitCanonicalEnumArrayBody(constants []string) error {
	ownerInternal, err := objectInternalName(l.class.Descriptor)
	if err != nil {
		return err
	}
	ownerClass, err := l.builder.Pool().Class(ownerInternal)
	if err != nil {
		return err
	}
	if err := l.pushInt(int32(len(constants))); err != nil {
		return err
	}
	l.code.CP(classfilebuilder.ANEWARRAY, ownerClass)
	for i, constName := range constants {
		constRef, err := l.builder.Pool().Fieldref(ownerInternal, constName, l.class.Descriptor)
		if err != nil {
			return err
		}
		l.code.Emit(classfilebuilder.DUP)
		if err := l.pushInt(int32(i)); err != nil {
			return err
		}
		l.code.CP(classfilebuilder.GETSTATIC, constRef)
		l.code.Emit(classfilebuilder.AASTORE)
	}
	return nil
}

func (l *methodLowerer) tryEmitCanonicalEnumValuesHelper() (bool, error) {
	constants, ok := orderedEnumConstantNames(l.file, l.class, l.hierarchy)
	if !ok {
		return false, nil
	}
	block := l.method.Blocks[0]
	if len(block.Instructions) < 2 {
		return false, nil
	}
	lastIdx := block.Instructions[len(block.Instructions)-1]
	lastInsn := &l.method.Instructions[lastIdx]
	if lastInsn.Opcode != 0x11 {
		return false, nil
	}
	arrRegs, matched := matchEnumArrayPacket(l.file, l.class.Descriptor, l.method.Instructions, block.Instructions[:len(block.Instructions)-1], constants, nil, nil)
	if !matched || !arrRegs[lastInsn.Regs[0]] {
		return false, nil
	}
	if err := l.emitCanonicalEnumArrayBody(constants); err != nil {
		return false, methodDiagnostic(l.method.Context, nil, "enum $values()", err)
	}
	l.code.Emit(classfilebuilder.ARETURN)
	l.code.SetMaxLocals(0)
	return true, nil
}

func (l *methodLowerer) detectCanonicalEnumClinitBacking() *canonicalClinitPlan {
	if !isCanonicalEnumClass(l.class) || l.encoded.Method.Name != "<clinit>" {
		return nil
	}
	backingName, ok := enumBackingField(l.class)
	if !ok {
		return nil
	}
	constants, ok := orderedEnumConstantNames(l.file, l.class, l.hierarchy)
	if !ok {
		return nil
	}
	arrDesc := "[" + l.class.Descriptor
	writeBlockIdx, writePos, writeCount := -1, -1, 0
	for bIdx, blk := range l.method.Blocks {
		if !blk.Reachable {
			continue
		}
		for pos, idx := range blk.Instructions {
			ins := &l.method.Instructions[idx]
			if ins.Opcode == 0x69 && int(ins.Index) < len(l.file.Fields) {
				f := l.file.Fields[ins.Index]
				if f.Class == l.class.Descriptor && f.Name == backingName && f.Type == arrDesc {
					writeBlockIdx = bIdx
					writePos = pos
					writeCount++
				}
			}
		}
	}
	if writeCount != 1 || writeBlockIdx < 0 || writePos < 0 {
		return nil
	}
	block := l.method.Blocks[writeBlockIdx]
	startPos := 0
	if len(constants) > 0 {
		lastConst := constants[len(constants)-1]
		lastConstPos := -1
		for p := 0; p < writePos; p++ {
			ins := &l.method.Instructions[block.Instructions[p]]
			if ins.Opcode == 0x69 && int(ins.Index) < len(l.file.Fields) {
				f := l.file.Fields[ins.Index]
				if f.Class == l.class.Descriptor && f.Name == lastConst && f.Type == l.class.Descriptor {
					lastConstPos = p
				}
			}
		}
		if lastConstPos < 0 {
			return nil
		}
		startPos = lastConstPos + 1
	} else if len(block.Predecessors) > 0 {
		return nil
	}
	if startPos >= writePos {
		return nil
	}

	constSet := make(map[string]bool, len(constants))
	for _, name := range constants {
		constSet[name] = true
	}
	preInt := make(map[uint16]int32)
	preConst := make(map[uint16]string)
	for p := 0; p < startPos; p++ {
		idx := block.Instructions[p]
		ins := &l.method.Instructions[idx]
		op := ins.Opcode
		switch {
		case op >= 0x12 && op <= 0x15:
			delete(preConst, ins.Regs[0])
			preInt[ins.Regs[0]] = int32(ins.Literal)
		case (op >= 0x01 && op <= 0x03) || (op >= 0x07 && op <= 0x09):
			dst, src := ins.Regs[0], ins.Regs[1]
			iv, hasIV := preInt[src]
			cn, hasCN := preConst[src]
			delete(preInt, dst)
			delete(preConst, dst)
			if hasIV {
				preInt[dst] = iv
			}
			if hasCN {
				preConst[dst] = cn
			}
		case op == 0x62 && int(ins.Index) < len(l.file.Fields):
			f := l.file.Fields[ins.Index]
			delete(preInt, ins.Regs[0])
			delete(preConst, ins.Regs[0])
			if f.Class == l.class.Descriptor && f.Type == l.class.Descriptor && constSet[f.Name] {
				preConst[ins.Regs[0]] = f.Name
			}
		case op == 0x69 && int(ins.Index) < len(l.file.Fields):
			f := l.file.Fields[ins.Index]
			if f.Class == l.class.Descriptor && f.Type == l.class.Descriptor && constSet[f.Name] {
				preConst[ins.Regs[0]] = f.Name
			}
		default:
			_, defs := l.method.EffectsAt(idx)
			for _, r := range defs {
				delete(preInt, r)
				delete(preConst, r)
			}
		}
	}

	helperName := ""
	backingSrcReg := l.method.Instructions[block.Instructions[writePos]].Regs[0]
	var matchedInline bool
	helperRegs := make(map[uint16]bool)
	matchedHelper := false
	// Option 1: synthetic helper call returning [LOwner;
	sawCall := false
	for p := startPos; p < writePos; p++ {
		ins := &l.method.Instructions[block.Instructions[p]]
		op := ins.Opcode
		switch {
		case op == 0x00:
			continue
		case (op == 0x71 || op == 0x77) && !sawCall && len(ins.Regs) == 0 && p+1 < writePos && int(ins.Index) < len(l.file.Methods):
			callee := l.file.Methods[ins.Index]
			next := &l.method.Instructions[block.Instructions[p+1]]
			if next.Opcode == 0x0c && callee.Class == l.class.Descriptor && int(callee.ProtoIndex) < len(l.file.Protos) {
				cp := l.file.Protos[callee.ProtoIndex]
				if len(cp.Parameters) == 0 && cp.ReturnType == arrDesc && l.isSyntheticEnumArrayHelper(callee.Name, arrDesc) {
					sawCall = true
					helperName = callee.Name
					helperRegs[next.Regs[0]] = true
					p++
					continue
				}
			}
			matchedHelper = false
			goto checkInline
		case sawCall && ((op >= 0x01 && op <= 0x03) || (op >= 0x07 && op <= 0x09)):
			dst, src := ins.Regs[0], ins.Regs[1]
			isH := helperRegs[src]
			delete(helperRegs, dst)
			if isH {
				helperRegs[dst] = true
			}
		default:
			matchedHelper = false
			goto checkInline
		}
	}
	if sawCall && helperRegs[backingSrcReg] {
		matchedHelper = true
	}

checkInline:
	if !matchedHelper {
		helperName = ""
		arrRegs, ok := matchEnumArrayPacket(l.file, l.class.Descriptor, l.method.Instructions, block.Instructions[startPos:writePos], constants, preInt, preConst)
		if !ok || !arrRegs[backingSrcReg] {
			return nil
		}
		matchedInline = true
	}
	if !matchedHelper && !matchedInline {
		return nil
	}

	endPos := writePos
	entriesField := ""
	if fieldName, ePos := l.matchKotlinEntriesTail(block.Instructions, writePos, backingName, arrDesc, backingSrcReg); ePos > writePos {
		endPos = ePos
		entriesField = fieldName
	}

	// Verify Span doesn't overlap any exception try region.
	startIdx := block.Instructions[startPos]
	endIdx := block.Instructions[endPos]
	spanStartOff := l.method.Instructions[startIdx].Offset
	spanEndOff := l.method.Instructions[endIdx].End()
	for _, t := range l.method.Code.Tries {
		tStart := t.StartAddress
		tEnd := t.StartAddress + uint32(t.InstructionCount)
		if tStart < spanEndOff && tEnd > spanStartOff {
			return nil
		}
	}

	// Verify no register defined inside [startPos..endPos] is live-read afterwards before redef.
	if !l.clinitFusedSpanRegsDeadAfter(writeBlockIdx, startPos, endPos) {
		return nil
	}

	skipSet := make(map[int]bool, endPos-startPos)
	if l.fusedNoLocals == nil {
		l.fusedNoLocals = make(map[int]bool, endPos-startPos+1)
	}
	for p := startPos; p <= endPos; p++ {
		idx := block.Instructions[p]
		l.fusedNoLocals[idx] = true
		if p > startPos {
			skipSet[idx] = true
		}
	}
	return &canonicalClinitPlan{
		startIdx:     startIdx,
		endIdx:       endIdx,
		skipSet:      skipSet,
		backingField: backingName,
		helperName:   helperName,
		constants:    constants,
		entriesField: entriesField,
	}
}

func (l *methodLowerer) isSyntheticEnumArrayHelper(name, arrDesc string) bool {
	for i := range l.class.DirectMethods {
		m := &l.class.DirectMethods[i]
		if m.Method.Name != name || int(m.Method.ProtoIndex) >= len(l.file.Protos) {
			continue
		}
		cp := l.file.Protos[m.Method.ProtoIndex]
		if len(cp.Parameters) != 0 || cp.ReturnType != arrDesc {
			continue
		}
		acc, err := methodAccess(m.AccessFlags, m.Method.Name)
		if err == nil && (acc == jvmSyntheticHelperFlags || acc == jvmSyntheticBackingFlags) {
			return true
		}
	}
	return false
}

func (l *methodLowerer) matchKotlinEntriesTail(blockInsns []int, writePos int, backingName, arrDesc string, backingSrcReg uint16) (string, int) {
	arrRegs := map[uint16]bool{backingSrcReg: true}
	var callResultReg uint16
	sawCall := false
	for p := writePos + 1; p < len(blockInsns); p++ {
		idx := blockInsns[p]
		ins := &l.method.Instructions[idx]
		op := ins.Opcode
		switch {
		case op == 0x00:
			continue
		case !sawCall && op == 0x62 && int(ins.Index) < len(l.file.Fields):
			f := l.file.Fields[ins.Index]
			if f.Class != l.class.Descriptor || f.Name != backingName || f.Type != arrDesc {
				return "", -1
			}
			arrRegs[ins.Regs[0]] = true
		case !sawCall && op == 0x1f && int(ins.Index) < len(l.file.Types):
			if !arrRegs[ins.Regs[0]] || l.file.Types[ins.Index].Descriptor != "[Ljava/lang/Enum;" {
				return "", -1
			}
		case !sawCall && ((op >= 0x01 && op <= 0x03) || (op >= 0x07 && op <= 0x09)):
			dst, src := ins.Regs[0], ins.Regs[1]
			isA := arrRegs[src]
			delete(arrRegs, dst)
			if isA {
				arrRegs[dst] = true
			}
		case !sawCall && (op == 0x71 || op == 0x77) && len(ins.Regs) == 1 && arrRegs[ins.Regs[0]] && p+1 < len(blockInsns) && int(ins.Index) < len(l.file.Methods):
			callee := l.file.Methods[ins.Index]
			next := &l.method.Instructions[blockInsns[p+1]]
			if next.Opcode == 0x0c && callee.Class == "Lkotlin/enums/EnumEntriesKt;" && callee.Name == "enumEntries" && int(callee.ProtoIndex) < len(l.file.Protos) {
				cp := l.file.Protos[callee.ProtoIndex]
				if len(cp.Parameters) == 1 && cp.Parameters[0] == "[Ljava/lang/Enum;" && cp.ReturnType == "Lkotlin/enums/EnumEntries;" {
					sawCall = true
					callResultReg = next.Regs[0]
					p++
					continue
				}
			}
			return "", -1
		case sawCall && ((op >= 0x01 && op <= 0x03) || (op >= 0x07 && op <= 0x09)):
			if ins.Regs[1] == callResultReg {
				callResultReg = ins.Regs[0]
			} else if ins.Regs[0] == callResultReg {
				return "", -1
			}
		case sawCall && op == 0x69 && ins.Regs[0] == callResultReg && int(ins.Index) < len(l.file.Fields):
			f := l.file.Fields[ins.Index]
			if f.Class == l.class.Descriptor && f.Type == "Lkotlin/enums/EnumEntries;" {
				return f.Name, p
			}
			return "", -1
		default:
			return "", -1
		}
	}
	return "", -1
}

func (l *methodLowerer) clinitFusedSpanRegsDeadAfter(blockIdx, startPos, endPos int) bool {
	written := make(map[uint16]bool)
	blk := l.method.Blocks[blockIdx]
	for p := startPos; p <= endPos; p++ {
		_, defs := l.method.EffectsAt(blk.Instructions[p])
		for _, r := range defs {
			written[r] = true
		}
	}
	if len(written) == 0 {
		return true
	}

	dirty := make(map[uint16]bool, len(written))
	for r := range written {
		dirty[r] = true
	}
	walkRange := func(insns []int, d map[uint16]bool) bool {
		for _, idx := range insns {
			if l.fusedNoLocals[idx] {
				continue
			}
			uses, defs := l.method.EffectsAt(idx)
			for _, r := range uses {
				if d[r] {
					return false
				}
			}
			for _, r := range defs {
				delete(d, r)
			}
		}
		return true
	}
	if !walkRange(blk.Instructions[endPos+1:], dirty) {
		return false
	}
	if len(dirty) == 0 {
		return true
	}

	inDirty := make([]map[uint16]bool, len(l.method.Blocks))
	var queue []int
	enqueue := func(succ int, d map[uint16]bool) {
		if len(d) == 0 {
			return
		}
		if inDirty[succ] == nil {
			inDirty[succ] = make(map[uint16]bool, len(d))
		}
		grew := false
		for r := range d {
			if !inDirty[succ][r] {
				inDirty[succ][r] = true
				grew = true
			}
		}
		if grew {
			queue = append(queue, succ)
		}
	}
	for _, succ := range blk.Successors {
		enqueue(succ, dirty)
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		out := make(map[uint16]bool, len(inDirty[cur]))
		for r := range inDirty[cur] {
			out[r] = true
		}
		if !walkRange(l.method.Blocks[cur].Instructions, out) {
			return false
		}
		for _, succ := range l.method.Blocks[cur].Successors {
			enqueue(succ, out)
		}
	}
	return true
}

func (l *methodLowerer) emitCanonicalEnumClinitPacket() error {
	plan := l.canonicalClinit
	ownerInternal, err := objectInternalName(l.class.Descriptor)
	if err != nil {
		return methodDiagnostic(l.method.Context, nil, "enum <clinit>", err)
	}
	arrDesc := "[" + l.class.Descriptor
	backingRef, err := l.builder.Pool().Fieldref(ownerInternal, plan.backingField, arrDesc)
	if err != nil {
		return methodDiagnostic(l.method.Context, nil, "enum <clinit>", err)
	}
	if plan.helperName != "" {
		helperRef, err := l.builder.Pool().Methodref(ownerInternal, plan.helperName, "()"+arrDesc)
		if err != nil {
			return methodDiagnostic(l.method.Context, nil, "enum <clinit>", err)
		}
		l.code.CP(classfilebuilder.INVOKESTATIC, helperRef)
	} else {
		if err := l.emitCanonicalEnumArrayBody(plan.constants); err != nil {
			return methodDiagnostic(l.method.Context, nil, "enum <clinit>", err)
		}
	}
	l.code.CP(classfilebuilder.PUTSTATIC, backingRef)
	if plan.entriesField != "" {
		enumArrClass, err := l.builder.Pool().Class("[Ljava/lang/Enum;")
		if err != nil {
			return methodDiagnostic(l.method.Context, nil, "enum <clinit>", err)
		}
		enumEntriesFn, err := l.builder.Pool().Methodref("kotlin/enums/EnumEntriesKt", "enumEntries", "([Ljava/lang/Enum;)Lkotlin/enums/EnumEntries;")
		if err != nil {
			return methodDiagnostic(l.method.Context, nil, "enum <clinit>", err)
		}
		entriesRef, err := l.builder.Pool().Fieldref(ownerInternal, plan.entriesField, "Lkotlin/enums/EnumEntries;")
		if err != nil {
			return methodDiagnostic(l.method.Context, nil, "enum <clinit>", err)
		}
		l.code.CP(classfilebuilder.GETSTATIC, backingRef)
		l.code.CP(classfilebuilder.CHECKCAST, enumArrClass)
		l.code.CP(classfilebuilder.INVOKESTATIC, enumEntriesFn)
		l.code.CP(classfilebuilder.PUTSTATIC, entriesRef)
	}
	return nil
}

func (l *methodLowerer) isEnumSelfOrSubclass(desc string) bool {
	if desc == l.class.Descriptor {
		return true
	}
	if l.hierarchy != nil {
		if sub, ok := l.hierarchy[desc]; ok && sub != nil && sub.Superclass == l.class.Descriptor {
			return true
		}
	}
	for i := range l.file.Classes {
		if l.file.Classes[i].Descriptor == desc && l.file.Classes[i].Superclass == l.class.Descriptor {
			return true
		}
	}
	prefix := strings.TrimSuffix(l.class.Descriptor, ";") + "$"
	return strings.HasPrefix(desc, prefix)
}

func resolvePureConstArg(paramDesc string, val pureConstVal) (canonicalConstArg, bool) {
	switch paramDesc {
	case "I", "Z", "B", "C", "S", "F":
		if val.kind != pure32Bits {
			return canonicalConstArg{}, false
		}
		return canonicalConstArg{paramDesc: paramDesc, kind: pure32Bits, bits: val.bits}, true
	case "J", "D":
		if val.kind != pure64Bits {
			return canonicalConstArg{}, false
		}
		return canonicalConstArg{paramDesc: paramDesc, kind: pure64Bits, bits: val.bits}, true
	case "Ljava/lang/String;":
		if val.kind == pureString {
			return canonicalConstArg{paramDesc: paramDesc, kind: pureString, text: val.text}, true
		}
		if val.kind == pure32Bits && val.bits == 0 {
			return canonicalConstArg{paramDesc: paramDesc, kind: pure32Bits, bits: 0}, true
		}
		return canonicalConstArg{}, false
	case "Ljava/lang/Class;":
		if val.kind == pureClass {
			return canonicalConstArg{paramDesc: paramDesc, kind: pureClass, text: val.text}, true
		}
		if val.kind == pure32Bits && val.bits == 0 {
			return canonicalConstArg{paramDesc: paramDesc, kind: pure32Bits, bits: 0}, true
		}
		return canonicalConstArg{}, false
	default:
		if (strings.HasPrefix(paramDesc, "L") || strings.HasPrefix(paramDesc, "[")) && val.kind == pure32Bits && val.bits == 0 {
			return canonicalConstArg{paramDesc: paramDesc, kind: pure32Bits, bits: 0}, true
		}
		return canonicalConstArg{}, false
	}
}

type hoistedConstVal struct {
	val     pureConstVal
	sources []int
}

func (l *methodLowerer) detectCanonicalEnumClinitConstSpans() {
	if !isCanonicalEnumClass(l.class) || l.encoded.Method.Name != "<clinit>" {
		return
	}
	enumFields := make(map[string]bool)
	for i := range l.class.StaticFields {
		sf := &l.class.StaticFields[i]
		if sf.AccessFlags&dexEnum != 0 && sf.Field.Class == l.class.Descriptor && sf.Field.Type == l.class.Descriptor {
			enumFields[sf.Field.Name] = true
		}
	}
	if len(enumFields) == 0 {
		return
	}

	for bIdx, blk := range l.method.Blocks {
		if !blk.Reachable {
			continue
		}
		blockVals := make(map[uint16]hoistedConstVal)
		consumedHoistedPos := make(map[int]bool)

		updateBlockVals := func(pos, idx int) {
			if l.fusedNoLocals[idx] {
				_, defs := l.method.EffectsAt(idx)
				for _, r := range defs {
					delete(blockVals, r)
				}
				return
			}
			ins := &l.method.Instructions[idx]
			op := ins.Opcode
			switch {
			case op == 0x00:
			case op >= 0x12 && op <= 0x15:
				blockVals[ins.Regs[0]] = hoistedConstVal{
					val:     pureConstVal{kind: pure32Bits, bits: ins.Literal},
					sources: []int{pos},
				}
			case op >= 0x16 && op <= 0x19:
				blockVals[ins.Regs[0]] = hoistedConstVal{
					val:     pureConstVal{kind: pure64Bits, bits: ins.Literal},
					sources: []int{pos},
				}
				delete(blockVals, ins.Regs[0]+1)
			case (op == 0x1a || op == 0x1b) && int(ins.Index) < len(l.file.Strings):
				blockVals[ins.Regs[0]] = hoistedConstVal{
					val:     pureConstVal{kind: pureString, text: l.file.Strings[ins.Index]},
					sources: []int{pos},
				}
			case op == 0x1c && int(ins.Index) < len(l.file.Types):
				blockVals[ins.Regs[0]] = hoistedConstVal{
					val:     pureConstVal{kind: pureClass, text: l.file.Types[ins.Index].Descriptor},
					sources: []int{pos},
				}
			case (op >= 0x01 && op <= 0x03) || (op >= 0x07 && op <= 0x09):
				dst, src := ins.Regs[0], ins.Regs[1]
				if hv, ok := blockVals[src]; ok {
					srcs := make([]int, len(hv.sources)+1)
					copy(srcs, hv.sources)
					srcs[len(hv.sources)] = pos
					blockVals[dst] = hoistedConstVal{val: hv.val, sources: srcs}
				} else {
					delete(blockVals, dst)
				}
			case op >= 0x04 && op <= 0x06:
				dst, src := ins.Regs[0], ins.Regs[1]
				if hv, ok := blockVals[src]; ok && hv.val.kind == pure64Bits {
					srcs := make([]int, len(hv.sources)+1)
					copy(srcs, hv.sources)
					srcs[len(hv.sources)] = pos
					blockVals[dst] = hoistedConstVal{val: hv.val, sources: srcs}
					delete(blockVals, dst+1)
				} else {
					delete(blockVals, dst)
					delete(blockVals, dst+1)
				}
			default:
				_, defs := l.method.EffectsAt(idx)
				for _, r := range defs {
					delete(blockVals, r)
				}
			}
		}

		for pos := 0; pos < len(blk.Instructions); pos++ {
			startIdx := blk.Instructions[pos]
			if l.fusedNoLocals[startIdx] || (pos == 0 && len(blk.Predecessors) > 0) {
				updateBlockVals(pos, startIdx)
				continue
			}
			startInsn := &l.method.Instructions[startIdx]
			if startInsn.Opcode != 0x22 || int(startInsn.Index) >= len(l.file.Types) {
				updateBlockVals(pos, startIdx)
				continue
			}
			allocDesc := l.file.Types[startInsn.Index].Descriptor
			if !l.isEnumSelfOrSubclass(allocDesc) {
				updateBlockVals(pos, startIdx)
				continue
			}
			allocReg := startInsn.Regs[0]
			vals := make(map[uint16]hoistedConstVal, len(blockVals))
			for r, hv := range blockVals {
				if r != allocReg {
					vals[r] = hv
				}
			}

			committed := false
			for k := pos + 1; k < len(blk.Instructions); k++ {
				idx := blk.Instructions[k]
				if l.fusedNoLocals[idx] {
					break
				}
				ins := &l.method.Instructions[idx]
				op := ins.Opcode
				switch {
				case op == 0x00:
					continue
				case op >= 0x12 && op <= 0x15:
					if ins.Regs[0] == allocReg {
						goto nextStart
					}
					vals[ins.Regs[0]] = hoistedConstVal{val: pureConstVal{kind: pure32Bits, bits: ins.Literal}}
				case op >= 0x16 && op <= 0x19:
					if ins.Regs[0] == allocReg || ins.Regs[0]+1 == allocReg {
						goto nextStart
					}
					vals[ins.Regs[0]] = hoistedConstVal{val: pureConstVal{kind: pure64Bits, bits: ins.Literal}}
					delete(vals, ins.Regs[0]+1)
				case op == 0x1a || op == 0x1b:
					if ins.Regs[0] == allocReg || int(ins.Index) >= len(l.file.Strings) {
						goto nextStart
					}
					vals[ins.Regs[0]] = hoistedConstVal{val: pureConstVal{kind: pureString, text: l.file.Strings[ins.Index]}}
				case op == 0x1c:
					if ins.Regs[0] == allocReg || int(ins.Index) >= len(l.file.Types) {
						goto nextStart
					}
					vals[ins.Regs[0]] = hoistedConstVal{val: pureConstVal{kind: pureClass, text: l.file.Types[ins.Index].Descriptor}}
				case (op >= 0x01 && op <= 0x03) || (op >= 0x07 && op <= 0x09):
					dst, src := ins.Regs[0], ins.Regs[1]
					if dst == allocReg || src == allocReg {
						goto nextStart
					}
					if hv, ok := vals[src]; ok {
						vals[dst] = hv
					} else {
						delete(vals, dst)
					}
				case op >= 0x04 && op <= 0x06:
					dst, src := ins.Regs[0], ins.Regs[1]
					if dst == allocReg || dst+1 == allocReg || src == allocReg || src+1 == allocReg {
						goto nextStart
					}
					if hv, ok := vals[src]; ok && hv.val.kind == pure64Bits {
						vals[dst] = hv
						delete(vals, dst+1)
					} else {
						delete(vals, dst)
						delete(vals, dst+1)
					}
				case (op == 0x70 || op == 0x76) && ins.ExtraNullArgs == 0 && len(ins.Regs) >= 3 && ins.Regs[0] == allocReg && int(ins.Index) < len(l.file.Methods):
					callee := l.file.Methods[ins.Index]
					if callee.Class != allocDesc || callee.Name != "<init>" || int(callee.ProtoIndex) >= len(l.file.Protos) {
						goto nextStart
					}
					calleeProto := l.file.Protos[callee.ProtoIndex]
					if len(calleeProto.Parameters) < 2 || calleeProto.Parameters[0] != "Ljava/lang/String;" || calleeProto.Parameters[1] != "I" || calleeProto.ReturnType != "V" {
						goto nextStart
					}
					regPos := 1
					args := make([]canonicalConstArg, 0, len(calleeProto.Parameters))
					var spanHoisted []int
					validArgs := true
					for _, pDesc := range calleeProto.Parameters {
						words := descriptorWords(pDesc)
						if regPos+words > len(ins.Regs) {
							validArgs = false
							break
						}
						hv, ok := vals[ins.Regs[regPos]]
						if !ok {
							validArgs = false
							break
						}
						arg, ok := resolvePureConstArg(pDesc, hv.val)
						if !ok {
							validArgs = false
							break
						}
						args = append(args, arg)
						spanHoisted = append(spanHoisted, hv.sources...)
						regPos += words
					}
					if !validArgs || regPos != len(ins.Regs) {
						goto nextStart
					}

					postAlias := map[uint16]bool{allocReg: true}
					endPos := -1
					fieldName := ""
					for sPos := k + 1; sPos < len(blk.Instructions); sPos++ {
						sIdx := blk.Instructions[sPos]
						if l.fusedNoLocals[sIdx] {
							break
						}
						sIns := &l.method.Instructions[sIdx]
						sOp := sIns.Opcode
						if sOp == 0x00 {
							continue
						}
						if (sOp >= 0x01 && sOp <= 0x03) || (sOp >= 0x07 && sOp <= 0x09) {
							dst, src := sIns.Regs[0], sIns.Regs[1]
							isA := postAlias[src]
							delete(postAlias, dst)
							if isA {
								postAlias[dst] = true
							}
							continue
						}
						if sOp == 0x69 && len(sIns.Regs) > 0 && postAlias[sIns.Regs[0]] && int(sIns.Index) < len(l.file.Fields) {
							f := l.file.Fields[sIns.Index]
							if f.Class == l.class.Descriptor && f.Type == l.class.Descriptor && enumFields[f.Name] {
								endPos = sPos
								fieldName = f.Name
							}
						}
						break
					}
					if endPos < 0 {
						goto nextStart
					}
					endIdx := blk.Instructions[endPos]
					spanStartOff := l.method.Instructions[startIdx].Offset
					spanEndOff := l.method.Instructions[endIdx].End()
					for _, t := range l.method.Code.Tries {
						tStart := t.StartAddress
						tEnd := t.StartAddress + uint32(t.InstructionCount)
						if tStart < spanEndOff && tEnd > spanStartOff {
							goto nextStart
						}
					}
					if !l.clinitFusedSpanRegsDeadAfter(bIdx, pos, endPos) {
						goto nextStart
					}
					if l.fusedNoLocals == nil {
						l.fusedNoLocals = make(map[int]bool, endPos-pos+1)
					}
					if l.canonicalClinitConsts == nil {
						l.canonicalClinitConsts = make(map[int]*canonicalEnumConstSpan)
					}
					if l.canonicalClinitConstSkip == nil {
						l.canonicalClinitConstSkip = make(map[int]bool)
					}
					for p := pos; p <= endPos; p++ {
						pIdx := blk.Instructions[p]
						l.fusedNoLocals[pIdx] = true
						if p > pos {
							l.canonicalClinitConstSkip[pIdx] = true
						}
						_, defs := l.method.EffectsAt(pIdx)
						for _, r := range defs {
							delete(blockVals, r)
						}
					}
					for _, hp := range spanHoisted {
						consumedHoistedPos[hp] = true
					}
					l.canonicalClinitConsts[startIdx] = &canonicalEnumConstSpan{
						startIdx:  startIdx,
						allocDesc: allocDesc,
						ctorProto: calleeProto,
						args:      args,
						fieldName: fieldName,
					}
					pos = endPos
					committed = true
					goto nextStart
				default:
					goto nextStart
				}
			}
		nextStart:
			if !committed {
				updateBlockVals(pos, startIdx)
			}
		}

		if len(consumedHoistedPos) > 0 {
			hPositions := make([]int, 0, len(consumedHoistedPos))
			for hp := range consumedHoistedPos {
				hPositions = append(hPositions, hp)
			}
			sort.Slice(hPositions, func(i, j int) bool { return hPositions[i] > hPositions[j] })
			for _, hp := range hPositions {
				hIdx := blk.Instructions[hp]
				if l.fusedNoLocals[hIdx] {
					continue
				}
				hInsn := &l.method.Instructions[hIdx]
				overlapsTry := false
				for _, t := range l.method.Code.Tries {
					tStart := t.StartAddress
					tEnd := t.StartAddress + uint32(t.InstructionCount)
					if tStart < hInsn.End() && tEnd > hInsn.Offset {
						overlapsTry = true
						break
					}
				}
				if overlapsTry || !l.clinitFusedSpanRegsDeadAfter(bIdx, hp, hp) {
					continue
				}
				if l.fusedNoLocals == nil {
					l.fusedNoLocals = make(map[int]bool)
				}
				if l.canonicalClinitConstSkip == nil {
					l.canonicalClinitConstSkip = make(map[int]bool)
				}
				l.fusedNoLocals[hIdx] = true
				l.canonicalClinitConstSkip[hIdx] = true
			}
		}
	}
}

func (l *methodLowerer) emitCanonicalEnumConstSpan(span *canonicalEnumConstSpan) error {
	allocInternal, err := objectInternalName(span.allocDesc)
	if err != nil {
		return methodDiagnostic(l.method.Context, nil, "enum <clinit> const", err)
	}
	ownerInternal, err := objectInternalName(l.class.Descriptor)
	if err != nil {
		return methodDiagnostic(l.method.Context, nil, "enum <clinit> const", err)
	}
	allocClass, err := l.builder.Pool().Class(allocInternal)
	if err != nil {
		return methodDiagnostic(l.method.Context, nil, "enum <clinit> const", err)
	}
	ctorDesc, err := methodDescriptor(span.ctorProto)
	if err != nil {
		return methodDiagnostic(l.method.Context, nil, "enum <clinit> const", err)
	}
	ctorRef, err := l.builder.Pool().Methodref(allocInternal, "<init>", ctorDesc)
	if err != nil {
		return methodDiagnostic(l.method.Context, nil, "enum <clinit> const", err)
	}
	fieldRef, err := l.builder.Pool().Fieldref(ownerInternal, span.fieldName, l.class.Descriptor)
	if err != nil {
		return methodDiagnostic(l.method.Context, nil, "enum <clinit> const", err)
	}

	l.code.CP(classfilebuilder.NEW, allocClass).Emit(classfilebuilder.DUP)
	for _, arg := range span.args {
		switch {
		case arg.kind == pureString:
			c, err := l.builder.Pool().String(arg.text)
			if err != nil {
				return methodDiagnostic(l.method.Context, nil, "enum <clinit> const", err)
			}
			l.code.LDC(c)
		case arg.kind == pureClass:
			if owner, ok := primitiveClassOwner(arg.text); ok {
				c, err := l.builder.Pool().Fieldref(owner, "TYPE", "Ljava/lang/Class;")
				if err != nil {
					return methodDiagnostic(l.method.Context, nil, "enum <clinit> const", err)
				}
				l.code.CP(classfilebuilder.GETSTATIC, c)
			} else {
				name, err := ir.InternalName(arg.text)
				if err != nil {
					return methodDiagnostic(l.method.Context, nil, "enum <clinit> const", err)
				}
				c, err := l.builder.Pool().Class(name)
				if err != nil {
					return methodDiagnostic(l.method.Context, nil, "enum <clinit> const", err)
				}
				l.code.LDC(c)
			}
		case arg.paramDesc == "F":
			c, err := l.builder.Pool().Float(uint32(arg.bits))
			if err != nil {
				return methodDiagnostic(l.method.Context, nil, "enum <clinit> const", err)
			}
			l.code.LDC(c)
		case arg.paramDesc == "J":
			c, err := l.builder.Pool().Long(arg.bits)
			if err != nil {
				return methodDiagnostic(l.method.Context, nil, "enum <clinit> const", err)
			}
			l.code.LDC2(c)
		case arg.paramDesc == "D":
			c, err := l.builder.Pool().Double(uint64(arg.bits))
			if err != nil {
				return methodDiagnostic(l.method.Context, nil, "enum <clinit> const", err)
			}
			l.code.LDC2(c)
		case strings.HasPrefix(arg.paramDesc, "L") || strings.HasPrefix(arg.paramDesc, "["):
			l.code.Emit(classfilebuilder.ACONST_NULL)
		default:
			if err := l.pushInt(int32(arg.bits)); err != nil {
				return methodDiagnostic(l.method.Context, nil, "enum <clinit> const", err)
			}
		}
	}
	l.code.CP(classfilebuilder.INVOKESPECIAL, ctorRef)
	l.code.CP(classfilebuilder.PUTSTATIC, fieldRef)
	return nil
}

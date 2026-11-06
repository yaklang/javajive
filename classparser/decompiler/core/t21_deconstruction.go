package core

import (
	"fmt"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// matchExceptionNew reports whether op is `new java.lang.MatchException`.
func matchExceptionNew(d *Decompiler, op *OpCode) bool {
	if d == nil || op == nil || op.Instr == nil || op.Instr.OpCode != OP_NEW ||
		d.constantPoolGetter == nil || len(op.Data) < 2 {
		return false
	}
	cv, ok := d.constantPoolGetter(int(Convert2bytesToInt(op.Data))).(*values.JavaClassValue)
	if !ok || cv == nil || cv.JavaType == nil {
		return false
	}
	name, ok := types.ClassFQNOf(cv.JavaType)
	if !ok {
		name = cv.JavaType.String(&class_context.ClassContext{})
	}
	return strings.ReplaceAll(name, "/", ".") == "java.lang.MatchException"
}

func matchExceptionConstructor(d *Decompiler, op *OpCode) bool {
	if d == nil || op == nil || op.Instr == nil || op.Instr.OpCode != OP_INVOKESPECIAL ||
		d.constantPoolGetter == nil || len(op.Data) < 2 {
		return false
	}
	member, ok := d.constantPoolGetter(int(Convert2bytesToInt(op.Data))).(*values.JavaClassMember)
	if !ok || member == nil || member.Member != "<init>" {
		return false
	}
	owner := strings.ReplaceAll(member.Name, "/", ".")
	return owner == "java.lang.MatchException" &&
		member.Description == "(Ljava/lang/String;Ljava/lang/Throwable;)V"
}

func anyMatchExceptionType(d *Decompiler) bool {
	if d == nil {
		return false
	}
	for _, op := range d.opCodes {
		if matchExceptionNew(d, op) {
			return true
		}
	}
	return false
}

// hasCompilerDeconstructionHandler proves the javac record-pattern wrapper:
// a Throwable handler whose first instruction stores the caught value and then
// constructs MatchException. The reconstructed pattern switch lets javac
// regenerate this wrapper, so the original handler is not lowered separately.
func hasCompilerDeconstructionHandler(d *Decompiler, byOff map[int]*OpCode) bool {
	if d == nil {
		return false
	}
	for _, entry := range d.ExceptionTable {
		if entry == nil || entry.CatchType == 0 {
			continue
		}
		op := byOff[int(entry.HandlerPc)]
		if op == nil || GetStoreIdx(op) < 0 {
			continue
		}
		cur := nextOp(op, byOff)
		for i := 0; i < 4 && cur != nil; i++ {
			if matchExceptionNew(d, cur) {
				return true
			}
			if GetStoreIdx(cur) >= 0 {
				break
			}
			cur = nextOp(cur, byOff)
		}
	}
	return false
}

// isCompilerMatchDefault recognizes the compiler-generated default arm of a
// deconstruction pattern switch:
//
//	new java/lang/MatchException
//	dup
//	aconst_null
//	aconst_null
//	invokespecial MatchException.<init>(String, Throwable)
//	athrow
//
// Source pattern switches over an exhaustive sealed hierarchy omit this arm;
// javac regenerates the same throw. T21 emits an explicit equivalent throw so
// the flat source remains exhaustive even without sealed/permits metadata.
func isCompilerMatchDefault(d *Decompiler, byOff map[int]*OpCode, pc int) bool {
	op := byOff[pc]
	if !matchExceptionNew(d, op) {
		return false
	}
	dup := nextOp(op, byOff)
	if dup == nil || dup.Instr == nil || dup.Instr.OpCode != OP_DUP {
		return false
	}
	firstNull := nextOp(dup, byOff)
	secondNull := nextOp(firstNull, byOff)
	if firstNull == nil || secondNull == nil ||
		firstNull.Instr == nil || firstNull.Instr.OpCode != OP_ACONST_NULL ||
		secondNull.Instr == nil || secondNull.Instr.OpCode != OP_ACONST_NULL {
		return false
	}
	ctor := nextOp(secondNull, byOff)
	if !matchExceptionConstructor(d, ctor) {
		return false
	}
	throw := nextOp(ctor, byOff)
	return throw != nil && throw.Instr != nil && throw.Instr.OpCode == OP_ATHROW
}

// recordComponentSourceType renders the erased source type of one record
// component. Array components are rejected until an import-aware renderer is
// wired; the common record shapes are primitives, strings, and record classes.
func recordComponentSourceType(d *Decompiler, descriptor string) (string, bool) {
	typ, err := types.ParseDescriptor(descriptor)
	if err != nil || typ == nil {
		return "", false
	}
	return recordComponentSourceTypeOf(d, typ)
}

func recordComponentSourceTypeOf(d *Decompiler, typ types.JavaType) (string, bool) {
	if typ == nil {
		return "", false
	}
	if typ.IsArray() {
		element, ok := recordComponentSourceTypeOf(d, typ.ElementType())
		if !ok {
			return "", false
		}
		return element + "[]", true
	}
	if primer, ok := typ.RawType().(*types.JavaPrimer); ok && primer != nil {
		if primer.Name == types.JavaVoid {
			return "", false
		}
		return primer.Name, true
	}
	name, ok := types.ClassFQNOf(typ)
	if !ok {
		return "", false
	}
	if d.FunctionContext != nil {
		return d.FunctionContext.ShortTypeName(name), true
	}
	return name, true
}

// recoverRecordPattern consumes the javac deconstruction accessor prefix:
//
//	aload <bind>; invokevirtual Record.comp()T; store temp
//	(load temp; store patternVar)*
//
// It returns the source component list, the first body opcode, and an env that
// maps every accessor temporary/copy slot to the component name. The record's
// declared component list is authoritative; an arbitrary no-arg method is
// never mistaken for a component accessor.
func (d *Decompiler) recoverRecordPattern(start *OpCode, byOff map[int]*OpCode, bindSlot int,
	recordType string, mergePC, loopPC, restartSlot, caseIndex int, env map[int]string,
	bindNames map[string]int) (pattern string, bodyStart *OpCode, outEnv map[int]string, ok bool) {
	if d == nil || d.RecordComponents == nil || start == nil || env == nil {
		return "", nil, nil, false
	}
	components, known := d.RecordComponents(recordType)
	if !known || len(components) == 0 {
		return "", nil, nil, false
	}
	expectedOwner := strings.ReplaceAll(recordType, ".", "/")
	outEnv = make(map[int]string, len(env)+len(components)*2)
	for slot, name := range env {
		outEnv[slot] = name
	}
	rendered := make([]string, len(components))
	type nestedPlan struct {
		index    int
		typeName string
		bindSlot int
	}
	var plans []nestedPlan
	op := start
	for i, component := range components {
		if op == nil || GetRetrieveIdx(op) != bindSlot {
			return "", nil, nil, false
		}
		call := nextOp(op, byOff)
		if call == nil || call.Instr == nil ||
			(call.Instr.OpCode != OP_INVOKEVIRTUAL && call.Instr.OpCode != OP_INVOKEINTERFACE) ||
			d.constantPoolGetter == nil || len(call.Data) < 2 {
			return "", nil, nil, false
		}
		member, ok := d.constantPoolGetter(int(Convert2bytesToInt(call.Data))).(*values.JavaClassMember)
		if !ok || member == nil || strings.ReplaceAll(member.Name, ".", "/") != expectedOwner ||
			member.Member != component.Name || member.Description != "()"+component.Descriptor {
			return "", nil, nil, false
		}
		store := nextOp(call, byOff)
		if store == nil || GetStoreIdx(store) < 0 {
			return "", nil, nil, false
		}
		tempSlot := GetStoreIdx(store)
		componentType, typeOK := recordComponentSourceType(d, component.Descriptor)
		if !typeOK || class_context.SafeIdentifier(component.Name) != component.Name {
			return "", nil, nil, false
		}
		if nestedType, nestedBind, next, isNested, nestedOK := d.detectNestedComponent(store, tempSlot, component,
			byOff, loopPC, restartSlot, caseIndex); isNested {
			if !nestedOK {
				return "", nil, nil, false
			}
			// javac binds every nested object first, then destructures them in
			// component order. Queue the recursive parse until the sibling
			// accessor binds have been consumed.
			plans = append(plans, nestedPlan{index: i, typeName: nestedType, bindSlot: nestedBind})
			op = next
			continue
		}
		bindName := nextPatternBindName(component.Name, bindNames)
		rendered[i] = componentType + " " + bindName
		outEnv[tempSlot] = bindName
		cursor := nextOp(store, byOff)
		// javac copies the accessor result into the source pattern variable.
		// Multiple copies can occur when the body reads the component several
		// times through the same JVM slot.
		for cursor != nil && GetRetrieveIdx(cursor) == tempSlot {
			copyStore := nextOp(cursor, byOff)
			if copyStore == nil || GetStoreIdx(copyStore) < 0 {
				break
			}
			outEnv[GetStoreIdx(copyStore)] = bindName
			cursor = nextOp(copyStore, byOff)
		}
		op = cursor
		if op != nil && int(op.CurrentOffset) >= mergePC {
			break
		}
	}
	if len(rendered) != len(components) || op == nil || int(op.CurrentOffset) >= mergePC {
		return "", nil, nil, false
	}
	for _, plan := range plans {
		componentType, typeOK := recordComponentSourceType(d, components[plan.index].Descriptor)
		if !typeOK {
			return "", nil, nil, false
		}
		nestedPattern, next, nestedEnv, nestedOK := d.recoverRecordPattern(op, byOff, plan.bindSlot, plan.typeName,
			mergePC, loopPC, restartSlot, caseIndex, outEnv, bindNames)
		if !nestedOK {
			return "", nil, nil, false
		}
		rendered[plan.index] = componentType + "(" + nestedPattern + ")"
		outEnv = nestedEnv
		op = next
		if op == nil || int(op.CurrentOffset) >= mergePC {
			return "", nil, nil, false
		}
	}
	// The pattern has no source binding for the whole record; a body that reads
	// the record object outside the consumed accessor prefix cannot be rendered.
	for cursor := op; cursor != nil && int(cursor.CurrentOffset) < mergePC; cursor = nextOp(cursor, byOff) {
		if GetRetrieveIdx(cursor) == bindSlot {
			return "", nil, nil, false
		}
	}
	return strings.Join(rendered, ", "), op, outEnv, true
}

func nextPatternBindName(base string, used map[string]int) string {
	if used == nil {
		return base
	}
	if used[base] == 0 {
		used[base] = 1
		return base
	}
	used[base]++
	return fmt.Sprintf("%s_%d", base, used[base])
}

func seedPatternBindNames(env map[int]string, used map[string]int) {
	for _, value := range env {
		if value == "" || class_context.SafeIdentifier(value) != value {
			continue
		}
		if used[value] == 0 {
			used[value] = 1
		}
	}
}

// detectNestedComponent recognizes the nested-pattern binding suffix after an
// accessor result has been stored in tempSlot:
//
//	aload temp; instanceof Nested; <restart fail>; aload temp; astore nestedBind
//
// It returns the nested bind slot and the opcode after the binding, but leaves
// component destructuring to the caller's ordered plan. A scalar component
// returns isNested=false.
func (d *Decompiler) detectNestedComponent(afterStore *OpCode, tempSlot int, component RecordComponent,
	byOff map[int]*OpCode, loopPC, restartSlot, caseIndex int) (nestedType string, bindSlot int, next *OpCode, isNested, ok bool) {
	load := nextOp(afterStore, byOff)
	if load == nil || GetRetrieveIdx(load) != tempSlot {
		return "", 0, nil, false, true
	}
	instanceOf := nextOp(load, byOff)
	if instanceOf == nil || instanceOf.Instr == nil || instanceOf.Instr.OpCode != OP_INSTANCEOF {
		return "", 0, nil, false, true
	}
	isNested = true
	nestedType, typeOK := recordComponentClass(component.Descriptor)
	if !typeOK || !d.instanceOfClass(instanceOf, nestedType) {
		return "", 0, nil, true, false
	}
	ifOp := nextOp(instanceOf, byOff)
	if ifOp == nil || ifOp.Instr == nil || !isIfOp(ifOp.Instr.OpCode) {
		return "", 0, nil, true, false
	}
	branch := byOff[ifBranchPC(ifOp)]
	fall := nextOp(ifOp, byOff)
	var success *OpCode
	switch {
	case isRestartFail(branch, byOff, loopPC, restartSlot, caseIndex+1):
		success = fall
	case isRestartFail(fall, byOff, loopPC, restartSlot, caseIndex+1):
		success = branch
	default:
		return "", 0, nil, true, false
	}
	if success == nil || GetRetrieveIdx(success) != tempSlot {
		return "", 0, nil, true, false
	}
	bindOp := nextOp(success, byOff)
	if bindOp == nil || GetStoreIdx(bindOp) < 0 {
		return "", 0, nil, true, false
	}
	return nestedType, GetStoreIdx(bindOp), nextOp(bindOp, byOff), true, true
}

func recordComponentClass(descriptor string) (string, bool) {
	typ, err := types.ParseDescriptor(descriptor)
	if err != nil || typ == nil || typ.IsArray() {
		return "", false
	}
	name, ok := types.ClassFQNOf(typ)
	if !ok {
		return "", false
	}
	return strings.ReplaceAll(name, "/", "."), true
}

func (d *Decompiler) instanceOfClass(op *OpCode, want string) bool {
	if op == nil || op.Instr == nil || op.Instr.OpCode != OP_INSTANCEOF ||
		d.constantPoolGetter == nil || len(op.Data) < 2 {
		return false
	}
	cv, ok := d.constantPoolGetter(int(Convert2bytesToInt(op.Data))).(*values.JavaClassValue)
	if !ok || cv == nil || cv.JavaType == nil {
		return false
	}
	name, ok := types.ClassFQNOf(cv.JavaType)
	if !ok {
		name = cv.JavaType.String(&class_context.ClassContext{})
	}
	return strings.ReplaceAll(name, "/", ".") == strings.ReplaceAll(want, "/", ".")
}

// patternDeconstructionPrefix is the legacy shape check used only to fail
// closed when a multi-accessor prefix exists but record metadata is missing.
func patternDeconstructionPrefix(start *OpCode, byOff map[int]*OpCode, bindSlot, mergePC, loopPC int) bool {
	invokes := 0
	op := start
	for i := 0; i < 16 && op != nil; i++ {
		pc := int(op.CurrentOffset)
		if pc == mergePC || pc == loopPC {
			break
		}
		if op.Instr.OpCode == OP_GOTO || op.Instr.OpCode == OP_GOTO_W || isReturnOp(op.Instr.OpCode) {
			break
		}
		if op.Instr.OpCode == OP_INVOKEVIRTUAL || op.Instr.OpCode == OP_INVOKEINTERFACE {
			invokes++
		}
		op = nextOp(op, byOff)
	}
	return invokes >= 2
}

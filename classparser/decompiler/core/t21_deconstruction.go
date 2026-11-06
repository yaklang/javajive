package core

import (
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
	if err != nil || typ == nil || typ.IsArray() {
		return "", false
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
	recordType string, mergePC int, env map[int]string) (pattern string, bodyStart *OpCode, outEnv map[int]string, ok bool) {
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
	rendered := make([]string, 0, len(components))
	op := start
	for _, component := range components {
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
		componentType, ok := recordComponentSourceType(d, component.Descriptor)
		if !ok || class_context.SafeIdentifier(component.Name) != component.Name {
			return "", nil, nil, false
		}
		rendered = append(rendered, componentType+" "+component.Name)
		tempSlot := GetStoreIdx(store)
		outEnv[tempSlot] = component.Name
		cursor := nextOp(store, byOff)
		// javac copies the accessor result into the source pattern variable.
		// Multiple copies can occur when the body reads the component several
		// times through the same JVM slot.
		for cursor != nil && GetRetrieveIdx(cursor) == tempSlot {
			copyStore := nextOp(cursor, byOff)
			if copyStore == nil || GetStoreIdx(copyStore) < 0 {
				break
			}
			outEnv[GetStoreIdx(copyStore)] = component.Name
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
	// The pattern has no source binding for the whole record; a body that reads
	// the record object outside the consumed accessor prefix cannot be rendered.
	for cursor := op; cursor != nil && int(cursor.CurrentOffset) < mergePC; cursor = nextOp(cursor, byOff) {
		if GetRetrieveIdx(cursor) == bindSlot {
			return "", nil, nil, false
		}
	}
	return strings.Join(rendered, ", "), op, outEnv, true
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

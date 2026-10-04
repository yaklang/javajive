package core

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// CHECKCAST consumes one reference and produces one reference. Retain it in
// a call's argument tree when the intervening instructions only build later
// arguments from retained expression trees and value-returning calls wholly above
// this operand. Java evaluates arguments left to right, so calls in those later
// argument trees still follow this cast. No store, duplicate, discarded result,
// allocation or alternative entry may intervene; the handler domain must stay
// identical. Counting values above the cast identifies the actual consumer and
// formal parameter, and excludes a cast used as the invocation receiver.
func (d *Decompiler) canInlineCheckcastArgument(op *OpCode) bool {
	if d == nil || op == nil || op.Instr == nil || op.Instr.OpCode != OP_CHECKCAST || len(op.Target) != 1 || d.constantPoolGetter == nil {
		return false
	}
	consumer, previous := op.Target[0], op
	later := 0
	var member *values.JavaClassMember
	var method *types.JavaFuncType
	for steps := 0; ; steps++ {
		if steps >= 64 || consumer == nil || consumer.IsCustom || consumer.Instr == nil ||
			consumer.IsCatch || consumer.IsTryCatchParent || len(consumer.Source) != 1 || consumer.Source[0] != previous ||
			consumer.CurrentOffset <= previous.CurrentOffset ||
			!sameHandlerCoverage(d.handlersAt(op), d.handlersAt(consumer)) {
			return false
		}
		instruction := consumer.Instr.OpCode
		if instruction == OP_INVOKEVIRTUAL || instruction == OP_INVOKEINTERFACE || instruction == OP_INVOKESTATIC || instruction == OP_INVOKESPECIAL {
			if len(consumer.Data) < 2 {
				return false
			}
			var ok bool
			member, ok = d.constantPoolGetter(int(Convert2bytesToInt(consumer.Data))).(*values.JavaClassMember)
			if !ok || member == nil || member.JavaType == nil {
				return false
			}
			method = member.JavaType.FunctionType()
			if method == nil {
				return false
			}
			consumed := len(method.ParamTypes)
			if instruction != OP_INVOKESTATIC {
				consumed++ // instance receiver, including category-2 arguments as one value each
			}
			if consumed > later {
				break // this invocation reaches the checked operand
			}
			// A call wholly above the operand builds a later argument. Void
			// calls/constructors cannot be nested there without changing their
			// statement order; only a retained return value is admissible.
			if member.Member == "<init>" || method.ReturnType == nil {
				return false
			}
			if p, ok := method.ReturnType.RawType().(*types.JavaPrimer); ok && p.Name == types.JavaVoid {
				return false
			}
			later = later - consumed + 1
		} else {
			access := LocalAccessOf(instruction)
			switch {
			case access.Read && !access.Write && instruction != OP_RET:
				later++ // each load pushes one value, including category-2 values
			case instruction == OP_CHECKCAST, instruction == OP_NOP:
				// The top value stays in place, including repeated casts of the
				// original argument before any later arguments are pushed.
			case instruction == OP_ACONST_NULL,
				instruction >= OP_ICONST_M1 && instruction <= OP_DCONST_1,
				instruction == OP_BIPUSH, instruction == OP_SIPUSH:
				later++
			default:
				consumed, produced, ok := d.checkcastLaterExpressionEffect(consumer)
				if !ok || later < consumed {
					return false
				}
				later = later - consumed + produced
			}
		}
		if len(consumer.Target) != 1 || consumer.Target[0] == nil || consumer.Target[0].CurrentOffset <= consumer.CurrentOffset {
			return false
		}
		previous, consumer = consumer, consumer.Target[0]
	}
	if method == nil || len(method.ParamTypes) <= later {
		return false
	}
	// The already-evaluated uninitialized receiver and preceding operands
	// stay below the cast; constructor allocation is not moved by this rule.
	if member.Member == "<init>" {
		if consumer.Instr.OpCode != OP_INVOKESPECIAL || method.ReturnType == nil {
			return false
		}
		ret, ok := method.ReturnType.RawType().(*types.JavaPrimer)
		if !ok || ret.Name != types.JavaVoid {
			return false
		}
	}
	last := method.ParamTypes[len(method.ParamTypes)-1-later]
	if last == nil {
		return false
	}
	_, primitive := last.RawType().(*types.JavaPrimer)
	return !primitive
}

// canInlineImmediateZeroArgCheckcast keeps a checked value in its use expression when the
// immediately following instruction invokes a zero-argument instance method on that exact type.
// This preserves branch-local evaluation across a CFG merge (for example, a Boolean checkcast in
// one arm of a ternary) without inventing a local whose definition is scoped to only one arm.
func (d *Decompiler) canInlineImmediateZeroArgCheckcast(op *OpCode, castType types.JavaType) bool {
	if d == nil || d.getenv("JDEC_CHECKCAST_IMMEDIATE_INVOKE_OFF") != "" || op == nil || len(op.Target) != 1 {
		return false
	}
	consumer := op.Target[0]
	if consumer == nil || consumer.IsCustom || consumer.Instr == nil {
		return false
	}
	switch consumer.Instr.OpCode {
	case OP_INVOKEVIRTUAL, OP_INVOKEINTERFACE:
	default:
		return false
	}
	if d.constantPoolGetter == nil {
		return false
	}
	member, ok := d.constantPoolGetter(int(Convert2bytesToInt(consumer.Data))).(*values.JavaClassMember)
	if !ok || member == nil || member.JavaType == nil || member.JavaType.FunctionType() == nil || len(member.JavaType.FunctionType().ParamTypes) != 0 {
		return false
	}
	castOwner, ok := types.ClassFQNOf(castType)
	if !ok || castOwner == "" || strings.ReplaceAll(member.Name, "/", ".") != castOwner {
		return false
	}
	return sameIntSlice(d.handlersAt(op), d.handlersAt(consumer))
}

func sameIntSlice(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// An immediate CHECKCAST/ATHROW pair is one typed throw expression. Keeping
// the checked operand on this private stack edge avoids materializing a local
// that may join unrelated catch variables and lose the cast's declaration type.
// The check still runs before ATHROW in the same handler domain; in particular
// a failed cast and a null throw keep their original exception routing.
func (d *Decompiler) canInlineImmediateCheckcastThrow(op *OpCode) bool {
	if d == nil || op == nil || op.Instr == nil || op.Instr.OpCode != OP_CHECKCAST || op.IsCustom || op.IsCatch || op.IsTryCatchParent || len(op.Target) != 1 {
		return false
	}
	throw := op.Target[0]
	return throw != nil && throw.Instr != nil && throw.Instr.OpCode == OP_ATHROW &&
		!throw.IsCustom && !throw.IsCatch && !throw.IsTryCatchParent &&
		throw.CurrentOffset > op.CurrentOffset && len(throw.Source) == 1 && throw.Source[0] == op &&
		sameHandlerCoverage(d.handlersAt(op), d.handlersAt(throw))
}

// A CHECKCAST immediately consumed by GETFIELD is one checked receiver
// expression. Creating a separate local strands its definition when a guarded
// field read becomes &&/||/?:. Inline only this single-use stack edge, with the
// same exception coverage and an exact field-owner witness; do not move an
// earlier cast stored in a local into a conditional read.
func (d *Decompiler) canInlineImmediateCheckcastField(op *OpCode, castType types.JavaType) bool {
	if d == nil || op == nil || op.Instr == nil || op.Instr.OpCode != OP_CHECKCAST || len(op.Target) != 1 || d.constantPoolGetter == nil {
		return false
	}
	read := op.Target[0]
	if read == nil || read.IsCustom || read.Instr == nil || read.Instr.OpCode != OP_GETFIELD || len(read.Data) != 2 || len(read.Source) != 1 || read.Source[0] != op || !sameHandlerCoverage(d.handlersAt(op), d.handlersAt(read)) {
		return false
	}
	member, ok := d.constantPoolGetter(int(Convert2bytesToInt(read.Data))).(*values.JavaClassMember)
	if !ok || member == nil {
		return false
	}
	owner, ok := types.ClassFQNOf(castType)
	return ok && owner != "" && strings.ReplaceAll(member.Name, "/", ".") == owner
}

// CHECKCAST on the top stack value immediately before AASTORE checks the RHS.
// Keep that cast in the RHS tree: a separate local could evaluate it before
// an earlier effectful array index. Only further casts/NOPs may intervene;
// stores, duplication, alternate entries and handler changes reject the fold.
func (d *Decompiler) canInlineCheckcastArrayStore(op *OpCode, castType types.JavaType) bool {
	if d == nil || op == nil || op.Instr == nil || op.Instr.OpCode != OP_CHECKCAST || len(op.stackConsumed) != 1 || castType == nil {
		return false
	}
	if _, primitive := castType.RawType().(*types.JavaPrimer); primitive {
		return false
	}
	previous := op
	for steps := 0; steps < 8; steps++ {
		if len(previous.Target) != 1 {
			return false
		}
		next := previous.Target[0]
		if next == nil || next.Instr == nil || next.IsCustom || next.IsCatch || next.IsTryCatchParent || next.CurrentOffset <= previous.CurrentOffset || len(next.Source) != 1 || next.Source[0] != previous || !sameHandlerCoverage(d.handlersAt(op), d.handlersAt(next)) {
			return false
		}
		switch next.Instr.OpCode {
		case OP_AASTORE:
			return true
		case OP_CHECKCAST, OP_NOP:
			previous = next
		default:
			return false
		}
	}
	return false
}

// An immediately consumed CHECKCAST is the field assignment's RHS. Its
// receiver (when present) already lies below the checked value on the JVM
// stack. Retaining the cast in that RHS preserves receiver/value evaluation,
// the check before the store and both exception points. Alternate entries,
// duplication and differing handler domains cannot use this private edge.
func (d *Decompiler) canInlineImmediateCheckcastFieldStore(op *OpCode, castType types.JavaType) bool {
	if d == nil || op == nil || op.Instr == nil || op.Instr.OpCode != OP_CHECKCAST || len(op.Data) != 2 || castType == nil || op.IsCustom || op.IsCatch || op.IsTryCatchParent || len(op.Target) != 1 || d.constantPoolGetter == nil {
		return false
	}
	if _, primitive := castType.RawType().(*types.JavaPrimer); primitive {
		return false
	}
	store := op.Target[0]
	if store == nil || store.Instr == nil || store.IsCustom || store.IsCatch || store.IsTryCatchParent || store.CurrentOffset <= op.CurrentOffset || len(store.Data) != 2 || len(store.Source) != 1 || store.Source[0] != op || !sameHandlerCoverage(d.handlersAt(op), d.handlersAt(store)) {
		return false
	}
	if store.Instr.OpCode != OP_PUTFIELD && store.Instr.OpCode != OP_PUTSTATIC {
		return false
	}
	field, known := d.constantPoolGetter(int(Convert2bytesToInt(store.Data))).(*values.JavaClassMember)
	if !known || field == nil {
		return false
	}
	target, err := types.ParseDescriptor(field.Description)
	if err != nil || target == nil {
		return false
	}
	_, primitive := target.RawType().(*types.JavaPrimer)
	return !primitive && target.FunctionType() == nil
}

// checkcastLaterExpressionEffect counts JVM values rather than words: a long
// occupies one expression operand even though it occupies two verifier slots.
// Only instructions represented as retained Java expressions are admissible.
// Their operands must lie wholly above the checked value; this prevents a
// later array/field/arithmetic operation from becoming its actual consumer.
// Java evaluates these trees after the preceding cast in argument order,
// preserving their null/bounds/division failures, volatile reads and clinit.
// Stores, DUP, NEW, branches and unrepresented constant-dynamic effects remain
// barriers. Object allocation cannot be moved to its constructor invocation.
func (d *Decompiler) checkcastLaterExpressionEffect(op *OpCode) (consumed, produced int, ok bool) {
	if op == nil || op.Instr == nil {
		return 0, 0, false
	}
	switch op.Instr.OpCode {
	case OP_GETSTATIC, OP_GETFIELD:
		if len(op.Data) != 2 || d.constantPoolGetter == nil {
			return 0, 0, false
		}
		member, valid := d.constantPoolGetter(int(Convert2bytesToInt(op.Data))).(*values.JavaClassMember)
		if !valid || member == nil || member.JavaType == nil || member.JavaType.FunctionType() != nil {
			return 0, 0, false
		}
		typ, err := types.ParseDescriptor(member.Description)
		if err != nil || typ == nil || typ.FunctionType() != nil {
			return 0, 0, false
		}
		if primitive, isPrimitive := typ.RawType().(*types.JavaPrimer); isPrimitive && primitive.Name == types.JavaVoid {
			return 0, 0, false
		}
		if op.Instr.OpCode == OP_GETFIELD {
			return 1, 1, true
		}
		return 0, 1, true
	case OP_LDC, OP_LDC_W, OP_LDC2_W:
		width := 2
		if op.Instr.OpCode == OP_LDC {
			width = 1
		}
		if len(op.Data) != width || d.ConstantPoolLiteralGetter == nil {
			return 0, 0, false
		}
		index := int(op.Data[0])
		if width == 2 {
			index = int(Convert2bytesToInt(op.Data))
		}
		literal, valid := d.ConstantPoolLiteralGetter(index).(*values.JavaLiteral)
		if !valid || literal == nil || literal.Type() == nil {
			return 0, 0, false
		}
		primitive, isPrimitive := literal.Type().RawType().(*types.JavaPrimer)
		if !isPrimitive || primitive.Name == types.JavaVoid {
			return 0, 0, false
		}
		wide := primitive.Name == types.JavaLong || primitive.Name == types.JavaDouble
		if wide != (op.Instr.OpCode == OP_LDC2_W) {
			return 0, 0, false
		}
		return 0, 1, true
	case OP_ARRAYLENGTH, OP_INEG, OP_LNEG, OP_FNEG, OP_DNEG,
		OP_I2B, OP_I2C, OP_I2D, OP_I2F, OP_I2L, OP_I2S, OP_L2D, OP_L2F, OP_L2I, OP_F2D, OP_F2I, OP_F2L, OP_D2F, OP_D2I, OP_D2L:
		if len(op.Data) != 0 {
			return 0, 0, false
		}
		return 1, 1, true
	case OP_AALOAD, OP_IALOAD, OP_BALOAD, OP_CALOAD, OP_FALOAD, OP_LALOAD, OP_DALOAD, OP_SALOAD,
		OP_LSUB, OP_ISUB, OP_DSUB, OP_FSUB, OP_LADD, OP_IADD, OP_FADD, OP_DADD, OP_IREM, OP_FREM, OP_LREM, OP_DREM, OP_IDIV, OP_FDIV, OP_DDIV, OP_LDIV, OP_IMUL, OP_DMUL, OP_FMUL, OP_LMUL, OP_LAND, OP_LOR, OP_LXOR, OP_ISHR, OP_ISHL, OP_LSHL, OP_LSHR, OP_IUSHR, OP_LUSHR, OP_IOR, OP_IAND, OP_IXOR:
		if len(op.Data) != 0 {
			return 0, 0, false
		}
		return 2, 1, true
	}
	return 0, 0, false
}

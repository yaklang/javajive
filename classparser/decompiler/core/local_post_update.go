package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// These are JVM numeric update protocols, not source spelling heuristics.
// Narrow locals require the original narrowing conversion: an int-word store
// without I2B/I2S/I2C is not Java's byte/short/char postfix operation.
type localPostProtocol struct {
	load, store, dup, one, add, sub, narrow int
}

var localPostProtocols = map[string]localPostProtocol{
	types.JavaInteger: {OP_ILOAD, OP_ISTORE, OP_DUP, OP_ICONST_1, OP_IADD, OP_ISUB, 0},
	types.JavaByte:    {OP_ILOAD, OP_ISTORE, OP_DUP, OP_ICONST_1, OP_IADD, OP_ISUB, OP_I2B},
	types.JavaShort:   {OP_ILOAD, OP_ISTORE, OP_DUP, OP_ICONST_1, OP_IADD, OP_ISUB, OP_I2S},
	types.JavaChar:    {OP_ILOAD, OP_ISTORE, OP_DUP, OP_ICONST_1, OP_IADD, OP_ISUB, OP_I2C},
	types.JavaLong:    {OP_LLOAD, OP_LSTORE, OP_DUP2, OP_LCONST_1, OP_LADD, OP_LSUB, 0},
	types.JavaFloat:   {OP_FLOAD, OP_FSTORE, OP_DUP, OP_FCONST_1, OP_FADD, OP_FSUB, 0},
	types.JavaDouble:  {OP_DLOAD, OP_DSTORE, OP_DUP2, OP_DCONST_1, OP_DADD, OP_DSUB, 0},
}

// A private load; dup (or a second identical local load); typed-one;
// add/sub; [narrow]; store leaves the OLD load word on the stack and writes
// the NEW word to the same binding. Prove
// both identities before replacing that surviving word with an enumerable
// postfix expression. No shared SlotValue is mutated: the arithmetic RHS and
// any earlier operand retain their own old-value views.
func (d *Decompiler) immediateLocalPostUpdate(store *OpCode, ref *values.JavaRef, old values.JavaValue) (string, bool) {
	if d == nil || store == nil || ref == nil || ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil || ref.Type() == nil || old == nil {
		return "", false
	}
	primitive, ok := ref.Type().RawType().(*types.JavaPrimer)
	if !ok || primitive == nil {
		return "", false
	}
	protocol, ok := localPostProtocols[primitive.Name]
	if !ok || values.UnpackSoltValue(old) != ref {
		return "", false
	}
	// Backward traversal is bounded by this protocol's five or six instructions.
	n := 5
	if protocol.narrow != 0 {
		n++
	}
	path, closed := d.localUpdatePrivatePath(store, n)
	if !closed {
		return "", false
	}
	load := path[0]
	if !localPostAccess(load, protocol.load, true) || !localPostAccess(store, protocol.store, false) || GetRetrieveIdx(load) != GetStoreIdx(store) || path[2].Instr.OpCode != protocol.one {
		return "", false
	}
	duplicate := path[1]
	if duplicate.Instr.OpCode != protocol.dup {
		// Some compilers reload narrow locals instead of using DUP. The two
		// adjacent reads have no intervening effects, but equal slot numbers
		// alone do not prove they resolved to the same local binding.
		if !localPostAccess(duplicate, protocol.load, true) || GetRetrieveIdx(duplicate) != GetRetrieveIdx(load) || len(duplicate.stackProduced) == 0 {
			return "", false
		}
		word := duplicate.stackProduced[len(duplicate.stackProduced)-1]
		if word == nil || word == old || d.stackValueProducers[word] != duplicate || values.UnpackSoltValue(word) != ref {
			return "", false
		}
	}
	op := values.INC
	switch path[3].Instr.OpCode {
	case protocol.add:
	case protocol.sub:
		op = values.DEC
	default:
		return "", false
	}
	if protocol.narrow != 0 && path[4].Instr.OpCode != protocol.narrow {
		return "", false
	}
	// Validate the store encoding too; it has no successor in this certificate.
	if _, valid := localPostInstructionWidth(store); !valid || d.stackValueProducers[old] != load {
		return "", false
	}
	for _, produced := range load.stackProduced {
		if produced == old {
			return op, true
		}
	}
	return "", false
}

func localPostAccess(node *OpCode, base int, read bool) bool {
	if node == nil || node.Instr == nil {
		return false
	}
	opcode := node.Instr.OpCode
	if opcode == base {
		return true
	}
	fixed := OP_ILOAD_0 + (base-OP_ILOAD)*4
	if !read {
		fixed = OP_ISTORE_0 + (base-OP_ISTORE)*4
	}
	return opcode >= fixed && opcode < fixed+4
}

func localPostInstructionWidth(node *OpCode) (int, bool) {
	if node == nil || node.Instr == nil {
		return 0, false
	}
	access := LocalAccessOf(node.Instr.OpCode)
	if access.Read || access.Write {
		if node.IsWide {
			return 4, access.Wide && len(node.Data) == 2 && node.Instr.OpCode != OP_IINC
		}
		if access.Slot < 0 {
			return 2, len(node.Data) == 1 && node.Instr.OpCode != OP_IINC
		}
		return 1, len(node.Data) == 0
	}
	return 1, !node.IsWide && len(node.Data) == 0
}

// Both result variants share the same bounded, original private-edge proof.
func (d *Decompiler) localUpdatePrivatePath(store *OpCode, n int) ([]*OpCode, bool) {
	if d == nil || store == nil || n < 5 || n > 6 {
		return nil, false
	}
	path := make([]*OpCode, n)
	path[n-1] = store
	for i := n - 1; i > 0; i-- {
		node := path[i]
		if node == nil || len(node.Source) != 1 {
			return nil, false
		}
		path[i-1] = node.Source[0]
	}
	var handlers []int
	for i, node := range path {
		if node == nil || node.Instr == nil || node.IsCustom || node.IsCatch || node.IsTryCatchParent || d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1+int64(len(d.ExceptionTable))) != nil {
			return nil, false
		}
		coverage := d.handlersAt(node)
		if i == 0 {
			handlers = coverage
			continue
		}
		if !sameHandlerCoverage(handlers, coverage) {
			return nil, false
		}
		prev := path[i-1]
		width, valid := localPostInstructionWidth(prev)
		if !valid || len(prev.Target) != 1 || prev.Target[0] != node || int(prev.CurrentOffset)+width != int(node.CurrentOffset) {
			return nil, false
		}
	}
	return path, true
}

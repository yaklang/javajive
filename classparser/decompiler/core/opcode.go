package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/omap"
)

// OpCode is kept densely packed: its layout is part of parser memory behavior.
// All pointer/slice/interface fields come first, followed by the few 4/2-byte
// fields and finally the boolean flags, so alignment padding does not make every
// decoded instruction larger. Several historical fields (GetStackChange,
// StackInfo, IfNode) were never read and were removed.
type OpCode struct {
	Id                             int
	Instr                          *Instruction
	Data                           []byte
	Jmp                            int
	BranchTarget                   int
	TryNode                        *OpCode
	CatchNode                      []*CatchNode
	StackEntry                     *StackItem
	Ref                            *values.JavaRef
	ExceptionTypeIndexes           []uint16
	SwitchJmpCase                  *omap.OrderedMap[int, int32]
	SwitchJmpCase1                 *omap.OrderedMap[int, int]
	SwitchDefaultIndex             int
	stackProduced                  []values.JavaValue
	stackConsumed                  []values.JavaValue
	Source                         []*OpCode
	Target                         []*OpCode
	TrueNode, FalseNode, MergeNode *OpCode
	Info                           any
	conditionOpId                  int

	SwitchDefaultOffset int32
	CurrentOffset       uint16
	ExceptionTypeIndex  uint16

	IsWide           bool
	IsCatch          bool
	IsTryCatchParent bool
	Negative         bool
	IsTernaryNode    bool
	IsCustom         bool
	// TernaryChainArm marks a condition opcode that supplies its value into a DISTINCT nested
	// ternary arm (a right-leaning chain a?:b?:c?: or a structurally-rebuilt tree), as opposed
	// to a short-circuit &&/|| whose conditions all feed the SAME ternary condition. The
	// MergeIf pass must not fold a chain-arm condition into a &&/|| expression: doing so
	// collapses several distinct conditions into one and leaves the others' callbacks unfired,
	// leaking an empty stack slot. Short-circuit conditions are NOT marked and merge normally.
	TernaryChainArm bool
	// SelfOpFolded marks a putfield/putstatic whose stored value is the post-increment /
	// post-decrement of the field itself, with the old value reused on the stack (the
	// dup_x1/dup idiom). When set, statement generation skips the standalone assignment
	// because the side effect is folded into the `x++` / `x--` expression left on the stack,
	// keeping ternary/expression branches side-effect-free so they can be structured.
	SelfOpFolded bool
	// IincFoldedToPostOp marks an `iinc` that has been folded into a post-increment / decrement
	// expression (`i++` / `i--`) left on the operand stack, because javac emitted `iload X; iinc X`
	// (the loaded OLD value is still live when the iinc runs -> a post-op used inside an expression
	// such as `a[i++] = v`, `int j = i++`, `return i++`). When set, statement generation skips the
	// standalone `i++` statement; emitting it would increment BEFORE the use and change semantics
	// (the historical `i++; a[i] = v` bug). An adjacent prefix update is proved
	// separately when its following load resolves to the same original binding.
	IincFoldedToPostOp bool
	// A private adjacent ILOAD can consume the updated value as ++x/--x.
	// Mark only after resolving that load to the same original local binding.
	IincFoldedToPreOp bool
	// The original scalar load/dup/update/store protocol has its old word
	// represented by one postfix expression in the consumer's execution domain.
	LocalStoreFoldedToPostOp     bool
	LocalStoreFoldedToTypedPreOp bool
	// A proved prefix update consumes this DUP's one numeric materialization.
	LocalUpdateDupFolded bool
}
type CatchNode struct {
	ExceptionTypeIndex uint16
	StartIndex         uint16
	EndIndex           uint16
	OpCode             *OpCode
}

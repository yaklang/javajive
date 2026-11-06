package core

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// EdgeKind records JVM flow, independently of the mutable structuring graph.
type EdgeKind uint8

const (
	EdgeFallthrough EdgeKind = iota
	EdgeTaken
	EdgeCase
	EdgeDefault
	EdgeException
)

type SemanticEdge struct {
	From, To     *OpCode
	Kind         EdgeKind
	CaseValue    int32
	HandlerOrder int
}

// SemanticCFG is a method-local snapshot taken before expression/region rewrites.
// Exception edges leave the throwing instruction's INPUT local state. They are
// never confused with the synthetic try-entry anchors used by the old structurer.
type SemanticCFG struct {
	Nodes      []*OpCode
	Edges      []SemanticEdge
	incoming   map[*OpCode][]int
	outgoing   map[*OpCode][]int
	slots      map[int]*slotDefinitions
	Updates    int
	order      map[*OpCode]int
	Err        error
	MaxUpdates int
	Work       *workbudget.Budget
	GraphScans int
	SetWork    int
	// IndexPeak/IndexEmitted/IndexWork are T24 stream stats (not a linearity proof).
	IndexPeak    int
	IndexEmitted uint64
	IndexWork    uint64
	FunctionType *types.JavaFuncType
	IsStatic     bool
	entryWidth   map[int]int
	// IrreducibleError records a normal-flow reducibility failure that is safe to
	// defer until the statement CFG can lower the region to source dispatch.
	IrreducibleError error
	// analysis holds the T26 graph-analysis cache. SemanticCFG is immutable after
	// construction; epoch stays 0. Lazy-initialized on first GetOrCompute.
	analysis *graphAnalysisState
}

// MayThrowOpcode conservatively includes resolution/linkage, allocation, array,
// monitor and invocation failures, not only explicit athrow instructions.
func MayThrowOpcode(op int) bool {
	switch op {
	case OP_IDIV, OP_LDIV, OP_IREM, OP_LREM, OP_ATHROW,
		OP_IALOAD, OP_LALOAD, OP_FALOAD, OP_DALOAD, OP_AALOAD, OP_BALOAD, OP_CALOAD, OP_SALOAD,
		OP_IASTORE, OP_LASTORE, OP_FASTORE, OP_DASTORE, OP_AASTORE, OP_BASTORE, OP_CASTORE, OP_SASTORE,
		OP_GETFIELD, OP_PUTFIELD, OP_GETSTATIC, OP_PUTSTATIC,
		OP_INVOKEVIRTUAL, OP_INVOKESPECIAL, OP_INVOKESTATIC, OP_INVOKEINTERFACE, OP_INVOKEDYNAMIC,
		OP_NEW, OP_NEWARRAY, OP_ANEWARRAY, OP_MULTIANEWARRAY, OP_ARRAYLENGTH,
		OP_CHECKCAST, OP_INSTANCEOF, OP_MONITORENTER, OP_MONITOREXIT, OP_LDC, OP_LDC_W, OP_LDC2_W:
		return true
	}
	return false
}

func (d *Decompiler) buildSemanticCFG() (*SemanticCFG, error) {
	if err := d.validateControlFlow(); err != nil {
		return nil, err
	}
	g := &SemanticCFG{order: map[*OpCode]int{}, MaxUpdates: 1000000, incoming: map[*OpCode][]int{}, outgoing: map[*OpCode][]int{}, slots: map[int]*slotDefinitions{}, Work: d.Work, FunctionType: d.FunctionType}
	if d.FunctionContext != nil {
		g.IsStatic = d.FunctionContext.IsStatic
	}
	g.entryWidth = liveInSlotWidths(g.IsStatic, g.FunctionType)
	if d.MaxAnalysisUpdates > 0 {
		g.MaxUpdates = d.MaxAnalysisUpdates
	}
	for _, op := range d.opCodes {
		if op.Instr.OpCode != OP_START && op.Instr.OpCode != OP_END {
			g.order[op] = len(g.Nodes)
			g.Nodes = append(g.Nodes, op)
		}
	}
	target := func(pc int) *OpCode { return d.opCodes[d.offsetToOpcodeIndex[uint16(pc)]] }
	add := func(from, to *OpCode, kind EdgeKind, key int32, order int) error {
		if err := g.Work.Charge(workbudget.CounterGraphEdges, 1); err != nil {
			g.Err = err
			return err
		}
		i := len(g.Edges)
		g.Edges = append(g.Edges, SemanticEdge{from, to, kind, key, order})
		g.incoming[to] = append(g.incoming[to], i)
		g.outgoing[from] = append(g.outgoing[from], i)
		return nil
	}
	for i, op := range g.Nodes {
		if g.Err != nil {
			return g, g.Err
		}
		fall := true
		switch op.Instr.OpCode {
		case OP_RETURN, OP_IRETURN, OP_LRETURN, OP_FRETURN, OP_DRETURN, OP_ARETURN, OP_ATHROW:
			fall = false
		case OP_GOTO, OP_GOTO_W:
			if err := add(op, target(op.BranchTarget), EdgeTaken, 0, 0); err != nil {
				return g, err
			}
			fall = false
		case OP_JSR, OP_JSR_W, OP_RET:
			return nil, fmt.Errorf("semantic CFG requires inlined jsr/ret at PC %d", op.CurrentOffset)
		case OP_LOOKUPSWITCH, OP_TABLESWITCH:
			fall = false
			var switchErr error
			op.SwitchJmpCase.ForEach(func(v int, pc int32) bool {
				switchErr = add(op, target(int(pc)), EdgeCase, int32(v), 0)
				return switchErr == nil
			})
			if switchErr != nil {
				return g, switchErr
			}
			if err := add(op, target(int(op.SwitchDefaultOffset)), EdgeDefault, 0, 0); err != nil {
				return g, err
			}
		case OP_IFEQ, OP_IFNE, OP_IFLT, OP_IFGE, OP_IFGT, OP_IFLE, OP_IF_ICMPEQ, OP_IF_ICMPNE, OP_IF_ICMPLT, OP_IF_ICMPGE, OP_IF_ICMPGT, OP_IF_ICMPLE, OP_IF_ACMPEQ, OP_IF_ACMPNE, OP_IFNULL, OP_IFNONNULL:
			if err := add(op, target(op.BranchTarget), EdgeTaken, 0, 0); err != nil {
				return g, err
			}
		}
		if fall && i+1 < len(g.Nodes) {
			if err := add(op, g.Nodes[i+1], EdgeFallthrough, 0, 0); err != nil {
				return g, err
			}
		}
	}
	if err := g.addIndexedExceptionEdges(d, target, add); err != nil {
		return g, err
	}
	if g.Err != nil {
		return g, g.Err
	}
	if err := g.ValidateReducible(); err != nil {
		if !g.canDeferIrreducible() {
			return nil, err
		}
		g.IrreducibleError = err
	}
	return g, nil
}

// HasDeferredIrreducibleControlFlow reports whether SemanticCFG admitted a
// normal-edge irreducible method for later statement-level state dispatch.
func (d *Decompiler) HasDeferredIrreducibleControlFlow() bool {
	return d != nil && d.semanticCFG != nil && d.semanticCFG.IrreducibleError != nil
}

// irreducibleBoundaryStackSupported decides whether live JVM stack words may
// cross a control boundary that the statement-level state dispatcher will
// rewrite. An empty stack is always safe. A non-empty stack is safe when every
// carried word is already a source-level value (constant or local) or when an
// effectful word's producer reaches the boundary through an effect-free carrier
// path. Otherwise the dispatcher would move the word's evaluation past the
// boundary (for example, a field read carried across an invocation) and change
// observable behavior.
//
// The certificate is per word rather than per edge: a conditional can leave an
// older operand below the branch itself, and every remaining word must be
// independently stable.
func (d *Decompiler) irreducibleBoundaryStackSupported() bool {
	if d == nil || d.semanticCFG == nil {
		return false
	}
	normalIncoming := make(map[*OpCode]int)
	for _, edge := range d.semanticCFG.Edges {
		if edge.Kind == EdgeException {
			return false
		}
		if edge.To != nil {
			normalIncoming[edge.To]++
		}
	}
	for _, edge := range d.semanticCFG.Edges {
		if edge.From == nil || edge.From.Instr == nil {
			return false
		}
		op := edge.From.Instr.OpCode
		boundary := edge.Kind != EdgeFallthrough || normalIncoming[edge.To] > 1 ||
			isConditionalBranchOpcode(op) || op == OP_GOTO || op == OP_GOTO_W ||
			op == OP_TABLESWITCH || op == OP_LOOKUPSWITCH
		if !boundary {
			continue
		}
		for item := edge.From.StackEntry; item != nil && item.parent != nil; item = item.parent {
			if !d.boundaryStackWordSupported(item.value, edge.From) {
				return false
			}
		}
	}
	return d.irreducibleBoundaryStackMergesSupported()
}

// irreducibleBoundaryStackMergesSupported proves that a target with several
// incoming edges does not reuse one predecessor's expression for another path.
// Equal values need no phi. Unequal values are safe only when the target stack
// word is a materialized stack-merge phi populated on each edge.
func (d *Decompiler) irreducibleBoundaryStackMergesSupported() bool {
	incoming := map[*OpCode][]*OpCode{}
	for _, edge := range d.semanticCFG.Edges {
		if edge.Kind == EdgeException {
			return false
		}
		if edge.To == nil || edge.From == nil {
			continue
		}
		incoming[edge.To] = append(incoming[edge.To], edge.From)
	}
	for target, sources := range incoming {
		if target == nil || len(sources) < 2 {
			continue
		}
		perSource := make([][]values.JavaValue, 0, len(sources))
		for _, source := range sources {
			perSource = append(perSource, boundaryStackWords(source.StackEntry))
		}
		depth := len(perSource[0])
		for _, words := range perSource[1:] {
			if len(words) != depth {
				return false
			}
		}
		if depth == 0 {
			continue
		}
		for index := 0; index < depth; index++ {
			same := true
			for _, words := range perSource[1:] {
				if !boundaryStackWordsEquivalent(perSource[0][index], words[index]) {
					same = false
					break
				}
			}
			if same {
				continue
			}
			// Unequal per-path values must be materialized before the target runs.
			// stackConsumed[0] is the top word the target actually reads; using the
			// target's post-stack here would inspect a value the instruction just
			// produced (or an empty stack after a store).
			if index != 0 || len(target.stackConsumed) == 0 || !boundaryStackWordIsPhi(target.stackConsumed[0]) {
				return false
			}
		}
	}
	return true
}

func boundaryStackWords(entry *StackItem) []values.JavaValue {
	words := []values.JavaValue{}
	for item := entry; item != nil && item.parent != nil; item = item.parent {
		words = append(words, item.value)
	}
	return words
}

func boundaryStackWordIsPhi(value values.JavaValue) bool {
	value = values.UnpackSoltValue(value)
	ref, ok := value.(*values.JavaRef)
	return ok && ref != nil && ref.IsStackMergePhi()
}

// boundaryStackWordsEquivalent compares the values on two incoming paths. It
// is intentionally conservative: unknown value kinds compare by identity only,
// while literals, locals, field reads and calls compare structurally so the
// dispatcher may reuse an equal expression on every path.
func boundaryStackWordsEquivalent(first, second values.JavaValue) bool {
	return boundaryStackWordsEquivalentDepth(first, second, 0)
}

func boundaryStackWordsEquivalentDepth(first, second values.JavaValue, depth int) bool {
	if depth > 32 {
		return false
	}
	first = values.UnpackSoltValue(first)
	second = values.UnpackSoltValue(second)
	if first == nil || second == nil {
		return first == nil && second == nil
	}
	if first == second {
		return true
	}
	if values.IsNullLiteral(first) || values.IsNullLiteral(second) {
		return values.IsNullLiteral(first) && values.IsNullLiteral(second)
	}
	next := depth + 1
	switch left := first.(type) {
	case *values.JavaLiteral:
		right, ok := second.(*values.JavaLiteral)
		return ok && right != nil &&
			boundaryStackTypeEquivalent(left.Type(), right.Type()) &&
			reflect.DeepEqual(left.Data, right.Data) &&
			reflect.DeepEqual(left.Units, right.Units)
	case *values.JavaRef:
		right, ok := second.(*values.JavaRef)
		if !ok || right == nil {
			return false
		}
		if left.IsThis || right.IsThis {
			return left.IsThis && right.IsThis
		}
		return left.VarUid != "" && left.VarUid == right.VarUid
	case *values.JavaClassMember:
		right, ok := second.(*values.JavaClassMember)
		return ok && right != nil && left.Name == right.Name &&
			left.Member == right.Member && left.Description == right.Description &&
			boundaryStackTypeEquivalent(left.JavaType, right.JavaType)
	case *values.RefMember:
		right, ok := second.(*values.RefMember)
		return ok && right != nil && left.Member == right.Member &&
			boundaryStackTypeEquivalent(left.JavaType, right.JavaType) &&
			boundaryStackWordsEquivalentDepth(left.Object, right.Object, next)
	case *values.FunctionCallExpression:
		right, ok := second.(*values.FunctionCallExpression)
		if !ok || right == nil || left.ClassName != right.ClassName ||
			left.FunctionName != right.FunctionName || left.Descriptor != right.Descriptor ||
			left.IsStatic != right.IsStatic || len(left.Arguments) != len(right.Arguments) ||
			!boundaryStackTypeEquivalent(left.Type(), right.Type()) ||
			!boundaryStackTypeEquivalent(left.SourceReturnType, right.SourceReturnType) {
			return false
		}
		if !left.IsStatic && !boundaryStackWordsEquivalentDepth(left.Object, right.Object, next) {
			return false
		}
		for i := range left.Arguments {
			if !boundaryStackWordsEquivalentDepth(left.Arguments[i], right.Arguments[i], next) {
				return false
			}
		}
		return true
	case *values.JavaExpression:
		right, ok := second.(*values.JavaExpression)
		if !ok || right == nil || left.Op != right.Op || len(left.Values) != len(right.Values) ||
			!boundaryStackTypeEquivalent(left.Typ, right.Typ) {
			return false
		}
		for i := range left.Values {
			if !boundaryStackWordsEquivalentDepth(left.Values[i], right.Values[i], next) {
				return false
			}
		}
		return true
	case *values.CastExpression:
		right, ok := second.(*values.CastExpression)
		return ok && right != nil && boundaryStackTypeEquivalent(left.TargetType, right.TargetType) &&
			boundaryStackWordsEquivalentDepth(left.Value, right.Value, next)
	case *values.TernaryExpression:
		right, ok := second.(*values.TernaryExpression)
		return ok && right != nil &&
			boundaryStackWordsEquivalentDepth(left.Condition, right.Condition, next) &&
			boundaryStackWordsEquivalentDepth(left.TrueValue, right.TrueValue, next) &&
			boundaryStackWordsEquivalentDepth(left.FalseValue, right.FalseValue, next)
	case *values.JavaArrayMember:
		right, ok := second.(*values.JavaArrayMember)
		return ok && right != nil &&
			boundaryStackWordsEquivalentDepth(left.Object, right.Object, next) &&
			boundaryStackWordsEquivalentDepth(left.Index, right.Index, next)
	case *values.ArrayLengthExpression:
		right, ok := second.(*values.ArrayLengthExpression)
		return ok && right != nil && boundaryStackWordsEquivalentDepth(left.Array, right.Array, next)
	case *values.JavaCompare:
		right, ok := second.(*values.JavaCompare)
		return ok && right != nil &&
			boundaryStackWordsEquivalentDepth(left.JavaValue1, right.JavaValue1, next) &&
			boundaryStackWordsEquivalentDepth(left.JavaValue2, right.JavaValue2, next)
	default:
		return false
	}
}

func boundaryStackTypeEquivalent(first, second types.JavaType) bool {
	if first == nil || second == nil {
		return first == nil && second == nil
	}
	return reflect.DeepEqual(first.RawType(), second.RawType()) && first.IsArray() == second.IsArray() &&
		first.ArrayDim() == second.ArrayDim()
}

// boundaryStackWordSupported proves one live boundary word. A reference or
// constant is already a source value. A composite value is safe only when it
// has no unmodelled effects and its producer reaches the boundary directly
// through instructions which cannot observe or change its evaluation.
func (d *Decompiler) boundaryStackWordSupported(value values.JavaValue, source *OpCode) bool {
	if value == nil || values.IsNullLiteral(value) {
		return true
	}
	plain := values.UnpackSoltValue(value)
	if plain == nil {
		return true
	}
	switch plain := plain.(type) {
	case *values.JavaLiteral:
		return true
	case *values.JavaRef:
		// A JavaRef is an already materialized local read. It remains valid
		// unless the same JVM slot is overwritten before the boundary, which
		// would make a source-level name read observe the new value.
		return d.boundaryLocalRefSupported(plain, source)
	}
	effect, _ := values.InspectValue(plain)
	if effect&values.EffectOpaque != 0 {
		return false
	}
	if effect == 0 {
		return true
	}
	if _, allocation := plain.(*values.NewExpression); allocation {
		// An uninitialized allocation identity cannot be represented by a
		// source-level state variable; keep it on its original straight-line
		// constructor path instead of moving it into a dispatcher case.
		return false
	}
	producer := d.stackValueProducers[plain]
	if producer == nil {
		return false
	}
	return d.effectFreeCarrierPath(producer, source, boundaryStackValueIsReference(plain))
}

// boundaryLocalRefSupported keeps a loaded local value stable across the
// boundary. The producer load and the boundary source are the only permitted
// uses of that JVM slot on the carrier path; a store to the same slot would
// make later source text read a different value than the word already on the
// JVM stack.
func (d *Decompiler) boundaryLocalRefSupported(ref *values.JavaRef, source *OpCode) bool {
	if ref == nil {
		return true
	}
	producer := d.stackValueProducers[ref]
	slot := -1
	if producer != nil && producer.Instr != nil {
		slot = GetRetrieveIdx(producer)
	}
	if slot < 0 && ref.IsParam {
		if parameterSlot, ok := ref.OriginalParameterSlot(); ok {
			slot = parameterSlot
		}
	}
	if slot < 0 || producer == nil {
		// No load/store evidence: parameter, receiver, synthetic stack-merge
		// local, or a value already materialized by another proof.
		return true
	}
	return !d.sameSlotStoreOnCarrierPath(producer, source, slot)
}

func (d *Decompiler) sameSlotStoreOnCarrierPath(producer, source *OpCode, slot int) bool {
	if producer == nil || source == nil || slot < 0 {
		return false
	}
	found := false
	d.walkBoundaryCarrierPath(producer, source, func(node *OpCode) bool {
		if node == nil || node.Instr == nil {
			return false
		}
		if node != producer && GetStoreIdx(node) == slot {
			found = true
			return false
		}
		return true
	})
	return found
}

// boundaryStackValueIsReference treats unknown types as references so that
// writes on the carrier path are not assumed to be harmless.
func boundaryStackValueIsReference(value values.JavaValue) bool {
	if value == nil || value.Type() == nil {
		return true
	}
	typ := value.Type()
	if typ.IsArray() {
		return true
	}
	primer, ok := typ.RawType().(*types.JavaPrimer)
	if !ok || primer == nil {
		return true
	}
	return primer.Name == types.JavaString
}

// effectFreeCarrierPath checks every instruction on a producer-to-boundary path
// except the producer and the boundary source itself. The boundary source may
// be a conditional or goto: it has no operand evaluation left, and the target
// starts immediately after the synthetic state assignment. Local stores are
// harmless for primitive words because preserveStackAcrossWrite snapshots the
// word before such a store; references are not snapshotted, so any write is
// treated as a barrier for them.
func (d *Decompiler) effectFreeCarrierPath(producer, source *OpCode, reference bool) bool {
	return d.walkBoundaryCarrierPath(producer, source, func(node *OpCode) bool {
		return !boundaryCarrierBarrier(node.Instr, reference)
	})
}

// walkBoundaryCarrierPath visits every node on a producer-to-boundary path
// except the producer itself. It includes the boundary source: a fallthrough
// source can be an effectful instruction even though a branch or goto source
// is only control flow. Producer == source is accepted because the target case
// consumes the value the source already produced.
func (d *Decompiler) walkBoundaryCarrierPath(producer, source *OpCode, visit func(*OpCode) bool) bool {
	if producer == nil || producer.Instr == nil || source == nil || source.Instr == nil {
		return false
	}
	if producer == source {
		return true
	}
	canReach := map[*OpCode]bool{}
	pending := []*OpCode{source}
	for len(pending) > 0 {
		n := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if n == nil || canReach[n] {
			continue
		}
		if len(canReach) >= 512 {
			return false
		}
		canReach[n] = true
		if n == producer {
			continue
		}
		for _, pred := range n.Source {
			if pred != nil {
				pending = append(pending, pred)
			}
		}
	}
	if !canReach[producer] {
		return false
	}
	seen := map[*OpCode]bool{}
	pending = []*OpCode{producer}
	edges := 0
	for len(pending) > 0 {
		n := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if n == nil || seen[n] {
			continue
		}
		if len(seen) >= 512 {
			return false
		}
		seen[n] = true
		if n != producer && !visit(n) {
			return false
		}
		if n == source {
			continue
		}
		for _, next := range n.Target {
			if next == nil || !canReach[next] {
				continue
			}
			edges++
			if edges > 8192 {
				return false
			}
			pending = append(pending, next)
		}
	}
	return seen[source]
}

func boundaryCarrierBarrier(instr *Instruction, reference bool) bool {
	if instr == nil {
		return true
	}
	if reference && stackLifetimeWrite(instr.OpCode) {
		return true
	}
	switch instr.OpCode {
	case OP_INVOKEVIRTUAL, OP_INVOKESPECIAL, OP_INVOKESTATIC, OP_INVOKEINTERFACE, OP_INVOKEDYNAMIC,
		OP_GETFIELD, OP_PUTFIELD, OP_GETSTATIC, OP_PUTSTATIC,
		OP_IALOAD, OP_LALOAD, OP_FALOAD, OP_DALOAD, OP_AALOAD, OP_BALOAD, OP_CALOAD, OP_SALOAD,
		OP_IASTORE, OP_LASTORE, OP_FASTORE, OP_DASTORE, OP_AASTORE, OP_BASTORE, OP_CASTORE, OP_SASTORE,
		OP_ARRAYLENGTH, OP_NEW, OP_NEWARRAY, OP_ANEWARRAY, OP_MULTIANEWARRAY,
		OP_MONITORENTER, OP_MONITOREXIT, OP_ATHROW,
		OP_CHECKCAST, OP_INSTANCEOF,
		OP_IDIV, OP_LDIV, OP_FDIV, OP_DDIV, OP_IREM, OP_LREM, OP_FREM, OP_DREM,
		OP_LDC, OP_LDC_W, OP_LDC2_W:
		return true
	}
	return false
}

func (g *SemanticCFG) canDeferIrreducible() bool {
	for _, edge := range g.Edges {
		if edge.Kind == EdgeException {
			return false
		}
	}
	for _, node := range g.Nodes {
		if node == nil || node.Instr == nil {
			return false
		}
		switch node.Instr.OpCode {
		case OP_NEW, OP_MONITORENTER, OP_MONITOREXIT:
			return false
		}
	}
	return true
}

func liveInSlotWidths(isStatic bool, ft *types.JavaFuncType) map[int]int {
	out := map[int]int{}
	slot := 0
	if !isStatic {
		out[0] = 1
		slot = 1
	}
	if ft == nil {
		return out
	}
	for _, p := range ft.ParamTypes {
		w := 1
		if p != nil {
			w = GetTypeSize(p)
		}
		if w < 1 {
			w = 1
		}
		out[slot] = w
		slot += w
	}
	return out
}

func (g *SemanticCFG) liveInWidth(slot int) int {
	if g == nil || g.entryWidth == nil {
		return 0
	}
	return g.entryWidth[slot]
}

func (g *SemanticCFG) liveInIsCat2Tail(slot int) bool {
	if g == nil || slot <= 0 {
		return false
	}
	return g.liveInWidth(slot-1) == 2
}

func (g *SemanticCFG) addIndexedExceptionEdges(d *Decompiler, target func(int) *OpCode, add func(*OpCode, *OpCode, EdgeKind, int32, int) error) error {
	ranges := ExceptionTableAsRanges(d.ExceptionTable)
	throwOps := make([]*OpCode, 0, 8)
	for _, op := range g.Nodes {
		if op == nil || op.Instr == nil {
			continue
		}
		if MayThrowOpcode(op.Instr.OpCode) {
			throwOps = append(throwOps, op)
		}
	}
	var stats ExceptionIndexStats
	sweep, err := newExceptionSweep(ranges, g.Work, &stats)
	if err != nil {
		g.Err = err
		g.GraphScans += int(stats.IndexWork)
		g.IndexWork = stats.IndexWork
		return err
	}
	g.GraphScans += int(stats.IndexWork)
	var lastPC uint16
	have := false
	for _, op := range throwOps {
		if err := g.Work.Check(); err != nil {
			g.Err = err
			g.recordIndexStats(stats)
			return err
		}
		pc := op.CurrentOffset
		if !have || pc != lastPC {
			before := stats.IndexWork
			if err := sweep.advanceTo(pc, g.Work, &stats); err != nil {
				g.Err = err
				g.GraphScans += int(stats.IndexWork - before)
				g.recordIndexStats(stats)
				return err
			}
			g.GraphScans += int(stats.IndexWork - before)
			lastPC, have = pc, true
		}
		if len(sweep.active) == 0 {
			continue
		}
		buffered := 0
		for _, order := range sweep.active {
			if err := g.Work.Charge(workbudget.CounterGraphScans, 1); err != nil {
				g.Err = err
				g.recordIndexStats(stats)
				return err
			}
			g.GraphScans++
			stats.CandidateScans++
			h := sweep.table[order]
			buffered++
			if buffered > stats.PeakBuffered {
				stats.PeakBuffered = buffered
			}
			// add() charges CounterGraphEdges before appending the retained edge.
			if err := add(op, target(int(h.HandlerPc)), EdgeException, 0, order); err != nil {
				g.recordIndexStats(stats)
				return err
			}
			stats.EmittedEdges++
			if h.CatchType == 0 {
				break
			}
		}
	}
	g.recordIndexStats(stats)
	return nil
}

func (g *SemanticCFG) recordIndexStats(stats ExceptionIndexStats) {
	g.IndexPeak = stats.PeakBuffered
	g.IndexEmitted = stats.EmittedEdges
	g.IndexWork = stats.IndexWork
}

type definitionSet map[*OpCode]struct{} // nil is the stable method-entry definition
// Each slot is solved once on the immutable graph, then every load/store query is O(defs).
// This avoids repeated reverse walks and makes loop joins independent of DFS order.
type slotDefinitions struct {
	in        map[*OpCode]definitionSet
	reachable map[*OpCode]bool
}

func (g *SemanticCFG) solveSlot(slot int) *slotDefinitions {
	if cached := g.slots[slot]; cached != nil {
		return cached
	}
	facts := g.sparseSolveToSlotDefs(slot)
	g.slots[slot] = facts
	return facts
}

func (g *SemanticCFG) sparseSolveToSlotDefs(slot int) *slotDefinitions {
	facts := &slotDefinitions{in: map[*OpCode]definitionSet{}, reachable: map[*OpCode]bool{}}
	if g == nil || len(g.Nodes) == 0 {
		return facts
	}
	s := NewSparseReaching(g)
	if err := s.solve(slot); err != nil {
		if g.Err == nil {
			g.Err = err
		}
		return facts
	}
	arr := s.in[slot]
	reach := s.reach[slot]
	g.SetWork += int(s.merges)
	for i, set := range arr {
		if err := g.Work.Check(); err != nil {
			g.Err = err
			return facts
		}
		n := g.Nodes[i]
		if i >= len(reach) || !reach[i] {
			continue
		}
		if err := g.Work.Charge(workbudget.CounterSetElementWork, int64(len(set.ids))); err != nil {
			g.Err = err
			return facts
		}
		ds := definitionSet{}
		for _, id := range set.ids {
			if id == entryDefID {
				ds[nil] = struct{}{}
				continue
			}
			if int(id) >= 0 && int(id) < len(g.Nodes) {
				ds[g.Nodes[id]] = struct{}{}
			}
		}
		facts.in[n] = ds
		facts.reachable[n] = true
	}
	if len(g.Nodes) > 0 && facts.in[g.Nodes[0]] == nil {
		facts.in[g.Nodes[0]] = definitionSet{nil: {}}
		facts.reachable[g.Nodes[0]] = true
	}
	return facts
}

// ReachingDefinitions returns definition identities in bytecode order, including
// iinc's read/write identity. The separate entry flag is not a fabricated store.
func (g *SemanticCFG) ReachingDefinitions(at *OpCode, slot int) ([]*OpCode, bool) {
	facts := g.solveSlot(slot)
	set := facts.in[at]
	out := make([]*OpCode, 0, len(set))
	_, entry := set[nil]
	for op := range set {
		if op != nil {
			out = append(out, op)
		}
	}
	sort.Slice(out, func(i, j int) bool { return g.order[out[i]] < g.order[out[j]] })
	return out, entry
}

// reachingStores is the adapter used by existing JavaRef/web consumers. iinc
// updates the same source variable; follow its input definitions rather than
// requiring the old simulator to invent a second JavaRef for the increment.
func (d *Decompiler) reachingStores(at *OpCode, slot int) ([]*OpCode, bool) {
	if d.semanticCFG == nil {
		return reachingStoresOf(at, slot)
	}
	g := d.semanticCFG
	pending := []*OpCode{at}
	seen := map[*OpCode]bool{}
	defs := map[*OpCode]bool{}
	entry := false
	for len(pending) > 0 {
		cur := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if seen[cur] {
			continue
		}
		seen[cur] = true
		stores, e := g.ReachingDefinitions(cur, slot)
		entry = entry || e
		for _, store := range stores {
			if store.Instr.OpCode == OP_IINC {
				pending = append(pending, store)
			} else {
				defs[store] = true
			}
		}
	}
	out := make([]*OpCode, 0, len(defs))
	for op := range defs {
		out = append(out, op)
	}
	sort.Slice(out, func(i, j int) bool { return g.order[out[i]] < g.order[out[j]] })
	return out, entry
}

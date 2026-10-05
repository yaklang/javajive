package core

import (
	"slices"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A handler cannot access an array that exists only on the protected operand
// stack: that stack is discarded on exceptional entry. Prove that every use
// is an initializer store or the sole later invocation, with no local store,
// publication, alias, branch entry or handler-domain change. Named arrays keep
// the conservative rule because their partially filled state can be observed.
func (d *Decompiler) privateArrayFillUnobservable(first, last *Node, firstOp, lastOp *OpCode) bool {
	if d == nil || first == nil || last == nil || firstOp == nil || lastOp == nil {
		return false
	}
	assign, ok := first.Statement.(*statements.AssignStatement)
	if !ok || assign.ArrayMember != nil {
		return false
	}
	ref, ok := values.UnpackSoltValue(assign.LeftValue).(*values.JavaRef)
	array, arrayOK := assign.JavaValue.(*values.NewExpression)
	if !ok || ref == nil || ref.IsParam || ref.IsThis || !arrayOK || !array.HasOriginPC {
		return false
	}
	allocation := d.opcodeAtOffset(array.OriginPC)
	if allocation == nil || allocation.Instr == nil || allocation.Instr.OpCode != OP_ANEWARRAY ||
		!branchArraySinglePath(d, allocation, lastOp) {
		return false
	}
	uses := 0
	for _, op := range d.opCodes {
		if op == nil || op.Instr == nil {
			return false
		}
		for index, consumed := range op.stackConsumed {
			if values.UnpackSoltValue(consumed) != ref {
				continue
			}
			switch {
			case op.Instr.OpCode == OP_AASTORE && index == 2 && op.CurrentOffset > allocation.CurrentOffset:
				// The caller checks all element indices and removes the stores
				// atomically. Reject stores outside the one protected prefix.
				if !branchArraySinglePath(d, allocation, op) {
					return false
				}
			case op.Instr.OpCode == OP_DUP && len(op.stackConsumed) == 1 && op.CurrentOffset > allocation.CurrentOffset && op.CurrentOffset < lastOp.CurrentOffset:
			case isInvokeOpcode(op.Instr.OpCode) && op.CurrentOffset > lastOp.CurrentOffset && branchArraySinglePath(d, allocation, op):
				uses++
			default:
				return false
			}
		}
	}
	return uses == 1
}

// Constructor operands join the value DAG only during ParseStatement. Revisit
// proved ternary roots after that phase so calls nested in constructors are
// visible. Registration alone permits no motion; both inlining passes still
// require a private allocation/store/invocation path and complete use evidence.
func (d *Decompiler) RegisterNestedBranchArrayCalls() {
	if d == nil || d.Work != nil && (d.Work.CheckAlloc(int64(len(d.opcodeIdToRef)+len(d.opCodes)+len(d.branchArrayCalls)+len(d.valueTernaryMerges)+1)*128) != nil || d.Work.Charge(workbudget.CounterGraphScans, int64(len(d.opcodeIdToRef)+len(d.opCodes)+len(d.branchArrayCalls))) != nil) {
		return
	}
	dupRefs := map[string]bool{}
	for op, infos := range d.opcodeIdToRef {
		if d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, int64(len(infos))) != nil {
			return
		}
		if op == nil || op.Instr == nil {
			continue
		}
		switch op.Instr.OpCode {
		case OP_DUP, OP_DUP_X1, OP_DUP_X2, OP_DUP2, OP_DUP2_X1, OP_DUP2_X2:
			for _, info := range infos {
				if ref, ok := info[0].(*values.JavaRef); ok && ref != nil {
					dupRefs[ref.VarUid] = true
				}
			}
		}
	}
	seenCalls := map[*values.FunctionCallExpression]bool{}
	for _, candidate := range d.branchArrayCalls {
		seenCalls[candidate.call] = true
	}
	// Traversal provenance is part of the discovery state: a constructor
	// operand does not confer its source context on a nested ordinary call.
	type discovery struct {
		value           values.JavaValue
		constructorOnly bool
	}
	seen := map[discovery]bool{}
	pending := []discovery{}
	addConstructors := func(roots []values.JavaValue) {
		for _, root := range roots {
			pending = append(pending, discovery{root, true})
		}
	}
	// Constructor-owned operands are original invocation roots too.
	// Ordinary call/field contexts may acquire a generic source view in a later
	// dumper phase; their erased descriptor alone cannot authorize removing a
	// declaration. Keep those contexts until a source type-view proof exists.
	// A branch merge is one discovery path, never an ownership prerequisite.
	// Registration grants no motion: the same complete single-use, handler,
	// operand-order and source-reference proof still gates each transfer.
	for _, op := range d.opCodes {
		if op != nil && op.Instr != nil && isInvokeOpcode(op.Instr.OpCode) {
			if call := d.invokeFuncCall[op]; call != nil && call.IsSpecialInvoke && call.FunctionName == "<init>" && call.HasOriginPC && call.OriginPC == int(op.CurrentOffset) {
				// ParseStatement attaches a fresh constructor node to the original
				// NEW. Prefer that active source witness to the provisional invoke node.
				if n, ok := values.UnpackSoltValue(call.Object).(*values.NewExpression); ok && n != nil && sameBranchArrayInvocation(n.ConstructorCall, call) {
					call = n.ConstructorCall
				}
				pending = append(pending, discovery{call, true})
			}
		}
	}
	// Use the live source graph too: constructor statement construction can
	// replace a provisional invocation witness after stack simulation. Original
	// immutable invocation metadata still gates the active source node below.
	nodeSeen := map[*Node]bool{}
	nodes := []*Node{d.RootNode}
	for remaining := 512; len(nodes) > 0 && remaining > 0; remaining-- {
		n := nodes[len(nodes)-1]
		nodes = nodes[:len(nodes)-1]
		if n == nil || nodeSeen[n] {
			continue
		}
		nodeSeen[n] = true
		if d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return
		}
		switch st := n.Statement.(type) {
		case *statements.AssignStatement:
			addConstructors(d.branchArraySourceConstructors(st.JavaValue))
		case *statements.ReturnStatement:
			addConstructors(d.branchArraySourceConstructors(st.JavaValue))
		case *statements.ExpressionStatement:
			addConstructors(d.branchArraySourceConstructors(st.Expression))
		case *statements.ConditionStatement:
			addConstructors(d.branchArraySourceConstructors(st.Condition))
		}
		nodes = append(nodes, n.Next...)
	}
	for root := range d.valueTernaryMerges {
		pending = append(pending, discovery{root, false})
	}
	for budget := 512; len(pending) > 0 && budget > 0; budget-- {
		if d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return
		}
		item := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		v := item.value
		if v == nil || seen[item] {
			continue
		}
		seen[item] = true
		if n, ok := v.(*values.NewExpression); ok && n != nil && n.ConstructorCall != nil {
			pending = append(pending, discovery{n.ConstructorCall, item.constructorOnly})
		}
		if call, ok := v.(*values.FunctionCallExpression); ok && call != nil && !seenCalls[call] && (!item.constructorOnly || call.IsSpecialInvoke && call.FunctionName == "<init>") {
			seenCalls[call] = true
			original := d.opcodeAtOffset(call.OriginPC)
			if original == nil || !sameBranchArrayInvocation(d.invokeFuncCall[original], call) {
				continue
			}
			if d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, int64(len(call.Arguments))) != nil {
				return
			}
			for i, arg := range call.Arguments {
				ref, ok := values.UnpackSoltValue(arg).(*values.JavaRef)
				if !ok || ref == nil || !dupRefs[ref.VarUid] {
					continue
				}
				array := d.branchArrayReferenceOrigin(ref)
				if array != nil {
					d.branchArrayCalls = append(d.branchArrayCalls, branchArrayCall{call, i, ref, array})
				}
			}
		}
		if children, known := values.Children(v); known {
			for _, child := range children {
				pending = append(pending, discovery{child, item.constructorOnly})
			}
		}
	}
}

// Only NEW owns a completed constructor expression. Walk the known source DAG
// to find those roots without following JavaRef.Val or assigning ordinary calls
// a constructor's source type/target context. Unknown/cyclic/excess work yields
// no discovery rather than a partially inspected tree.
func (d *Decompiler) branchArraySourceConstructors(root values.JavaValue) []values.JavaValue {
	if d == nil || d.Work != nil && d.Work.CheckAlloc(512*128) != nil {
		return nil
	}
	remaining := 512
	path := map[values.JavaValue]bool{}
	var out []values.JavaValue
	var walk func(values.JavaValue) bool
	walk = func(v values.JavaValue) bool {
		remaining--
		if remaining < 0 || path[v] || d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return false
		}
		if v == nil {
			return true
		}
		path[v] = true
		defer delete(path, v)
		if n, ok := v.(*values.NewExpression); ok && n != nil && n.ConstructorCall != nil {
			out = append(out, n.ConstructorCall)
		}
		children, known := values.Children(v)
		if !known {
			return false
		}
		for _, child := range children {
			if !walk(child) {
				return false
			}
		}
		return true
	}
	if !walk(root) {
		return nil
	}
	return out
}

// A source NEW outside a conditional argument must still execute before the
// original condition and allocation. Linear paths are sufficient; across a
// proved value diamond use the original semantic CFG's dominance relation,
// including exceptional entries. PC order and endpoint handlers alone are not
// dominance and cannot establish that a producer ran on every incoming path.
func (d *Decompiler) branchArrayOriginalPrefixPrecedes(producer, target *OpCode) bool {
	if d == nil || producer == nil || target == nil || producer.CurrentOffset >= target.CurrentOffset || !sameHandlerCoverage(d.handlersAt(producer), d.handlersAt(target)) {
		return false
	}
	if singleLinearOpcodePathInHandlers(d, producer, target, d.handlersAt(target)) {
		return true
	}
	if d.semanticCFG == nil || d.Work != nil && (d.Work.CheckAlloc(int64(len(d.semanticCFG.Nodes))*128) != nil || d.Work.Charge(workbudget.CounterGraphScans, int64(len(d.semanticCFG.Nodes)+len(d.semanticCFG.Edges))) != nil) {
		return false
	}
	a, b := d.semanticCFG.indexOfNode(producer), d.semanticCFG.indexOfNode(target)
	return a >= 0 && b >= 0 && d.semanticCFG.GetOrCompute(AnalysisDominators, GraphAnalysisDomain{Roots: []int{0}, IncludeException: true}).Dominates(a, b)
}

// Condition callbacks have not necessarily filled the source SlotValue yet.
// Its immutable condition opcode identifies the already-proved value diamond.
// Transfer only into the one source arm that contains this exact invocation,
// and require that same original branch entry to dominate the allocation.
func (d *Decompiler) branchArraySelectedSourceArm(tern *values.TernaryExpression, call *values.FunctionCallExpression, allocation *OpCode) (values.JavaValue, *OpCode) {
	if d == nil || tern == nil || d.semanticCFG == nil || allocation == nil {
		return nil, nil
	}
	var branch *OpCode
	for _, op := range d.opCodes {
		if op != nil && op.Id == tern.ConditionFromOp {
			if branch != nil {
				return nil, nil
			}
			branch = op
		}
	}
	if branch == nil || branch.Instr == nil || !isConditionalBranchOpcode(branch.Instr.OpCode) || !branch.TernaryChainArm || !d.branchArrayOriginalPrefixPrecedes(branch, allocation) {
		return nil, nil
	}
	yes, yKnown := d.branchArrayCallOccurrences(tern.TrueValue, call)
	no, nKnown := d.branchArrayCallOccurrences(tern.FalseValue, call)
	if !yKnown || !nKnown || yes+no != 1 {
		return nil, nil
	}
	var taken, fallthroughEntry *OpCode
	for _, edge := range d.semanticCFG.Edges {
		if edge.From != branch {
			continue
		}
		switch edge.Kind {
		case EdgeTaken:
			if taken != nil {
				return nil, nil
			}
			taken = edge.To
		case EdgeFallthrough:
			if fallthroughEntry != nil {
				return nil, nil
			}
			fallthroughEntry = edge.To
		case EdgeException:
		default:
			return nil, nil
		}
	}
	if taken == nil || fallthroughEntry == nil || taken == fallthroughEntry {
		return nil, nil
	}
	// The value builder uses the inverse jump predicate: source true is
	// original fallthrough; source false is the original taken edge.
	arm, entry := tern.TrueValue, fallthroughEntry
	if no == 1 {
		arm, entry = tern.FalseValue, taken
	}
	a, b := d.semanticCFG.indexOfNode(entry), d.semanticCFG.indexOfNode(allocation)
	if a < 0 || b < 0 || !d.semanticCFG.GetOrCompute(AnalysisDominators, GraphAnalysisDomain{Roots: []int{0}, IncludeException: true}).Dominates(a, b) {
		return nil, nil
	}
	return arm, branch
}

// Reference cycles/deep alias chains are unknown, never permission to inline.
// This discovery does not mutate a reference, assign it a source name or change
// which original NEW belongs to a later consumer.
func (d *Decompiler) branchArrayReferenceOrigin(value values.JavaValue) *values.NewExpression {
	seen := map[values.JavaValue]bool{}
	for remaining := 512; remaining > 0; remaining-- {
		if value == nil || seen[value] || d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return nil
		}
		seen[value] = true
		switch v := value.(type) {
		case *values.JavaRef:
			if v == nil || v.IsParam || v.IsThis || v.CustomValue != nil || v.StackVar != nil {
				return nil
			}
			value = v.Val
		case *values.SlotValue:
			if v == nil {
				return nil
			}
			value = v.GetValue()
		case *values.NewExpression:
			if v != nil && v.HasOriginPC && v.JavaType != nil && v.JavaType.IsArray() {
				return v
			}
			return nil
		default:
			return nil
		}
	}
	return nil
}

// Transfer a private branch array to its invocation before condition callbacks
// flatten the branch graph. A delayed transfer leaves the array definition as
// a second normal successor of a try node; it must never become a catch body.
func (d *Decompiler) InlinePrivateBranchArrayDefinitions() int {
	if d == nil || len(d.branchArrayCalls) == 0 || d.getenv("JDEC_BRANCH_ARRAY_INLINE_OFF") != "" {
		return 0
	}
	if d.Work != nil && (d.Work.CheckAlloc(int64(len(d.branchArrayCalls))*64) != nil || d.Work.Charge(workbudget.CounterGraphScans, int64(len(d.branchArrayCalls))) != nil) {
		return 0
	}
	changed := 0
	// Fold the last operand first. Earlier arrays become adjacent to the same
	// consumer only if every intervening array definition was safely transferred.
	// A refused/public later array remains a barrier, never a partial reordering.
	candidates := slices.Clone(d.branchArrayCalls)
	slices.SortStableFunc(candidates, func(a, b branchArrayCall) int {
		if a.array == nil || b.array == nil {
			return 0
		}
		return b.array.OriginPC - a.array.OriginPC
	})
	for _, candidate := range candidates {
		call, ref, array := candidate.call, candidate.ref, candidate.array
		if call == nil || ref == nil || array == nil || call.FuncType == nil || candidate.argIndex < 0 ||
			candidate.argIndex >= len(call.Arguments) || candidate.argIndex >= len(call.FuncType.ParamTypes) ||
			values.UnpackSoltValue(call.Arguments[candidate.argIndex]) != ref || !array.HasOriginPC ||
			!array.HasEvaluationEndPC || len(array.Initializer) == 0 || array.EvaluationEndPC >= call.OriginPC ||
			!branchArrayKeepsArgumentView(array, ref, call.FuncType.ParamTypes[candidate.argIndex]) {
			continue
		}
		valid := true
		for _, item := range array.Initializer {
			_, known := values.Children(item)
			effect, _ := values.InspectValue(item)
			valid = valid && known && effect&values.EffectOpaque == 0
		}
		allocation, invocation := d.opcodeAtOffset(array.OriginPC), d.opcodeAtOffset(call.OriginPC)
		if !valid || allocation == nil || invocation == nil || !branchArraySinglePath(d, allocation, invocation) {
			continue
		}
		valid = d.branchArrayReceiverPrecedes(call, allocation)
		for _, earlier := range call.Arguments[:candidate.argIndex] {
			valid = valid && d.branchOperandPrecedesArray(earlier, allocation)
		}
		uses, stores := 0, 0
		for _, op := range d.opCodes {
			if op == nil || op.Instr == nil {
				valid = false
				continue
			}
			if op.CurrentOffset >= allocation.CurrentOffset && op.CurrentOffset < invocation.CurrentOffset && LocalAccessOf(op.Instr.OpCode).Write {
				valid = false
			}
			for index, v := range op.stackConsumed {
				if values.UnpackSoltValue(v) != ref {
					continue
				}
				if op == invocation {
					uses++
				} else if op.Instr.OpCode == OP_AASTORE && index == 2 && int(op.CurrentOffset) > array.OriginPC && int(op.CurrentOffset) <= array.EvaluationEndPC {
					stores++
				} else {
					valid = false
				}
			}
		}
		if !valid || uses != 1 || stores != len(array.Initializer) {
			continue
		}
		nodes, _ := guardGraph(d.RootNode)
		var definition *Node
		for _, node := range nodes {
			assign, ok := node.Statement.(*statements.AssignStatement)
			if ok && assign.ArrayMember == nil && values.UnpackSoltValue(assign.LeftValue) == ref && assign.JavaValue == array {
				if definition != nil {
					valid = false
				}
				definition = node
			}
		}
		if !valid || definition == nil || definition.IsTryCatch || definition.IsCatchStart || !d.branchArrayImmediateConsumer(definition, call, allocation) {
			continue
		}
		original := call.Arguments[candidate.argIndex]
		call.Arguments[candidate.argIndex] = array
		for _, node := range nodes {
			if assign, ok := node.Statement.(*statements.AssignStatement); ok && node.IsCatchStart {
				if exception, ok := values.UnpackSoltValue(assign.JavaValue).(*values.CustomValue); ok && exception.Flag == "exception" &&
					len(exception.Captures) == 0 && values.UnpackSoltValue(assign.LeftValue) != ref {
					// The handler's synthetic stack exception has no local input.
					// Its source closure is opaque but cannot hide an array read.
					continue
				}
			}
			if node != definition && statementReferencesLocal(node.Statement, ref) {
				valid = false
			}
		}
		if !valid {
			call.Arguments[candidate.argIndex] = original
			continue
		}
		for _, source := range slices.Clone(definition.Source) {
			source.ReplaceNextSliceKeepOrder(definition, slices.Clone(definition.Next))
		}
		definition.RemoveAllNext()
		definition.RemoveAllSource()
		changed++
	}
	return changed
}

// Source reconstruction may replace a provisional constructor node, but
// only the original invocation's immutable position/kind/member/descriptor
// identifies the same evaluation. Printed text and equal parameter counts do
// not establish that identity.
func sameBranchArrayInvocation(a, b *values.FunctionCallExpression) bool {
	return a != nil && b != nil && a.HasOriginPC && b.HasOriginPC && a.OriginPC == b.OriginPC && a.Kind == b.Kind && a.IsSpecialInvoke == b.IsSpecialInvoke && a.IsStatic == b.IsStatic && a.ClassName == b.ClassName && a.FunctionName == b.FunctionName && a.Descriptor == b.Descriptor
}

// A constructor receiver is an original uninitialized NEW, not the completed
// object expression (whose arguments include this array). DUP reproduces its
// stack identity, never a second allocation. Check the original NEW witness and
// constructor identity instead of treating the completed cyclic tree as a
// receiver effect or counting DUP as another receiver producer.
func (d *Decompiler) branchArrayReceiverPrecedes(call *values.FunctionCallExpression, allocation *OpCode) bool {
	if call == nil {
		return false
	}
	n, ok := values.UnpackSoltValue(call.Object).(*values.NewExpression)
	if !ok {
		return d.branchOperandPrecedesArray(call.Object, allocation)
	}
	if n == nil || allocation == nil || n.ConstructorCall != call || !n.HasOriginPC || !call.HasOriginPC || !call.IsSpecialInvoke || call.FunctionName != "<init>" {
		return false
	}
	original := d.opcodeAtOffset(n.OriginPC)
	invoke := d.opcodeAtOffset(call.OriginPC)
	if original == nil || original.Instr == nil || original.Instr.OpCode != OP_NEW || original.CurrentOffset >= allocation.CurrentOffset || invoke == nil || invoke.Instr == nil || invoke.Instr.OpCode != OP_INVOKESPECIAL || !sameBranchArrayInvocation(d.invokeFuncCall[invoke], call) || !singleLinearOpcodePathInHandlers(d, original, allocation, d.handlersAt(allocation)) {
		return false
	}
	for _, v := range original.stackProduced {
		if values.UnpackSoltValue(v) == n {
			return true
		}
	}
	return false
}

// A private reference and a linear bytecode path prove ownership, not motion.
// Its definition must now be immediately followed by its unique source consumer;
// otherwise a void call, publication or another allocation could be crossed.
// Within that consumer, every expression evaluated before the invocation must
// already have its original producer before the array. References are uses, not
// permission to follow another definition's effects into the prospective tree.
func (d *Decompiler) branchArrayImmediateConsumer(definition *Node, call *values.FunctionCallExpression, allocation *OpCode) bool {
	if d == nil || definition == nil || call == nil || allocation == nil || len(definition.Next) != 1 || d.Work != nil && d.Work.CheckAlloc(512*128) != nil {
		return false
	}
	consumer := definition.Next[0]
	if consumer == nil || consumer.IsTryCatch || consumer.IsCatchStart {
		return false
	}
	var root values.JavaValue
	var prefix []values.JavaValue
	switch s := consumer.Statement.(type) {
	case *statements.AssignStatement:
		// Assignment evaluates an address, not a field/element load. Prove
		// receiver/index producers without inventing a GETFIELD/AALOAD witness.
		switch left := values.UnpackSoltValue(s.LeftValue).(type) {
		case *values.JavaRef:
		case *values.JavaClassMember: // PUTSTATIC/class initialization remains after its RHS.
		case *values.RefMember:
			if left == nil || !d.branchArrayEarlierOperandPrecedes(left.Object, allocation) {
				return false
			}
			prefix = append(prefix, left.Object)
		default:
			if s.ArrayMember == nil {
				return false
			}
		}
		if s.ArrayMember != nil && (!d.branchArrayEarlierOperandPrecedes(s.ArrayMember.Object, allocation) || !d.branchArrayEarlierOperandPrecedes(s.ArrayMember.Index, allocation)) {
			return false
		}
		if s.ArrayMember != nil {
			prefix = append(prefix, s.ArrayMember.Object, s.ArrayMember.Index)
		}
		root = s.JavaValue
	case *statements.ReturnStatement:
		root = s.JavaValue
	case *statements.ExpressionStatement:
		root = s.Expression
	case *statements.ConditionStatement:
		root = s.Condition
	default:
		return false
	}
	count, known := d.branchArrayCallOccurrences(root, call)
	if !known || count != 1 {
		return false
	}
	seen := map[values.JavaValue]bool{}
	remaining := 512
	var contains func(values.JavaValue) bool
	contains = func(v values.JavaValue) bool {
		remaining--
		if remaining < 0 || v == nil || seen[v] || d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return false
		}
		seen[v] = true
		if v == call {
			return true
		}
		var children []values.JavaValue
		if n, ok := v.(*values.NewExpression); ok {
			// Allocation/class initialization happens before its constructor arguments.
			if n == nil || n.ConstructorCall == nil || !n.HasOriginPC {
				return false
			}
			producer := d.opcodeAtOffset(n.OriginPC)
			if producer == nil || producer.Instr == nil || producer.Instr.OpCode != OP_NEW || producer.CurrentOffset >= allocation.CurrentOffset || !d.branchArrayOriginalPrefixPrecedes(producer, allocation) {
				return false
			}
			// The enclosing NEW must also retain its order relative to an
			// assignment address or an enclosing call's earlier operands. Proving
			// only that they precede the array would miss NEW/class-init ordering.
			for _, earlier := range prefix {
				if !d.branchArrayEarlierOperandPrecedes(earlier, producer) {
					return false
				}
			}
			if n.ConstructorCall == call {
				return true
			}
			prefix = append(prefix, n)
			children = n.ConstructorCall.Arguments
		} else if tern, ok := v.(*values.TernaryExpression); ok {
			arm, branch := d.branchArraySelectedSourceArm(tern, call, allocation)
			if arm == nil || branch == nil {
				return false
			}
			for _, earlier := range prefix {
				if n, ok := earlier.(*values.NewExpression); ok && n.HasOriginPC {
					if !d.branchArrayOriginalPrefixPrecedes(d.opcodeAtOffset(n.OriginPC), branch) {
						return false
					}
				} else if !d.branchArrayEarlierOperandPrecedes(earlier, branch) {
					return false
				}
			}
			return contains(arm)
		} else {
			var known bool
			children, known = values.Children(v)
			if !known {
				return false
			}
		}
		for _, child := range children {
			count, known := d.branchArrayCallOccurrences(child, call)
			// NewExpression exposes constructor arguments, so the constructor call
			// identity itself needs an explicit witness below as well.
			n, isNew := values.UnpackSoltValue(child).(*values.NewExpression)
			if known && (count > 0 || isNew && n != nil && n.ConstructorCall == call) {
				return contains(child)
			}
			if !known || !d.branchArrayEarlierOperandPrecedes(child, allocation) {
				return false
			}
			prefix = append(prefix, child)
		}
		return false
	}
	return contains(root)
}

// A source invocation evaluated before a conditional operand remains earlier
// on every path to that operand. A linear mutable graph cannot prove this when
// the conditional creates a diamond. Require the exact original invocation,
// original produced value identity, same handlers and exceptional dominance.
func (d *Decompiler) branchArrayEarlierOperandPrecedes(value values.JavaValue, allocation *OpCode) bool {
	if d.branchOperandPrecedesArray(value, allocation) {
		return true
	}
	if n, ok := values.UnpackSoltValue(value).(*values.NewExpression); ok {
		if n == nil || !n.HasOriginPC || n.ConstructorCall == nil || !n.ConstructorCall.HasOriginPC {
			return false
		}
		create, invoke := d.opcodeAtOffset(n.OriginPC), d.opcodeAtOffset(n.ConstructorCall.OriginPC)
		if create == nil || create.Instr == nil || create.Instr.OpCode != OP_NEW || invoke == nil || invoke.Instr == nil || invoke.Instr.OpCode != OP_INVOKESPECIAL || !sameBranchArrayInvocation(d.invokeFuncCall[invoke], n.ConstructorCall) || !d.branchArrayOriginalPrefixPrecedes(create, invoke) || !d.branchArrayOriginalPrefixPrecedes(invoke, allocation) {
			return false
		}
		for _, produced := range create.stackProduced {
			if values.UnpackSoltValue(produced) == n {
				return true
			}
		}
		return false
	}
	call, ok := values.UnpackSoltValue(value).(*values.FunctionCallExpression)
	if !ok || call == nil || !call.HasOriginPC {
		return false
	}
	producer := d.opcodeAtOffset(call.OriginPC)
	if producer == nil || producer.Instr == nil || !isInvokeOpcode(producer.Instr.OpCode) || !sameBranchArrayInvocation(d.invokeFuncCall[producer], call) || !d.branchArrayOriginalPrefixPrecedes(producer, allocation) {
		return false
	}
	for _, produced := range producer.stackProduced {
		if values.UnpackSoltValue(produced) == call {
			return true
		}
	}
	return false
}

// Count the actual source invocation, including the constructor node owned
// by NEW. Shared occurrences count twice; cycles, opaque trees and exhausted
// work are unknown. JavaRef is deliberately a leaf, never a hidden definition.
func (d *Decompiler) branchArrayCallOccurrences(root values.JavaValue, target *values.FunctionCallExpression) (int, bool) {
	remaining := 512
	path := map[values.JavaValue]bool{}
	var walk func(values.JavaValue) (int, bool)
	walk = func(v values.JavaValue) (int, bool) {
		remaining--
		if remaining < 0 || d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return 0, false
		}
		if v == nil {
			return 0, true
		}
		if path[v] {
			return 0, false
		}
		if v == target {
			return 1, true
		}
		path[v] = true
		defer delete(path, v)
		if n, ok := v.(*values.NewExpression); ok && n != nil && n.ConstructorCall == target {
			return 1, true
		}
		children, known := values.Children(v)
		if !known {
			return 0, false
		}
		count := 0
		for _, child := range children {
			n, known := walk(child)
			if !known {
				return 0, false
			}
			count += n
		}
		return count, true
	}
	return walk(root)
}

// Java evaluates a receiver and earlier arguments before this array argument.
// Effectful operands are admitted only when their original producer already
// lies on that same private prefix, so moving the array into the call retains
// their relative order. Identity alone is insufficient across a branch.
func (d *Decompiler) branchOperandPrecedesArray(value values.JavaValue, allocation *OpCode) bool {
	effect, _ := values.InspectValue(value)
	if effect == 0 {
		return true
	}
	if effect&values.EffectOpaque != 0 || allocation == nil {
		return false
	}
	matches := 0
	for _, producer := range d.opCodes {
		if producer == nil || producer.Instr == nil || producer.CurrentOffset >= allocation.CurrentOffset {
			continue
		}
		for _, produced := range producer.stackProduced {
			if values.UnpackSoltValue(produced) == values.UnpackSoltValue(value) &&
				singleLinearOpcodePathInHandlers(d, producer, allocation, d.handlersAt(allocation)) {
				matches++
			}
		}
	}
	return matches == 1
}

// valueOccurrences counts an exact expression object in the unreduced value tree.
// A cycle or an opaque value is not proof that a call occurs only once.
func valueOccurrences(value, target values.JavaValue, path map[values.JavaValue]bool) (int, bool) {
	if value == nil {
		return 0, true
	}
	if path[value] {
		return 0, false
	}
	if value == target {
		return 1, true
	}
	path[value] = true
	defer delete(path, value)
	children, known := values.Children(value)
	if !known {
		return 0, false
	}
	count := 0
	for _, child := range children {
		n, ok := valueOccurrences(child, target, path)
		if !ok {
			return 0, false
		}
		count += n
	}
	return count, true
}

type branchArrayReturn struct {
	list  *[]statements.Statement
	index int
}

// The allocation-to-invocation suffix must have no alternate entry. A single
// successor alone would allow an unrelated branch to join after allocation and
// execute the newly inlined allocation when the original path never did.
func branchArraySinglePath(d *Decompiler, allocation, invocation *OpCode) bool {
	if !singleLinearOpcodePathInHandlers(d, allocation, invocation, d.handlersAt(allocation)) {
		return false
	}
	for cur := allocation; cur != invocation; {
		next := cur.Target[0]
		if len(next.Source) != 1 || next.Source[0] != cur {
			return false
		}
		cur = next
	}
	return true
}

// findBranchArrayReturn accepts only statement/value trees it can inspect.
// Boolean reduction can fold a shared call that occurs twice in the value DAG
// into one rendered occurrence; require that final occurrence to be unique.
func findBranchArrayReturn(body *[]statements.Statement, call *values.FunctionCallExpression, ctx *class_context.ClassContext) (*branchArrayReturn, bool) {
	var found *branchArrayReturn
	count := 0
	callText := call.String(ctx)
	remaining := 512
	var walk func(*[]statements.Statement) bool
	walk = func(list *[]statements.Statement) bool {
		for i, statement := range *list {
			remaining--
			if remaining < 0 || statement == nil {
				return false
			}
			var valuesToCheck []values.JavaValue
			switch s := statement.(type) {
			case *statements.ReturnStatement:
				valuesToCheck = []values.JavaValue{s.JavaValue}
			case *statements.AssignStatement:
				valuesToCheck = []values.JavaValue{s.JavaValue, s.ArrayMember}
			case *statements.ExpressionStatement:
				valuesToCheck = []values.JavaValue{s.Expression}
			case *statements.ConditionStatement:
				valuesToCheck = []values.JavaValue{s.Condition}
			case *statements.IfStatement:
				valuesToCheck = []values.JavaValue{s.Condition}
				if !walk(&s.IfBody) || !walk(&s.ElseBody) {
					return false
				}
			case *statements.TryCatchStatement:
				if !walk(&s.TryBody) {
					return false
				}
				for i := range s.CatchBodies {
					if !walk(&s.CatchBodies[i]) {
						return false
					}
				}
			case *statements.DoWhileStatement:
				valuesToCheck = []values.JavaValue{s.ConditionValue}
				if !walk(&s.Body) {
					return false
				}
			case *statements.SynchronizedStatement:
				valuesToCheck = []values.JavaValue{s.Argument}
				if !walk(&s.Body) {
					return false
				}
			case *statements.MiddleStatement, *statements.NewStatement, *statements.GOTOStatement:
				continue
			default:
				return false
			}
			for _, value := range valuesToCheck {
				n, known := valueOccurrences(value, call, map[values.JavaValue]bool{})
				if !known {
					return false
				}
				if n != 0 {
					valueConsumer := false
					switch consumer := statement.(type) {
					case *statements.ReturnStatement:
						valueConsumer = true
					case *statements.AssignStatement:
						_, local := values.UnpackSoltValue(consumer.LeftValue).(*values.JavaRef)
						valueConsumer = local && consumer.ArrayMember == nil && value == consumer.JavaValue
					}
					if !valueConsumer || strings.Count(value.String(ctx), callText) != 1 {
						return false
					}
					count++
					found = &branchArrayReturn{list: list, index: i}
				}
			}
		}
		return true
	}
	ok := walk(body)
	return found, ok && count == 1 && found != nil
}

func branchArrayInertElement(item values.JavaValue) bool {
	switch v := values.UnpackSoltValue(item).(type) {
	case *values.JavaLiteral:
		return v != nil
	case *values.JavaRef:
		return v != nil && v.CustomValue == nil && v.StackVar == nil
	default:
		return false
	}
}

// InlineDroppedBranchArrayCalls repairs only a branch-local DUP temporary whose
// definition was removed when a short-circuit value merge became one return
// expression. A live allocation in the final statement tree is never
// duplicated. The bytecode checks also keep the allocation in its original arm,
// after all earlier call operands and under the same exception handlers.
func (d *Decompiler) InlineDroppedBranchArrayCalls(body []statements.Statement) int {
	if d == nil || d.getenv("JDEC_BRANCH_ARRAY_INLINE_OFF") != "" {
		return 0
	}
	changed := 0
	for _, candidate := range d.branchArrayCalls {
		call, ref, array := candidate.call, candidate.ref, candidate.array
		if call == nil || ref == nil || array == nil || call.FuncType == nil ||
			candidate.argIndex < 0 || candidate.argIndex >= len(call.Arguments) ||
			candidate.argIndex >= len(call.FuncType.ParamTypes) ||
			values.UnpackSoltValue(call.Arguments[candidate.argIndex]) != ref ||
			!array.IsArray() || !array.HasOriginPC || !array.HasEvaluationEndPC ||
			len(array.Initializer) == 0 || array.EvaluationEndPC >= call.OriginPC ||
			!branchArrayKeepsArgumentView(array, ref, call.FuncType.ParamTypes[candidate.argIndex]) {
			continue
		}
		// The whole private allocation-to-call path preserves initializer
		// order. Closed field/call operands can remain inside that expression;
		// opaque captures cannot establish the complete evaluation tree.
		literal := true
		for _, item := range array.Initializer {
			if !branchArrayInertElement(item) {
				effect, _ := values.InspectValue(item)
				_, known := values.Children(item)
				if !known || effect&values.EffectOpaque != 0 {
					literal = false
					break
				}
			}
		}
		if !call.IsStatic && call.Object != nil {
			if effect, _ := values.InspectValue(call.Object); effect != 0 {
				literal = false
			}
		}

		for _, earlier := range call.Arguments[:candidate.argIndex] {
			effect, _ := values.InspectValue(earlier)
			if effect != 0 {
				literal = false
				break
			}
		}
		if !literal {
			continue
		}
		allocation, invocation := d.opcodeAtOffset(array.OriginPC), d.opcodeAtOffset(call.OriginPC)
		if allocation == nil || invocation == nil || allocation.Instr == nil || invocation.Instr == nil ||
			allocation.Instr.OpCode != OP_ANEWARRAY ||
			!branchArraySinglePath(d, allocation, invocation) {
			continue
		}
		// A local element must retain its value at the original array store.
		// Even an unrelated slot write is conservatively rejected here; this
		// avoids changing captured values while moving the initializer.
		writesLocal := false
		for cur := allocation; cur != invocation; cur = cur.Target[0] {
			if cur.Instr != nil && LocalAccessOf(cur.Instr.OpCode).Write {
				writesLocal = true
				break
			}
		}
		if writesLocal {
			continue
		}
		uses, stores := 0, 0
		for _, op := range d.opCodes {
			for index, consumed := range op.stackConsumed {
				if values.UnpackSoltValue(consumed) != ref {
					continue
				}
				if op == invocation {
					uses++
				} else if op.Instr != nil && op.Instr.OpCode == OP_AASTORE && index == 2 &&
					int(op.CurrentOffset) > array.OriginPC && int(op.CurrentOffset) <= array.EvaluationEndPC {
					stores++
				} else {
					uses += 2
				}
			}
		}
		if uses != 1 || stores != len(array.Initializer) {
			continue
		}
		// A stranded array assignment immediately AFTER its return cannot run;
		// the CFG builder placed it there when flattening the short-circuit arm.
		// Remove exactly that dead definition as the call takes ownership of its
		// allocation. An earlier, live definition blocks this rewrite.
		location, ok := findBranchArrayReturn(&body, call, d.FunctionContext)
		if !ok {
			continue
		}
		var stranded *statements.AssignStatement
		items := *location.list
		if location.index+1 < len(items) {
			assign, isAssign := items[location.index+1].(*statements.AssignStatement)
			if isAssign && assign.ArrayMember == nil && assign.JavaValue == array {
				left, isRef := values.UnpackSoltValue(assign.LeftValue).(*values.JavaRef)
				if !isRef || !values.SameLocal(left, ref) || location.index+2 != len(items) {
					continue
				}
				stranded = assign
			} else if _, returned := items[location.index].(*statements.ReturnStatement); returned {
				continue
			}
		}
		original := call.Arguments[candidate.argIndex]
		call.Arguments[candidate.argIndex] = array
		if statementTreeReferencesLocal(body, ref, stranded) {
			call.Arguments[candidate.argIndex] = original
			continue
		}
		if stranded != nil {
			*location.list = items[:len(items)-1]
		}
		changed++
	}
	return changed
}

// Inline the same array view that the existing argument already presents.
// A generic/covariant array parameter may have a wider erased component type;
// replacing a Token[] local with a Token[] allocation changes no overload or
// type inference input. A widened local declaration does not prove this.
func branchArrayKeepsArgumentView(array *values.NewExpression, ref *values.JavaRef, parameter types.JavaType) bool {
	if array == nil || ref == nil || parameter == nil || !parameter.IsArray() || !sameExactArrayType(array.Type(), ref.Type()) {
		return false
	}
	return ref.WebDeclType == nil || sameExactArrayType(array.Type(), ref.WebDeclType)
}

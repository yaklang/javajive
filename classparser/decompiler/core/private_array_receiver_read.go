package core

import (
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Rebuild a fresh reference-array literal at its one original AALOAD, used as
// an invocation receiver. DUP materialization is not a source local definition.
// Leaving it outside a conditional operand makes allocation/elements eager
// and can put a declaration before SUPER. Source placement, every original
// store/use, effect interval and handler domain must be proved together.
func (d *Decompiler) inlinePrivateArrayReceiverReads(origins map[int]*OpCode) {
	if d == nil || d.RootNode == nil || d.FunctionContext == nil {
		return
	}
	fresh := false
	for _, op := range d.opCodes {
		if op != nil && op.Instr != nil && op.Instr.OpCode == OP_ANEWARRAY {
			fresh = true
			break
		}
	}
	if !fresh {
		return
	}
	nodes, _ := guardGraph(d.RootNode)
	if len(nodes) > 4096 || d.Work != nil && d.Work.CheckAlloc(int64(len(nodes)+1)*256) != nil {
		return
	}
	type candidate struct {
		call *values.FunctionCallExpression
		read *values.JavaArrayMember
	}
	var candidates []candidate
	occurrences := map[*values.JavaArrayMember]int{}
	path := map[values.JavaValue]bool{}
	remaining := 4096
	var visit func(values.JavaValue) bool
	visit = func(value values.JavaValue) bool {
		value = values.UnpackSoltValue(value)
		if value == nil {
			return true
		}
		remaining--
		if remaining < 0 || path[value] || d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return false
		}
		path[value] = true
		defer delete(path, value)
		if read, ok := value.(*values.JavaArrayMember); ok {
			occurrences[read]++
		}
		if call, ok := value.(*values.FunctionCallExpression); ok && !call.IsStatic {
			if read, ok := values.UnpackSoltValue(call.Object).(*values.JavaArrayMember); ok {
				if len(candidates) >= 128 {
					return false
				}
				candidates = append(candidates, candidate{call, read})
			}
		}
		children, known := values.Children(value)
		if !known {
			return false
		}
		for _, child := range children {
			if !visit(child) {
				return false
			}
		}
		return true
	}
	for _, node := range nodes {
		roots, known := constructorInlineNodeValues(node.Statement)
		if !known {
			return
		}
		for _, root := range roots {
			if !visit(root) {
				return
			}
		}
	}
	for _, c := range candidates {
		read, call := c.read, c.call
		ref, ok := values.UnpackSoltValue(read.Object).(*values.JavaRef)
		if !ok || ref == nil || ref.IsThis || ref.IsParam || ref.CustomValue != nil || ref.StackVar != nil || occurrences[read] != 1 || !read.HasOriginPC || !call.HasOriginPC {
			continue
		}
		var definition *Node
		var array *values.NewExpression
		count := 0
		for _, node := range nodes {
			if d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
				return
			}
			if assign, ok := node.Statement.(*statements.AssignStatement); ok && assign.ArrayMember == nil && delegationArraySameRef(assign.LeftValue, ref) {
				count++
				definition = node
				array, _ = values.UnpackSoltValue(assign.JavaValue).(*values.NewExpression)
			}
		}
		load, invoke := d.opcodeAtOffset(read.OriginPC), d.opcodeAtOffset(call.OriginPC)
		if count != 1 || definition == nil || definition.IsCatchStart || definition.IsTryCatch || array == nil || load == nil || invoke == nil || !sameBranchArrayInvocation(d.invokeFuncCall[invoke], call) || values.UnpackSoltValue(d.invokeFuncCall[invoke].Object) != read || !d.privateArrayReceiverRead(array, ref, read, load) {
			continue
		}
		allocation := d.opcodeAtOffset(array.OriginPC)
		materialization := origins[definition.Id]
		if materialization == nil || materialization.Instr == nil || materialization.Instr.OpCode != OP_DUP || len(materialization.stackConsumed) != 1 || values.UnpackSoltValue(materialization.stackConsumed[0]) != array || !d.privateArrayReadSourceConsumer(definition, call, allocation, origins) {
			continue
		}
		// Prospective replacement lets complete source-use inspection reject
		// opaque or escaping references without publishing a partial transfer.
		old := read.Object
		read.Object = array
		valid := true
		for _, node := range nodes {
			if d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
				valid = false
				break
			}
			if node != definition && statementReferencesLocal(node.Statement, ref) {
				valid = false
				break
			}
		}
		if !valid {
			read.Object = old
			continue
		}
		next := definition.Next[0]
		for _, source := range slices.Clone(definition.Source) {
			source.ReplaceNextSliceKeepOrder(definition, []*Node{next})
			source.ReplaceSwitchTarget(definition, next)
			if source.JmpNode == definition {
				source.JmpNode = next
			}
		}
		if definition == d.RootNode {
			d.RootNode = next
		}
		definition.RemoveAllNext()
		definition.RemoveAllSource()
		if d.varUserMap != nil {
			d.varUserMap.Delete(ref)
		}
	}
}

func (d *Decompiler) privateArrayReceiverRead(array *values.NewExpression, ref *values.JavaRef, read *values.JavaArrayMember, load *OpCode) bool {
	if d == nil || array == nil || ref == nil || read == nil || load == nil || load.Instr == nil || load.Instr.OpCode != OP_AALOAD || !read.HasOriginPC || read.OriginPC != int(load.CurrentOffset) || array.Type() == nil || !array.IsArray() || !array.HasOriginPC || !array.HasEvaluationEndPC || len(array.Initializer) == 0 || len(array.Initializer) > 128 || len(array.Length) != 1 || array.EvaluationEndPC >= read.OriginPC || len(load.stackConsumed) != 2 || !delegationArraySameRef(load.stackConsumed[1], ref) || values.UnpackSoltValue(load.stackConsumed[0]) != values.UnpackSoltValue(read.Index) || len(load.stackProduced) != 1 || values.UnpackSoltValue(load.stackProduced[0]) != read {
		return false
	}
	component := array.Type().ElementType()
	if component == nil {
		return false
	}
	if _, proper := component.RawType().(*types.JavaClass); !proper {
		return false
	}
	if name, named := types.ClassFQNOf(component); !named || d.FunctionContext == nil || d.FunctionContext.IsTypeParam(name) || component.IsArray() {
		return false
	}
	length, literal := values.UnpackSoltValue(array.Length[0]).(*values.JavaLiteral)
	if !literal || length.Data != len(array.Initializer) {
		return false
	}
	allocation := d.opcodeAtOffset(array.OriginPC)
	if allocation == nil || allocation.Instr == nil || allocation.Instr.OpCode != OP_ANEWARRAY || len(allocation.Data) != 2 || len(allocation.stackConsumed) != 1 || values.UnpackSoltValue(allocation.stackConsumed[0]) != values.UnpackSoltValue(array.Length[0]) || d.constantPoolGetter == nil || !branchArraySinglePath(d, allocation, load) {
		return false
	}
	original, known := d.constantPoolGetter(int(Convert2bytesToInt(allocation.Data))).(*values.JavaClassValue)
	if !known || original == nil || values.ReferenceTypeDescriptor(original.Type(), d.FunctionContext) == "" || values.ReferenceTypeDescriptor(original.Type(), d.FunctionContext) != values.ReferenceTypeDescriptor(component, d.FunctionContext) {
		return false
	}
	var stores []*OpCode
	for op := allocation; op != load; op = op.Target[0] {
		if d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return false
		}
		if op.Instr.OpCode != OP_AASTORE {
			continue
		}
		index := len(stores)
		if index >= len(array.Initializer) || len(op.stackConsumed) != 3 || !delegationArraySameRef(op.stackConsumed[2], ref) || values.UnpackSoltValue(op.stackConsumed[0]) != values.UnpackSoltValue(array.Initializer[index]) {
			return false
		}
		slot, literal := values.UnpackSoltValue(op.stackConsumed[1]).(*values.JavaLiteral)
		if !literal || slot.Data != index {
			return false
		}
		stores = append(stores, op)
	}
	if len(stores) != len(array.Initializer) || int(stores[len(stores)-1].CurrentOffset) != array.EvaluationEndPC {
		return false
	}
	return d.privateArrayValueDAG(allocation, load, ref, array, stores, array.Initializer, 1, read.Index)
}

// A recovered operand diamond may retain routing nodes before its source
// consumer. Cross only finite, effect-free scaffolding whose conditions are
// owned by that exact consumer's value trees. The same source-arm/dominance
// and earlier-operand proof used by immediate consumers still applies.
func (d *Decompiler) privateArrayReadSourceConsumer(definition *Node, call *values.FunctionCallExpression, allocation *OpCode, origins map[int]*OpCode) bool {
	if definition == nil || len(definition.Next) != 1 {
		return false
	}
	var consumer *Node
	var conditions []*Node
	color := map[*Node]uint8{}
	var visit func(*Node) bool
	visit = func(node *Node) bool {
		if node == nil || node.IsTryCatch || node.IsCatchStart || color[node] == 1 || len(color) >= 256 || d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return false
		}
		if color[node] == 2 {
			return true
		}
		color[node] = 1
		switch s := node.Statement.(type) {
		case *statements.GOTOStatement:
			if len(node.Next) != 1 {
				return false
			}
		case *statements.MiddleStatement:
			if s.Flag != "start" || s.Data != nil || len(node.Next) != 1 {
				return false
			}
		case *statements.ConditionStatement:
			if !s.TernaryChainArm || len(node.Next) == 0 || len(node.Next) > 2 {
				return false
			}
			conditions = append(conditions, node)
		default:
			if consumer != nil && consumer != node {
				return false
			}
			consumer = node
			color[node] = 2
			return true
		}
		for _, next := range node.Next {
			if !visit(next) {
				return false
			}
		}
		color[node] = 2
		return true
	}
	if !visit(definition.Next[0]) || consumer == nil || !d.branchArraySourceConsumer(consumer, call, allocation) {
		return false
	}
	roots, known := constructorInlineNodeValues(consumer.Statement)
	return known && constructorConditionsBelongToPrefixArguments(conditions, origins, roots)
}

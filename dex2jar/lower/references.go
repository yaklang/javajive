package lower

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/yaklang/javajive/dex2jar/ir"
	"github.com/yaklang/javajive/dex2jar/model"
)

const nullDescriptor = "<null>"

// descriptorSet is an immutable sorted set. States share these slices, so a
// fixpoint copy is shallow and a merge allocates only when the union grows.
type descriptorSet []string

// referenceState is sparse: only registers with a provable reference descriptor
// appear. The previous dense []map[string]bool allocated one map per register
// for every block and deep-copied all of them on every pass.
type referenceState map[uint16]descriptorSet

func (s referenceState) setOne(register uint16, descriptor string) {
	if descriptor == "" {
		return
	}
	s[register] = descriptorSet{descriptor}
}

func (s referenceState) set(register uint16, descriptors descriptorSet) {
	if len(descriptors) == 0 {
		delete(s, register)
		return
	}
	s[register] = descriptors
}

func descriptorSetsEqual(left, right descriptorSet) bool {
	if len(left) != len(right) {
		return false
	}
	for _, descriptor := range left {
		found := false
		for _, other := range right {
			if descriptor == other {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func unionDescriptorSets(left, right descriptorSet) descriptorSet {
	if len(left) == 0 {
		return right
	}
	if len(right) == 0 {
		return left
	}
	newCount := 0
	for _, descriptor := range right {
		present := false
		for _, existing := range left {
			if existing == descriptor {
				present = true
				break
			}
		}
		if !present {
			newCount++
		}
	}
	if newCount == 0 {
		// No new descriptor: keep sharing the original immutable set instead of
		// retaining a fresh identical copy on every fixpoint pass.
		return left
	}
	out := make(descriptorSet, len(left), len(left)+newCount)
	copy(out, left)
	for _, descriptor := range right {
		present := false
		for _, existing := range out {
			if existing == descriptor {
				present = true
				break
			}
		}
		if !present {
			out = append(out, descriptor)
		}
	}
	sort.Strings(out)
	return out
}

func (l *methodLowerer) validateReferenceUses() error {
	registerCount := int(l.method.Code.RegistersSize)
	entry := newReferenceState(registerCount)
	parameter := int(l.method.Code.RegistersSize - l.method.Code.InsSize)
	if l.encoded.AccessFlags&dexStatic == 0 {
		entry.setOne(uint16(parameter), l.class.Descriptor)
		parameter++
	}
	for _, descriptor := range l.method.Proto.Parameters {
		kind, _ := ir.DescriptorCategory(descriptor)
		if kind == ir.Reference && parameter < registerCount {
			entry.setOne(uint16(parameter), descriptor)
		}
		// A category-2 parameter occupies two registers, so later parameters
		// start after both of them.
		parameter += kind.Words()
	}
	in := make([]referenceState, len(l.method.Blocks))
	out := make([]referenceState, len(l.method.Blocks))
	for index := range l.method.Blocks {
		in[index] = newReferenceState(registerCount)
		out[index] = newReferenceState(registerCount)
	}
	in[0] = copyReferenceState(entry)
	limit := len(l.method.Blocks)*int(l.method.Code.RegistersSize+1) + 1
	for pass := 0; pass < limit; pass++ {
		changed := false
		for blockIndex, block := range l.method.Blocks {
			if blockIndex != 0 {
				merged := newReferenceState(registerCount)
				if block.Handler {
					merged = l.handlerReferenceEntry(blockIndex, in, out)
				} else {
					for _, predecessor := range block.Predecessors {
						if !l.method.Blocks[predecessor].Reachable {
							continue
						}
						mergeReferenceState(merged, out[predecessor])
					}
				}
				if !referenceStatesEqual(in[blockIndex], merged) {
					in[blockIndex] = merged
					changed = true
				}
			}
			state := copyReferenceState(in[blockIndex])
			for _, instructionIndex := range block.Instructions {
				l.transferReferences(instructionIndex, state)
			}
			if !referenceStatesEqual(out[blockIndex], state) {
				out[blockIndex] = state
				changed = true
			}
		}
		if !changed {
			break
		}
		if pass == limit-1 {
			return methodDiagnostic(l.method.Context, nil, "cfg", fmt.Errorf("reference descriptor analysis did not converge"))
		}
	}
	for blockIndex, block := range l.method.Blocks {
		if !block.Reachable {
			continue
		}
		state := copyReferenceState(in[blockIndex])
		for _, instructionIndex := range block.Instructions {
			if err := l.checkReferenceInstruction(instructionIndex, state); err != nil {
				return err
			}
			l.transferReferences(instructionIndex, state)
		}
	}
	return nil
}

// handlerReferenceEntry merges the descriptor state observed at the start of
// every protected instruction that dispatches to this handler. A protected
// block's exit state can describe a later lifetime, which would invent types
// the handler never observes.
func (l *methodLowerer) handlerReferenceEntry(blockIndex int, in, out []referenceState) referenceState {
	merged := newReferenceState(len(in[0]))
	protected := l.method.Blocks[blockIndex].Protected
	if len(protected) == 0 {
		for _, predecessor := range l.method.Blocks[blockIndex].Predecessors {
			mergeReferenceState(merged, out[predecessor])
		}
		return merged
	}
	byBlock := make(map[int]map[int]bool)
	for _, instructionIndex := range protected {
		block := l.blockOfInstruction(instructionIndex)
		if block < 0 {
			continue
		}
		if byBlock[block] == nil {
			byBlock[block] = make(map[int]bool)
		}
		byBlock[block][instructionIndex] = true
	}
	for _, predecessor := range l.method.Blocks[blockIndex].Predecessors {
		if byBlock[predecessor] == nil && l.method.Blocks[predecessor].Reachable {
			mergeReferenceState(merged, out[predecessor])
		}
	}
	for block, wanted := range byBlock {
		state := copyReferenceState(in[block])
		for _, instructionIndex := range l.method.Blocks[block].Instructions {
			if wanted[instructionIndex] {
				mergeReferenceState(merged, state)
			}
			l.transferReferences(instructionIndex, state)
		}
	}
	return merged
}

func (l *methodLowerer) checkReferenceInstruction(index int, state referenceState) error {
	instruction := &l.method.Instructions[index]
	if allocation, ok := l.allocationByCtor[index]; ok {
		// The constructor consumes the value new-instance produced: the alias
		// registers hold that object, not whatever an earlier lifetime left
		// behind.
		for _, register := range allocation.AliasRegisters {
			state.setOne(register, allocation.Descriptor)
		}
	}
	check := func(register uint16, expected, role string) error {
		if !isReferenceKind(l.method.CategoryAt(index, register).Kind) {
			return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("%s v%d is not a reference", role, register))
		}
		if len(state[register]) == 0 {
			return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("%s v%d has no provable reference descriptor", role, register))
		}
		for _, actual := range state[register] {
			if !l.provablyAssignable(actual, expected) {
				return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("%s v%d descriptor %q is not provably assignable to %q", role, register, actual, expected))
			}
		}
		return nil
	}
	switch {
	case instruction.Opcode == 0x11:
		return check(instruction.Regs[0], l.method.Proto.ReturnType, "return value")
	case instruction.Opcode == 0x27:
		return check(instruction.Regs[0], "Ljava/lang/Throwable;", "thrown value")
	case instruction.Opcode == 0x21:
		return l.checkArrayDescriptors(instruction, state[instruction.Regs[1]], "")
	case instruction.Opcode == 0x44 || instruction.Opcode == 0x4b:
		return l.checkArrayDescriptors(instruction, state[instruction.Regs[1]], "I")
	case instruction.Opcode == 0x46 || instruction.Opcode == 0x4d:
		if err := l.checkArrayDescriptors(instruction, state[instruction.Regs[1]], "reference"); err != nil {
			return err
		}
		if instruction.Opcode == 0x4d {
			// aastore performs a runtime array-store check, so the verifier
			// accepts any reference value; only the array itself must be a
			// reference array.
			if kind := l.method.CategoryAt(index, instruction.Regs[0]).Kind; !isReferenceKind(kind) {
				return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("array value v%d is not a reference", instruction.Regs[0]))
			}
		}
	case instruction.Opcode >= 0x52 && instruction.Opcode <= 0x5f:
		field := l.file.Fields[instruction.Index]
		if err := check(instruction.Regs[1], field.Class, "field receiver"); err != nil {
			return err
		}
		if instruction.Opcode >= 0x59 {
			kind, _ := ir.DescriptorCategory(field.Type)
			if kind == ir.Reference {
				return check(instruction.Regs[0], field.Type, "field value")
			}
		}
	case instruction.Opcode >= 0x67 && instruction.Opcode <= 0x6d:
		field := l.file.Fields[instruction.Index]
		kind, _ := ir.DescriptorCategory(field.Type)
		if kind == ir.Reference {
			return check(instruction.Regs[0], field.Type, "field value")
		}
	case isInvoke(instruction.Opcode):
		callee := l.file.Methods[instruction.Index]
		proto := l.file.Protos[callee.ProtoIndex]
		argument := 0
		if !isStaticInvoke(instruction.Opcode) {
			if err := check(instruction.Regs[0], callee.Class, "invoke receiver"); err != nil {
				return err
			}
			argument++
		}
		for _, descriptor := range proto.Parameters {
			kind, _ := ir.DescriptorCategory(descriptor)
			if argument >= len(instruction.Regs) {
				break
			}
			if kind == ir.Reference {
				if err := check(instruction.Regs[argument], descriptor, "invoke argument"); err != nil {
					return err
				}
			}
			argument += kind.Words()
		}
	}
	return nil
}

func (l *methodLowerer) transferReferences(index int, state referenceState) {
	instruction := l.method.Instructions[index]
	// descriptorSets are immutable, so assigning one shares it instead of
	// copying every descriptor on every instruction.
	set := func(register uint16, descriptors descriptorSet) {
		state.set(register, descriptors)
	}
	setOne := func(register uint16, descriptor string) {
		state.setOne(register, descriptor)
	}
	if allocation, ok := l.allocationByCtor[index]; ok {
		for _, register := range allocation.AliasRegisters {
			setOne(register, allocation.Descriptor)
		}
		return
	}
	if allocation, ok := l.allocationByNew[index]; ok {
		// new-instance defines the register, whether lowering keeps the value in
		// the home or fuses allocation and initialization.
		setOne(instruction.Regs[0], allocation.Descriptor)
		return
	}
	switch {
	case instruction.Opcode >= 0x01 && instruction.Opcode <= 0x09:
		set(instruction.Regs[0], state[instruction.Regs[1]])
	case instruction.Opcode == 0x0c:
		if index > 0 {
			previous := l.method.Instructions[index-1]
			if previous.Opcode == 0x24 || previous.Opcode == 0x25 {
				// filled-new-array yields the array type itself; its Index names
				// a type, not a method.
				setOne(instruction.Regs[0], l.file.Types[previous.Index].Descriptor)
				break
			}
			callee := l.file.Methods[previous.Index]
			setOne(instruction.Regs[0], l.file.Protos[callee.ProtoIndex].ReturnType)
		}
	case instruction.Opcode >= 0x12 && instruction.Opcode <= 0x15 && instruction.Literal == 0:
		// The null constant types the register it defines, not its previous
		// lifetime, so the definition category decides.
		if definition, ok := l.method.DefinitionAt(index); ok && isReferenceKind(definition.Category.Kind) {
			setOne(instruction.Regs[0], nullDescriptor)
		}
	case instruction.Opcode == 0x0d:
		setOne(instruction.Regs[0], l.method.ExceptionType(instruction.Offset))
	case instruction.Opcode == 0x1a || instruction.Opcode == 0x1b:
		setOne(instruction.Regs[0], "Ljava/lang/String;")
	case instruction.Opcode == 0x1c:
		setOne(instruction.Regs[0], "Ljava/lang/Class;")
	case instruction.Opcode == 0x1f:
		setOne(instruction.Regs[0], l.file.Types[instruction.Index].Descriptor)
	case instruction.Opcode == 0x22 || instruction.Opcode == 0x23:
		setOne(instruction.Regs[0], l.file.Types[instruction.Index].Descriptor)
	case instruction.Opcode == 0x46:
		var components descriptorSet
		for _, array := range state[instruction.Regs[1]] {
			if strings.HasPrefix(array, "[") {
				components = unionDescriptorSets(components, descriptorSet{array[1:]})
			}
		}
		set(instruction.Regs[0], components)
	case instruction.Opcode >= 0x52 && instruction.Opcode <= 0x58 || instruction.Opcode >= 0x60 && instruction.Opcode <= 0x66:
		field := l.file.Fields[instruction.Index]
		kind, _ := ir.DescriptorCategory(field.Type)
		if kind == ir.Reference {
			setOne(instruction.Regs[0], field.Type)
		}
	}
}

func (l *methodLowerer) checkArrayDescriptors(instruction *ir.Instruction, descriptors descriptorSet, component string) error {
	if len(descriptors) == 0 {
		return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("array register has no provable descriptor"))
	}
	for _, descriptor := range descriptors {
		if descriptor == nullDescriptor {
			continue
		}
		if !strings.HasPrefix(descriptor, "[") || len(descriptor) < 2 {
			return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("descriptor %q is not an array", descriptor))
		}
		if component == "I" && descriptor != "[I" && descriptor != "[F" {
			return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("int array opcode does not match descriptor %q", descriptor))
		}
		if component == "reference" && descriptor[1] != 'L' && descriptor[1] != '[' {
			return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("object array opcode does not match descriptor %q", descriptor))
		}
	}
	return nil
}

// maxHierarchyCacheEntries bounds the per-file class graphs kept alive while an
// application is lowered; inspect walks one file at a time, so a small window is
// enough and a long-lived process cannot grow this without limit.
const maxHierarchyCacheEntries = 8

var hierarchyCache = struct {
	sync.Mutex
	entries map[*model.File]map[string]*model.Class
	order   []*model.File
}{entries: make(map[*model.File]map[string]*model.Class)}

// classIndex returns the application-wide hierarchy when the caller provided
// one, and otherwise the classes of the file being lowered.
func (l *methodLowerer) classIndex() map[string]*model.Class {
	if l.hierarchy != nil {
		return l.hierarchy
	}
	return classHierarchy(l.file)
}

// classHierarchy indexes one DEX file's classes once. Rebuilding the graph for
// every method would be quadratic on large applications.
func classHierarchy(file *model.File) map[string]*model.Class {
	hierarchyCache.Lock()
	if cached, ok := hierarchyCache.entries[file]; ok {
		hierarchyCache.Unlock()
		return cached
	}
	hierarchyCache.Unlock()

	index := make(map[string]*model.Class, len(file.Classes))
	for position := range file.Classes {
		class := &file.Classes[position]
		index[class.Descriptor] = class
	}

	hierarchyCache.Lock()
	if len(hierarchyCache.order) >= maxHierarchyCacheEntries {
		evicted := hierarchyCache.order[0]
		hierarchyCache.order = hierarchyCache.order[1:]
		delete(hierarchyCache.entries, evicted)
	}
	hierarchyCache.entries[file] = index
	hierarchyCache.order = append(hierarchyCache.order, file)
	hierarchyCache.Unlock()
	return index
}

// maxAssignabilityDepth bounds the ancestry walk so pathological or cyclic
// hierarchies cannot stall translation.
const maxAssignabilityDepth = 32

func (l *methodLowerer) provablyAssignable(actual, expected string) bool {
	if actual == nullDescriptor || actual == expected || expected == "Ljava/lang/Object;" {
		return true
	}
	if strings.HasPrefix(actual, "[") {
		if expected == "Ljava/lang/Cloneable;" || expected == "Ljava/io/Serializable;" {
			return true
		}
		// Array covariance: a reference array is assignable to another reference
		// array when its component is, while primitive arrays are invariant.
		if strings.HasPrefix(expected, "[") {
			actualComponent, expectedComponent := actual[1:], expected[1:]
			if actualComponent == expectedComponent {
				return true
			}
			actualReference := strings.HasPrefix(actualComponent, "L") || strings.HasPrefix(actualComponent, "[")
			expectedReference := strings.HasPrefix(expectedComponent, "L") || strings.HasPrefix(expectedComponent, "[")
			if actualReference && expectedReference {
				return l.provablyAssignable(actualComponent, expectedComponent)
			}
		}
		return false
	}
	if actual == l.class.Descriptor {
		if expected == l.class.Superclass {
			return true
		}
		for _, implemented := range l.class.Interfaces {
			if expected == implemented {
				return true
			}
		}
	}
	index := l.classIndex()
	if _, ok := index[expected]; !ok {
		// The expected type lives outside the application. Proving the relation
		// needs the platform classpath the input was verified against, which the
		// DEX file does not carry; rejecting here would refuse valid code such as
		// String to CharSequence or IllegalArgumentException to Throwable.
		return true
	}
	// The expected type is declared in this application, so the declared
	// hierarchy must prove the relation. A platform ancestor cannot introduce an
	// application type, so an unresolved ancestor cannot rescue the assignment.
	visited := map[string]bool{actual: true}
	queue := []string{actual}
	for depth := 0; depth < maxAssignabilityDepth && len(queue) > 0; depth++ {
		var next []string
		for _, current := range queue {
			class, ok := index[current]
			if !ok {
				continue
			}
			if class.Superclass == expected {
				return true
			}
			for _, implemented := range class.Interfaces {
				if implemented == expected {
					return true
				}
			}
			candidates := append([]string{class.Superclass}, class.Interfaces...)
			for _, candidate := range candidates {
				if candidate == "" || visited[candidate] {
					continue
				}
				visited[candidate] = true
				next = append(next, candidate)
			}
		}
		queue = next
	}
	return false
}

func newReferenceState(registers int) referenceState {
	// Only non-empty registers are stored. Cap the hint: large register files
	// are usually sparse even when every register is allocated.
	hint := registers / 4
	if hint > 32 {
		hint = 32
	}
	if hint < 0 {
		hint = 0
	}
	return make(referenceState, hint)
}

func copyReferenceState(source referenceState) referenceState {
	result := make(referenceState, len(source))
	for register, descriptors := range source {
		result[register] = descriptors
	}
	return result
}

func mergeReferenceState(destination, source referenceState) {
	for register, descriptors := range source {
		destination[register] = unionDescriptorSets(destination[register], descriptors)
	}
}

func referenceStatesEqual(left, right referenceState) bool {
	if len(left) != len(right) {
		return false
	}
	for register, descriptors := range left {
		if !descriptorSetsEqual(descriptors, right[register]) {
			return false
		}
	}
	return true
}

package lower

import (
	"fmt"
	"strings"
	"sync"

	"github.com/yaklang/javajive/dex2jar/ir"
	"github.com/yaklang/javajive/dex2jar/model"
)

const nullDescriptor = "<null>"

type referenceState []map[string]bool

func (l *methodLowerer) validateReferenceUses() error {
	entry := newReferenceState(int(l.method.Code.RegistersSize))
	parameter := int(l.method.Code.RegistersSize - l.method.Code.InsSize)
	if l.encoded.AccessFlags&dexStatic == 0 {
		entry[parameter][l.class.Descriptor] = true
		parameter++
	}
	for _, descriptor := range l.method.Proto.Parameters {
		kind, _ := ir.DescriptorCategory(descriptor)
		if kind == ir.Reference {
			entry[parameter][descriptor] = true
		}
		parameter++
	}
	in := make([]referenceState, len(l.method.Blocks))
	out := make([]referenceState, len(l.method.Blocks))
	for index := range l.method.Blocks {
		in[index] = newReferenceState(len(entry))
		out[index] = newReferenceState(len(entry))
	}
	in[0] = copyReferenceState(entry)
	limit := len(l.method.Blocks)*int(l.method.Code.RegistersSize+1) + 1
	for pass := 0; pass < limit; pass++ {
		changed := false
		for blockIndex, block := range l.method.Blocks {
			if blockIndex != 0 {
				merged := newReferenceState(len(entry))
				for _, predecessor := range block.Predecessors {
					mergeReferenceState(merged, out[predecessor])
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

func (l *methodLowerer) checkReferenceInstruction(index int, state referenceState) error {
	instruction := &l.method.Instructions[index]
	if allocation, ok := l.allocationByCtor[index]; ok {
		// The constructor consumes the value new-instance produced: the alias
		// registers hold that object, not whatever an earlier lifetime left
		// behind.
		for _, register := range allocation.AliasRegisters {
			state[register] = map[string]bool{allocation.Descriptor: true}
		}
	}
	check := func(register uint16, expected, role string) error {
		if !isReferenceKind(l.method.CategoryAt(index, register).Kind) {
			return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("%s v%d is not a reference", role, register))
		}
		if len(state[register]) == 0 {
			return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("%s v%d has no provable reference descriptor", role, register))
		}
		for actual := range state[register] {
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
			for array := range state[instruction.Regs[1]] {
				if array != nullDescriptor {
					if err := check(instruction.Regs[0], array[1:], "array value"); err != nil {
						return err
					}
				}
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
			if kind == ir.Reference {
				if err := check(instruction.Regs[argument], descriptor, "invoke argument"); err != nil {
					return err
				}
			}
			argument++
		}
	}
	return nil
}

func (l *methodLowerer) transferReferences(index int, state referenceState) {
	instruction := l.method.Instructions[index]
	set := func(register uint16, descriptors map[string]bool) {
		state[register] = copyDescriptorSet(descriptors)
	}
	setOne := func(register uint16, descriptor string) {
		state[register] = map[string]bool{descriptor: true}
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
	case instruction.Opcode >= 0x07 && instruction.Opcode <= 0x09:
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
		setOne(instruction.Regs[0], "Ljava/lang/Throwable;")
	case instruction.Opcode == 0x1a || instruction.Opcode == 0x1b:
		setOne(instruction.Regs[0], "Ljava/lang/String;")
	case instruction.Opcode == 0x1c:
		setOne(instruction.Regs[0], "Ljava/lang/Class;")
	case instruction.Opcode == 0x1f:
		setOne(instruction.Regs[0], l.file.Types[instruction.Index].Descriptor)
	case instruction.Opcode == 0x22 || instruction.Opcode == 0x23:
		setOne(instruction.Regs[0], l.file.Types[instruction.Index].Descriptor)
	case instruction.Opcode == 0x46:
		components := make(map[string]bool)
		for array := range state[instruction.Regs[1]] {
			if strings.HasPrefix(array, "[") {
				components[array[1:]] = true
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

func (l *methodLowerer) checkArrayDescriptors(instruction *ir.Instruction, descriptors map[string]bool, component string) error {
	if len(descriptors) == 0 {
		return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("array register has no provable descriptor"))
	}
	for descriptor := range descriptors {
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
			if strings.HasPrefix(actualComponent, "L") && strings.HasPrefix(expectedComponent, "L") {
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
	index := classHierarchy(l.file)
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
	state := make(referenceState, registers)
	for index := range state {
		state[index] = make(map[string]bool)
	}
	return state
}

func copyReferenceState(source referenceState) referenceState {
	result := newReferenceState(len(source))
	for register := range source {
		result[register] = copyDescriptorSet(source[register])
	}
	return result
}

func copyDescriptorSet(source map[string]bool) map[string]bool {
	result := make(map[string]bool, len(source))
	for descriptor := range source {
		result[descriptor] = true
	}
	return result
}

func mergeReferenceState(destination, source referenceState) {
	for register := range destination {
		for descriptor := range source[register] {
			destination[register][descriptor] = true
		}
	}
}

func referenceStatesEqual(left, right referenceState) bool {
	if len(left) != len(right) {
		return false
	}
	for register := range left {
		if len(left[register]) != len(right[register]) {
			return false
		}
		for descriptor := range left[register] {
			if !right[register][descriptor] {
				return false
			}
		}
	}
	return true
}

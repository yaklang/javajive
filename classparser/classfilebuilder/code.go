package classfilebuilder

import (
	"fmt"
	"math"
)

// Label is an opaque symbolic bytecode position. Labels belong to one Code.
type Label struct {
	owner *Code
	id    int
}

type instructionKind byte

const (
	kindSimple instructionKind = iota
	kindByte
	kindShort
	kindCP
	kindLocal
	kindIInc
	kindLDC
	kindBranch
	kindInvokeInterface
	kindTableSwitch
	kindLookupSwitch
)

type instruction struct {
	kind       instructionKind
	op         Opcode
	arg        int
	arg2       int
	target     *Label
	targets    []*Label
	keys       []int32
	wideBranch bool
	expanded   bool
}

type codeNode struct {
	label *Label
	insn  *instruction
}

type exceptionHandler struct {
	start, end, handler *Label
	catchType           uint16
}

type lineNumber struct {
	at   *Label
	line uint16
}

type localVariable struct {
	start, end       *Label
	name, descriptor uint16
	index            uint16
}

// Code is a symbolic Code attribute assembler. It is not safe for concurrent
// mutation. Errors are sticky and returned by Class.Bytes.
type Code struct {
	pool      *ConstantPool
	nodes     []codeNode
	nextLabel int
	handlers  []exceptionHandler
	lines     []lineNumber
	locals    []localVariable
	lineName  uint16
	localName uint16
	err       error
	maxStack  *uint16
	maxLocals *uint16
}

func newCode(pool *ConstantPool) *Code { return &Code{pool: pool} }

func (c *Code) fail(format string, args ...any) {
	if c.err == nil {
		c.err = fmt.Errorf(format, args...)
	}
}

// NewLabel creates an unbound label.
func (c *Code) NewLabel() *Label {
	label := &Label{owner: c, id: c.nextLabel}
	c.nextLabel++
	return label
}

// Mark binds label at the current bytecode position. A label may be bound once.
func (c *Code) Mark(label *Label) *Code {
	if !c.validLabel(label) {
		return c
	}
	for _, node := range c.nodes {
		if node.label == label {
			c.fail("label %d is already bound", label.id)
			return c
		}
	}
	c.nodes = append(c.nodes, codeNode{label: label})
	return c
}

func (c *Code) validLabel(label *Label) bool {
	if label == nil {
		c.fail("nil label")
		return false
	}
	if label.owner != c {
		c.fail("label %d belongs to another Code", label.id)
		return false
	}
	return true
}

// Emit appends an operandless JVM instruction.
func (c *Code) Emit(op Opcode) *Code {
	if _, ok := simpleEffect(op); !ok {
		c.fail("opcode 0x%02x is not an operandless supported instruction", byte(op))
		return c
	}
	c.nodes = append(c.nodes, codeNode{insn: &instruction{kind: kindSimple, op: op}})
	return c
}

// Byte appends a supported instruction with one signed or unsigned byte
// operand: BIPUSH or NEWARRAY.
func (c *Code) Byte(op Opcode, operand int) *Code {
	switch op {
	case BIPUSH:
		if operand < math.MinInt8 || operand > math.MaxInt8 {
			c.fail("BIPUSH operand %d is outside signed byte range", operand)
			return c
		}
	case NEWARRAY:
		if operand < 4 || operand > 11 {
			c.fail("NEWARRAY atype %d is invalid", operand)
			return c
		}
	default:
		c.fail("opcode 0x%02x does not take a one-byte operand", byte(op))
		return c
	}
	c.nodes = append(c.nodes, codeNode{insn: &instruction{kind: kindByte, op: op, arg: operand}})
	return c
}

// Short appends SIPUSH with a signed 16-bit operand.
func (c *Code) Short(op Opcode, operand int) *Code {
	if op != SIPUSH {
		c.fail("opcode 0x%02x does not take a signed short operand", byte(op))
		return c
	}
	if operand < math.MinInt16 || operand > math.MaxInt16 {
		c.fail("SIPUSH operand %d is outside signed short range", operand)
		return c
	}
	c.nodes = append(c.nodes, codeNode{insn: &instruction{kind: kindShort, op: op, arg: operand}})
	return c
}

// CP appends a supported instruction with a u2 constant-pool operand.
func (c *Code) CP(op Opcode, index uint16) *Code {
	entry, err := c.pool.entry(index)
	if err != nil {
		c.fail("opcode 0x%02x: %v", byte(op), err)
		return c
	}
	valid := false
	switch op {
	case GETSTATIC, PUTSTATIC, GETFIELD, PUTFIELD:
		valid = entry.tag == cpFieldref
	case INVOKEVIRTUAL, INVOKESPECIAL, INVOKESTATIC:
		// Major version 49 predates interface static/default methods; these
		// opcodes therefore require CONSTANT_Methodref.
		valid = entry.tag == cpMethodref
	case NEW, ANEWARRAY, CHECKCAST, INSTANCEOF:
		valid = entry.tag == cpClass
	}
	if !valid {
		c.fail("opcode 0x%02x cannot use constant pool index %d (tag %d)", byte(op), index, entry.tag)
		return c
	}
	c.nodes = append(c.nodes, codeNode{insn: &instruction{kind: kindCP, op: op, arg: int(index)}})
	return c
}

// InvokeInterface appends invokeinterface. The count byte is derived from the
// descriptor and includes the receiver.
func (c *Code) InvokeInterface(index uint16) *Code {
	entry, err := c.pool.entry(index)
	if err != nil {
		c.fail("invokeinterface: %v", err)
		return c
	}
	if entry.tag != cpInterfaceMethodref {
		c.fail("invokeinterface requires InterfaceMethodref, got tag %d at index %d", entry.tag, index)
		return c
	}
	args, _, err := methodDescriptorWords(entry.text)
	if err != nil {
		c.fail("invokeinterface descriptor %q: %v", entry.text, err)
		return c
	}
	if args+1 > math.MaxUint8 {
		c.fail("invokeinterface argument word count %d exceeds 255", args+1)
		return c
	}
	c.nodes = append(c.nodes, codeNode{insn: &instruction{kind: kindInvokeInterface, op: INVOKEINTERFACE, arg: int(index), arg2: args + 1}})
	return c
}

// Load appends a typed local load, selecting compact or WIDE encoding.
func (c *Code) Load(kind LocalKind, index uint16) *Code {
	op, ok := localOpcode(kind, true)
	if !ok {
		c.fail("invalid local kind %d", kind)
		return c
	}
	c.nodes = append(c.nodes, codeNode{insn: &instruction{kind: kindLocal, op: op, arg: int(index), arg2: localWidth(kind)}})
	return c
}

// Store appends a typed local store, selecting compact or WIDE encoding.
func (c *Code) Store(kind LocalKind, index uint16) *Code {
	op, ok := localOpcode(kind, false)
	if !ok {
		c.fail("invalid local kind %d", kind)
		return c
	}
	c.nodes = append(c.nodes, codeNode{insn: &instruction{kind: kindLocal, op: op, arg: int(index), arg2: localWidth(kind)}})
	return c
}

// IInc appends iinc, selecting WIDE when either operand requires it.
func (c *Code) IInc(index uint16, delta int16) *Code {
	c.nodes = append(c.nodes, codeNode{insn: &instruction{kind: kindIInc, op: IINC, arg: int(index), arg2: int(delta)}})
	return c
}

// LDC appends ldc or ldc_w according to the final constant-pool index.
func (c *Code) LDC(index uint16) *Code {
	entry, err := c.pool.entry(index)
	if err != nil {
		c.fail("ldc: %v", err)
		return c
	}
	if entry.tag != cpInteger && entry.tag != cpFloat && entry.tag != cpString && entry.tag != cpClass {
		c.fail("ldc does not support constant pool tag %d at index %d", entry.tag, index)
		return c
	}
	c.nodes = append(c.nodes, codeNode{insn: &instruction{kind: kindLDC, op: LDC, arg: int(index)}})
	return c
}

// LDC2 appends ldc2_w, which pushes a long or double constant.
func (c *Code) LDC2(index uint16) *Code {
	entry, err := c.pool.entry(index)
	if err != nil {
		c.fail("ldc2_w: %v", err)
		return c
	}
	if entry.tag != cpLong && entry.tag != cpDouble {
		c.fail("ldc2_w does not support constant pool tag %d at index %d", entry.tag, index)
		return c
	}
	c.nodes = append(c.nodes, codeNode{insn: &instruction{kind: kindCP, op: LDC2_W, arg: int(index)}})
	return c
}

// Branch appends goto or a supported conditional branch. Goto is widened to
// goto_w as needed; long conditionals are expanded to inverse-if + goto_w.
func (c *Code) Branch(op Opcode, target *Label) *Code {
	if !c.validLabel(target) {
		return c
	}
	if op != GOTO && !isConditional(op) {
		c.fail("opcode 0x%02x is not a supported branch", byte(op))
		return c
	}
	c.nodes = append(c.nodes, codeNode{insn: &instruction{kind: kindBranch, op: op, target: target}})
	return c
}

// TableSwitch appends a tableswitch whose consecutive case values start at
// low. targets must contain at least one case target. Offsets and alignment
// padding are resolved during assembly.
func (c *Code) TableSwitch(low int32, defaultTarget *Label, targets ...*Label) *Code {
	if !c.validLabel(defaultTarget) {
		return c
	}
	if len(targets) == 0 {
		c.fail("tableswitch requires at least one case target")
		return c
	}
	if int64(low)+int64(len(targets))-1 > math.MaxInt32 {
		c.fail("tableswitch high value exceeds signed 32-bit range")
		return c
	}
	for _, target := range targets {
		if !c.validLabel(target) {
			return c
		}
	}
	c.nodes = append(c.nodes, codeNode{insn: &instruction{
		kind: kindTableSwitch, op: TABLESWITCH, arg: int(low), target: defaultTarget,
		targets: append([]*Label(nil), targets...),
	}})
	return c
}

// LookupSwitch appends a lookupswitch. keys and targets are corresponding
// pairs, and keys must be strictly increasing as required by the class format.
func (c *Code) LookupSwitch(defaultTarget *Label, keys []int32, targets ...*Label) *Code {
	if !c.validLabel(defaultTarget) {
		return c
	}
	if len(keys) != len(targets) {
		c.fail("lookupswitch has %d keys and %d targets", len(keys), len(targets))
		return c
	}
	for index, target := range targets {
		if index > 0 && keys[index] <= keys[index-1] {
			c.fail("lookupswitch keys must be strictly increasing: key %d at index %d follows %d", keys[index], index, keys[index-1])
			return c
		}
		if !c.validLabel(target) {
			return c
		}
	}
	c.nodes = append(c.nodes, codeNode{insn: &instruction{
		kind: kindLookupSwitch, op: LOOKUPSWITCH, target: defaultTarget,
		keys: append([]int32(nil), keys...), targets: append([]*Label(nil), targets...),
	}})
	return c
}

// Catch adds an exception-table entry. catchClass is an internal class name;
// use an empty string for a finally/all-exceptions handler. The protected range
// is [start,end), and all labels must be bound before assembly.
func (c *Code) Catch(start, end, handler *Label, catchClass string) *Code {
	if len(c.handlers) >= math.MaxUint16 {
		c.fail("exception_table_length exceeds 65535")
		return c
	}
	if !c.validLabel(start) || !c.validLabel(end) || !c.validLabel(handler) {
		return c
	}
	var catchType uint16
	if catchClass != "" {
		if err := validateInternalName(catchClass); err != nil {
			c.fail("catch type %q: %v", catchClass, err)
			return c
		}
		var err error
		catchType, err = c.pool.Class(catchClass)
		if err != nil {
			c.fail("catch type %q: %v", catchClass, err)
			return c
		}
	}
	c.handlers = append(c.handlers, exceptionHandler{start: start, end: end, handler: handler, catchType: catchType})
	return c
}

// Line records a source line at a symbolic bytecode position.
func (c *Code) Line(at *Label, line uint16) *Code {
	if !c.validLabel(at) {
		return c
	}
	if line == 0 {
		c.fail("line number must be positive")
		return c
	}
	if len(c.lines) >= math.MaxUint16 {
		c.fail("line_number_table_length exceeds 65535")
		return c
	}
	if c.lineName == 0 {
		var err error
		c.lineName, err = c.pool.UTF8("LineNumberTable")
		if err != nil {
			c.fail("LineNumberTable attribute name: %v", err)
			return c
		}
	}
	c.lines = append(c.lines, lineNumber{at: at, line: line})
	return c
}

// Local records one LocalVariableTable range. The range is [start,end).
func (c *Code) Local(start, end *Label, name, descriptor string, index uint16) *Code {
	if !c.validLabel(start) || !c.validLabel(end) {
		return c
	}
	if name == "" {
		c.fail("local variable name is empty")
		return c
	}
	width, err := fieldDescriptorWords(descriptor)
	if err != nil {
		c.fail("local variable %q: %v", name, err)
		return c
	}
	if int(index)+width > math.MaxUint16 {
		c.fail("local variable %q index %d with width %d exceeds 65535", name, index, width)
		return c
	}
	if len(c.locals) >= math.MaxUint16 {
		c.fail("local_variable_table_length exceeds 65535")
		return c
	}
	nameIndex, err := c.pool.UTF8(name)
	if err != nil {
		c.fail("local variable name %q: %v", name, err)
		return c
	}
	descriptorIndex, err := c.pool.UTF8(descriptor)
	if err != nil {
		c.fail("local variable descriptor %q: %v", descriptor, err)
		return c
	}
	if c.localName == 0 {
		c.localName, err = c.pool.UTF8("LocalVariableTable")
		if err != nil {
			c.fail("LocalVariableTable attribute name: %v", err)
			return c
		}
	}
	c.locals = append(c.locals, localVariable{start: start, end: end, name: nameIndex, descriptor: descriptorIndex, index: index})
	return c
}

// SetMaxStack requires the calculated maximum stack to equal value.
func (c *Code) SetMaxStack(value uint16) *Code { c.maxStack = &value; return c }

// SetMaxLocals requires value to be at least the descriptor/local-instruction
// requirement. This permits reserving otherwise unused locals explicitly.
func (c *Code) SetMaxLocals(value uint16) *Code { c.maxLocals = &value; return c }

func localOpcode(kind LocalKind, load bool) (Opcode, bool) {
	loads := [...]Opcode{ILOAD, LLOAD, FLOAD, DLOAD, ALOAD}
	stores := [...]Opcode{ISTORE, LSTORE, FSTORE, DSTORE, ASTORE}
	if int(kind) >= len(loads) {
		return 0, false
	}
	if load {
		return loads[kind], true
	}
	return stores[kind], true
}

func localWidth(kind LocalKind) int {
	if kind == LongLocal || kind == DoubleLocal {
		return 2
	}
	return 1
}

func isConditional(op Opcode) bool {
	return op >= IFEQ && op <= IF_ACMPNE || op == IFNULL || op == IFNONNULL
}

func inverseConditional(op Opcode) Opcode {
	if op >= IFEQ && op <= IFLE {
		return IFEQ + ((op - IFEQ) ^ 1)
	}
	if op >= IF_ICMPEQ && op <= IF_ACMPNE {
		return IF_ICMPEQ + ((op - IF_ICMPEQ) ^ 1)
	}
	if op == IFNULL {
		return IFNONNULL
	}
	return IFNULL
}

func (i *instruction) size(pc int) int {
	switch i.kind {
	case kindSimple:
		return 1
	case kindByte:
		return 2
	case kindShort, kindCP:
		return 3
	case kindLocal:
		if i.arg <= 3 {
			return 1
		}
		if i.arg <= math.MaxUint8 {
			return 2
		}
		return 4
	case kindIInc:
		if i.arg <= math.MaxUint8 && i.arg2 >= math.MinInt8 && i.arg2 <= math.MaxInt8 {
			return 3
		}
		return 6
	case kindLDC:
		if i.arg <= math.MaxUint8 {
			return 2
		}
		return 3
	case kindBranch:
		if i.expanded {
			return 8
		}
		if i.wideBranch {
			return 5
		}
		return 3
	case kindInvokeInterface:
		return 5
	case kindTableSwitch:
		padding := (4 - (pc+1)%4) % 4
		return 1 + padding + 12 + 4*len(i.targets)
	case kindLookupSwitch:
		padding := (4 - (pc+1)%4) % 4
		return 1 + padding + 8 + 8*len(i.targets)
	default:
		return 0
	}
}

type assembledCode struct {
	bytes               []byte
	maxStack, maxLocals uint16
	handlers            []assembledHandler
	lines               []assembledLine
	locals              []assembledLocal
	lineName            uint16
	localName           uint16
}

type assembledHandler struct {
	start, end, handler uint16
	catchType           uint16
}

type assembledLine struct {
	start uint16
	line  uint16
}

type assembledLocal struct {
	start, length    uint16
	name, descriptor uint16
	index            uint16
}

func (c *Code) assemble(descriptor string, access uint16) (*assembledCode, error) {
	if c == nil {
		return nil, fmt.Errorf("nil Code")
	}
	if c.err != nil {
		return nil, c.err
	}
	labelPC, insnPC, codeLength, err := c.layout()
	if err != nil {
		return nil, err
	}
	if codeLength == 0 {
		return nil, fmt.Errorf("Code attribute has empty code")
	}
	if codeLength > 65535 {
		return nil, fmt.Errorf("code_length %d exceeds 65535", codeLength)
	}
	maxStack, maxLocals, err := c.calculateFrames(descriptor, access, labelPC, insnPC)
	if err != nil {
		return nil, err
	}
	if c.maxStack != nil && *c.maxStack != maxStack {
		return nil, fmt.Errorf("explicit max_stack %d does not equal calculated %d", *c.maxStack, maxStack)
	}
	if c.maxLocals != nil {
		if *c.maxLocals < maxLocals {
			return nil, fmt.Errorf("explicit max_locals %d is less than required %d", *c.maxLocals, maxLocals)
		}
		maxLocals = *c.maxLocals
	}
	out := &byteWriter{data: make([]byte, 0, codeLength)}
	insnIndex := 0
	for _, node := range c.nodes {
		if node.insn == nil {
			continue
		}
		pc := insnPC[insnIndex]
		if err := c.writeInstruction(out, node.insn, pc, labelPC); err != nil {
			return nil, err
		}
		insnIndex++
	}
	if len(out.data) != codeLength {
		return nil, fmt.Errorf("internal assembler length mismatch: wrote %d, laid out %d", len(out.data), codeLength)
	}
	handlers := make([]assembledHandler, len(c.handlers))
	for i, handler := range c.handlers {
		start, ok1 := labelPC[handler.start]
		end, ok2 := labelPC[handler.end]
		entry, ok3 := labelPC[handler.handler]
		if !ok1 || !ok2 || !ok3 {
			return nil, fmt.Errorf("exception table entry %d contains an unbound label", i)
		}
		if start >= end {
			return nil, fmt.Errorf("exception table entry %d has empty or reversed range [%d,%d)", i, start, end)
		}
		if end > codeLength || entry >= codeLength {
			return nil, fmt.Errorf("exception table entry %d is outside code_length %d", i, codeLength)
		}
		handlers[i] = assembledHandler{uint16(start), uint16(end), uint16(entry), handler.catchType}
	}
	lines := make([]assembledLine, len(c.lines))
	for i, line := range c.lines {
		pc, ok := labelPC[line.at]
		if !ok {
			return nil, fmt.Errorf("line number entry %d contains an unbound label", i)
		}
		if pc >= codeLength {
			return nil, fmt.Errorf("line number entry %d is outside code_length %d", i, codeLength)
		}
		lines[i] = assembledLine{start: uint16(pc), line: line.line}
	}
	locals := make([]assembledLocal, len(c.locals))
	for i, local := range c.locals {
		start, ok1 := labelPC[local.start]
		end, ok2 := labelPC[local.end]
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("local variable entry %d contains an unbound label", i)
		}
		if start >= end || end > codeLength {
			return nil, fmt.Errorf("local variable entry %d has invalid range [%d,%d) for code_length %d", i, start, end, codeLength)
		}
		locals[i] = assembledLocal{start: uint16(start), length: uint16(end - start), name: local.name, descriptor: local.descriptor, index: local.index}
	}
	return &assembledCode{
		bytes: out.data, maxStack: maxStack, maxLocals: maxLocals,
		handlers: handlers, lines: lines, locals: locals,
		lineName: c.lineName, localName: c.localName,
	}, nil
}

func (c *Code) layout() (map[*Label]int, []int, int, error) {
	for pass := 0; pass <= len(c.nodes)+1; pass++ {
		labelPC := make(map[*Label]int)
		insnPC := make([]int, 0)
		pc := 0
		for _, node := range c.nodes {
			if node.label != nil {
				labelPC[node.label] = pc
			} else if node.insn != nil {
				insnPC = append(insnPC, pc)
				pc += node.insn.size(pc)
			}
		}
		changed := false
		insnIndex := 0
		for _, node := range c.nodes {
			insn := node.insn
			if insn == nil {
				continue
			}
			if insn.kind == kindBranch {
				target, ok := labelPC[insn.target]
				if !ok {
					return nil, nil, 0, fmt.Errorf("branch at instruction %d targets unbound label %d", insnIndex, insn.target.id)
				}
				offset := target - insnPC[insnIndex]
				if offset < math.MinInt16 || offset > math.MaxInt16 {
					if insn.op == GOTO && !insn.wideBranch {
						insn.wideBranch, changed = true, true
					} else if isConditional(insn.op) && !insn.expanded {
						insn.expanded, changed = true, true
					}
				}
			} else if insn.kind == kindTableSwitch || insn.kind == kindLookupSwitch {
				targets := append([]*Label{insn.target}, insn.targets...)
				for _, target := range targets {
					if _, ok := labelPC[target]; !ok {
						return nil, nil, 0, fmt.Errorf("switch at instruction %d targets unbound label %d", insnIndex, target.id)
					}
				}
			}
			insnIndex++
		}
		if !changed {
			if err := c.validateControlTargets(labelPC, insnPC); err != nil {
				return nil, nil, 0, err
			}
			return labelPC, insnPC, pc, nil
		}
	}
	return nil, nil, 0, fmt.Errorf("branch layout did not converge")
}

func (c *Code) validateControlTargets(labels map[*Label]int, instructionPCs []int) error {
	instructionAt := make(map[int]struct{}, len(instructionPCs))
	for _, pc := range instructionPCs {
		instructionAt[pc] = struct{}{}
	}
	validate := func(kind, role string, label *Label) error {
		pc := labels[label]
		if _, ok := instructionAt[pc]; !ok {
			return fmt.Errorf("%s %s label %d at offset %d is not an instruction", kind, role, label.id, pc)
		}
		return nil
	}
	for _, node := range c.nodes {
		insn := node.insn
		if insn == nil {
			continue
		}
		switch insn.kind {
		case kindBranch:
			if err := validate("branch", "target", insn.target); err != nil {
				return err
			}
		case kindTableSwitch:
			if err := validate("tableswitch", "default target", insn.target); err != nil {
				return err
			}
			for index, target := range insn.targets {
				if err := validate("tableswitch", fmt.Sprintf("case target %d", index), target); err != nil {
					return err
				}
			}
		case kindLookupSwitch:
			if err := validate("lookupswitch", "default target", insn.target); err != nil {
				return err
			}
			for index, target := range insn.targets {
				if err := validate("lookupswitch", fmt.Sprintf("key %d target", insn.keys[index]), target); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (c *Code) writeInstruction(out *byteWriter, i *instruction, pc int, labels map[*Label]int) error {
	switch i.kind {
	case kindSimple:
		out.u1(byte(i.op))
	case kindByte:
		out.u1(byte(i.op))
		out.u1(byte(i.arg))
	case kindShort:
		out.u1(byte(i.op))
		out.i2(int16(i.arg))
	case kindCP:
		out.u1(byte(i.op))
		out.u2(uint16(i.arg))
	case kindLocal:
		if i.arg <= 3 {
			compactBase := map[Opcode]byte{
				ILOAD: 0x1a, LLOAD: 0x1e, FLOAD: 0x22, DLOAD: 0x26, ALOAD: 0x2a,
				ISTORE: 0x3b, LSTORE: 0x3f, FSTORE: 0x43, DSTORE: 0x47, ASTORE: 0x4b,
			}[i.op]
			out.u1(compactBase + byte(i.arg))
		} else if i.arg <= math.MaxUint8 {
			out.u1(byte(i.op))
			out.u1(byte(i.arg))
		} else {
			out.u1(byte(WIDE))
			out.u1(byte(i.op))
			out.u2(uint16(i.arg))
		}
	case kindIInc:
		if i.arg <= math.MaxUint8 && i.arg2 >= math.MinInt8 && i.arg2 <= math.MaxInt8 {
			out.u1(byte(IINC))
			out.u1(byte(i.arg))
			out.u1(byte(int8(i.arg2)))
		} else {
			out.u1(byte(WIDE))
			out.u1(byte(IINC))
			out.u2(uint16(i.arg))
			out.i2(int16(i.arg2))
		}
	case kindLDC:
		if i.arg <= math.MaxUint8 {
			out.u1(byte(LDC))
			out.u1(byte(i.arg))
		} else {
			out.u1(byte(LDC_W))
			out.u2(uint16(i.arg))
		}
	case kindBranch:
		target := labels[i.target]
		if i.expanded {
			out.u1(byte(inverseConditional(i.op)))
			out.i2(8)
			out.u1(byte(GOTO_W))
			out.i4(int32(target - (pc + 3)))
		} else if i.wideBranch {
			out.u1(byte(GOTO_W))
			out.i4(int32(target - pc))
		} else {
			offset := target - pc
			if offset < math.MinInt16 || offset > math.MaxInt16 {
				return fmt.Errorf("internal short branch overflow: %d", offset)
			}
			out.u1(byte(i.op))
			out.i2(int16(offset))
		}
	case kindInvokeInterface:
		out.u1(byte(INVOKEINTERFACE))
		out.u2(uint16(i.arg))
		out.u1(byte(i.arg2))
		out.u1(0)
	case kindTableSwitch:
		out.u1(byte(TABLESWITCH))
		for padding := (4 - (pc+1)%4) % 4; padding > 0; padding-- {
			out.u1(0)
		}
		out.i4(int32(labels[i.target] - pc))
		out.i4(int32(i.arg))
		out.i4(int32(int64(i.arg) + int64(len(i.targets)) - 1))
		for _, target := range i.targets {
			out.i4(int32(labels[target] - pc))
		}
	case kindLookupSwitch:
		out.u1(byte(LOOKUPSWITCH))
		for padding := (4 - (pc+1)%4) % 4; padding > 0; padding-- {
			out.u1(0)
		}
		out.i4(int32(labels[i.target] - pc))
		out.i4(int32(len(i.keys)))
		for index, key := range i.keys {
			out.i4(key)
			out.i4(int32(labels[i.targets[index]] - pc))
		}
	default:
		return fmt.Errorf("unknown internal instruction kind %d", i.kind)
	}
	return nil
}

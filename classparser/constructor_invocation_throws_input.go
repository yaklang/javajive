package javaclassparser

import (
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"math"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// The source graph and the original typed JVM operands are independent
// evidence. Seal the actual prefix's constant dependencies, including lossless
// UTF-16/raw CP payloads, and match its source operands before admitting it.
// Rebuilding this packet also checks original frame word limits and handlers.
type constructorThrowsInput struct {
	hash    [32]byte
	ir      *methodir.MethodIR
	records map[uint16]ssabuild.InstructionValues
	invoke  ssabuild.InstructionValues
}

type constructorThrowsInputHash struct {
	c      *ClassObjectDumper
	h      hash.Hash
	seen   map[uint16]bool
	active map[uint16]bool
	bytes  int
}

func (p *constructorThrowsInputHash) word(v uint64) {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], v)
	p.h.Write(buf[:])
}

func (p *constructorThrowsInputHash) data(b []byte) bool {
	p.bytes += len(b)
	if p.bytes > 65536 || !nativeProofWork(p.c.Work, int64(len(b))+1) {
		return false
	}
	p.word(uint64(len(b)))
	p.h.Write(b)
	return true
}

func (p *constructorThrowsInputHash) text(s string) bool {
	if len(s) > 65536 || p.bytes+len(s) > 65536 || p.c.Work != nil && p.c.Work.CheckAlloc(int64(len(s))) != nil {
		return false
	}
	return p.data([]byte(s))
}

func (p *constructorThrowsInputHash) constant(index uint16) bool {
	if index == 0 || int(index) > len(p.c.obj.ConstantPool) || p.active[index] || !nativeProofWork(p.c.Work, 1) {
		return false
	}
	if p.seen[index] {
		return true
	}
	if len(p.seen) >= 2048 {
		return false
	}
	p.active[index] = true
	defer delete(p.active, index)
	p.seen[index] = true
	p.word(uint64(index))
	member := func(tag uint64, m ConstantMemberrefInfo) bool {
		p.word(tag)
		p.word(uint64(m.ClassIndex))
		p.word(uint64(m.NameAndTypeIndex))
		return p.constant(m.ClassIndex) && p.constant(m.NameAndTypeIndex)
	}
	switch x := p.c.obj.ConstantPool[index-1].(type) {
	case *ConstantUtf8Info:
		if x == nil || x.DecodeErr != nil || len(x.Units) > 65535 {
			return false
		}
		p.word(1)
		if !p.text(x.Value) || !p.data(x.Raw) {
			return false
		}
		p.bytes += len(x.Units) * 2
		if p.bytes > 65536 {
			return false
		}
		p.word(uint64(len(x.Units)))
		for _, unit := range x.Units {
			if !nativeProofWork(p.c.Work, 1) {
				return false
			}
			p.word(uint64(unit))
		}
		return true
	case *ConstantIntegerInfo:
		if x == nil {
			return false
		}
		p.word(3)
		p.word(uint64(uint32(x.Value)))
	case *ConstantFloatInfo:
		if x == nil {
			return false
		}
		p.word(4)
		p.word(uint64(math.Float32bits(x.Value)))
	case *ConstantLongInfo:
		if x == nil {
			return false
		}
		p.word(5)
		p.word(uint64(x.Value))
	case *ConstantDoubleInfo:
		if x == nil {
			return false
		}
		p.word(6)
		p.word(math.Float64bits(x.Value))
	case *ConstantClassInfo:
		if x == nil {
			return false
		}
		p.word(7)
		p.word(uint64(x.NameIndex))
		return p.constant(x.NameIndex)
	case *ConstantStringInfo:
		if x == nil {
			return false
		}
		p.word(8)
		p.word(uint64(x.StringIndex))
		return p.constant(x.StringIndex)
	case *ConstantFieldrefInfo:
		return x != nil && member(9, x.ConstantMemberrefInfo)
	case *ConstantMethodrefInfo:
		return x != nil && member(10, x.ConstantMemberrefInfo)
	case *ConstantInterfaceMethodrefInfo:
		return x != nil && member(11, x.ConstantMemberrefInfo)
	case *ConstantNameAndTypeInfo:
		if x == nil {
			return false
		}
		p.word(12)
		p.word(uint64(x.NameIndex))
		p.word(uint64(x.DescriptorIndex))
		return p.constant(x.NameIndex) && p.constant(x.DescriptorIndex)
	default:
		return false
	}
	return true
}

func (c *ClassObjectDumper) constructorThrowsOriginalInput(method *MemberInfo, code *CodeAttribute, pc int) (*constructorThrowsInput, bool) {
	if c == nil || c.obj == nil || code == nil || len(code.Code) > 4096 || len(code.ExceptionTable) > 128 || pc < 0 || pc > 65535 || !nativeProofWork(c.Work, 1) || c.Work != nil && c.Work.CheckAlloc(256<<10) != nil {
		return nil, false
	}
	ir, fn, known := c.nativeOriginalMethodSnapshot(method, code)
	if !known || len(ir.Instrs) > 2048 || len(fn.Instructions) > 2048 {
		return nil, false
	}
	p := &constructorThrowsInputHash{c: c, h: sha256.New(), seen: map[uint16]bool{}, active: map[uint16]bool{}}
	p.word(uint64(code.MaxLocals))
	p.word(uint64(code.MaxStack))
	p.word(uint64(len(code.ExceptionTable)))
	for _, handler := range code.ExceptionTable {
		if handler == nil || !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		p.word(uint64(handler.StartPc))
		p.word(uint64(handler.EndPc))
		p.word(uint64(handler.HandlerPc))
		p.word(uint64(handler.CatchType))
		if handler.CatchType != 0 && !p.constant(handler.CatchType) {
			return nil, false
		}
	}
	for _, instruction := range ir.Instrs {
		if !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		if int(instruction.PC) > pc {
			continue
		}
		p.word(uint64(instruction.PC))
		p.word(uint64(instruction.Opcode))
		p.word(uint64(instruction.CPIndex))
		if !p.data(instruction.Data) || !p.text(instruction.Class) || !p.text(instruction.Member) || !p.text(instruction.Desc) {
			return nil, false
		}
		p.word(uint64(instruction.Const.Kind))
		p.word(uint64(uint32(instruction.Const.Int)))
		p.word(uint64(instruction.Const.Long))
		p.word(uint64(instruction.Const.FloatBits))
		p.word(instruction.Const.DoubleBits)
		if !p.text(instruction.Const.String) || !p.text(instruction.Const.Class) || instruction.CPIndex != 0 && !p.constant(instruction.CPIndex) {
			return nil, false
		}
	}
	result := &constructorThrowsInput{ir: ir, records: map[uint16]ssabuild.InstructionValues{}}
	matches := 0
	for _, record := range fn.Instructions {
		if !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		result.records[record.PC] = record
		if int(record.PC) == pc {
			result.invoke = record
			matches++
		}
	}
	if matches != 1 {
		return nil, false
	}
	copy(result.hash[:], p.h.Sum(nil))
	return result, true
}

func constructorThrowsCanonicalLiteral(literal *values.JavaLiteral, erasure string) bool {
	if literal == nil || literal.Units != nil || literal.Data == nil {
		return false
	}
	if values.IsNullLiteral(literal) {
		return callbinding.Reference(erasure) && erasure != "Ljava/lang/String;"
	}
	switch erasure {
	case "Z":
		switch n := literal.Data.(type) {
		case bool:
			return true
		case int:
			return n == 0 || n == 1
		}
	case "I":
		switch n := literal.Data.(type) {
		case int:
			return int64(n) >= math.MinInt32 && int64(n) <= math.MaxInt32
		case int32:
			return true
		}
	case "J":
		_, ok := literal.Data.(int64)
		return ok
	case "F":
		n, ok := literal.Data.(float32)
		return ok && !math.IsNaN(float64(n))
	case "D":
		n, ok := literal.Data.(float64)
		return ok && !math.IsNaN(n)
	case "Ljava/lang/String;":
		_, ok := literal.Data.(string)
		return ok
	}
	return false
}

func constructorThrowsLiteralMatches(literal *values.JavaLiteral, erasure string, instruction methodir.Instr) bool {
	// Immediate constants have no CP Const field in MethodIR. Decode their
	// original width/value without consulting a rendered literal.
	switch op := instruction.Opcode; {
	case op >= core.OP_ICONST_M1 && op <= core.OP_ICONST_5 && len(instruction.Data) == 0:
		instruction.Const = methodir.Const{Kind: methodir.ConstInt, Int: int32(op - core.OP_ICONST_0)}
	case op == core.OP_BIPUSH && len(instruction.Data) == 1:
		instruction.Const = methodir.Const{Kind: methodir.ConstInt, Int: int32(int8(instruction.Data[0]))}
	case op == core.OP_SIPUSH && len(instruction.Data) == 2:
		instruction.Const = methodir.Const{Kind: methodir.ConstInt, Int: int32(int16(binary.BigEndian.Uint16(instruction.Data)))}
	case op >= core.OP_LCONST_0 && op <= core.OP_LCONST_1 && len(instruction.Data) == 0:
		instruction.Const = methodir.Const{Kind: methodir.ConstLong, Long: int64(op - core.OP_LCONST_0)}
	case op >= core.OP_FCONST_0 && op <= core.OP_FCONST_2 && len(instruction.Data) == 0:
		instruction.Const = methodir.Const{Kind: methodir.ConstFloat, FloatBits: math.Float32bits(float32(op - core.OP_FCONST_0))}
	case op >= core.OP_DCONST_0 && op <= core.OP_DCONST_1 && len(instruction.Data) == 0:
		instruction.Const = methodir.Const{Kind: methodir.ConstDouble, DoubleBits: math.Float64bits(float64(op - core.OP_DCONST_0))}
	}
	if !constructorThrowsCanonicalLiteral(literal, erasure) {
		return false
	}
	if values.IsNullLiteral(literal) {
		return instruction.Opcode == core.OP_ACONST_NULL
	}
	switch erasure {
	case "Z", "I":
		var word int32
		switch n := literal.Data.(type) {
		case bool:
			if n {
				word = 1
			}
		case int:
			word = int32(n)
		case int32:
			word = n
		default:
			return false
		}
		return instruction.Const.Kind == methodir.ConstInt && instruction.Const.Int == word
	case "J":
		return instruction.Const.Kind == methodir.ConstLong && instruction.Const.Long == literal.Data.(int64)
	case "F":
		return instruction.Const.Kind == methodir.ConstFloat && instruction.Const.FloatBits == math.Float32bits(literal.Data.(float32))
	case "D":
		return instruction.Const.Kind == methodir.ConstDouble && instruction.Const.DoubleBits == math.Float64bits(literal.Data.(float64))
	case "Ljava/lang/String;":
		return instruction.Const.Kind == methodir.ConstString && instruction.Const.String == literal.Data.(string)
	}
	return false
}

func (c *ClassObjectDumper) constructorThrowsMatchOperand(input *constructorThrowsInput, value values.JavaValue, origin ssabuild.Origin, remaining *int) bool {
	*remaining--
	if *remaining < 0 || value == nil || !nativeProofWork(c.Work, 1) {
		return false
	}
	_, erasure, known := constructorThrowsTypeKey(value.Type(), c.FuncCtx)
	if !known {
		return false
	}
	switch x := value.(type) {
	case *values.SlotValue:
		return c.constructorThrowsMatchOperand(input, x.GetValue(), origin, remaining)
	case *values.JavaRef:
		slot, known := x.OriginalParameterSlot()
		return known && origin.Kind == ssabuild.OriginParam && slot == origin.Slot
	case *values.JavaLiteral:
		instruction, found := input.ir.InstrByID(methodir.InstrID(origin.PC))
		return origin.Kind == ssabuild.OriginInstr && found && constructorThrowsLiteralMatches(x, erasure, instruction)
	case *values.CastExpression:
		if x.Binding {
			return false
		}
		if x.OriginalCheckCast {
			pc, target, known := x.OriginalCheckCastWitness(c.FuncCtx)
			instruction, found := input.ir.InstrByID(methodir.InstrID(origin.PC))
			record := input.records[origin.PC]
			return known && origin.Kind == ssabuild.OriginInstr && int(origin.PC) == pc && found && instruction.Opcode == core.OP_CHECKCAST && target == "L"+strings.ReplaceAll(instruction.Class, ".", "/")+";" && len(record.Uses) == 1 && c.constructorThrowsMatchOperand(input, x.Value, record.Uses[0], remaining)
		}
		// A generated widening source cast has no JVM check to acquire. A
		// narrowing cast needs the original CHECKCAST witness above.
		_, from, known := constructorThrowsTypeKey(x.Value.Type(), c.FuncCtx)
		q := newConstructorWideningQuery(func(name string) (callbinding.Class, bool) {
			if !nativeProofWork(c.Work, 1) {
				return callbinding.Class{}, false
			}
			return c.FuncCtx.InvocationMetadata(name)
		})
		return known && (from == erasure || callbinding.Reference(from) && callbinding.Reference(erasure) && q.assignable(from, erasure) && !q.exhausted) && c.constructorThrowsMatchOperand(input, x.Value, origin, remaining)
	default:
		// A phi needs a separate condition/selected-edge certificate. Equal
		// nominal arm types alone do not own its original source selection.
		return false
	}
}

func (c *ClassObjectDumper) constructorThrowsMatchOriginal(input *constructorThrowsInput, plan *constructorSourceBoundary) bool {
	if input == nil || plan == nil || plan.delegate == nil || len(input.invoke.Uses) != len(plan.delegate.Arguments)+1 {
		return false
	}
	receiver, ok := values.UnpackSoltValue(plan.delegate.Object).(*values.JavaRef)
	if !ok {
		return false
	}
	slot, original := receiver.OriginalReceiverSlot()
	if !original || slot != 0 || input.invoke.Uses[0].Kind != ssabuild.OriginParam || input.invoke.Uses[0].Slot != 0 {
		return false
	}
	remaining := 256
	for i, argument := range plan.delegate.Arguments {
		if !c.constructorThrowsMatchOperand(input, argument, input.invoke.Uses[i+1], &remaining) {
			return false
		}
	}
	return true
}

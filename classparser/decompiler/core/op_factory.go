package core

import (
	"fmt"
	"github.com/yaklang/javajive/internal/omap"
)

type OpFactory func(reader *JavaByteCodeReader, opcode *OpCode) error

var OpFactories = map[string]OpFactory{
	"OperationFactoryDefault":      DefaultFactory,
	"OperationFactoryTableSwitch":  OperationFactoryTableSwitch,
	"OperationFactoryLookupSwitch": OperationFactoryLookupSwitch,
}

func DefaultFactory(reader *JavaByteCodeReader, opcode *OpCode) error {
	if opcode.Instr.Length == 0 {
		return nil
	}
	length := opcode.Instr.Length
	if opcode.IsWide {
		length *= 2
	}
	opcode.Data = make([]byte, length)
	_, err := reader.Read(opcode.Data)
	return err
}
func OperationFactoryLookupSwitch(reader *JavaByteCodeReader, opcode *OpCode) error {
	opcode.Data = make([]byte, opcode.Instr.Length)
	var startOffset uint16
	if overflow := (opcode.CurrentOffset + 1) % 4; overflow != 0 {
		startOffset = 4 - overflow
	}
	_, err := reader.Read(make([]byte, startOffset))
	if err != nil {
		return err
	}
	var defaultValue = make([]byte, 4)
	var pairsValue = make([]byte, 4)
	_, err = reader.Read(defaultValue)
	if err != nil {
		return err
	}
	_, err = reader.Read(pairsValue)
	if err != nil {
		return err
	}
	defaultTargetPos := Convert4bytesToInt(defaultValue)
	pairN := int32(Convert4bytesToInt(pairsValue))
	if pairN < 0 || int64(pairN)*8 > int64(reader.reader.Len()) {
		return fmt.Errorf("invalid lookupswitch pair count %d at PC %d", pairN, opcode.CurrentOffset)
	}
	var previous int32
	opcode.SwitchJmpCase = omap.NewEmptyOrderedMap[int, int32]()
	opcode.SwitchJmpCase1 = omap.NewEmptyOrderedMap[int, int]()
	for i := 0; i < int(pairN); i++ {
		val := make([]byte, 4)
		_, err = reader.Read(val)
		if err != nil {
			return err
		}
		target := make([]byte, 4)
		_, err = reader.Read(target)
		if err != nil {
			return err
		}
		targetPos := Convert4bytesToInt(target)

		// The lookupswitch match key is a signed 32-bit int (JVMS 6.5 lookupswitch); reading it as
		// uint32 turns negative labels like -5 into 4294967291, which renders as `case 4294967291`
		// and breaks recompilation ("integer number too large"). Sign-extend through int32.
		key := int32(Convert4bytesToInt(val))
		if i > 0 && key <= previous {
			return fmt.Errorf("unsorted lookupswitch keys at PC %d", opcode.CurrentOffset)
		}
		previous = key
		opcode.SwitchJmpCase.Set(int(key), int32(int64(int32(targetPos))+int64(opcode.CurrentOffset)))
	}
	opcode.SwitchDefaultOffset = int32(int64(int32(defaultTargetPos)) + int64(opcode.CurrentOffset))
	return nil
}
func OperationFactoryTableSwitch(reader *JavaByteCodeReader, opcode *OpCode) error {
	opcode.Data = make([]byte, opcode.Instr.Length)
	var startOffset uint16
	if overflow := (opcode.CurrentOffset + 1) % 4; overflow != 0 {
		startOffset = 4 - overflow
	}
	_, err := reader.Read(make([]byte, startOffset))
	if err != nil {
		return err
	}
	var defaultValue = make([]byte, 4)
	var lowValue = make([]byte, 4)
	var highValue = make([]byte, 4)
	_, err = reader.Read(defaultValue)
	if err != nil {
		return err
	}
	_, err = reader.Read(lowValue)
	if err != nil {
		return err
	}
	_, err = reader.Read(highValue)
	if err != nil {
		return err
	}
	// tableswitch low/high are signed 32-bit ints (JVMS 6.5 tableswitch); reading them as uint32
	// makes negative-key ranges (e.g. low=-5, high=-1) emit `case 4294967291` and fail to recompile.
	// Sign-extend through int32 so the generated labels keep their sign. The case count is the signed
	// span high-low+1 (the unsigned subtraction happens to cancel the 2^32 bias, but compute it
	// signed to stay obviously correct).
	startVal := int32(Convert4bytesToInt(lowValue))
	highVal := int32(Convert4bytesToInt(highValue))
	targetN := int64(highVal) - int64(startVal) + 1
	if targetN <= 0 || targetN*4 > int64(reader.reader.Len()) {
		return fmt.Errorf("invalid tableswitch range %d..%d at PC %d", startVal, highVal, opcode.CurrentOffset)
	}
	defaultTargetPos := Convert4bytesToInt(defaultValue)
	opcode.SwitchJmpCase = omap.NewEmptyOrderedMap[int, int32]()
	opcode.SwitchJmpCase1 = omap.NewEmptyOrderedMap[int, int]()
	for i := 0; int64(i) < targetN; i++ {
		target := make([]byte, 4)
		_, err = reader.Read(target)
		if err != nil {
			return err
		}
		targetPos := Convert4bytesToInt(target)

		opcode.SwitchJmpCase.Set(int(startVal)+i, int32(uint32(opcode.CurrentOffset)+targetPos))
	}
	opcode.SwitchDefaultOffset = int32(int64(int32(defaultTargetPos)) + int64(opcode.CurrentOffset))
	return nil
}

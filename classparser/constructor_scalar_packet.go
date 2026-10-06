package javaclassparser

import "github.com/yaklang/javajive/classparser/decompiler/core"

// This is the JVM scalar transfer contract, not constant evaluation or source
// simplification. Every arithmetic/comparison/conversion instruction retains
// its original Code site, category and exceptional behavior. In particular a
// long shift consumes an int distance, and FCMP/DCMP retain their original NaN
// direction while producing an unknown I for the following conditional.
func constructorScalarPacketContract(opcode int) (string, string, bool) {
	const categories = "IJFD"
	switch {
	case opcode >= core.OP_IADD && opcode <= core.OP_DREM:
		kind := categories[(opcode-core.OP_IADD)%4 : (opcode-core.OP_IADD)%4+1]
		return kind + kind, kind, true
	case opcode >= core.OP_INEG && opcode <= core.OP_DNEG:
		kind := categories[opcode-core.OP_INEG : opcode-core.OP_INEG+1]
		return kind, kind, true
	case opcode >= core.OP_ISHL && opcode <= core.OP_LXOR:
		kind := "I"
		if (opcode-core.OP_ISHL)%2 != 0 {
			kind = "J"
		}
		right := kind
		if opcode <= core.OP_LUSHR {
			right = "I"
		}
		return kind + right, kind, true
	case opcode >= core.OP_I2L && opcode <= core.OP_D2F:
		conversions := [...]string{"IJ", "IF", "ID", "JI", "JF", "JD", "FI", "FJ", "FD", "DI", "DJ", "DF"}
		pair := conversions[opcode-core.OP_I2L]
		return pair[:1], pair[1:], true
	case opcode >= core.OP_LCMP && opcode <= core.OP_DCMPG:
		kind := "D"
		if opcode == core.OP_LCMP {
			kind = "J"
		} else if opcode <= core.OP_FCMPG {
			kind = "F"
		}
		return kind + kind, "I", true
	default:
		// I2B/S/C have the separate logical narrowing proof. Reference and
		// storage effects cannot acquire a scalar contract by numeric range.
		return "", "", false
	}
}

func constructorScalarPacketOperands(arguments []string, inputs string) bool {
	if len(inputs) == 0 || len(arguments) < len(inputs) {
		return false
	}
	for i, expected := range []byte(inputs) {
		actual := arguments[len(arguments)-len(inputs)+i]
		// A Java boolean formal cannot borrow a numeric source conversion.
		// Original narrow B/S/C words already have their original truncation;
		// their computational category is I, without inventing another cast.
		valid := actual == string(expected) && (expected == 'I' || expected == 'J' || expected == 'F' || expected == 'D')
		if expected == 'I' && (actual == "B" || actual == "S" || actual == "C") {
			valid = true
		}
		if !valid {
			return false
		}
	}
	return true
}

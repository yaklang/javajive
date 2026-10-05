package javaclassparser

import (
	"encoding/binary"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Physical parameter slots and debug spans belong to the same proved straight
// accessor packet. Metadata cannot introduce a second, guessed local layout.
func nativeAccessorCodeMetadata(obj *ClassObject, code *CodeAttribute, params []string, work *workbudget.Budget) bool {
	seenLines, seenLocals := false, false
	if len(code.Attributes) > 2 {
		return false
	}
	for _, attr := range code.Attributes {
		if !nativeProofWork(work, 1) {
			return false
		}
		switch a := attr.(type) {
		case *LineNumberTableAttribute:
			if a == nil || seenLines || len(a.LineNumberTable) != 1 || a.LineNumberTable[0] == nil || a.LineNumberTable[0].StartPc != 0 {
				return false
			}
			seenLines = true
		case *UnparsedAttribute:
			if a == nil || seenLocals || a.Name != "LocalVariableTable" || len(a.Info) != 2+10*len(params) || !nativeProofWork(work, int64(len(a.Info))) {
				return false
			}
			u := func(i int) uint16 { return binary.BigEndian.Uint16(a.Info[i : i+2]) }
			if int(u(0)) != len(params) {
				return false
			}
			slots := make(map[int]int, len(params))
			physicalSlot := 0
			for i, param := range params {
				slots[physicalSlot] = i
				physicalSlot++
				if param == "J" || param == "D" {
					physicalSlot++
				}
			}
			seen := map[int]bool{}
			for offset := 2; offset < len(a.Info); offset += 10 {
				slot := int(u(offset + 8))
				index, known := slots[slot]
				n, nk := sourceBridgeUTF8(obj, u(offset+4))
				ds, dk := sourceBridgeUTF8(obj, u(offset+6))
				if !known || seen[slot] || u(offset) != 0 || int(u(offset+2)) != len(code.Code) || !nk || !dk || class_context.SafeIdentifier(n) != n || ds != params[index] {
					return false
				}
				seen[slot] = true
			}
			seenLocals = true
		default:
			return false
		}
	}

	return true
}

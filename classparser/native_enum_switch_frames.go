package javaclassparser

import (
	"encoding/binary"
	"github.com/yaklang/javajive/internal/workbudget"
)

// The proved clinit has no live locals across an assignment and exactly one
// NoSuchFieldError on each exceptional edge. Its branch target has an empty
// stack. Accept both JVM compact and extended encodings of those same frames.
func nativeEnumSwitchFrames(obj *ClassObject, code *CodeAttribute, raw *UnparsedAttribute, work *workbudget.Budget) bool {
	if raw == nil {
		return obj.MajorVersion < 50
	}
	if len(raw.Info) < 2 || raw.Length != uint32(len(raw.Info)) || len(raw.Info) > 2+12*len(code.ExceptionTable) || int(binary.BigEndian.Uint16(raw.Info[:2])) != 2*len(code.ExceptionTable) || !nativeProofWork(work, int64(len(raw.Info))) {
		return false
	}
	cursor, previous := 2, -1
	frame := func(pc int, exception bool) bool {
		if cursor >= len(raw.Info) {
			return false
		}
		tag := raw.Info[cursor]
		cursor++
		delta := 0
		if exception {
			switch {
			case tag >= 64 && tag <= 127:
				delta = int(tag - 64)
			case tag == 247:
				if cursor+2 > len(raw.Info) {
					return false
				}
				delta = int(binary.BigEndian.Uint16(raw.Info[cursor : cursor+2]))
				cursor += 2
			default:
				return false
			}
		} else {
			switch {
			case tag <= 63:
				delta = int(tag)
			case tag == 251:
				if cursor+2 > len(raw.Info) {
					return false
				}
				delta = int(binary.BigEndian.Uint16(raw.Info[cursor : cursor+2]))
				cursor += 2
			default:
				return false
			}
		}
		if previous+1+delta != pc {
			return false
		}
		previous = pc
		if exception {
			if cursor+3 > len(raw.Info) || raw.Info[cursor] != 7 {
				return false
			}
			index := binary.BigEndian.Uint16(raw.Info[cursor+1 : cursor+3])
			cursor += 3
			n, ok := sourceBridgeClassName(obj, index)
			if !ok || n != "java/lang/NoSuchFieldError" {
				return false
			}
		}
		return true
	}
	for _, h := range code.ExceptionTable {
		if h == nil || !frame(int(h.HandlerPc), true) || !frame(int(h.HandlerPc)+1, false) {
			return false
		}
	}
	return cursor == len(raw.Info)
}

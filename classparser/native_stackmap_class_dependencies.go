package javaclassparser

import (
	"encoding/binary"

	"github.com/yaklang/javajive/internal/workbudget"
)

// JVMS 4.7.4: walk the encoded verification types without expanding/copying
// inherited frame locals. This extracts class operands, not frame validity or
// reachability. In particular an uninitialized offset is never a CP index.
// https://docs.oracle.com/javase/specs/jvms/se8/html/jvms-4.html#jvms-4.7.4
func nativeStackMapClassDependencies(info []byte, retain func(uint16) bool, work *workbudget.Budget) bool {
	if len(info) < 2 || retain == nil || !nativeProofWork(work, int64(len(info))) {
		return false
	}
	count, cursor, offset := int(binary.BigEndian.Uint16(info[:2])), 2, -1
	u2 := func() (int, bool) {
		if cursor+2 > len(info) {
			return 0, false
		}
		value := int(binary.BigEndian.Uint16(info[cursor : cursor+2]))
		cursor += 2
		return value, true
	}
	verificationType := func() bool {
		if cursor >= len(info) {
			return false
		}
		tag := info[cursor]
		cursor++
		if tag <= 6 {
			return true
		}
		if tag != 7 && tag != 8 {
			return false
		}
		word, known := u2()
		return known && (tag == 8 || retain(uint16(word)))
	}
	for i := 0; i < count; i++ {
		if cursor >= len(info) {
			return false
		}
		frame := info[cursor]
		cursor++
		delta := int(frame)
		switch {
		case frame <= 63:
		case frame <= 127:
			delta -= 64
			if !verificationType() {
				return false
			}
		case frame < 247:
			return false
		default:
			var known bool
			delta, known = u2()
			if !known {
				return false
			}
			switch {
			case frame == 247:
				if !verificationType() {
					return false
				}
			case frame <= 251:
				// chop_frame/same_frame_extended add no verification types.
			case frame <= 254:
				for j := 0; j < int(frame)-251; j++ {
					if !verificationType() {
						return false
					}
				}
			case frame == 255:
				for table := 0; table < 2; table++ {
					types, known := u2()
					if !known {
						return false
					}
					for j := 0; j < types; j++ {
						if !verificationType() {
							return false
						}
					}
				}
			}
		}
		offset += delta + 1
		if offset > 65535 {
			return false
		}
	}
	return cursor == len(info)
}

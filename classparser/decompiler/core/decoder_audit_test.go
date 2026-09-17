package core

import (
	"encoding/binary"
	"testing"
)

func TestAuditLocalAccess(t *testing.T) {
	for _, base := range []int{OP_ILOAD_0, OP_LLOAD_0, OP_FLOAD_0, OP_DLOAD_0, OP_ALOAD_0} {
		for slot := 0; slot < 4; slot++ {
			code := op(base+slot, 0)
			if got := GetRetrieveIdx(code); got != slot {
				t.Errorf("load %#x: got %d want %d", base+slot, got, slot)
			}
			if got := GetStoreIdx(code); got != -1 {
				t.Errorf("load %#x is a store: %d", base+slot, got)
			}
		}
	}
	for _, base := range []int{OP_ISTORE_0, OP_LSTORE_0, OP_FSTORE_0, OP_DSTORE_0, OP_ASTORE_0} {
		for slot := 0; slot < 4; slot++ {
			code := op(base+slot, 0)
			if got := GetStoreIdx(code); got != slot {
				t.Errorf("store %#x: got %d want %d", base+slot, got, slot)
			}
			if got := GetRetrieveIdx(code); got != -1 {
				t.Errorf("store %#x is a load: %d", base+slot, got)
			}
		}
	}
	for _, wide := range []bool{false, true} {
		for _, slot := range []int{0, 3, 127, 255, 256, 32768, 65535} {
			if !wide && slot > 255 {
				continue
			}
			for _, spec := range []struct {
				code        int
				read, write bool
			}{
				{OP_ILOAD, true, false}, {OP_LLOAD, true, false}, {OP_FLOAD, true, false}, {OP_DLOAD, true, false}, {OP_ALOAD, true, false},
				{OP_ISTORE, false, true}, {OP_LSTORE, false, true}, {OP_FSTORE, false, true}, {OP_DSTORE, false, true}, {OP_ASTORE, false, true},
				{OP_IINC, true, true}, {OP_RET, true, false},
			} {
				code := op(spec.code, 0)
				code.IsWide = wide
				code.Data = []byte{byte(slot), 0}
				if wide {
					code.Data = []byte{byte(slot >> 8), byte(slot), 0, 0}
				}
				read, write := -1, -1
				if spec.read {
					read = slot
				}
				if spec.write {
					write = slot
				}
				if r, w := GetRetrieveIdx(code), GetStoreIdx(code); r != read || w != write {
					t.Errorf("opcode %#x wide=%v slot=%d got (%d,%d) want (%d,%d)", spec.code, wide, slot, r, w, read, write)
				}
			}
		}
	}
}

func TestAuditWideJump(t *testing.T) {
	for _, tc := range []struct {
		name             string
		pc, target, size int
	}{
		{"short", 0, 7, 9}, {"forward", 0, 40000, 40001}, {"backward", 40000, 0, 40005},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := make([]byte, tc.size)
			data[tc.pc] = OP_GOTO_W
			binary.BigEndian.PutUint32(data[tc.pc+1:], uint32(int32(tc.target-tc.pc)))
			data[tc.target] = OP_RETURN
			d := NewDecompiler(data, nil)
			if err := d.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			if err := d.ScanJmp(); err != nil {
				t.Fatal(err)
			}
			jump := d.opCodes[d.offsetToOpcodeIndex[uint16(tc.pc)]]
			if jump.BranchTarget != tc.target {
				t.Fatalf("decoded target %d want %d", jump.BranchTarget, tc.target)
			}
			if tc.pc == 0 && (len(jump.Target) != 1 || int(jump.Target[0].CurrentOffset) != tc.target) {
				t.Fatalf("wrong jump targets: %v", jump.Target)
			}
		})
	}
}

func TestAuditMalformedBytecode(t *testing.T) {
	for _, data := range [][]byte{
		{OP_GOTO, 0}, {OP_GOTO_W, 0, 0, 0}, {OP_WIDE}, {OP_WIDE, OP_NOP}, {OP_WIDE, OP_WIDE, OP_ILOAD, 0, 0},
		{OP_WIDE, OP_IINC, 0, 1, 0}, {OP_BIPUSH}, {OP_GOTO, 0, 1}, {OP_GOTO, 255, 255},
		{OP_GOTO_W, 127, 255, 255, 255}, {OP_GOTO_W, 0, 1, 0, 0},
		{OP_LOOKUPSWITCH, 0, 0, 0, 0, 0, 0, 0, 255, 255, 255, 255},
		{OP_TABLESWITCH, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 1},
	} {
		t.Run(stringHex(data), func(t *testing.T) {
			d := NewDecompiler(data, nil)
			err := d.ParseOpcode()
			if err == nil {
				err = d.ScanJmp()
			}
			if err == nil {
				t.Fatal("malformed bytecode accepted")
			}
		})
	}
}
func stringHex(data []byte) string {
	const h = "0123456789abcdef"
	b := make([]byte, len(data)*2)
	for i, v := range data {
		b[i*2] = h[v>>4]
		b[i*2+1] = h[v&15]
	}
	return string(b)
}

func FuzzAuditDecoder(f *testing.F) {
	for _, data := range [][]byte{{OP_RETURN}, {OP_GOTO_W, 0, 0, 0, 5, OP_RETURN}, {OP_WIDE, OP_FLOAD, 1, 0}, {OP_LOOKUPSWITCH, 0, 0, 0}, {OP_TABLESWITCH, 0, 0, 0}} {
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 65535 {
			return
		}
		d := NewDecompiler(data, nil)
		if err := d.ParseOpcode(); err != nil {
			return
		}
		// Decode and boundary validation must reject malformed data without panic
		// or allocations proportional to untrusted switch counts.
		_ = d.validateControlFlow()
	})
}

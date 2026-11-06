package ir

import "testing"

type expectedOpcode struct {
	name      string
	format    string
	width     uint16
	supported bool
	known     bool
}

func TestOpcodeTableExhaustiveContract(t *testing.T) {
	var want [256]expectedOpcode
	define := func(opcode byte, name, format string, width uint16, supported bool) {
		if want[opcode].known {
			t.Fatalf("test contract defines opcode 0x%02x twice", opcode)
		}
		want[opcode] = expectedOpcode{name, format, width, supported, true}
	}

	fixed := []expectedOpcode{
		{"nop", "10x", 1, true, true}, {"move", "12x", 1, true, true}, {"move/from16", "22x", 2, true, true}, {"move/16", "32x", 3, true, true},
		{"move-wide", "12x", 1, false, true}, {"move-wide/from16", "22x", 2, false, true}, {"move-wide/16", "32x", 3, false, true},
		{"move-object", "12x", 1, true, true}, {"move-object/from16", "22x", 2, true, true}, {"move-object/16", "32x", 3, true, true},
		{"move-result", "11x", 1, true, true}, {"move-result-wide", "11x", 1, false, true}, {"move-result-object", "11x", 1, true, true}, {"move-exception", "11x", 1, true, true},
		{"return-void", "10x", 1, true, true}, {"return", "11x", 1, true, true}, {"return-wide", "11x", 1, false, true}, {"return-object", "11x", 1, true, true},
		{"const/4", "11n", 1, true, true}, {"const/16", "21s", 2, true, true}, {"const", "31i", 3, true, true}, {"const/high16", "21h", 2, true, true},
		{"const-wide/16", "21s", 2, false, true}, {"const-wide/32", "31i", 3, false, true}, {"const-wide", "51l", 5, false, true}, {"const-wide/high16", "21h", 2, false, true},
		{"const-string", "21c", 2, true, true}, {"const-string/jumbo", "31c", 3, true, true}, {"const-class", "21c", 2, true, true},
		{"monitor-enter", "11x", 1, false, true}, {"monitor-exit", "11x", 1, false, true}, {"check-cast", "21c", 2, true, true}, {"instance-of", "22c", 2, true, true},
		{"array-length", "12x", 1, true, true}, {"new-instance", "21c", 2, true, true}, {"new-array", "22c", 2, true, true},
		{"filled-new-array", "35c", 3, true, true}, {"filled-new-array/range", "3rc", 3, true, true}, {"fill-array-data", "31t", 3, false, true}, {"throw", "11x", 1, true, true},
		{"goto", "10t", 1, true, true}, {"goto/16", "20t", 2, true, true}, {"goto/32", "30t", 3, true, true},
		{"packed-switch", "31t", 3, false, true}, {"sparse-switch", "31t", 3, false, true},
		{"cmpl-float", "23x", 2, false, true}, {"cmpg-float", "23x", 2, false, true}, {"cmpl-double", "23x", 2, false, true}, {"cmpg-double", "23x", 2, false, true}, {"cmp-long", "23x", 2, false, true},
		{"if-eq", "22t", 2, true, true}, {"if-ne", "22t", 2, true, true}, {"if-lt", "22t", 2, true, true}, {"if-ge", "22t", 2, true, true}, {"if-gt", "22t", 2, true, true}, {"if-le", "22t", 2, true, true},
		{"if-eqz", "21t", 2, true, true}, {"if-nez", "21t", 2, true, true}, {"if-ltz", "21t", 2, true, true}, {"if-gez", "21t", 2, true, true}, {"if-gtz", "21t", 2, true, true}, {"if-lez", "21t", 2, true, true},
	}
	for opcode, expected := range fixed {
		define(byte(opcode), expected.name, expected.format, expected.width, expected.supported)
	}

	defineRange := func(first byte, names []string, format string, width uint16, supported func(byte) bool) {
		for index, name := range names {
			opcode := first + byte(index)
			define(opcode, name, format, width, supported(opcode))
		}
	}
	array := []string{"aget", "aget-wide", "aget-object", "aget-boolean", "aget-byte", "aget-char", "aget-short", "aput", "aput-wide", "aput-object", "aput-boolean", "aput-byte", "aput-char", "aput-short"}
	defineRange(0x44, array, "23x", 2, func(opcode byte) bool { return opcode != 0x45 && opcode != 0x4c })
	instanceFields := []string{"iget", "iget-wide", "iget-object", "iget-boolean", "iget-byte", "iget-char", "iget-short", "iput", "iput-wide", "iput-object", "iput-boolean", "iput-byte", "iput-char", "iput-short"}
	defineRange(0x52, instanceFields, "22c", 2, func(opcode byte) bool { return opcode != 0x53 && opcode != 0x5a })
	staticFields := []string{"sget", "sget-wide", "sget-object", "sget-boolean", "sget-byte", "sget-char", "sget-short", "sput", "sput-wide", "sput-object", "sput-boolean", "sput-byte", "sput-char", "sput-short"}
	defineRange(0x60, staticFields, "21c", 2, func(opcode byte) bool { return opcode != 0x61 && opcode != 0x68 })
	invokes := []string{"invoke-virtual", "invoke-super", "invoke-direct", "invoke-static", "invoke-interface"}
	defineRange(0x6e, invokes, "35c", 3, func(byte) bool { return true })
	invokeRanges := []string{"invoke-virtual/range", "invoke-super/range", "invoke-direct/range", "invoke-static/range", "invoke-interface/range"}
	defineRange(0x74, invokeRanges, "3rc", 3, func(byte) bool { return true })
	unary := []string{"neg-int", "not-int", "neg-long", "not-long", "neg-float", "neg-double", "int-to-long", "int-to-float", "int-to-double", "long-to-int", "long-to-float", "long-to-double", "float-to-int", "float-to-long", "float-to-double", "double-to-int", "double-to-long", "double-to-float", "int-to-byte", "int-to-char", "int-to-short"}
	defineRange(0x7b, unary, "12x", 1, func(opcode byte) bool { return opcode == 0x7b || opcode == 0x7c || opcode >= 0x8d })
	binary := []string{"add-int", "sub-int", "mul-int", "div-int", "rem-int", "and-int", "or-int", "xor-int", "shl-int", "shr-int", "ushr-int", "add-long", "sub-long", "mul-long", "div-long", "rem-long", "and-long", "or-long", "xor-long", "shl-long", "shr-long", "ushr-long", "add-float", "sub-float", "mul-float", "div-float", "rem-float", "add-double", "sub-double", "mul-double", "div-double", "rem-double"}
	defineRange(0x90, binary, "23x", 2, func(opcode byte) bool { return opcode < 0x9b })
	binary2Addr := make([]string, len(binary))
	for index, name := range binary {
		binary2Addr[index] = name + "/2addr"
	}
	defineRange(0xb0, binary2Addr, "12x", 1, func(opcode byte) bool { return opcode < 0xbb })
	lit16 := []string{"add-int/lit16", "rsub-int", "mul-int/lit16", "div-int/lit16", "rem-int/lit16", "and-int/lit16", "or-int/lit16", "xor-int/lit16"}
	defineRange(0xd0, lit16, "22s", 2, func(byte) bool { return true })
	lit8 := []string{"add-int/lit8", "rsub-int/lit8", "mul-int/lit8", "div-int/lit8", "rem-int/lit8", "and-int/lit8", "or-int/lit8", "xor-int/lit8", "shl-int/lit8", "shr-int/lit8", "ushr-int/lit8"}
	defineRange(0xd8, lit8, "22b", 2, func(byte) bool { return true })
	define(0xfa, "invoke-polymorphic", "45cc", 4, false)
	define(0xfb, "invoke-polymorphic/range", "4rcc", 4, false)
	define(0xfc, "invoke-custom", "35c", 3, false)
	define(0xfd, "invoke-custom/range", "3rc", 3, false)
	define(0xfe, "const-method-handle", "21c", 2, false)
	define(0xff, "const-method-type", "21c", 2, false)

	known, supported, reserved := 0, 0, 0
	for slot := 0; slot < 256; slot++ {
		opcode := byte(slot)
		name, format, width, isSupported, isKnown := OpcodeInfo(opcode)
		got := expectedOpcode{name, format, width, isSupported, isKnown}
		if got != want[slot] {
			t.Errorf("opcode 0x%02x = %#v, want %#v", slot, got, want[slot])
		}
		if isKnown {
			known++
			if name == "" || format == "" || width == 0 {
				t.Errorf("known opcode 0x%02x has incomplete metadata: %#v", slot, got)
			}
			if isSupported {
				supported++
			}
		} else {
			reserved++
			if name != "" || format != "" || width != 0 || isSupported {
				t.Errorf("reserved opcode 0x%02x exposes metadata: %#v", slot, got)
			}
		}
	}
	if known != 224 || supported != 135 || reserved != 32 {
		t.Fatalf("opcode classification known=%d supported=%d reserved=%d", known, supported, reserved)
	}
}

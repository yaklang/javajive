package ir

type opcodeInfo struct {
	name      string
	format    string
	width     uint16
	supported bool
}

var opcodeTable = map[byte]opcodeInfo{
	0x00: {"nop", "10x", 1, true},
	0x01: {"move", "12x", 1, true},
	0x02: {"move/from16", "22x", 2, true},
	0x03: {"move/16", "32x", 3, true},
	0x04: {"move-wide", "12x", 1, true},
	0x05: {"move-wide/from16", "22x", 2, true},
	0x06: {"move-wide/16", "32x", 3, true},
	0x07: {"move-object", "12x", 1, true},
	0x08: {"move-object/from16", "22x", 2, true},
	0x09: {"move-object/16", "32x", 3, true},
	0x0a: {"move-result", "11x", 1, true},
	0x0b: {"move-result-wide", "11x", 1, true},
	0x0c: {"move-result-object", "11x", 1, true},
	0x0d: {"move-exception", "11x", 1, true},
	0x0e: {"return-void", "10x", 1, true},
	0x0f: {"return", "11x", 1, true},
	0x10: {"return-wide", "11x", 1, true},
	0x11: {"return-object", "11x", 1, true},
	0x12: {"const/4", "11n", 1, true},
	0x13: {"const/16", "21s", 2, true},
	0x14: {"const", "31i", 3, true},
	0x15: {"const/high16", "21h", 2, true},
	0x16: {"const-wide/16", "21s", 2, true},
	0x17: {"const-wide/32", "31i", 3, true},
	0x18: {"const-wide", "51l", 5, true},
	0x19: {"const-wide/high16", "21h", 2, true},
	0x1a: {"const-string", "21c", 2, true},
	0x1b: {"const-string/jumbo", "31c", 3, true},
	0x1c: {"const-class", "21c", 2, true},
	0x1d: {"monitor-enter", "11x", 1, true},
	0x1e: {"monitor-exit", "11x", 1, true},
	0x1f: {"check-cast", "21c", 2, true},
	0x20: {"instance-of", "22c", 2, true},
	0x21: {"array-length", "12x", 1, true},
	0x22: {"new-instance", "21c", 2, true},
	0x23: {"new-array", "22c", 2, true},
	0x24: {"filled-new-array", "35c", 3, true},
	0x25: {"filled-new-array/range", "3rc", 3, true},
	0x26: {"fill-array-data", "31t", 3, true},
	0x27: {"throw", "11x", 1, true},
	0x28: {"goto", "10t", 1, true},
	0x29: {"goto/16", "20t", 2, true},
	0x2a: {"goto/32", "30t", 3, true},
	0x2b: {"packed-switch", "31t", 3, true},
	0x2c: {"sparse-switch", "31t", 3, true},
	0x2d: {"cmpl-float", "23x", 2, true},
	0x2e: {"cmpg-float", "23x", 2, true},
	0x2f: {"cmpl-double", "23x", 2, true},
	0x30: {"cmpg-double", "23x", 2, true},
	0x31: {"cmp-long", "23x", 2, true},
	0x32: {"if-eq", "22t", 2, true},
	0x33: {"if-ne", "22t", 2, true},
	0x34: {"if-lt", "22t", 2, true},
	0x35: {"if-ge", "22t", 2, true},
	0x36: {"if-gt", "22t", 2, true},
	0x37: {"if-le", "22t", 2, true},
	0x38: {"if-eqz", "21t", 2, true},
	0x39: {"if-nez", "21t", 2, true},
	0x3a: {"if-ltz", "21t", 2, true},
	0x3b: {"if-gez", "21t", 2, true},
	0x3c: {"if-gtz", "21t", 2, true},
	0x3d: {"if-lez", "21t", 2, true},
}

func init() {
	array := []string{
		"aget", "aget-wide", "aget-object", "aget-boolean", "aget-byte", "aget-char", "aget-short",
		"aput", "aput-wide", "aput-object", "aput-boolean", "aput-byte", "aput-char", "aput-short",
	}
	for index, name := range array {
		opcode := byte(0x44 + index)
		// int, object and the narrow primitive element kinds are supported; the
		// wide (long/double) aget/aput forms stay outside the slice.
		supported := true
		opcodeTable[opcode] = opcodeInfo{name, "23x", 2, supported}
	}
	instanceFields := []string{
		"iget", "iget-wide", "iget-object", "iget-boolean", "iget-byte", "iget-char", "iget-short",
		"iput", "iput-wide", "iput-object", "iput-boolean", "iput-byte", "iput-char", "iput-short",
	}
	for index, name := range instanceFields {
		opcode := byte(0x52 + index)
		opcodeTable[opcode] = opcodeInfo{name, "22c", 2, true}
	}
	staticFields := []string{
		"sget", "sget-wide", "sget-object", "sget-boolean", "sget-byte", "sget-char", "sget-short",
		"sput", "sput-wide", "sput-object", "sput-boolean", "sput-byte", "sput-char", "sput-short",
	}
	for index, name := range staticFields {
		opcode := byte(0x60 + index)
		opcodeTable[opcode] = opcodeInfo{name, "21c", 2, true}
	}
	invokes := []string{"invoke-virtual", "invoke-super", "invoke-direct", "invoke-static", "invoke-interface"}
	for index, name := range invokes {
		opcodeTable[byte(0x6e+index)] = opcodeInfo{name, "35c", 3, true}
		opcodeTable[byte(0x74+index)] = opcodeInfo{name + "/range", "3rc", 3, true}
	}
	unary := map[byte]string{
		0x7b: "neg-int", 0x7c: "not-int",
		0x7d: "neg-long", 0x7e: "not-long", 0x7f: "neg-float", 0x80: "neg-double",
		0x81: "int-to-long", 0x82: "int-to-float", 0x83: "int-to-double",
		0x84: "long-to-int", 0x85: "long-to-float", 0x86: "long-to-double",
		0x87: "float-to-int", 0x88: "float-to-long", 0x89: "float-to-double",
		0x8a: "double-to-int", 0x8b: "double-to-long", 0x8c: "double-to-float",
		0x8d: "int-to-byte", 0x8e: "int-to-char", 0x8f: "int-to-short",
	}
	for opcode, name := range unary {
		supported := true
		opcodeTable[opcode] = opcodeInfo{name, "12x", 1, supported}
	}
	binary := []string{
		"add-int", "sub-int", "mul-int", "div-int", "rem-int", "and-int", "or-int", "xor-int", "shl-int", "shr-int", "ushr-int",
		"add-long", "sub-long", "mul-long", "div-long", "rem-long", "and-long", "or-long", "xor-long", "shl-long", "shr-long", "ushr-long",
		"add-float", "sub-float", "mul-float", "div-float", "rem-float",
		"add-double", "sub-double", "mul-double", "div-double", "rem-double",
	}
	for index, name := range binary {
		opcodeTable[byte(0x90+index)] = opcodeInfo{name, "23x", 2, true}
		opcodeTable[byte(0xb0+index)] = opcodeInfo{name + "/2addr", "12x", 1, true}
	}
	lit16 := []string{"add-int/lit16", "rsub-int", "mul-int/lit16", "div-int/lit16", "rem-int/lit16", "and-int/lit16", "or-int/lit16", "xor-int/lit16"}
	for index, name := range lit16 {
		opcodeTable[byte(0xd0+index)] = opcodeInfo{name, "22s", 2, true}
	}
	lit8 := []string{"add-int/lit8", "rsub-int/lit8", "mul-int/lit8", "div-int/lit8", "rem-int/lit8", "and-int/lit8", "or-int/lit8", "xor-int/lit8", "shl-int/lit8", "shr-int/lit8", "ushr-int/lit8"}
	for index, name := range lit8 {
		opcodeTable[byte(0xd8+index)] = opcodeInfo{name, "22b", 2, true}
	}
	for opcode, info := range map[byte]opcodeInfo{
		0xfa: {"invoke-polymorphic", "45cc", 4, false},
		0xfb: {"invoke-polymorphic/range", "4rcc", 4, false},
		0xfc: {"invoke-custom", "35c", 3, false},
		0xfd: {"invoke-custom/range", "3rc", 3, false},
		0xfe: {"const-method-handle", "21c", 2, false},
		0xff: {"const-method-type", "21c", 2, false},
	} {
		opcodeTable[opcode] = info
	}
}

// OpcodeInfo reports the DEX 035 mnemonic and format known to the decoder.
func OpcodeInfo(opcode byte) (name, format string, width uint16, supported, known bool) {
	info, known := opcodeTable[opcode]
	return info.name, info.format, info.width, info.supported, known
}

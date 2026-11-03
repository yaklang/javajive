// Package classfilebuilder builds deterministic Java 5 (major version 49)
// class files from symbolic constants, members, labels, and instructions.
package classfilebuilder

// Opcode is a JVM instruction opcode.
type Opcode byte

// Access flags shared by classes, fields, and methods. Callers are responsible
// for using flags in contexts where the JVM specification permits them.
const (
	ACC_PUBLIC       uint16 = 0x0001
	ACC_PRIVATE      uint16 = 0x0002
	ACC_PROTECTED    uint16 = 0x0004
	ACC_STATIC       uint16 = 0x0008
	ACC_FINAL        uint16 = 0x0010
	ACC_SUPER        uint16 = 0x0020
	ACC_SYNCHRONIZED uint16 = 0x0020
	ACC_VOLATILE     uint16 = 0x0040
	ACC_BRIDGE       uint16 = 0x0040
	ACC_TRANSIENT    uint16 = 0x0080
	ACC_VARARGS      uint16 = 0x0080
	ACC_NATIVE       uint16 = 0x0100
	ACC_INTERFACE    uint16 = 0x0200
	ACC_ABSTRACT     uint16 = 0x0400
	ACC_STRICT       uint16 = 0x0800
	ACC_SYNTHETIC    uint16 = 0x1000
	ACC_ANNOTATION   uint16 = 0x2000
	ACC_ENUM         uint16 = 0x4000
)

// JVM opcodes supported by the assembler. TABLESWITCH, LOOKUPSWITCH, JSR,
// JSR_W, RET, MULTIANEWARRAY, and invokedynamic intentionally have no helpers.
const (
	NOP             Opcode = 0x00
	ACONST_NULL     Opcode = 0x01
	ICONST_M1       Opcode = 0x02
	ICONST_0        Opcode = 0x03
	ICONST_1        Opcode = 0x04
	ICONST_2        Opcode = 0x05
	ICONST_3        Opcode = 0x06
	ICONST_4        Opcode = 0x07
	ICONST_5        Opcode = 0x08
	LCONST_0        Opcode = 0x09
	LCONST_1        Opcode = 0x0a
	FCONST_0        Opcode = 0x0b
	FCONST_1        Opcode = 0x0c
	FCONST_2        Opcode = 0x0d
	DCONST_0        Opcode = 0x0e
	DCONST_1        Opcode = 0x0f
	BIPUSH          Opcode = 0x10
	SIPUSH          Opcode = 0x11
	LDC             Opcode = 0x12
	LDC_W           Opcode = 0x13
	ILOAD           Opcode = 0x15
	LLOAD           Opcode = 0x16
	FLOAD           Opcode = 0x17
	DLOAD           Opcode = 0x18
	ALOAD           Opcode = 0x19
	IALOAD          Opcode = 0x2e
	LALOAD          Opcode = 0x2f
	FALOAD          Opcode = 0x30
	DALOAD          Opcode = 0x31
	AALOAD          Opcode = 0x32
	BALOAD          Opcode = 0x33
	CALOAD          Opcode = 0x34
	SALOAD          Opcode = 0x35
	ISTORE          Opcode = 0x36
	LSTORE          Opcode = 0x37
	FSTORE          Opcode = 0x38
	DSTORE          Opcode = 0x39
	ASTORE          Opcode = 0x3a
	IASTORE         Opcode = 0x4f
	LASTORE         Opcode = 0x50
	FASTORE         Opcode = 0x51
	DASTORE         Opcode = 0x52
	AASTORE         Opcode = 0x53
	BASTORE         Opcode = 0x54
	CASTORE         Opcode = 0x55
	SASTORE         Opcode = 0x56
	POP             Opcode = 0x57
	POP2            Opcode = 0x58
	DUP             Opcode = 0x59
	DUP_X1          Opcode = 0x5a
	DUP_X2          Opcode = 0x5b
	DUP2            Opcode = 0x5c
	DUP2_X1         Opcode = 0x5d
	DUP2_X2         Opcode = 0x5e
	SWAP            Opcode = 0x5f
	IADD            Opcode = 0x60
	LADD            Opcode = 0x61
	FADD            Opcode = 0x62
	DADD            Opcode = 0x63
	ISUB            Opcode = 0x64
	LSUB            Opcode = 0x65
	FSUB            Opcode = 0x66
	DSUB            Opcode = 0x67
	IMUL            Opcode = 0x68
	LMUL            Opcode = 0x69
	FMUL            Opcode = 0x6a
	DMUL            Opcode = 0x6b
	IDIV            Opcode = 0x6c
	LDIV            Opcode = 0x6d
	FDIV            Opcode = 0x6e
	DDIV            Opcode = 0x6f
	IREM            Opcode = 0x70
	LREM            Opcode = 0x71
	FREM            Opcode = 0x72
	DREM            Opcode = 0x73
	INEG            Opcode = 0x74
	LNEG            Opcode = 0x75
	FNEG            Opcode = 0x76
	DNEG            Opcode = 0x77
	ISHL            Opcode = 0x78
	LSHL            Opcode = 0x79
	ISHR            Opcode = 0x7a
	LSHR            Opcode = 0x7b
	IUSHR           Opcode = 0x7c
	LUSHR           Opcode = 0x7d
	IAND            Opcode = 0x7e
	LAND            Opcode = 0x7f
	IOR             Opcode = 0x80
	LOR             Opcode = 0x81
	IXOR            Opcode = 0x82
	LXOR            Opcode = 0x83
	IINC            Opcode = 0x84
	I2L             Opcode = 0x85
	I2F             Opcode = 0x86
	I2D             Opcode = 0x87
	L2I             Opcode = 0x88
	L2F             Opcode = 0x89
	L2D             Opcode = 0x8a
	F2I             Opcode = 0x8b
	F2L             Opcode = 0x8c
	F2D             Opcode = 0x8d
	D2I             Opcode = 0x8e
	D2L             Opcode = 0x8f
	D2F             Opcode = 0x90
	I2B             Opcode = 0x91
	I2C             Opcode = 0x92
	I2S             Opcode = 0x93
	LCMP            Opcode = 0x94
	FCMPL           Opcode = 0x95
	FCMPG           Opcode = 0x96
	DCMPL           Opcode = 0x97
	DCMPG           Opcode = 0x98
	IFEQ            Opcode = 0x99
	IFNE            Opcode = 0x9a
	IFLT            Opcode = 0x9b
	IFGE            Opcode = 0x9c
	IFGT            Opcode = 0x9d
	IFLE            Opcode = 0x9e
	IF_ICMPEQ       Opcode = 0x9f
	IF_ICMPNE       Opcode = 0xa0
	IF_ICMPLT       Opcode = 0xa1
	IF_ICMPGE       Opcode = 0xa2
	IF_ICMPGT       Opcode = 0xa3
	IF_ICMPLE       Opcode = 0xa4
	IF_ACMPEQ       Opcode = 0xa5
	IF_ACMPNE       Opcode = 0xa6
	GOTO            Opcode = 0xa7
	IRETURN         Opcode = 0xac
	LRETURN         Opcode = 0xad
	FRETURN         Opcode = 0xae
	DRETURN         Opcode = 0xaf
	ARETURN         Opcode = 0xb0
	RETURN          Opcode = 0xb1
	GETSTATIC       Opcode = 0xb2
	PUTSTATIC       Opcode = 0xb3
	GETFIELD        Opcode = 0xb4
	PUTFIELD        Opcode = 0xb5
	INVOKEVIRTUAL   Opcode = 0xb6
	INVOKESPECIAL   Opcode = 0xb7
	INVOKESTATIC    Opcode = 0xb8
	INVOKEINTERFACE Opcode = 0xb9
	NEW             Opcode = 0xbb
	NEWARRAY        Opcode = 0xbc
	ANEWARRAY       Opcode = 0xbd
	ARRAYLENGTH     Opcode = 0xbe
	ATHROW          Opcode = 0xbf
	CHECKCAST       Opcode = 0xc0
	INSTANCEOF      Opcode = 0xc1
	MONITORENTER    Opcode = 0xc2
	MONITOREXIT     Opcode = 0xc3
	WIDE            Opcode = 0xc4
	IFNULL          Opcode = 0xc6
	IFNONNULL       Opcode = 0xc7
	GOTO_W          Opcode = 0xc8
)

// LocalKind selects the typed JVM local-variable load/store family.
type LocalKind byte

const (
	IntLocal LocalKind = iota
	LongLocal
	FloatLocal
	DoubleLocal
	RefLocal
)
